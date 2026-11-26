package om

import (
	"github.com/gin-gonic/gin"
)

// TokenBridge lets the OpenList single-page app authenticate with the
// ourmemory HttpOnly cookie. The SPA stores its token in localStorage and
// sends it in Authorization; it cannot read this cookie itself.
func TokenBridge() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			if token, err := c.Cookie("om_token"); err == nil && token != "" {
				c.Request.Header.Set("Authorization", token)
			}
		}
		c.Next()
	}
}
