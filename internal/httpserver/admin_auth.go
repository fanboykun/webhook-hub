package httpserver

import (
	"net/http"
	"strings"

	"github.com/fanboykun/webhook-hub/internal/app"
	"github.com/gin-gonic/gin"
)

func adminAuthMiddleware(service *app.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
			c.Next()
			return
		}
		if service == nil || service.Authorize(c.GetHeader("Authorization")) != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
			return
		}
		c.Next()
	}
}
