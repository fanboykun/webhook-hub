package httpserver

import (
	"log/slog"
	"time"

	"github.com/fanboykun/webhook-hub/internal/id"
	"github.com/gin-gonic/gin"
)

func requestLoggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		start := time.Now().UTC()
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = id.New(start)
		}
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Set("request_id", requestID)

		c.Next()

		logger.Info(
			"http.request",
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.FullPath(),
			"request_path", c.Request.URL.Path,
			"query", c.Request.URL.RawQuery,
			"remote_ip", c.ClientIP(),
			"status", c.Writer.Status(),
			"size", c.Writer.Size(),
			"duration_ms", time.Since(start).Milliseconds(),
			"content_length", c.Request.ContentLength,
			"user_agent", c.Request.UserAgent(),
			"host", c.Request.Host,
		)
	}
}
