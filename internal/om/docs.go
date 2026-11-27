package om

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// Docs serves the raw API documentation as markdown text. Deployment keeps
// docs/ beside the working directory; a missing file yields an empty 404 and
// the frontend renders its built-in fallback.
func (h *Handlers) Docs(c *gin.Context) {
	b, err := os.ReadFile(filepath.Join(workDir(), "docs", "API.md"))
	if err != nil {
		c.String(http.StatusNotFound, "")
		return
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.String(http.StatusOK, string(b))
}
