package om

import (
	"html"
	"net/http"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/gin-gonic/gin"
)

// spaPassThrough lists path prefixes the OpenList group routes for real work:
// assets, API endpoints, raw downloads, WebDAV/S3 and the embedded protocol
// surfaces. Everything else under the base path is an SPA page.
var spaPassThrough = []string{
	"/api/", "/assets/", "/images/", "/streamer/", "/static/",
	"/dav", "/s3", "/d/", "/p/", "/ad/", "/ap/", "/ae/", "/sd/", "/sad/", "/tc/",
	"/@emby/", "/@rest/", "/i/",
	"/ping", "/favicon.ico", "/robots.txt", "/manifest.json",
}

// spaBootstrapScript mirrors the ourmemory session into the slot the OpenList
// single-page app reads. The SPA keeps its JWT in localStorage under "token" and
// sends it as the Authorization header; our session lives in an HttpOnly cookie
// the page cannot read. Injecting the value server-side keeps one login for the
// whole site while the cookie stays HttpOnly for every other request.
func spaBootstrapScript(token string) string {
	var b strings.Builder
	b.WriteString(`<script>(function(){try{`)
	if token == "" {
		b.WriteString(`localStorage.removeItem("token");`)
	} else {
		b.WriteString(`localStorage.setItem("token","`)
		b.WriteString(html.EscapeString(token))
		b.WriteString(`");`)
	}
	b.WriteString(`}catch(e){}})();</script>`)
	return b.String()
}

func injectBootstrap(page, script string) string {
	if script == "" {
		return page
	}
	lower := strings.ToLower(page)
	if i := strings.Index(lower, "</head>"); i >= 0 {
		return page[:i] + script + page[i:]
	}
	if i := strings.Index(lower, "<body"); i >= 0 {
		return page[:i] + script + page[i:]
	}
	return script + page
}

// SPASessionBootstrap serves the OpenList index page with the current session
// written into localStorage. It is registered as an engine middleware so it also
// covers the SPA catch-all route (gin only applies group middleware to routes the
// group itself registered). Assets, API calls and non-HTML traffic fall through.
func SPASessionBootstrap() gin.HandlerFunc {
	return func(c *gin.Context) {
		base := strings.TrimSuffix(conf.URL.Path, "/")
		if base == "" {
			// The SPA owns the whole site; nothing to bridge into.
			c.Next()
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, base) {
			c.Next()
			return
		}
		rel := strings.TrimPrefix(path, base)
		for _, prefix := range spaPassThrough {
			if strings.HasPrefix(rel, prefix) {
				c.Next()
				return
			}
		}
		token := ""
		if v, err := c.Cookie("om_token"); err == nil {
			token = v
		}
		page := conf.IndexHtml
		if strings.HasPrefix(rel, "/@manage") {
			page = conf.ManageHtml
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusOK)
		_, _ = c.Writer.WriteString(injectBootstrap(page, spaBootstrapScript(token)))
		c.Writer.Flush()
		c.Writer.WriteHeaderNow()
		c.Abort()
	}
}
