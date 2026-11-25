package om

import (
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) postView(p *Post) gin.H {
	name, avatar := "", ""
	if u, err := op.GetUserById(p.UserID); err == nil {
		name = u.Username
		if pf, err := EnsureProfile(u.ID, u.Username); err == nil {
			avatar = pf.AvatarFileID
		}
	}
	var media []PostMedia
	DB.Where("post_id = ?", p.ID).Order("sort ASC").Find(&media)
	files := make([]gin.H, 0, len(media))
	for _, m := range media {
		if f, err := h.fileByID(m.FileID); err == nil {
			files = append(files, gin.H{"id": m.FileID, "kind": f.Kind, "mime": f.Mime, "url": "/f/" + f.ID, "name": f.Name})
		}
	}
	var original *Post
	if p.RepostOf != 0 {
		var op Post
		if err := DB.First(&op, p.RepostOf).Error; err == nil {
			original = &op
		}
	}
	return gin.H{"post": p, "author": gin.H{"id": p.UserID, "username": name, "avatar": avatar}, "media": files, "original": original}
}

func (h *Handlers) PostList(c *gin.Context) {
	q := DB.Model(&Post{})
	if uid := c.Query("user"); uid != "" {
		q = q.Where("user_id = ?", uid)
	}
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		q = q.Where("content LIKE ?", "%"+kw+"%")
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	var total int64
	q.Count(&total)
	var posts []Post
	q.Order("id DESC").Offset((page - 1) * 30).Limit(30).Find(&posts)
	out := make([]gin.H, 0, len(posts))
	for i := range posts {
		out = append(out, h.postView(&posts[i]))
	}
	ok(c, gin.H{"items": out, "total": total, "page": page})
}

func (h *Handlers) PostGet(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var p Post
	if err := DB.First(&p, id).Error; err != nil {
		fail(c, "动态不存在", 404)
		return
	}
	data := h.postView(&p)
	var reaction []string
	if user := userOf(c); user != nil {
		DB.Model(&Reaction{}).Where("user_id = ? AND target_type = ? AND target_id = ?", user.ID, "post", p.ID).Pluck("kind", &reaction)
	}
	data["my_reactions"] = reaction
	ok(c, data)
}

func (h *Handlers) PostCreate(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Content     string   `json:"content"`
		MediaFileID []string `json:"media_file_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" && len(req.MediaFileID) == 0 {
		fail(c, "内容不能为空", 400)
		return
	}
	if len([]rune(req.Content)) > 2000 {
		fail(c, "正文最多 2000 字", 400)
		return
	}
	p := &Post{UserID: user.ID, Content: req.Content, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := DB.Create(p).Error; err != nil {
		fail(c, "发布失败", 500)
		return
	}
	for i, fid := range req.MediaFileID {
		f, err := h.fileByID(fid)
		if err != nil || !h.canRead(f, user.ID, user.IsAdmin()) {
			continue
		}
		if f.Kind != "image" && f.Kind != "video" && f.Kind != "audio" {
			continue
		}
		if f.Scope != "shared" {
			continue
		}
		DB.Create(&PostMedia{PostID: p.ID, FileID: fid, Sort: i})
	}
	Audit(user.ID, "post.create", strconv.Itoa(int(p.ID)), c.ClientIP(), "")
	ok(c, h.postView(p))
}

func (h *Handlers) PostRepost(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var src Post
	if err := DB.First(&src, id).Error; err != nil {
		fail(c, "动态不存在", 404)
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	_ = c.ShouldBindJSON(&req)
	root := src.RootID
	if root == 0 {
		root = src.ID
	}
	p := &Post{UserID: user.ID, Content: req.Content, RepostOf: src.ID, RootID: root, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := DB.Create(p).Error; err != nil {
		fail(c, "转发失败", 500)
		return
	}
	DB.Model(&Post{}).Where("id = ?", src.ID).UpdateColumn("repost_count", gorm.Expr("repost_count + 1"))
	ok(c, h.postView(p))
}

func (h *Handlers) PostCommentList(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var items []Comment
	DB.Where("target_type = ? AND target_id = ?", "post", uint(id)).Order("id ASC").Limit(300).Find(&items)
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

func (h *Handlers) PostCommentCreate(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
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
	cm := &Comment{TargetType: "post", TargetID: uint(id), UserID: user.ID, ParentID: req.Parent, Content: req.Content, CreatedAt: time.Now()}
	DB.Create(cm)
	DB.Model(&Post{}).Where("id = ?", id).UpdateColumn("comment_count", gorm.Expr("comment_count + 1"))
	ok(c, cm)
}

func (h *Handlers) FollowToggle(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	if uint(id) == user.ID {
		fail(c, "不能关注自己", 400)
		return
	}
	var f Follow
	err := DB.First(&f, "follower_id = ? AND followee_id = ?", user.ID, id).Error
	active := false
	if err == gorm.ErrRecordNotFound {
		DB.Create(&Follow{FollowerID: user.ID, FolloweeID: uint(id), CreatedAt: time.Now()})
		active = true
	} else {
		DB.Delete(&f)
	}
	ok(c, gin.H{"following": active})
}
