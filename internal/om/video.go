package om

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) videoView(v *Video) gin.H {
	var f *File
	if ff, err := h.fileByID(v.FileID); err == nil {
		f = ff
	}
	var cover *File
	if v.CoverFileID != "" {
		if cf, err := h.fileByID(v.CoverFileID); err == nil {
			cover = cf
		}
	}
	name, avatar := "", ""
	if u, err := op.GetUserById(v.UploaderID); err == nil {
		name = u.Username
		if pf, err := EnsureProfile(u.ID, u.Username); err == nil {
			avatar = pf.AvatarFileID
		}
	}
	return gin.H{"video": v, "file": f, "cover": cover, "uploader": gin.H{"id": v.UploaderID, "username": name, "avatar": avatar}}
}

func (h *Handlers) VideoList(c *gin.Context) {
	q := DB.Model(&Video{})
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR description LIKE ? OR tags LIKE ?", like, like, like)
	}
	if cat := strings.TrimSpace(c.Query("category")); cat != "" {
		q = q.Where("category = ?", cat)
	}
	if uid := c.Query("uploader"); uid != "" {
		q = q.Where("uploader_id = ?", uid)
	}
	switch c.DefaultQuery("sort", "new") {
	case "hot":
		q = q.Order("views DESC, like_count DESC")
	case "like":
		q = q.Order("like_count DESC")
	default:
		q = q.Order("id DESC")
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size := 24
	var total int64
	q.Count(&total)
	var vs []Video
	q.Offset((page - 1) * size).Limit(size).Find(&vs)
	out := make([]gin.H, 0, len(vs))
	for i := range vs {
		out = append(out, h.videoView(&vs[i]))
	}
	ok(c, gin.H{"items": out, "total": total, "page": page})
}

func (h *Handlers) VideoGet(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var v Video
	if err := DB.First(&v, id).Error; err != nil {
		fail(c, "视频不存在", 404)
		return
	}
	DB.Model(&v).UpdateColumn("views", gorm.Expr("views + 1"))
	v.Views++
	data := h.videoView(&v)
	var reaction []string
	if user := userOf(c); user != nil {
		DB.Model(&Reaction{}).Where("user_id = ? AND target_type = ? AND target_id = ?", user.ID, "video", v.ID).Pluck("kind", &reaction)
	}
	data["my_reactions"] = reaction
	ok(c, data)
}

func (h *Handlers) VideoCreate(c *gin.Context) {
	user := userOf(c)
	var req struct {
		FileID      string `json:"file_id"`
		CoverFileID string `json:"cover_file_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Tags        string `json:"tags"`
		Duration    int    `json:"duration"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	f, err := h.fileByID(req.FileID)
	if err != nil || f.IsDir {
		fail(c, "视频文件不存在", 404)
		return
	}
	if f.Kind != "video" && f.Kind != "audio" {
		fail(c, "该文件不是视频或音频", 400)
		return
	}
	if !h.canRead(f, user.ID, user.IsAdmin()) {
		fail(c, "无权使用该文件", 403)
		return
	}
	if f.Scope != "shared" {
		fail(c, "视频站只接受共享盘文件", 400)
		return
	}
	if req.CoverFileID != "" {
		cf, err := h.fileByID(req.CoverFileID)
		if err != nil || cf.Kind != "image" || cf.Scope != "shared" {
			fail(c, "封面必须是共享盘图片", 400)
			return
		}
	}
	if strings.TrimSpace(req.Title) == "" {
		req.Title = f.Name
	}
	v := &Video{
		FileID: f.ID, CoverFileID: req.CoverFileID, Title: req.Title, Description: req.Description,
		Category: req.Category, Tags: req.Tags, UploaderID: user.ID, Duration: req.Duration,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := DB.Create(v).Error; err != nil {
		fail(c, "创建视频失败", 500)
		return
	}
	ok(c, h.videoView(v))
}

func (h *Handlers) ImageList(c *gin.Context) {
	q := DB.Model(&Image{})
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR tags LIKE ?", like, like)
	}
	if uid := c.Query("uploader"); uid != "" {
		q = q.Where("uploader_id = ?", uid)
	}
	q = q.Order("id DESC")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	var total int64
	q.Count(&total)
	var items []Image
	q.Offset((page - 1) * 24).Limit(24).Find(&items)
	out := make([]gin.H, 0, len(items))
	for i := range items {
		out = append(out, h.imageView(&items[i]))
	}
	ok(c, gin.H{"items": out, "total": total, "page": page})
}

func (h *Handlers) imageView(img *Image) gin.H {
	var f *File
	if ff, err := h.fileByID(img.FileID); err == nil {
		f = ff
	}
	name := ""
	if u, err := op.GetUserById(img.UploaderID); err == nil {
		name = u.Username
	}
	return gin.H{"image": img, "file": f, "uploader": gin.H{"id": img.UploaderID, "username": name}}
}

func (h *Handlers) ImageGet(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var img Image
	if err := DB.First(&img, id).Error; err != nil {
		fail(c, "图片不存在", 404)
		return
	}
	DB.Model(&img).UpdateColumn("views", gorm.Expr("views + 1"))
	img.Views++
	data := h.imageView(&img)
	var reaction []string
	if user := userOf(c); user != nil {
		DB.Model(&Reaction{}).Where("user_id = ? AND target_type = ? AND target_id = ?", user.ID, "image", img.ID).Pluck("kind", &reaction)
	}
	data["my_reactions"] = reaction
	ok(c, data)
}

func (h *Handlers) ImageCreate(c *gin.Context) {
	user := userOf(c)
	var req struct {
		FileID      string `json:"file_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Tags        string `json:"tags"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	f, err := h.fileByID(req.FileID)
	if err != nil || f.Kind != "image" {
		fail(c, "图片文件不存在", 404)
		return
	}
	if !h.canRead(f, user.ID, user.IsAdmin()) {
		fail(c, "无权使用该文件", 403)
		return
	}
	if f.Scope != "shared" {
		fail(c, "图片站只接受共享盘文件", 400)
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		req.Title = f.Name
	}
	img := &Image{FileID: f.ID, Title: req.Title, Description: req.Description, Tags: req.Tags, UploaderID: user.ID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := DB.Create(img).Error; err != nil {
		fail(c, "创建图片失败", 500)
		return
	}
	ok(c, h.imageView(img))
}

func (h *Handlers) React(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Kind       string `json:"kind"`
		TargetType string `json:"target_type"`
		TargetID   uint   `json:"target_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	if req.TargetType == "" {
		req.TargetType = strings.TrimSuffix(c.FullPath(), "/:id/react")
		req.TargetType = strings.TrimPrefix(req.TargetType, "/api/om/")
	}
	if req.TargetID == 0 {
		n, _ := strconv.Atoi(c.Param("id"))
		req.TargetID = uint(n)
	}
	if req.Kind != "like" && req.Kind != "coin" && req.Kind != "favorite" {
		fail(c, "不支持的操作", 400)
		return
	}
	if req.TargetType != "video" && req.TargetType != "image" && req.TargetType != "post" && req.TargetType != "comment" {
		fail(c, "不支持的目标", 400)
		return
	}
	var existing Reaction
	err := DB.Where("user_id = ? AND target_type = ? AND target_id = ? AND kind = ?", user.ID, req.TargetType, req.TargetID, req.Kind).First(&existing).Error
	added := false
	if err == gorm.ErrRecordNotFound {
		if req.Kind == "coin" {
			pf, _ := EnsureProfile(user.ID, user.Username)
			if pf.Coins <= 0 {
				fail(c, "硬币不足", 400)
				return
			}
			pf.Coins--
			DB.Save(pf)
		}
		DB.Create(&Reaction{UserID: user.ID, TargetType: req.TargetType, TargetID: req.TargetID, Kind: req.Kind, Value: 1, CreatedAt: time.Now()})
		added = true
	} else if err == nil {
		DB.Delete(&existing)
		if req.Kind == "coin" {
			DB.Model(&Profile{}).Where("user_id = ?", user.ID).UpdateColumn("coins", gorm.Expr("coins + 1"))
		}
	}
	h.bumpReaction(req.TargetType, req.TargetID, req.Kind, added)
	ok(c, gin.H{"active": added, "kind": req.Kind})
}

func (h *Handlers) bumpReaction(targetType string, targetID uint, kind string, add bool) {
	delta := int64(1)
	if !add {
		delta = -1
	}
	col := map[string]string{"like": "like_count", "coin": "coin_count", "favorite": "fav_count"}[kind]
	if col == "" {
		return
	}
	switch targetType {
	case "video":
		DB.Model(&Video{}).Where("id = ?", targetID).UpdateColumn(col, gorm.Expr(col+" + ?", delta))
	case "image":
		if kind == "like" || kind == "favorite" {
			DB.Model(&Image{}).Where("id = ?", targetID).UpdateColumn(col, gorm.Expr(col+" + ?", delta))
		}
	case "post":
		if kind == "like" {
			DB.Model(&Post{}).Where("id = ?", targetID).UpdateColumn("like_count", gorm.Expr("like_count + ?", delta))
		}
	}
}

func (h *Handlers) CommentList(c *gin.Context) {
	targetType := c.DefaultQuery("target_type", "video")
	if targetType == "" {
		targetType = "video"
	}
	id, _ := strconv.Atoi(c.Param("id"))
	var items []Comment
	DB.Where("target_type = ? AND target_id = ?", targetType, uint(id)).Order("id DESC").Limit(200).Find(&items)
	out := make([]gin.H, 0, len(items))
	for _, cm := range items {
		name := ""
		if u, err := op.GetUserById(cm.UserID); err == nil {
			name = u.Username
		}
		out = append(out, gin.H{"comment": cm, "username": name})
	}
	ok(c, gin.H{"items": out})
}

func (h *Handlers) CommentCreate(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Content string `json:"content"`
		Parent  uint   `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || len([]rune(req.Content)) > 1000 {
		fail(c, "评论内容为空或过长", 400)
		return
	}
	targetType := c.DefaultQuery("target_type", "video")
	id, _ := strconv.Atoi(c.Param("id"))
	cm := &Comment{TargetType: targetType, TargetID: uint(id), UserID: user.ID, ParentID: req.Parent, Content: req.Content, CreatedAt: time.Now()}
	if err := DB.Create(cm).Error; err != nil {
		fail(c, "评论失败", 500)
		return
	}
	switch targetType {
	case "video":
		DB.Model(&Video{}).Where("id = ?", id).UpdateColumn("comment_count", gorm.Expr("comment_count + 1"))
	case "image":
		DB.Model(&Image{}).Where("id = ?", id).UpdateColumn("comment_count", gorm.Expr("comment_count + 1"))
	case "post":
		DB.Model(&Post{}).Where("id = ?", id).UpdateColumn("comment_count", gorm.Expr("comment_count + 1"))
	}
	Audit(user.ID, "comment", fmt.Sprint(id), c.ClientIP(), targetType)
	ok(c, cm)
}

func (h *Handlers) DanmakuList(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var items []Danmaku
	DB.Where("video_id = ?", id).Order("time_ms ASC").Limit(5000).Find(&items)
	ok(c, gin.H{"items": items})
}

func (h *Handlers) DanmakuCreate(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Content string `json:"content"`
		TimeMs  int64  `json:"time_ms"`
		Mode    int    `json:"mode"`
		Color   string `json:"color"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || len([]rune(req.Content)) > 80 {
		fail(c, "弹幕内容为空或过长", 400)
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	d := &Danmaku{VideoID: uint(id), UserID: user.ID, TimeMs: req.TimeMs, Mode: req.Mode, Color: req.Color, Content: req.Content, CreatedAt: time.Now()}
	DB.Create(d)
	DB.Model(&Video{}).Where("id = ?", id).UpdateColumn("danmaku_count", gorm.Expr("danmaku_count + 1"))
	ok(c, d)
}

func (h *Handlers) ProgressSave(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Position int `json:"position"`
		Duration int `json:"duration"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	id, _ := strconv.Atoi(c.Param("id"))
	var p WatchProgress
	err := DB.First(&p, "user_id = ? AND video_id = ?", user.ID, id).Error
	if err == gorm.ErrRecordNotFound {
		p = WatchProgress{UserID: user.ID, VideoID: uint(id), Position: req.Position, Duration: req.Duration, UpdatedAt: time.Now()}
		DB.Create(&p)
	} else {
		DB.Model(&p).Updates(map[string]any{"position": req.Position, "duration": req.Duration, "updated_at": time.Now()})
	}
	ok(c, p)
}

func (h *Handlers) ProgressGet(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var p WatchProgress
	if err := DB.First(&p, "user_id = ? AND video_id = ?", user.ID, id).Error; err != nil {
		ok(c, nil)
		return
	}
	ok(c, p)
}
