package om

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

// This file is the bridge used by drivers/om. It deliberately reuses the same
// om_files / om_blobs rows as the HTTP API, so the native OpenList file browser
// and the OM pages always see one dataset.

var driverHandlers = &Handlers{}

func driverScopeUser(scope string, u *model.User) (uint, error) {
	if scope == "shared" {
		return 0, nil
	}
	if u == nil || u.IsGuest() {
		return 0, errors.New("请先登录")
	}
	return u.ID, nil
}

func driverScope(scope string, ownerID uint) (string, uint) {
	if scope == "private" {
		return "private", ownerID
	}
	return "shared", 0
}

// driverRelPath converts /shared/foo or /private/foo to the scope-relative
// path stored in om_files. The native browser passes mount-relative paths, so
// the scope prefix must be removed before any DB operation.
func driverRelPath(scope, p string) string {
	p = normalizeParent(p)
	if scope == "shared" {
		p = strings.TrimPrefix(p, "/shared")
	} else {
		p = strings.TrimPrefix(p, "/private")
	}
	if p == "" {
		p = "/"
	}
	return normalizeParent(p)
}

// DriverList returns the children of a virtual directory inside one scope.
func DriverList(u *model.User, scope, dirPath string) ([]File, error) {
	owner, err := driverScopeUser(scope, u)
	if err != nil {
		return nil, err
	}
	scope, owner = driverScope(scope, owner)
	dirPath = normalizeParent(dirPath)
	parentID := ""
	if dirPath != "/" {
		p, err := DriverGet(u, scope, dirPath)
		if err != nil {
			return nil, err
		}
		if !p.IsDir {
			return nil, errors.New("不是文件夹")
		}
		parentID = p.ID
	}
	q := DB.Model(&File{}).Where("scope = ? AND parent_id = ?", scope, parentID)
	if scope == "private" {
		q = q.Where("owner_id = ?", owner)
	} else {
		q = q.Where("visibility <> ?", "restricted")
	}
	var files []File
	if err := q.Order("is_dir DESC, name ASC").Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

// DriverGet resolves a virtual path inside one scope.
func DriverGet(u *model.User, scope, p string) (*File, error) {
	owner, err := driverScopeUser(scope, u)
	if err != nil {
		return nil, err
	}
	scope, owner = driverScope(scope, owner)
	p = normalizeParent(p)
	q := DB.Model(&File{}).Where("scope = ? AND path = ?", scope, p)
	if scope == "private" {
		q = q.Where("owner_id = ?", owner)
	} else {
		q = q.Where("visibility <> ?", "restricted")
	}
	var f File
	if err := q.First(&f).Error; err != nil {
		return nil, errors.New("文件不存在")
	}
	return &f, nil
}

func driverParent(u *model.User, scope string, parentPath string) (*File, error) {
	parentPath = strings.TrimSpace(parentPath)
	switch parentPath {
	case "", "/":
		return nil, nil
	case "/shared":
		if scope != "shared" {
			return nil, errors.New("不能跨共享盘/私人盘操作")
		}
		return nil, nil
	case "/private":
		if scope != "private" {
			return nil, errors.New("不能跨共享盘/私人盘操作")
		}
		return nil, nil
	}
	if scope == "shared" {
		parentPath = strings.TrimPrefix(parentPath, "/shared")
	} else {
		parentPath = strings.TrimPrefix(parentPath, "/private")
	}
	p, err := DriverGet(u, scope, parentPath)
	if err != nil {
		return nil, err
	}
	if !p.IsDir {
		return nil, errors.New("父目录不是文件夹")
	}
	if scope == "private" && (u == nil || p.OwnerID != u.ID) {
		return nil, errors.New("父目录不存在")
	}
	return p, nil
}

// DriverFileByID is the permission-checked lookup used by the driver Link path.
func DriverFileByID(u *model.User, id string) (*File, error) {
	var f File
	if err := DB.Where("id = ?", id).First(&f).Error; err != nil {
		return nil, errors.New("文件不存在")
	}
	var uid uint
	var admin bool
	if u != nil {
		uid = u.ID
		admin = u.IsAdmin()
	}
	if !driverHandlers.canRead(&f, uid, admin) {
		return nil, errors.New("无权访问该文件")
	}
	return &f, nil
}

// DriverURL returns a CDN-friendly URL and its lifetime. Shared files are
// permanently addressable; private files use a short-lived signed token.
func DriverURL(f *File, privateTTL time.Duration) (string, time.Duration) {
	if f.Scope == "private" {
		hours := int(privateTTL / time.Hour)
		if hours < 1 {
			hours = 1
		}
		return Cfg.BaseURL + "/s/" + signFile(f, hours), privateTTL
	}
	return Cfg.BaseURL + "/dl/" + f.ID + "/" + url.PathEscape(f.Name), 0
}

// DriverSave stores an upload coming from the OpenList Put interface.
func DriverSave(u *model.User, scope, parentPath, name string, size int64, modTime time.Time, r io.Reader) (*File, error) {
	if u == nil || u.IsGuest() {
		return nil, errors.New("请先登录")
	}
	scope, _ = driverScope(scope, u.ID)
	parentPath = driverRelPath(scope, parentPath)
	name = sanitizeName(name)
	if name == "" {
		return nil, errors.New("文件名不能为空")
	}
	if blockedExt[extOf(name)] {
		return nil, errors.New("不允许上传可执行文件")
	}
	if size > maxUpload() {
		return nil, errors.New("文件超过大小限制")
	}
	parent, err := driverParent(u, scope, parentPath)
	if err != nil {
		return nil, err
	}
	parentID := ""
	if parent != nil {
		parentID = parent.ID
	}
	pf, err := EnsureProfile(u.ID, u.Username)
	if err != nil {
		return nil, err
	}
	if scope == "private" {
		if !pf.PrivateEnabled {
			return nil, errors.New("私人存储尚未开通")
		}
		if driverHandlers.privateUsed(u.ID)+size > pf.PrivateQuotaBytes {
			return nil, errors.New("私人空间不足")
		}
	} else if size > 0 && driverHandlers.sharedUsed()+size > Cfg.SharedMaxBytes {
		return nil, errors.New("共享盘容量已满")
	}

	// Overwriting is expected in the native browser. Only overwrite files the
	// current user is allowed to modify.
	var old *File
	var prev File
	q := DB.Where("scope = ? AND path = ? AND is_dir = ?", scope, joinVirtualPath(parentPath, name), false)
	if scope == "private" {
		q = q.Where("owner_id = ?", u.ID)
	}
	if err := q.First(&prev).Error; err == nil {
		old = &prev
	}
	if old != nil && !driverHandlers.canWrite(old, u.ID, u.IsAdmin()) {
		return nil, errors.New("只能覆盖自己上传的文件")
	}

	tmp, err := os.CreateTemp(Cfg.TmpDir, "olput-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	written, err := io.Copy(tmp, r)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if written > maxUpload() {
		return nil, errors.New("文件超过大小限制")
	}
	actual := written
	if scope == "private" && driverHandlers.privateUsed(u.ID)+actual > pf.PrivateQuotaBytes {
		return nil, errors.New("私人空间不足")
	}
	if scope == "shared" && driverHandlers.sharedUsed()+actual > Cfg.SharedMaxBytes {
		return nil, errors.New("共享盘容量已满")
	}
	head := make([]byte, 512)
	fh, _ := os.Open(tmpPath)
	n, _ := fh.Read(head)
	fh.Close()
	mimeType := detectMime(name, head[:n])
	kind := fileKind(name, mimeType)
	var key []byte
	if scope == "private" {
		key, _ = UserKey(u.ID)
	}
	blob, err := Store.Write(scope, u.ID, key, tmpPath, pf.CompressionLevel)
	if err != nil {
		return nil, err
	}
	if old != nil {
		if old.BlobID != "" {
			_ = Store.Delete(old.BlobID)
		}
		DB.Delete(old)
	}
	f := &File{
		ID:         newID(),
		OwnerID:    u.ID,
		Scope:      scope,
		ParentID:   parentID,
		Name:       name,
		Path:       joinVirtualPath(parentPath, name),
		Size:       actual,
		Mime:       mimeType,
		Kind:       kind,
		BlobID:     blob.ID,
		Visibility: "public",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if !modTime.IsZero() {
		f.CreatedAt = modTime
	}
	if err := DB.Create(f).Error; err != nil {
		return nil, err
	}
	if scope == "private" {
		driverHandlers.refreshPrivateUsed(u.ID)
	}
	Audit(u.ID, "upload", f.ID, "", scope+":"+f.Name)
	return f, nil
}

func driverRefreshFile(f *File, newPath, newName string) {
	f.Name = newName
	f.Path = newPath
	f.UpdatedAt = time.Now()
	DB.Save(f)
}

// DriverMkdir creates a folder; existing folders are treated as success.
func DriverMkdir(u *model.User, scope, parentPath, name string) (*File, error) {
	if u == nil || u.IsGuest() {
		return nil, errors.New("请先登录")
	}
	scope, owner := driverScope(scope, u.ID)
	parentPath = driverRelPath(scope, parentPath)
	name = sanitizeName(name)
	if name == "" {
		return nil, errors.New("文件夹名不能为空")
	}
	if strings.Contains(name, "/") {
		return nil, errors.New("文件夹名不能包含斜杠")
	}
	parent, err := driverParent(u, scope, parentPath)
	if err != nil {
		return nil, err
	}
	parentID := ""
	pp := normalizeParent(parentPath)
	if parent != nil {
		parentID = parent.ID
		pp = parent.Path
	}
	full := joinVirtualPath(pp, name)
	var existing File
	q := DB.Where("scope = ? AND path = ?", scope, full)
	if scope == "private" {
		q = q.Where("owner_id = ?", owner)
	}
	if err := q.First(&existing).Error; err == nil {
		if existing.IsDir {
			return &existing, nil
		}
		return nil, errors.New("同名文件已存在")
	}
	f := &File{
		ID: newID(), OwnerID: u.ID, Scope: scope, ParentID: parentID, Name: name,
		Path: full, IsDir: true, Kind: "folder", Mime: "inode/directory",
		Visibility: "public", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := DB.Create(f).Error; err != nil {
		return nil, err
	}
	Audit(u.ID, "mkdir", f.ID, "", scope+":"+f.Name)
	return f, nil
}

func driverCanWrite(u *model.User, f *File) bool {
	if u == nil || u.IsGuest() {
		return false
	}
	return driverHandlers.canWrite(f, u.ID, u.IsAdmin())
}

// DriverRename updates a file or folder name and all descendant paths.
func DriverRename(u *model.User, f *File, newName string) (*File, error) {
	if !driverCanWrite(u, f) {
		return nil, errors.New("只能修改自己上传的文件")
	}
	newName = sanitizeName(newName)
	if newName == "" || strings.Contains(newName, "/") {
		return nil, errors.New("文件名无效")
	}
	oldPath := f.Path
	parentPath := path.Dir(oldPath)
	if parentPath == "." {
		parentPath = "/"
	}
	newPath := joinVirtualPath(parentPath, newName)
	if newPath == oldPath {
		return f, nil
	}
	var clash int64
	q := DB.Model(&File{}).Where("scope = ? AND path = ? AND id <> ?", f.Scope, newPath, f.ID)
	if f.Scope == "private" {
		q = q.Where("owner_id = ?", f.OwnerID)
	}
	q.Count(&clash)
	if clash > 0 {
		return nil, errors.New("同名文件已存在")
	}
	driverRefreshFile(f, newPath, newName)
	if f.IsDir {
		driverReindexChildren(f.Scope, f.OwnerID, oldPath, newPath)
	}
	return f, nil
}

func driverReindexChildren(scope string, ownerID uint, oldPath, newPath string) {
	var children []File
	q := DB.Where("scope = ? AND path LIKE ?", scope, oldPath+"/%")
	if scope == "private" {
		q = q.Where("owner_id = ?", ownerID)
	}
	q.Find(&children)
	for i := range children {
		children[i].Path = newPath + strings.TrimPrefix(children[i].Path, oldPath)
		children[i].UpdatedAt = time.Now()
		DB.Save(&children[i])
	}
}

// DriverMove moves a file/folder under another directory.
func DriverMove(u *model.User, f *File, dstDirPath string) (*File, error) {
	if !driverCanWrite(u, f) {
		return nil, errors.New("只能移动自己上传的文件")
	}
	dstDirPath = driverRelPath(f.Scope, dstDirPath)
	dst, err := driverParent(u, f.Scope, dstDirPath)
	if err != nil {
		return nil, err
	}
	if dst != nil && dst.ID == f.ID {
		return nil, errors.New("不能移动到自身")
	}
	dstPath := normalizeParent(dstDirPath)
	if dst != nil {
		dstPath = dst.Path
	}
	oldPath := f.Path
	newPath := joinVirtualPath(dstPath, f.Name)
	if f.IsDir && strings.HasPrefix(newPath+"/", oldPath+"/") {
		return nil, errors.New("不能移动到子目录")
	}
	parentID := ""
	if dst != nil {
		parentID = dst.ID
	}
	f.ParentID = parentID
	driverRefreshFile(f, newPath, f.Name)
	if f.IsDir {
		driverReindexChildren(f.Scope, f.OwnerID, oldPath, newPath)
	}
	return f, nil
}

// DriverRemove removes a file or a whole folder tree. A shared folder is only
// removable when every item under it belongs to the current user (or the user
// is an administrator), preserving the "own uploads only" rule.
func DriverRemove(u *model.User, f *File) error {
	if !driverCanWrite(u, f) {
		return errors.New("只能删除自己上传的文件")
	}
	var files []File
	q := DB.Where("scope = ? AND path LIKE ?", f.Scope, f.Path+"/%")
	if f.Scope == "private" {
		q = q.Where("owner_id = ?", f.OwnerID)
	}
	q.Find(&files)
	for i := range files {
		if !driverCanWrite(u, &files[i]) {
			return errors.New("文件夹内包含他人的文件，无法删除")
		}
	}
	for i := range files {
		if files[i].IsDir {
			DB.Delete(&files[i])
			continue
		}
		if files[i].BlobID != "" {
			if err := Store.Delete(files[i].BlobID); err != nil {
				return err
			}
		}
		DB.Delete(&files[i])
	}
	if !f.IsDir && f.BlobID != "" {
		if err := Store.Delete(f.BlobID); err != nil {
			return err
		}
	}
	DB.Delete(f)
	if f.Scope == "private" {
		driverHandlers.refreshPrivateUsed(f.OwnerID)
	}
	Audit(u.ID, "delete", f.ID, "", f.Scope+":"+f.Name)
	return nil
}

func detectMime(name string, head []byte) string {
	mimeType := "application/octet-stream"
	if len(head) > 0 {
		mimeType = http.DetectContentType(head)
	}
	if byExt := mime.TypeByExtension(extOf(name)); byExt != "" {
		mimeType = byExt
	}
	return mimeType
}
