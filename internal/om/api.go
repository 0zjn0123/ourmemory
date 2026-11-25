package om

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/server/common"
	"github.com/gin-gonic/gin"
)

func keyHash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func (h *Handlers) APIKeyCreate(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Name   string `json:"name"`
		Scopes string `json:"scopes"`
	}
	_ = c.ShouldBindJSON(&req)
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "default"
	}
	if req.Scopes == "" {
		req.Scopes = "files:read,files:write"
	}
	allowed := map[string]bool{"files:read": true, "files:write": true}
	var normalized []string
	for _, s := range strings.Split(req.Scopes, ",") {
		s = strings.TrimSpace(s)
		if !allowed[s] {
			fail(c, "权限范围无效", 400)
			return
		}
		normalized = append(normalized, s)
	}
	if len(normalized) == 0 {
		fail(c, "至少需要一个权限", 400)
		return
	}
	req.Scopes = strings.Join(normalized, ",")
	secret := "om_" + randomHex(24)
	prefix := secret[:12]
	k := &APIKey{UserID: user.ID, Name: req.Name, Prefix: prefix, Hash: keyHash(secret), Scopes: req.Scopes, CreatedAt: time.Now()}
	if err := DB.Create(k).Error; err != nil {
		fail(c, "创建密钥失败", 500)
		return
	}
	ok(c, gin.H{"id": k.ID, "name": k.Name, "prefix": prefix, "secret": secret, "scopes": k.Scopes})
}

func (h *Handlers) APIKeyList(c *gin.Context) {
	user := userOf(c)
	var keys []APIKey
	DB.Where("user_id = ?", user.ID).Order("id DESC").Find(&keys)
	ok(c, gin.H{"items": keys})
}

func (h *Handlers) APIKeyDelete(c *gin.Context) {
	user := userOf(c)
	id, _ := strconv.Atoi(c.Param("id"))
	DB.Where("id = ? AND user_id = ?", id, user.ID).Delete(&APIKey{})
	ok(c, nil)
}

// APIAuth accepts either a normal user JWT or an ourmemory API key. API keys
// are intentionally scoped to the owning user; a key never sees another user's
// private vault.
func APIAuth(c *gin.Context) {
	if user := CurrentUser(c); user != nil {
		common.GinAppendValues(c, conf.UserKey, user)
		c.Next()
		return
	}
	secret := strings.TrimSpace(c.GetHeader("X-API-Key"))
	if secret == "" {
		secret = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	}
	if secret == "" {
		fail(c, "缺少 API key", http.StatusUnauthorized)
		return
	}
	var k APIKey
	if err := DB.First(&k, "hash = ?", keyHash(secret)).Error; err != nil {
		fail(c, "API key 无效", http.StatusUnauthorized)
		return
	}
	u, err := op.GetUserById(k.UserID)
	if err != nil || u.Disabled {
		fail(c, "API key 所属用户不可用", http.StatusUnauthorized)
		return
	}
	now := time.Now()
	DB.Model(&k).UpdateColumn("last_used_at", now)
	c.Set("om_key_scopes", k.Scopes)
	common.GinAppendValues(c, conf.UserKey, u)
	c.Next()
}

// RequireScope limits API-key callers to their granted scopes. JWT sessions are
// not restricted. An empty legacy scope is treated as full access.
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		v, ok := c.Get("om_key_scopes")
		if !ok {
			c.Next()
			return
		}
		scopes, _ := v.(string)
		if scopes == "" || strings.Contains(scopes, scope) || strings.Contains(scopes, "*") {
			c.Next()
			return
		}
		fail(c, "API key 缺少权限："+scope, http.StatusForbidden)
	}
}

func (h *Handlers) NodeRegister(c *gin.Context) {
	if Cfg.NodeSecret == "" || c.GetHeader("X-Node-Secret") != Cfg.NodeSecret {
		fail(c, "节点注册密钥无效", http.StatusUnauthorized)
		return
	}
	var req struct {
		Name           string `json:"name"`
		URL            string `json:"url"`
		CapacityBytes  int64  `json:"capacity_bytes"`
		PrivateCapable bool   `json:"private_capable"`
		Username       string `json:"username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	if req.Name == "" {
		req.Name = "storage-node"
	}
	if req.CapacityBytes <= 0 {
		fail(c, "容量必须大于 0", 400)
		return
	}
	if req.URL == "" {
		req.URL = c.Request.Host
	}
	var ownerID uint
	if req.Username != "" {
		if u, err := op.GetUserByName(req.Username); err == nil {
			ownerID = u.ID
		}
	}
	token := randomHex(24)
	n := &Node{
		Name: req.Name, URL: req.URL, TokenHash: keyHash(token), CapacityBytes: req.CapacityBytes,
		PrivateCapable: req.PrivateCapable, Status: "online", OwnerID: ownerID, LastSeenAt: time.Now(), CreatedAt: time.Now(),
	}
	if err := DB.Create(n).Error; err != nil {
		fail(c, "注册节点失败", 500)
		return
	}
	if ownerID != 0 {
		pf, _ := EnsureProfile(ownerID, req.Username)
		// Credit is based on the owner's total contributed capacity, not on
		// each registration, so re-running the installer cannot inflate quota.
		var total int64
		DB.Model(&Node{}).Where("owner_id = ?", ownerID).Select("COALESCE(SUM(capacity_bytes),0)").Scan(&total)
		contribution := total * 50 / 100 * 90 / 100
		delta := contribution - pf.ContributionQuota
		pf.PrivateQuotaBytes += delta
		if pf.PrivateQuotaBytes < pf.PrivateUsedBytes {
			pf.PrivateQuotaBytes = pf.PrivateUsedBytes
		}
		pf.ContributionQuota = contribution
		pf.UpdatedAt = time.Now()
		DB.Save(pf)
		GrantTitle(ownerID, "存储贡献者")
	}
	ok(c, gin.H{"node_id": n.ID, "token": token, "name": n.Name})
}

func nodeByToken(c *gin.Context) (*Node, bool) {
	token := strings.TrimSpace(c.GetHeader("X-Node-Token"))
	if token == "" {
		token = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	}
	var n Node
	if err := DB.First(&n, "token_hash = ?", keyHash(token)).Error; err != nil || n.Disabled {
		fail(c, "节点令牌无效", 401)
		return nil, false
	}
	return &n, true
}

func (h *Handlers) NodeHeartbeat(c *gin.Context) {
	n, okNode := nodeByToken(c)
	if !okNode {
		return
	}
	var req struct {
		UsedBytes int64 `json:"used_bytes"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.UsedBytes < 0 {
		req.UsedBytes = 0
	}
	if n.CapacityBytes > 0 && req.UsedBytes > n.CapacityBytes {
		req.UsedBytes = n.CapacityBytes
	}
	DB.Model(n).Updates(map[string]any{"used_bytes": req.UsedBytes, "status": "online", "last_seen_at": time.Now()})
	ok(c, gin.H{"ok": true, "node_id": n.ID})
}

func (h *Handlers) NodeAssign(c *gin.Context) {
	n, okNode := nodeByToken(c)
	if !okNode {
		return
	}
	want := 20
	if v, _ := strconv.Atoi(c.Query("limit")); v > 0 && v < 100 {
		want = v
	}
	var existing []string
	DB.Model(&Replica{}).Where("node_id = ? AND status IN ?", n.ID, []string{"done", "pending"}).Pluck("blob_id", &existing)
	skip := make(map[string]bool, len(existing))
	for _, id := range existing {
		skip[id] = true
	}
	q := DB.Model(&Blob{}).Order("created_at ASC").Limit(500)
	if !n.PrivateCapable {
		q = q.Where("encrypted = ?", false)
	}
	var blobs []Blob
	q.Find(&blobs)
	available := n.CapacityBytes - n.UsedBytes
	if available < 0 {
		available = 0
	}
	out := make([]gin.H, 0, want)
	var planned int64
	for _, b := range blobs {
		if len(out) >= want || skip[b.ID] {
			continue
		}
		target := b.TargetCopies
		if target < 2 {
			target = 2
		}
		var done int64
		DB.Model(&Replica{}).Where("blob_id = ? AND status = ?", b.ID, "done").Count(&done)
		if done >= int64(target-1) {
			continue
		}
		if planned+b.StoredSize > available {
			continue
		}
		planned += b.StoredSize
		DB.Create(&Replica{BlobID: b.ID, NodeID: n.ID, Status: "pending", Size: b.StoredSize, CreatedAt: time.Now()})
		out = append(out, gin.H{"blob_id": b.ID, "size": b.StoredSize, "url": fmt.Sprintf("/api/om/nodes/blob/%s", b.ID)})
	}
	ok(c, gin.H{"items": out})
}

func (h *Handlers) NodeReport(c *gin.Context) {
	n, okNode := nodeByToken(c)
	if !okNode {
		return
	}
	var req struct {
		BlobID string `json:"blob_id"`
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	if req.Status == "" {
		req.Status = "done"
	}
	if req.Status != "done" && req.Status != "failed" {
		fail(c, "状态无效", 400)
		return
	}
	res := DB.Model(&Replica{}).Where("node_id = ? AND blob_id = ?", n.ID, req.BlobID).UpdateColumn("status", req.Status)
	if res.RowsAffected == 0 {
		fail(c, "没有该分配", 404)
		return
	}
	ok(c, nil)
}

func (h *Handlers) NodeBlob(c *gin.Context) {
	n, okNode := nodeByToken(c)
	if !okNode {
		return
	}
	var rp Replica
	if err := DB.First(&rp, "node_id = ? AND blob_id = ?", n.ID, c.Param("id")).Error; err != nil {
		fail(c, "没有该分配", 404)
		return
	}
	r, err := Store.RawOpen(c.Param("id"))
	if err != nil {
		fail(c, "内容不存在", 404)
		return
	}
	defer r.Close()
	c.Header("Content-Type", "application/octet-stream")
	c.Status(200)
	_, _ = io.Copy(c.Writer, r)
}
