package om

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/gin-gonic/gin"
)

var tierQuota = map[int]int64{
	0: 1 * 1024 * 1024 * 1024,
	1: 11 * 1024 * 1024 * 1024,
	2: 61 * 1024 * 1024 * 1024,
	3: 121 * 1024 * 1024 * 1024,
}

var tierCopies = map[int]int{0: 2, 1: 3, 2: 5, 3: 8}
var tierLevel = map[int]int{0: 7, 1: 5, 2: 5, 3: 3}
var tierName = map[int]string{0: "普通用户", 1: "Ⅰ级捐赠", 2: "Ⅱ级捐赠", 3: "Ⅲ级捐赠"}

func applyTier(pf *Profile, tier int) {
	if tier < 0 || tier > 3 {
		return
	}
	pf.Tier = tier
	if q := tierQuota[tier]; q > pf.PrivateQuotaBytes {
		pf.PrivateQuotaBytes = q
	}
	pf.CompressionLevel = tierLevel[tier]
	GrantTitle(pf.UserID, tierName[tier])
}

// targetCopies is the total number of copies including the main site.
func targetCopies(ownerID uint) int {
	if ownerID == 0 {
		return 2
	}
	var pf Profile
	if err := DB.First(&pf, "user_id = ?", ownerID).Error; err == nil {
		if n := tierCopies[pf.Tier]; n > 0 {
			return n
		}
	}
	return 2
}

func (h *Handlers) PrivateStatus(c *gin.Context) {
	user := userOf(c)
	pf, err := EnsureProfile(user.ID, user.Username)
	if err != nil {
		fail(c, "读取资料失败", 500)
		return
	}
	var reserved int64
	DB.Model(&Reservation{}).Where("user_id = ? AND expires_at > ?", user.ID, time.Now()).Select("COALESCE(SUM(bytes),0)").Scan(&reserved)
	ok(c, gin.H{
		"enabled":         pf.PrivateEnabled,
		"quota_bytes":     pf.PrivateQuotaBytes,
		"used_bytes":      h.privateUsed(user.ID),
		"reserved_bytes":  reserved,
		"compression":     pf.CompressionLevel,
		"tier":            pf.Tier,
		"copies":          tierCopies[pf.Tier],
		"shared_max":      Cfg.SharedMaxBytes,
		"shared_used":     h.sharedUsed(),
		"default_quota":   Cfg.DefaultQuota,
		"contribution":    pf.ContributionQuota,
		"private_capable": true,
	})
}

func (h *Handlers) EnablePrivate(c *gin.Context) {
	user := userOf(c)
	pf, err := EnsureProfile(user.ID, user.Username)
	if err != nil {
		fail(c, "读取资料失败", 500)
		return
	}
	if pf.PrivateEnabled {
		ok(c, gin.H{"enabled": true})
		return
	}
	if err := EnablePrivate(user.ID); err != nil {
		fail(c, "开通私人存储失败："+err.Error(), 500)
		return
	}
	Audit(user.ID, "private.enable", "", c.ClientIP(), "")
	ok(c, gin.H{"enabled": true, "quota_bytes": pf.PrivateQuotaBytes, "compression": pf.CompressionLevel})
}

func (h *Handlers) UpdateProfile(c *gin.Context) {
	user := userOf(c)
	pf, err := EnsureProfile(user.ID, user.Username)
	if err != nil {
		fail(c, "读取资料失败", 500)
		return
	}
	var req struct {
		DisplayName  string `json:"display_name"`
		Bio          string `json:"bio"`
		AvatarFileID string `json:"avatar_file_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	if strings.TrimSpace(req.DisplayName) != "" {
		pf.DisplayName = strings.TrimSpace(req.DisplayName)
		if len([]rune(pf.DisplayName)) > 40 {
			pf.DisplayName = string([]rune(pf.DisplayName)[:40])
		}
	}
	pf.Bio = req.Bio
	if len([]rune(pf.Bio)) > 500 {
		pf.Bio = string([]rune(pf.Bio)[:500])
	}
	if req.AvatarFileID != "" {
		if f, err := h.fileByID(req.AvatarFileID); err == nil && f.Kind == "image" && h.canRead(f, user.ID, user.IsAdmin()) {
			pf.AvatarFileID = req.AvatarFileID
		}
	}
	pf.UpdatedAt = time.Now()
	DB.Save(pf)
	ok(c, pf)
}

func (h *Handlers) UserProfile(c *gin.Context) {
	name := strings.TrimSpace(c.Param("username"))
	u, err := op.GetUserByName(name)
	if err != nil {
		var pf Profile
		if err := DB.Where("display_name = ?", name).First(&pf).Error; err != nil {
			fail(c, "用户不存在", 404)
			return
		}
		u, err = op.GetUserById(pf.UserID)
		if err != nil {
			fail(c, "用户不存在", 404)
			return
		}
	}
	pf, _ := EnsureProfile(u.ID, u.Username)
	var titles []Title
	DB.Table("om_titles").Joins("JOIN om_user_titles ut ON ut.title_id = om_titles.id").Where("ut.user_id = ?", u.ID).Find(&titles)
	var files, videos, images, posts, followers, following int64
	DB.Model(&File{}).Where("owner_id = ? AND scope = ? AND is_dir = ?", u.ID, "shared", false).Count(&files)
	DB.Model(&Video{}).Where("uploader_id = ?", u.ID).Count(&videos)
	DB.Model(&Image{}).Where("uploader_id = ?", u.ID).Count(&images)
	DB.Model(&Post{}).Where("user_id = ?", u.ID).Count(&posts)
	DB.Model(&Follow{}).Where("followee_id = ?", u.ID).Count(&followers)
	DB.Model(&Follow{}).Where("follower_id = ?", u.ID).Count(&following)
	ok(c, gin.H{
		"user":       gin.H{"id": u.ID, "username": u.Username, "role": u.Role},
		"user_id":    u.ID,
		"username":   u.Username,
		"created_at": pf.CreatedAt.Unix(),
		"profile": gin.H{
			"display_name":   pf.DisplayName,
			"bio":            pf.Bio,
			"avatar_file_id": pf.AvatarFileID,
			"tier":           pf.Tier,
		},
		"titles": titles,
		"stats":  gin.H{"files": files, "videos": videos, "images": images, "posts": posts, "followers": followers, "following": following},
	})
}

func (h *Handlers) Titles(c *gin.Context) {
	var titles []Title
	DB.Order("id ASC").Find(&titles)
	ok(c, gin.H{"items": titles})
}

func (h *Handlers) Redeem(c *gin.Context) {
	user := userOf(c)
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	code := strings.TrimSpace(req.Code)
	var rc RedeemCode
	if err := DB.First(&rc, "code = ?", code).Error; err != nil {
		fail(c, "兑换码不存在", 404)
		return
	}
	if rc.UsedBy != 0 {
		fail(c, "兑换码已使用", 409)
		return
	}
	now := time.Now()
	rc.UsedBy = user.ID
	rc.UsedAt = &now
	DB.Save(&rc)
	pf, _ := EnsureProfile(user.ID, user.Username)
	switch rc.Kind {
	case "tier":
		applyTier(pf, rc.Tier)
	case "reservation":
		DB.Create(&Reservation{UserID: user.ID, Bytes: rc.Bytes, ExpiresAt: now.AddDate(0, 0, rc.Days), CreatedAt: now})
	case "quota":
		pf.PrivateQuotaBytes += rc.Bytes
	}
	pf.UpdatedAt = now
	DB.Save(pf)
	Audit(user.ID, "redeem", code, c.ClientIP(), rc.Kind)
	ok(c, pf)
}

func (h *Handlers) AdminStats(c *gin.Context) {
	var users, files, videos, images, posts, messages, nodes int64
	DB.Model(&Profile{}).Count(&users)
	DB.Model(&File{}).Count(&files)
	DB.Model(&Video{}).Count(&videos)
	DB.Model(&Image{}).Count(&images)
	DB.Model(&Post{}).Count(&posts)
	DB.Model(&Message{}).Count(&messages)
	DB.Model(&Node{}).Count(&nodes)
	var privateUsed int64
	DB.Model(&File{}).Where("scope = ?", "private").Select("COALESCE(SUM(size),0)").Scan(&privateUsed)
	ok(c, gin.H{
		"users": users, "files": files, "videos": videos, "images": images, "posts": posts,
		"messages": messages, "nodes": nodes, "shared_used": h.sharedUsed(), "private_used": privateUsed,
		"shared_max": Cfg.SharedMaxBytes, "cache_max": Cfg.CacheMaxBytes,
	})
}

func (h *Handlers) AdminCodes(c *gin.Context) {
	var codes []RedeemCode
	DB.Order("id DESC").Limit(500).Find(&codes)
	ok(c, gin.H{"items": codes})
}

func (h *Handlers) AdminCreateCode(c *gin.Context) {
	var req struct {
		Kind  string `json:"kind"`
		Tier  int    `json:"tier"`
		Bytes int64  `json:"bytes"`
		Days  int    `json:"days"`
		Count int    `json:"count"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	if req.Count <= 0 || req.Count > 100 {
		req.Count = 1
	}
	var out []string
	for i := 0; i < req.Count; i++ {
		code := "OM-" + strings.ToUpper(randomHex(6))
		rc := RedeemCode{Code: code, Kind: req.Kind, Tier: req.Tier, Bytes: req.Bytes, Days: req.Days, CreatedAt: time.Now()}
		if err := DB.Create(&rc).Error; err != nil {
			fail(c, "生成失败", 500)
			return
		}
		out = append(out, code)
	}
	ok(c, gin.H{"codes": out})
}

func (h *Handlers) AdminUsers(c *gin.Context) {
	var pfs []Profile
	DB.Order("user_id ASC").Limit(500).Find(&pfs)
	type row struct {
		Profile
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	out := make([]row, 0, len(pfs))
	for _, p := range pfs {
		r := row{Profile: p}
		if u, err := op.GetUserById(p.UserID); err == nil {
			r.Username = u.Username
			r.Role = fmt.Sprint(u.Role)
		}
		out = append(out, r)
	}
	ok(c, gin.H{"items": out})
}

func (h *Handlers) AdminNodes(c *gin.Context) {
	var nodes []Node
	DB.Order("id ASC").Find(&nodes)
	ok(c, gin.H{"items": nodes})
}

func (h *Handlers) AdminPacks(c *gin.Context) {
	var packs []Pack
	DB.Order("id ASC").Find(&packs)
	ok(c, gin.H{"items": packs})
}

func (h *Handlers) AdminCompact(c *gin.Context) {
	var req struct {
		Pack string `json:"pack"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Pack) == "" {
		fail(c, "参数错误", 400)
		return
	}
	before, after, err := Store.Compact(strings.TrimSpace(req.Pack))
	if err != nil {
		fail(c, "整理失败："+err.Error(), 500)
		return
	}
	Audit(userOf(c).ID, "compact", req.Pack, c.ClientIP(), "")
	ok(c, gin.H{"before": before, "after": after, "saved": before - after})
}

func (h *Handlers) notFound(c *gin.Context) { fail(c, "接口不存在", http.StatusNotFound) }

var _ = fmt.Sprintf
