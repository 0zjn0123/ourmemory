package om

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) isHallMember(hallID, userID uint) (*HallMember, bool) {
	var m HallMember
	if err := DB.First(&m, "hall_id = ? AND user_id = ?", hallID, userID).Error; err != nil {
		return nil, false
	}
	return &m, m.Status == "active"
}

func (h *Handlers) HallList(c *gin.Context) {
	var count int64
	DB.Model(&Hall{}).Count(&count)
	if count == 0 {
		DB.Create(&Hall{Name: "主大厅", Description: "所有用户的公共大厅", OwnerID: 0, JoinPolicy: "open", ReadPolicy: "public", CreatedAt: time.Now()})
	}
	var halls []Hall
	DB.Order("id ASC").Find(&halls)
	user := userOf(c)
	out := make([]gin.H, 0, len(halls))
	for i := range halls {
		hall := &halls[i]
		var members int64
		DB.Model(&HallMember{}).Where("hall_id = ? AND status = ?", hall.ID, "active").Count(&members)
		joined := false
		if user != nil {
			if _, ok := h.isHallMember(hall.ID, user.ID); ok {
				joined = true
			}
		}
		out = append(out, gin.H{"hall": hall, "members": members, "joined": joined})
	}
	ok(c, gin.H{"items": out})
}

func (h *Handlers) HallCreate(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		JoinPolicy  string `json:"join_policy"`
		ReadPolicy  string `json:"read_policy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 40 {
		fail(c, "大厅名称需为 1-40 字", 400)
		return
	}
	if req.JoinPolicy == "" {
		req.JoinPolicy = "open"
	}
	if req.ReadPolicy == "" {
		req.ReadPolicy = "public"
	}
	hall := &Hall{Name: req.Name, Description: req.Description, OwnerID: user.ID, JoinPolicy: req.JoinPolicy, ReadPolicy: req.ReadPolicy, CreatedAt: time.Now()}
	if err := DB.Create(hall).Error; err != nil {
		fail(c, "创建大厅失败", 500)
		return
	}
	DB.Create(&HallMember{HallID: hall.ID, UserID: user.ID, Role: "owner", Status: "active", JoinedAt: time.Now()})
	ok(c, hall)
}

func (h *Handlers) HallJoin(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var hall Hall
	if err := DB.First(&hall, id).Error; err != nil {
		fail(c, "大厅不存在", 404)
		return
	}
	if hall.JoinPolicy == "closed" {
		fail(c, "该大厅不允许加入", 403)
		return
	}
	var m HallMember
	err := DB.First(&m, "hall_id = ? AND user_id = ?", id, user.ID).Error
	status := "active"
	if hall.JoinPolicy == "approval" {
		status = "pending"
	}
	if err == gorm.ErrRecordNotFound {
		DB.Create(&HallMember{HallID: uint(id), UserID: user.ID, Role: "member", Status: status, JoinedAt: time.Now()})
	} else {
		DB.Model(&m).UpdateColumn("status", status)
	}
	ok(c, gin.H{"status": status})
}

func (h *Handlers) HallMessages(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var hall Hall
	if err := DB.First(&hall, id).Error; err != nil {
		fail(c, "大厅不存在", 404)
		return
	}
	user := userOf(c)
	if hall.ReadPolicy != "public" {
		if user == nil {
			fail(c, "该大厅仅成员可读", 403)
			return
		}
		if _, ok := h.isHallMember(uint(id), user.ID); !ok {
			fail(c, "该大厅仅成员可读", 403)
			return
		}
	}
	h.messages(c, "hall", uint(id))
}

func (h *Handlers) HallSend(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var hall Hall
	if err := DB.First(&hall, id).Error; err != nil {
		fail(c, "大厅不存在", 404)
		return
	}
	if _, ok := h.isHallMember(uint(id), user.ID); !ok {
		if hall.JoinPolicy == "open" {
			DB.Create(&HallMember{HallID: uint(id), UserID: user.ID, Role: "member", Status: "active", JoinedAt: time.Now()})
		} else {
			fail(c, "请先加入大厅", 403)
			return
		}
	}
	h.postMessage(c, "hall", uint(id), hall.OwnerID, hall.ReadPolicy == "public")
}

func (h *Handlers) HallSettings(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var hall Hall
	if err := DB.First(&hall, id).Error; err != nil {
		fail(c, "大厅不存在", 404)
		return
	}
	if hall.OwnerID != user.ID && !user.IsAdmin() {
		fail(c, "只有创建者可以修改", 403)
		return
	}
	var req struct {
		JoinPolicy string `json:"join_policy"`
		ReadPolicy string `json:"read_policy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	if req.JoinPolicy != "" {
		if req.JoinPolicy != "open" && req.JoinPolicy != "approval" && req.JoinPolicy != "closed" {
			fail(c, "加入策略无效", 400)
			return
		}
		hall.JoinPolicy = req.JoinPolicy
	}
	if req.ReadPolicy != "" {
		if req.ReadPolicy != "public" && req.ReadPolicy != "members" {
			fail(c, "读取策略无效", 400)
			return
		}
		hall.ReadPolicy = req.ReadPolicy
	}
	DB.Save(&hall)
	// Migrate an existing archive immediately when the read policy changes.
	if hall.ChatFileID != "" {
		if err := h.syncChatFile("hall", uint(id), hall.OwnerID, hall.ReadPolicy == "public"); err != nil {
			fail(c, "保存成功，但聊天归档迁移失败："+err.Error(), 500)
			return
		}
	}
	ok(c, hall)
}

func (h *Handlers) GroupList(c *gin.Context) {
	user := userOf(c)
	var ids []uint
	DB.Model(&GroupMember{}).Where("user_id = ?", user.ID).Pluck("group_id", &ids)
	if len(ids) == 0 {
		ok(c, gin.H{"items": []any{}})
		return
	}
	var groups []Group
	DB.Where("id IN ?", ids).Order("id DESC").Find(&groups)
	ok(c, gin.H{"items": groups})
}

func (h *Handlers) GroupCreate(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		fail(c, "参数错误", 400)
		return
	}
	pf, _ := EnsureProfile(user.ID, user.Username)
	if !pf.PrivateEnabled {
		fail(c, "请先开通私人存储", 403)
		return
	}
	g := &Group{Name: strings.TrimSpace(req.Name), OwnerID: user.ID, CreatedAt: time.Now()}
	if err := DB.Create(g).Error; err != nil {
		fail(c, "创建群组失败", 500)
		return
	}
	DB.Create(&GroupMember{GroupID: g.ID, UserID: user.ID, Role: "owner", JoinedAt: time.Now()})
	ok(c, g)
}

func (h *Handlers) GroupAddMember(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var g Group
	if err := DB.First(&g, id).Error; err != nil {
		fail(c, "群组不存在", 404)
		return
	}
	if g.OwnerID != user.ID && !user.IsAdmin() {
		fail(c, "只有群主可以添加成员", 403)
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	u, err := op.GetUserByName(strings.TrimSpace(req.Username))
	if err != nil {
		fail(c, "用户不存在", 404)
		return
	}
	DB.FirstOrCreate(&GroupMember{}, GroupMember{GroupID: uint(id), UserID: u.ID, Role: "member", JoinedAt: time.Now()})
	ok(c, gin.H{"ok": true})
}

func (h *Handlers) GroupMessages(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var g Group
	if err := DB.First(&g, id).Error; err != nil {
		fail(c, "群组不存在", 404)
		return
	}
	var m GroupMember
	if err := DB.First(&m, "group_id = ? AND user_id = ?", id, user.ID).Error; err != nil {
		fail(c, "你不是群成员", 403)
		return
	}
	h.messages(c, "group", uint(id))
}

func (h *Handlers) GroupSend(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var g Group
	if err := DB.First(&g, id).Error; err != nil {
		fail(c, "群组不存在", 404)
		return
	}
	var m GroupMember
	if err := DB.First(&m, "group_id = ? AND user_id = ?", id, user.ID).Error; err != nil {
		fail(c, "你不是群成员", 403)
		return
	}
	h.postMessage(c, "group", uint(id), g.OwnerID, false)
}

func (h *Handlers) messages(c *gin.Context, roomType string, roomID uint) {
	var msgs []Message
	DB.Where("room_type = ? AND room_id = ?", roomType, roomID).Order("id ASC").Limit(500).Find(&msgs)
	out := make([]gin.H, 0, len(msgs))
	for _, m := range msgs {
		name := ""
		if u, err := op.GetUserById(m.UserID); err == nil {
			name = u.Username
		}
		out = append(out, gin.H{"message": m, "username": name})
	}
	ok(c, gin.H{"items": out})
}

func (h *Handlers) postMessage(c *gin.Context, roomType string, roomID uint, ownerID uint, shared bool) {
	user := userOf(c)
	var req struct {
		Content string `json:"content"`
		Kind    string `json:"kind"`
		FileID  string `json:"file_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" && req.FileID == "" {
		fail(c, "消息不能为空", 400)
		return
	}
	if len([]rune(req.Content)) > 2000 {
		fail(c, "消息过长", 400)
		return
	}
	if req.Kind == "" {
		req.Kind = "text"
	}
	m := &Message{RoomType: roomType, RoomID: roomID, UserID: user.ID, Content: req.Content, Kind: req.Kind, FileID: req.FileID, CreatedAt: time.Now()}
	if err := DB.Create(m).Error; err != nil {
		fail(c, "发送失败", 500)
		return
	}
	// Mirror the whole room into one archive file. Public halls use the shared
	// scope (everyone can inspect it); private groups use the owner's vault.
	if err := h.syncChatFile(roomType, roomID, ownerID, shared); err != nil {
		DB.Delete(m)
		fail(c, "聊天记录归档失败："+err.Error(), 500)
		return
	}
	ok(c, m)
}

func (h *Handlers) syncChatFile(roomType string, roomID uint, ownerID uint, shared bool) error {
	visibility := "public"
	if roomType == "hall" {
		var hall Hall
		if err := DB.First(&hall, roomID).Error; err == nil {
			// Only a public lobby may keep its archive in the shared pack.
			// Restricted lobbies use the owner's private vault and are never
			// listed or served through shared direct links.
			shared = hall.ReadPolicy == "public"
		}
	}
	if !shared {
		visibility = "restricted"
	}
	scope := "private"
	if shared {
		scope = "shared"
	}
	path := fmt.Sprintf("/halls/%s-%d.jsonl", roomType, roomID)
	if roomType == "group" {
		path = fmt.Sprintf("/groups/%d.jsonl", roomID)
	}
	var msgs []Message
	DB.Where("room_type = ? AND room_id = ?", roomType, roomID).Order("id ASC").Find(&msgs)
	var b strings.Builder
	enc := json.NewEncoder(&b)
	for i := range msgs {
		_ = enc.Encode(gin.H{"id": msgs[i].ID, "user_id": msgs[i].UserID, "content": msgs[i].Content, "kind": msgs[i].Kind, "file_id": msgs[i].FileID, "created_at": msgs[i].CreatedAt})
	}
	tmp, err := os.CreateTemp(Cfg.TmpDir, "chat-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	var pf *Profile
	if ownerID != 0 {
		pf, _ = EnsureProfile(ownerID, "")
	}
	level := 7
	if pf != nil && pf.CompressionLevel > 0 {
		level = pf.CompressionLevel
	}
	var old *File
	if roomType == "hall" {
		var hall Hall
		if err := DB.First(&hall, roomID).Error; err == nil && hall.ChatFileID != "" {
			if f, err := h.fileByID(hall.ChatFileID); err == nil {
				old = f
			}
		}
	} else {
		var g Group
		if err := DB.First(&g, roomID).Error; err == nil && g.ChatFileID != "" {
			if f, err := h.fileByID(g.ChatFileID); err == nil {
				old = f
			}
		}
	}
	// If the lobby policy changed, move the archive between the shared pack and
	// the owner's private vault instead of leaving an old copy readable.
	if old != nil && (old.Scope != scope || old.OwnerID != ownerID) {
		oldBlob := old.BlobID
		DB.Delete(old)
		if oldBlob != "" {
			_ = Store.Delete(oldBlob)
		}
		old = nil
	}
	// The rewritten archive replaces the old one; check the projected usage
	// before appending a new blob.
	oldSize := int64(0)
	if old != nil {
		oldSize = old.Size
	}
	if scope == "private" && pf != nil && h.privateUsed(ownerID)-oldSize+int64(len(b.String())) > pf.PrivateQuotaBytes {
		return fmt.Errorf("私人空间不足")
	}
	if scope == "shared" && h.sharedUsed()-oldSize+int64(len(b.String())) > Cfg.SharedMaxBytes {
		return fmt.Errorf("共享盘容量已满")
	}
	var key []byte
	if scope == "private" {
		key, err = UserKey(ownerID)
		if err != nil {
			return err
		}
	}
	blob, err := Store.Write(scope, ownerID, key, tmpPath, level)
	if err != nil {
		return err
	}
	if old != nil {
		oldBlob := old.BlobID
		old.BlobID = blob.ID
		old.Size = blob.PlainSize
		old.Visibility = visibility
		old.UpdatedAt = time.Now()
		DB.Save(old)
		if scope == "private" {
			h.refreshPrivateUsed(ownerID)
		}
		// The previous blob is now unreferenced; keep the archive append-only
		// but release the index entry.
		if oldBlob != "" && oldBlob != blob.ID {
			_ = Store.Delete(oldBlob)
		}
		return nil
	}
	f := &File{ID: newID(), OwnerID: ownerID, Scope: scope, Name: fmt.Sprintf("%s-%d.jsonl", roomType, roomID), Path: path, Size: blob.PlainSize, Mime: "application/x-ndjson", Kind: "file", BlobID: blob.ID, Visibility: visibility, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := DB.Create(f).Error; err != nil {
		_ = Store.Delete(blob.ID)
		return err
	}
	if roomType == "hall" {
		DB.Model(&Hall{}).Where("id = ?", roomID).UpdateColumn("chat_file_id", f.ID)
	} else {
		DB.Model(&Group{}).Where("id = ?", roomID).UpdateColumn("chat_file_id", f.ID)
	}
	if scope == "private" {
		h.refreshPrivateUsed(ownerID)
	}
	return nil
}
