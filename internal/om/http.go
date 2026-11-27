package om

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handlers owns the ourmemory HTTP surface. OpenList's own /api routes remain
// available for administrators; these routes are the product API.
type Handlers struct{}

func Register(e *gin.Engine) {
	h := &Handlers{}
	assets := filepath.Join(workDir(), "public", "om")
	e.Static("/om-assets", assets)

	page := func(c *gin.Context) {
		index := filepath.Join(assets, "index.html")
		if _, err := os.Stat(index); err != nil {
			c.String(http.StatusInternalServerError, "ourmemory frontend is missing: %s", index)
			return
		}
		c.File(index)
	}
	for _, p := range []string{"/", "/login", "/register", "/video", "/video/:id", "/image", "/image/:id", "/weibo", "/im", "/settings", "/admin", "/u/:username", "/docs"} {
		e.GET(p, page)
	}
	e.GET("/api/om/docs", h.Docs)

	api := e.Group("/api/om")
	api.GET("/me", AuthOptional, h.Me)
	api.POST("/auth/register/request", h.RegisterRequest)
	api.POST("/auth/register/verify", h.RegisterVerify)
	api.POST("/auth/login", h.Login)
	api.POST("/auth/logout", h.Logout)

	api.GET("/files", AuthOptional, h.ListFiles)
	api.POST("/files/upload", AuthRequired, h.Upload)
	api.POST("/folders", AuthRequired, h.CreateFolder)
	api.POST("/files/rename", AuthRequired, h.Rename)
	api.POST("/files/move", AuthRequired, h.Move)
	api.POST("/files/delete", AuthRequired, h.Delete)
	api.GET("/files/:id", AuthOptional, h.FileMeta)
	api.POST("/files/:id/sign", AuthRequired, h.SignFile)

	api.GET("/private", AuthRequired, h.PrivateStatus)
	api.POST("/private/enable", AuthRequired, h.EnablePrivate)
	api.POST("/profile", AuthRequired, h.UpdateProfile)
	api.GET("/u/:username", AuthOptional, h.UserProfile)
	api.POST("/redeem", AuthRequired, h.Redeem)
	api.GET("/titles", AuthOptional, h.Titles)

	api.GET("/videos", AuthOptional, h.VideoList)
	api.GET("/videos/:id", AuthOptional, h.VideoGet)
	api.POST("/videos", AuthRequired, h.VideoCreate)
	api.POST("/videos/:id/react", AuthRequired, h.React)
	api.GET("/videos/:id/comments", AuthOptional, h.CommentList)
	api.POST("/videos/:id/comments", AuthRequired, h.CommentCreate)
	api.GET("/videos/:id/danmaku", AuthOptional, h.DanmakuList)
	api.POST("/videos/:id/danmaku", AuthRequired, h.DanmakuCreate)
	api.POST("/videos/:id/progress", AuthRequired, h.ProgressSave)
	api.GET("/videos/:id/progress", AuthRequired, h.ProgressGet)

	api.GET("/images", AuthOptional, h.ImageList)
	api.GET("/images/:id", AuthOptional, h.ImageGet)
	api.POST("/images", AuthRequired, h.ImageCreate)
	api.POST("/images/:id/react", AuthRequired, h.React)

	api.GET("/posts", AuthOptional, h.PostList)
	api.GET("/posts/:id", AuthOptional, h.PostGet)
	api.POST("/posts", AuthRequired, h.PostCreate)
	api.POST("/posts/:id/react", AuthRequired, h.React)
	api.POST("/posts/:id/repost", AuthRequired, h.PostRepost)
	api.POST("/posts/:id/comments", AuthRequired, h.PostCommentCreate)
	api.GET("/posts/:id/comments", AuthOptional, h.PostCommentList)
	api.POST("/follow/:id", AuthRequired, h.FollowToggle)

	api.GET("/halls", AuthOptional, h.HallList)
	api.POST("/halls", AuthRequired, h.HallCreate)
	api.POST("/halls/:id/join", AuthRequired, h.HallJoin)
	api.GET("/halls/:id/messages", AuthOptional, h.HallMessages)
	api.POST("/halls/:id/messages", AuthRequired, h.HallSend)
	api.POST("/halls/:id/settings", AuthRequired, h.HallSettings)

	api.GET("/groups", AuthRequired, h.GroupList)
	api.POST("/groups", AuthRequired, h.GroupCreate)
	api.POST("/groups/:id/members", AuthRequired, h.GroupAddMember)
	api.GET("/groups/:id/messages", AuthRequired, h.GroupMessages)
	api.POST("/groups/:id/messages", AuthRequired, h.GroupSend)

	api.POST("/keys", AuthRequired, h.APIKeyCreate)
	api.GET("/keys", AuthRequired, h.APIKeyList)
	api.POST("/keys/:id/delete", AuthRequired, h.APIKeyDelete)

	api.POST("/nodes/register", h.NodeRegister)
	api.POST("/nodes/heartbeat", h.NodeHeartbeat)
	api.POST("/nodes/assign", h.NodeAssign)
	api.POST("/nodes/report", h.NodeReport)
	api.GET("/nodes/blob/:id", h.NodeBlob)

	api.GET("/admin/stats", AuthAdmin, h.AdminStats)
	api.GET("/admin/codes", AuthAdmin, h.AdminCodes)
	api.POST("/admin/codes", AuthAdmin, h.AdminCreateCode)
	api.GET("/admin/users", AuthAdmin, h.AdminUsers)
	api.GET("/admin/nodes", AuthAdmin, h.AdminNodes)
	api.GET("/admin/packs", AuthAdmin, h.AdminPacks)
	api.POST("/admin/compact", AuthAdmin, h.AdminCompact)

	// Open storage API for third-party apps. Authentication is either a user
	// session or an API key (X-API-Key / Authorization: Bearer om_...).
	v1 := e.Group("/api/v1", APIAuth)
	v1.GET("/me", h.Me)
	v1.GET("/files", RequireScope("files:read"), h.ListFiles)
	v1.POST("/files/upload", RequireScope("files:write"), h.Upload)
	v1.POST("/folders", RequireScope("files:write"), h.CreateFolder)
	v1.POST("/files/rename", RequireScope("files:write"), h.Rename)
	v1.POST("/files/move", RequireScope("files:write"), h.Move)
	v1.POST("/files/delete", RequireScope("files:write"), h.Delete)
	v1.GET("/files/:id", RequireScope("files:read"), h.FileMeta)
	v1.POST("/files/:id/sign", RequireScope("files:read"), h.SignFile)

	// Public links. /f and /dl are intentionally stable so Cloudflare can cache
	// shared resources; /s is a short-lived signed URL for private files.
	e.GET("/f/:id", h.PublicFile)
	e.GET("/f/:id/:name", h.PublicFile)
	e.GET("/dl/:id", h.DownloadFile)
	e.GET("/dl/:id/:name", h.DownloadFile)
	e.GET("/s/:token", h.SignedFile)
	e.HEAD("/f/:id", h.PublicFile)
	e.HEAD("/f/:id/:name", h.PublicFile)
	e.HEAD("/dl/:id", h.DownloadFile)
	e.HEAD("/dl/:id/:name", h.DownloadFile)
	e.HEAD("/s/:token", h.SignedFile)
}

func workDir() string {
	d, _ := os.Getwd()
	return d
}

func BaseHost() string {
	return strings.TrimSpace(os.Getenv("OM_BASE_HOST"))
}

// SubdomainRewrite maps video.om.example.com/ to /video while keeping API calls
// untouched. It is installed before routes in server.Init.
func SubdomainRewrite(baseHost string) gin.HandlerFunc {
	baseHost = strings.TrimSpace(strings.ToLower(baseHost))
	return func(c *gin.Context) {
		if baseHost == "" {
			c.Next()
			return
		}
		host := strings.ToLower(c.Request.Host)
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || host == baseHost || !strings.HasSuffix(host, "."+baseHost) {
			c.Next()
			return
		}
		sub := strings.TrimSuffix(host, "."+baseHost)
		if strings.Contains(sub, ".") {
			c.Next()
			return
		}
		switch sub {
		case "video", "image", "weibo", "im", "files":
			// Subdomains are entry points only. Keeping the rest of the paths
			// untouched means in-app links such as /files still work when the
			// visitor arrived through video.om.example.com.
			if c.Request.URL.Path == "/" {
				c.Request.URL.Path = "/" + sub
			}
		}
		c.Next()
	}
}
