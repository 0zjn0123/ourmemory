package om

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/server/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func tokenFrom(c *gin.Context) string {
	if h := strings.TrimSpace(c.GetHeader("Authorization")); h != "" {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if v, err := c.Cookie("om_token"); err == nil {
		return v
	}
	return ""
}

func cookieDomain(c *gin.Context) string {
	base := strings.TrimSpace(Cfg.BaseHost)
	if base == "" {
		return ""
	}
	host := c.Request.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	base = strings.ToLower(base)
	if host == base || strings.HasSuffix(host, "."+base) {
		return "." + base
	}
	return ""
}

func setSession(c *gin.Context, token string) {
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	c.SetSameSite(http.SameSiteLaxMode)
	// Drop legacy host-only cookies so an old token cannot shadow the shared
	// subdomain cookie during login.
	c.SetCookie("om_token", "", -1, "/", "", secure, true)
	c.SetCookie("om_token", token, 7*24*3600, "/", cookieDomain(c), secure, true)
}

func clearSession(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("om_token", "", -1, "/", "", false, true)
	c.SetCookie("om_token", "", -1, "/", cookieDomain(c), false, true)
}

// CurrentUser resolves the JWT from the Authorization header or the om_token
// cookie. It returns nil for guests instead of failing the request.
func CurrentUser(c *gin.Context) *model.User {
	token := tokenFrom(c)
	if token == "" {
		return nil
	}
	claims, err := common.ParseToken(token)
	if err != nil {
		return nil
	}
	user, err := op.GetUserByName(claims.Username)
	if err != nil || user.Disabled || claims.PwdTS != user.PwdTS {
		return nil
	}
	return user
}

func AuthRequired(c *gin.Context) {
	user := CurrentUser(c)
	if user == nil {
		if strings.Contains(c.GetHeader("Accept"), "text/html") || c.Request.Method == http.MethodGet && !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Redirect(http.StatusFound, "/login?next="+c.Request.URL.Path)
			c.Abort()
			return
		}
		fail(c, "请先登录", http.StatusUnauthorized)
		return
	}
	common.GinAppendValues(c, conf.UserKey, user)
	c.Next()
}

func AuthOptional(c *gin.Context) {
	if user := CurrentUser(c); user != nil {
		common.GinAppendValues(c, conf.UserKey, user)
	}
	c.Next()
}

func AuthAdmin(c *gin.Context) {
	user := CurrentUser(c)
	if user == nil {
		fail(c, "请先登录", http.StatusUnauthorized)
		return
	}
	if !user.IsAdmin() {
		fail(c, "需要管理员权限", http.StatusForbidden)
		return
	}
	common.GinAppendValues(c, conf.UserKey, user)
	c.Next()
}

func userOf(c *gin.Context) *model.User {
	if v, ok := c.Request.Context().Value(conf.UserKey).(*model.User); ok {
		return v
	}
	return nil
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, msg string, status int) {
	c.AbortWithStatusJSON(status, gin.H{"code": status, "message": msg, "data": nil})
}

func randomCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte("ourmemory:" + code))
	return hex.EncodeToString(sum[:])
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) RegisterRequest(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !usernameRe.MatchString(req.Username) {
		fail(c, "用户名需为 3-32 位字母、数字、下划线或短横线", 400)
		return
	}
	if !emailRe.MatchString(req.Email) {
		fail(c, "邮箱格式不正确", 400)
		return
	}
	if len(req.Password) < 8 {
		fail(c, "密码至少 8 位", 400)
		return
	}
	if _, err := op.GetUserByName(req.Username); err == nil {
		fail(c, "用户名已存在", 409)
		return
	}
	var n int64
	DB.Model(&Profile{}).Where("email = ? AND email <> ''", req.Email).Count(&n)
	if n > 0 {
		fail(c, "邮箱已注册", 409)
		return
	}
	code := randomCode()
	u := &model.User{Username: req.Username}
	u.SetPassword(req.Password)
	// SetPassword already created a random salt; keep it and the derived hash.
	p := PendingRegistration{
		ID:           randomHex(16),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: u.PwdHash,
		Salt:         u.Salt,
		CodeHash:     hashCode(code),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
		CreatedAt:    time.Now(),
	}
	if err := DB.Create(&p).Error; err != nil {
		fail(c, "创建注册请求失败", 500)
		return
	}
	html := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:520px;margin:auto"><h2>ourmemory 邮箱验证</h2><p>你的验证码是：</p><p style="font-size:32px;letter-spacing:8px;font-weight:700">%s</p><p>10 分钟内有效。如果不是你本人操作，请忽略本邮件。</p></div>`, code)
	if err := SendMail(req.Email, "ourmemory 注册验证码", html, "你的 ourmemory 验证码："+code+"（10 分钟内有效）"); err != nil {
		if Cfg.Debug {
			fmt.Printf("[om] mail disabled, dev code for %s: %s\n", req.Email, code)
		} else {
			fail(c, "邮件发送失败："+err.Error(), 502)
			return
		}
	}
	ok(c, gin.H{"register_id": p.ID, "expires_in": 600})
}

type verifyRequest struct {
	RegisterID string `json:"register_id"`
	Code       string `json:"code"`
}

func (h *Handlers) RegisterVerify(c *gin.Context) {
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	var p PendingRegistration
	if err := DB.First(&p, "id = ?", req.RegisterID).Error; err != nil {
		fail(c, "注册请求不存在", 404)
		return
	}
	if time.Now().After(p.ExpiresAt) {
		fail(c, "验证码已过期", 410)
		return
	}
	if p.Attempts >= 5 {
		fail(c, "尝试次数过多，请重新获取验证码", 429)
		return
	}
	if subtle.ConstantTimeCompare([]byte(p.CodeHash), []byte(hashCode(strings.TrimSpace(req.Code)))) != 1 {
		DB.Model(&p).UpdateColumn("attempts", p.Attempts+1)
		fail(c, "验证码错误", 400)
		return
	}
	if _, err := op.GetUserByName(p.Username); err == nil {
		fail(c, "用户名已存在", 409)
		return
	}
	u := &model.User{
		Username:   p.Username,
		PwdHash:    p.PasswordHash,
		Salt:       p.Salt,
		PwdTS:      time.Now().Unix(),
		Role:       model.GENERAL,
		BasePath:   "/",
		Permission: 0x71FF,
		Authn:      "[]",
	}
	if err := op.CreateUser(u); err != nil {
		fail(c, "创建用户失败", 500)
		return
	}
	pf, err := EnsureProfile(u.ID, u.Username)
	if err != nil {
		fail(c, "创建资料失败", 500)
		return
	}
	pf.Email = p.Email
	pf.EmailVerified = true
	pf.UpdatedAt = time.Now()
	DB.Save(pf)
	DB.Delete(&p)
	token, err := common.GenerateToken(u)
	if err != nil {
		fail(c, "签发登录凭证失败", 500)
		return
	}
	setSession(c, token)
	ok(c, gin.H{"token": token, "username": u.Username, "user_id": u.ID})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handlers) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, "参数错误", 400)
		return
	}
	user, err := op.GetUserByName(strings.TrimSpace(req.Username))
	if err != nil {
		fail(c, "用户名或密码错误", 401)
		return
	}
	if err := user.ValidateRawPassword(req.Password); err != nil {
		fail(c, "用户名或密码错误", 401)
		return
	}
	if user.Disabled {
		fail(c, "账号已被禁用", 403)
		return
	}
	token, err := common.GenerateToken(user)
	if err != nil {
		fail(c, "登录失败", 500)
		return
	}
	setSession(c, token)
	pf, _ := EnsureProfile(user.ID, user.Username)
	ok(c, gin.H{"token": token, "username": user.Username, "user_id": user.ID, "tier": pf.Tier})
}

func (h *Handlers) Logout(c *gin.Context) {
	clearSession(c)
	ok(c, nil)
}

type publicUser struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Role     int    `json:"role"`
}

func (h *Handlers) Me(c *gin.Context) {
	user := userOf(c)
	if user == nil {
		ok(c, nil)
		return
	}
	pf, err := EnsureProfile(user.ID, user.Username)
	if err != nil {
		fail(c, "读取资料失败", 500)
		return
	}
	var titles []Title
	DB.Table("om_titles").Joins("JOIN om_user_titles ut ON ut.title_id = om_titles.id").Where("ut.user_id = ?", user.ID).Find(&titles)
	ok(c, gin.H{"user": publicUser{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	}, "profile": pf, "titles": titles})
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func isNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
