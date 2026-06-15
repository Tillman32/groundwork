package web

import (
	"embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed static
var staticFiles embed.FS

func ServeUI(r *gin.Engine) {
	// Serve index.html for all non-API routes (SPA)
	r.GET("/", func(c *gin.Context) {
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// Serve static assets
	r.GET("/static/*filepath", func(c *gin.Context) {
		path := c.Param("filepath")
		data, err := staticFiles.ReadFile("static" + path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", data)
	})
}