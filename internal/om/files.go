package om

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/gin-gonic/gin"
)

var blockedExt = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bat": true, ".cmd": true,
	".ps1": true, ".sh": true, ".bash": true, ".py": true, ".pyc": true, ".pyo": true,
	".jar": true, ".msi": true, ".com": true, ".scr": true, ".vbs": true, ".vbe": true,
	".wsf": true, ".hta": true, ".cpl": true, ".lnk": true, ".apk": true, ".deb": true,
	".rpm": true, ".php": true, ".phtml": true, ".asp": true, ".aspx": true, ".jsp": true,
	".cgi": true, ".pl": true, ".rb": true, ".go": true, ".rs": true, ".c": true, ".cpp": true,
}

var textExt = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".json": true, ".csv": true, ".tsv": true,
	".log": true, ".xml": true, ".yml": true, ".yaml": true, ".ini": true, ".conf": true,
	".toml": true, ".css": true, ".js": true, ".ts": true, ".sql": true, ".srt": true,
	".vtt": true, ".tex": true, ".rst": true,
}

func maxUpload() int64 {
	if v := envInt64("OM_MAX_UPLOAD", 8*1024*1024*1024); v > 0 {
		return v
	}
	return 8 * 1024 * 1024 * 1024
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	if utf8.RuneCountInString(name) > 180 {
		name = string([]rune(name)[:180])
	}
	return name
}

func extOf(name string) string { return strings.ToLower(filepath.Ext(name)) }

func fileKind(name, mimeType string) string {
	ext := extOf(name)
	switch {
	case ext == ".html" || ext == ".htm":
		return "html"
	case ext == ".pdf":
		return "pdf"
	case textExt[ext]:
		return "text"
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	default:
		switch ext {
		case ".mp4", ".webm", ".mkv", ".mov", ".m4v", ".avi", ".ts", ".flv":
			return "video"
		case ".mp3", ".flac", ".wav", ".aac", ".ogg", ".m4a", ".opus":
			return "audio"
		case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".bmp", ".svg":
			return "image"
		case ".html", ".htm":
			return "html"
		}
	}
	return "file"
}

func kindAllowedInline(kind string) bool {
	switch kind {
	case "image", "video", "audio", "html", "pdf", "text":
		return true
	}
	return false
}

func normalizeParent(parent string) string {
	parent = strings.TrimSpace(parent)
	if parent == "" || parent == "root" {
		return ""
	}
	return parent
}

func joinVirtualPath(parentPath, name string) string {
	if parentPath == "" || parentPath == "/" {
		return "/" + name
	}
	return strings.TrimRight(parentPath, "/") + "/" + name
}

func (h *Handlers) fileByID(id string) (*File, error) {
	var f File
	if err := DB.First(&f, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

func (h *Handlers) canRead(f *File, userID uint, admin bool) bool {
	if f.Scope == "shared" {
		return f.Visibility != "restricted"
	}
	// Private archives are owner-only, including for administrators.
	return f.OwnerID == userID
}

func (h *Handlers) canWrite(f *File, userID uint, admin bool) bool {
	if f.Scope == "private" {
		return f.OwnerID == userID
	}
	return admin || f.OwnerID == userID
}

func (h *Handlers) uniqueName(scope string, ownerID uint, parentID, name string) string {
	base := name
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		var n int64
		q := DB.Model(&File{}).Where("scope = ? AND parent_id = ? AND name = ?", scope, parentID, name)
		if scope == "private" {
			q = q.Where("owner_id = ?", ownerID)
		}
		q.Count(&n)
		if n == 0 {
			return name
		}
		name = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if len(name) > 240 {
			name = base
			break
		}
	}
	return fmt.Sprintf("%s-%d%s", stem, time.Now().UnixNano(), ext)
}

func (h *Handlers) parentPath(scope string, parentID string) string {
	if parentID == "" {
		return "/"
	}
	var p File
	if err := DB.First(&p, "id = ?", parentID).Error; err != nil {
		return "/"
	}
	return p.Path
}

// parentFor validates that a parent folder exists in the same scope and, for
// private archives, belongs to the same user. This prevents writing entries into
// another user's private tree or mixing shared/private parents.
func (h *Handlers) parentFor(scope string, ownerID uint, parentID string) (*File, error) {
	if parentID == "" {
		return nil, nil
	}
	var p File
	if err := DB.First(&p, "id = ? AND scope = ? AND is_dir = ?", parentID, scope, true).Error; err != nil {
		return nil, fmt.Errorf("父目录不存在")
	}
	if scope == "private" && p.OwnerID != ownerID {
		return nil, fmt.Errorf("父目录不存在")
	}
	return &p, nil
}

func (h *Handlers) sharedUsed() int64 {
	var n int64
	DB.Model(&File{}).Where("scope = ? AND is_dir = ?", "shared", false).Select("COALESCE(SUM(size),0)").Scan(&n)
	return n
}

func (h *Handlers) privateUsed(userID uint) int64 {
	var n int64
	DB.Model(&File{}).Where("scope = ? AND owner_id = ? AND is_dir = ?", "private", userID, false).Select("COALESCE(SUM(size),0)").Scan(&n)
	return n
}

func (h *Handlers) refreshPrivateUsed(userID uint) {
	n := h.privateUsed(userID)
	DB.Model(&Profile{}).Where("user_id = ?", userID).UpdateColumn("private_used_bytes", n)
}

func (h *Handlers) ListFiles(c *gin.Context) {
	user := userOf(c)
	scope := c.DefaultQuery("scope", "shared")
	if user == nil && scope != "shared" {
		fail(c, "请先登录", 401)
		return
	}
	if scope != "shared" && scope != "private" {
		fail(c, "scope 只能是 shared 或 private", 400)
		return
	}
	parent := normalizeParent(c.Query("parent"))
	q := DB.Model(&File{}).Where("scope = ? AND parent_id = ?", scope, parent)
	if scope == "private" {
		q = q.Where("owner_id = ?", user.ID)
	} else {
		// Restricted hall archives are stored in the shared pack to avoid a
		// second private copy, but they must never appear in public listings.
		q = q.Where("visibility <> ?", "restricted")
	}
	var files []File
	if err := q.Order("is_dir DESC, name ASC").Find(&files).Error; err != nil {
		fail(c, "读取目录失败", 500)
		return
	}
	type fileView struct {
		File
		Uploader string `json:"uploader"`
	}
	out := make([]fileView, 0, len(files))
	for i := range files {
		name := ""
		if u, err := op.GetUserById(files[i].OwnerID); err == nil {
			name = u.Username
		}
		out = append(out, fileView{File: files[i], Uploader: name})
	}
	ok(c, gin.H{"items": out, "parent": parent, "scope": scope})
}

func (h *Handlers) Upload(c *gin.Context) {
	user := userOf(c)
	if user == nil {
		fail(c, "请先登录", 401)
		return
	}
	scope := c.PostForm("scope")
	if scope == "" {
		scope = "shared"
	}
	if scope != "shared" && scope != "private" {
		fail(c, "scope 只能是 shared 或 private", 400)
		return
	}
	parent := normalizeParent(c.PostForm("parent"))
	if _, err := h.parentFor(scope, user.ID, parent); err != nil {
		fail(c, "父目录不存在", 404)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUpload()+32*1024*1024)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, "读取上传文件失败："+err.Error(), 400)
		return
	}
	defer file.Close()
	name := sanitizeName(header.Filename)
	if blockedExt[extOf(name)] {
		fail(c, "不允许上传可执行文件", 403)
		return
	}
	if header.Size > maxUpload() {
		fail(c, "文件超过大小限制", 413)
		return
	}
	pf, err := EnsureProfile(user.ID, user.Username)
	if err != nil {
		fail(c, "读取用户资料失败", 500)
		return
	}
	used := h.privateUsed(user.ID)
	if scope == "private" {
		if !pf.PrivateEnabled {
			fail(c, "私人存储尚未开通", 403)
			return
		}
		if used+header.Size > pf.PrivateQuotaBytes {
			fail(c, fmt.Sprintf("私人空间不足（已用 %d MB / 配额 %d MB）", used>>20, pf.PrivateQuotaBytes>>20), 413)
			return
		}
	} else {
		if h.sharedUsed()+header.Size > Cfg.SharedMaxBytes {
			fail(c, "共享盘容量已满", 413)
			return
		}
	}
	tmp, err := os.CreateTemp(Cfg.TmpDir, "upload-*")
	if err != nil {
		fail(c, "创建临时文件失败", 500)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		fail(c, "写入上传文件失败", 500)
		return
	}
	if err := tmp.Close(); err != nil {
		fail(c, "写入上传文件失败", 500)
		return
	}
	st, err := os.Stat(tmpPath)
	if err != nil {
		fail(c, "读取上传文件失败", 500)
		return
	}
	actualSize := st.Size()
	if scope == "shared" && h.sharedUsed()+actualSize > Cfg.SharedMaxBytes {
		fail(c, "共享盘容量已满", 413)
		return
	}
	if scope == "private" && used+actualSize > pf.PrivateQuotaBytes {
		fail(c, "私人空间不足", 413)
		return
	}
	head := make([]byte, 512)
	fh, _ := os.Open(tmpPath)
	n, _ := fh.Read(head)
	fh.Close()
	mimeType := http.DetectContentType(head[:n])
	if byExt := mime.TypeByExtension(extOf(name)); byExt != "" {
		mimeType = byExt
	}
	kind := fileKind(name, mimeType)
	var key []byte
	if scope == "private" {
		key, _ = UserKey(user.ID)
	}
	blob, err := Store.Write(scope, user.ID, key, tmpPath, pf.CompressionLevel)
	if err != nil {
		fail(c, "归档写入失败："+err.Error(), 500)
		return
	}
	parentPath := h.parentPath(scope, parent)
	uniq := h.uniqueName(scope, user.ID, parent, name)
	f := &File{
		ID:         newID(),
		OwnerID:    user.ID,
		Scope:      scope,
		ParentID:   parent,
		Name:       uniq,
		Path:       joinVirtualPath(parentPath, uniq),
		Size:       actualSize,
		Mime:       mimeType,
		Kind:       kind,
		BlobID:     blob.ID,
		Visibility: "public",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := DB.Create(f).Error; err != nil {
		fail(c, "保存文件记录失败", 500)
		return
	}
	if scope == "private" {
		h.refreshPrivateUsed(user.ID)
	}
	Audit(user.ID, "upload", f.ID, c.ClientIP(), scope+":"+f.Name)
	ok(c, gin.H{"file": f, "blob": blob})
}

func (h *Handlers) CreateFolder(c *gin.Context) {
	user := userOf(c)
	if user == nil {
		fail(c, "请先登录", 401)
		return
	}
	var req struct {
		Scope    string `json:"scope"`
		ParentID string `json:"parent_id"`
		Parent   string `json:"parent"`
		Name     string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	if req.Scope != "shared" && req.Scope != "private" {
		fail(c, "scope 错误", 400)
		return
	}
	if req.ParentID == "" {
		req.ParentID = req.Parent
	}
	name := sanitizeName(req.Name)
	if name == "" {
		fail(c, "文件夹名不能为空", 400)
		return
	}
	parent := normalizeParent(req.ParentID)
	if _, err := h.parentFor(req.Scope, user.ID, parent); err != nil {
		fail(c, "父目录不存在", 404)
		return
	}
	name = h.uniqueName(req.Scope, user.ID, parent, name)
	pp := h.parentPath(req.Scope, parent)
	f := &File{
		ID: newID(), OwnerID: user.ID, Scope: req.Scope, ParentID: parent, Name: name,
		Path: joinVirtualPath(pp, name), IsDir: true, Kind: "folder", Mime: "inode/directory",
		Visibility: "public", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := DB.Create(f).Error; err != nil {
		fail(c, "创建文件夹失败", 500)
		return
	}
	ok(c, f)
}

func (h *Handlers) Rename(c *gin.Context) {
	user := userOf(c)
	if user == nil {
		fail(c, "请先登录", 401)
		return
	}
	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	f, err := h.fileByID(req.ID)
	if err != nil {
		fail(c, "文件不存在", 404)
		return
	}
	if !h.canWrite(f, user.ID, user.IsAdmin()) {
		fail(c, "只能修改自己上传的文件", 403)
		return
	}
	name := sanitizeName(req.Name)
	f.Name = name
	f.Path = joinVirtualPath(h.parentPath(f.Scope, f.ParentID), name)
	f.UpdatedAt = time.Now()
	DB.Save(f)
	ok(c, f)
}

func (h *Handlers) Move(c *gin.Context) {
	user := userOf(c)
	if user == nil {
		fail(c, "请先登录", 401)
		return
	}
	var req struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	f, err := h.fileByID(req.ID)
	if err != nil {
		fail(c, "文件不存在", 404)
		return
	}
	if !h.canWrite(f, user.ID, user.IsAdmin()) {
		fail(c, "只能移动自己上传的文件", 403)
		return
	}
	parent := normalizeParent(req.ParentID)
	if parent != "" {
		p, err := h.parentFor(f.Scope, f.OwnerID, parent)
		if err != nil || p == nil {
			fail(c, "目标文件夹无效", 400)
			return
		}
		if f.IsDir && (p.ID == f.ID || strings.HasPrefix(p.Path+"/", f.Path+"/")) {
			fail(c, "不能移动到自身或子目录", 400)
			return
		}
	}
	f.ParentID = parent
	f.Path = joinVirtualPath(h.parentPath(f.Scope, parent), f.Name)
	f.UpdatedAt = time.Now()
	DB.Save(f)
	ok(c, f)
}

func (h *Handlers) Delete(c *gin.Context) {
	user := userOf(c)
	if user == nil {
		fail(c, "请先登录", 401)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	f, err := h.fileByID(req.ID)
	if err != nil {
		fail(c, "文件不存在", 404)
		return
	}
	if !h.canWrite(f, user.ID, user.IsAdmin()) {
		fail(c, "只能删除自己上传的文件", 403)
		return
	}
	if f.IsDir {
		var count int64
		DB.Model(&File{}).Where("parent_id = ?", f.ID).Count(&count)
		if count > 0 {
			fail(c, "文件夹非空，请先删除内部文件", 409)
			return
		}
		DB.Delete(f)
		ok(c, nil)
		return
	}
	if f.BlobID != "" {
		if err := Store.Delete(f.BlobID); err != nil {
			fail(c, "删除归档内容失败", 500)
			return
		}
	}
	DB.Delete(f)
	if f.Scope == "private" {
		h.refreshPrivateUsed(f.OwnerID)
	}
	Audit(user.ID, "delete", f.ID, c.ClientIP(), f.Scope+":"+f.Name)
	ok(c, nil)
}

func (h *Handlers) FileMeta(c *gin.Context) {
	user := userOf(c)
	f, err := h.fileByID(c.Param("id"))
	if err != nil {
		fail(c, "文件不存在", 404)
		return
	}
	uploaderName := ""
	if u, err := op.GetUserById(f.OwnerID); err == nil {
		uploaderName = u.Username
	}
	var uid uint
	if user != nil {
		uid = user.ID
	}
	if !h.canRead(f, uid, user != nil && user.IsAdmin()) {
		fail(c, "无权访问", 403)
		return
	}
	ok(c, gin.H{"file": f, "uploader": gin.H{"id": f.OwnerID, "username": uploaderName}, "inline": kindAllowedInline(f.Kind), "url": "/f/" + f.ID + "/" + url.PathEscape(f.Name), "download": "/dl/" + f.ID + "/" + url.PathEscape(f.Name)})
}

func (h *Handlers) inlineHeaders(c *gin.Context, f *File) bool {
	if blockedExt[extOf(f.Name)] {
		fail(c, "不允许在线打开该类型文件", 403)
		return false
	}
	c.Header("X-Content-Type-Options", "nosniff")
	if f.Kind == "html" {
		c.Header("Content-Security-Policy", "sandbox allow-scripts allow-forms")
	} else if strings.Contains(f.Mime, "svg") {
		c.Header("Content-Security-Policy", "sandbox")
	}
	return true
}

func (h *Handlers) serveFile(c *gin.Context, f *File, attachment bool) {
	if !attachment && !kindAllowedInline(f.Kind) {
		attachment = true
	}
	if !h.inlineHeaders(c, f) {
		return
	}
	var key []byte
	if f.Scope == "private" {
		key, _ = UserKey(f.OwnerID)
	}
	path, temp, err := Store.Materialize(f.BlobID, key)
	if err != nil {
		fail(c, "读取文件内容失败", 500)
		return
	}
	if temp {
		defer os.Remove(path)
	}
	name := url.PathEscape(f.Name)
	if attachment {
		c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+name)
	} else {
		c.Header("Content-Disposition", "inline; filename*=UTF-8''"+name)
	}
	if f.Mime != "" {
		c.Header("Content-Type", f.Mime)
	}
	if f.Scope == "shared" && !attachment {
		// Content-addressed data is immutable; this lets Cloudflare cache direct
		// links. Private responses must never land in a shared cache.
		c.Header("Cache-Control", "public, max-age=86400")
	} else if f.Scope == "shared" {
		c.Header("Cache-Control", "public, max-age=86400")
	} else {
		c.Header("Cache-Control", "private, no-store")
	}
	http.ServeFile(c.Writer, c.Request, path)
}

func (h *Handlers) PublicFile(c *gin.Context) {
	f, err := h.fileByID(c.Param("id"))
	if err != nil {
		fail(c, "文件不存在", 404)
		return
	}
	user := CurrentUser(c)
	var uid uint
	if user != nil {
		uid = user.ID
	}
	if !h.canRead(f, uid, user != nil && user.IsAdmin()) {
		fail(c, "私人文件需要登录或签名链接", 403)
		return
	}
	h.serveFile(c, f, false)
}

func (h *Handlers) DownloadFile(c *gin.Context) {
	f, err := h.fileByID(c.Param("id"))
	if err != nil {
		fail(c, "文件不存在", 404)
		return
	}
	user := CurrentUser(c)
	var uid uint
	if user != nil {
		uid = user.ID
	}
	if !h.canRead(f, uid, user != nil && user.IsAdmin()) {
		fail(c, "无权下载", 403)
		return
	}
	h.serveFile(c, f, true)
}

func signFile(f *File, hours int) string {
	exp := time.Now().Add(time.Duration(hours) * time.Hour).Unix()
	payload := fmt.Sprintf("%s|%d", f.ID, exp)
	mac := hmac.New(sha256.New, Cfg.MasterKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifyFileToken(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	payload := string(raw)
	bits := strings.Split(payload, "|")
	if len(bits) != 2 {
		return "", false
	}
	exp, _ := strconv.ParseInt(bits[1], 10, 64)
	if time.Now().Unix() > exp {
		return "", false
	}
	mac := hmac.New(sha256.New, Cfg.MasterKey)
	mac.Write([]byte(payload))
	sig, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	return bits[0], true
}

func (h *Handlers) SignedFile(c *gin.Context) {
	id, valid := verifyFileToken(c.Param("token"))
	if !valid {
		fail(c, "链接无效或已过期", 403)
		return
	}
	f, err := h.fileByID(id)
	if err != nil {
		fail(c, "文件不存在", 404)
		return
	}
	h.serveFile(c, f, false)
}

func (h *Handlers) SignFile(c *gin.Context) {
	user := userOf(c)
	if user == nil {
		fail(c, "请先登录", 401)
		return
	}
	f, err := h.fileByID(c.Param("id"))
	if err != nil {
		fail(c, "文件不存在", 404)
		return
	}
	if !h.canRead(f, user.ID, user.IsAdmin()) {
		fail(c, "无权访问", 403)
		return
	}
	hours := 24
	ok(c, gin.H{"url": "/s/" + signFile(f, hours), "expires_in": hours * 3600})
}

func (h *Handlers) Inherit() {}

func newID() string {
	return randomHex(16)
}

// sorting helper used by list endpoints
func sortFiles(files []File) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})
}

func hexString(b []byte) string { return hex.EncodeToString(b) }
