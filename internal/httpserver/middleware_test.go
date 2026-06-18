package httpserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestLoggingMiddlewareLogsAbortedRequests(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(requestLoggingMiddleware(logger))
	engine.Use(func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/v1/watcher/watcher-production", strings.NewReader(`{"test":true}`))
	req.Header.Set("User-Agent", "debug-client/1.0")
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	logs := buf.String()
	if !strings.Contains(logs, "http.request") {
		t.Fatalf("missing request log: %s", logs)
	}
	if !strings.Contains(logs, "status=401") {
		t.Fatalf("missing status in log: %s", logs)
	}
	if !strings.Contains(logs, "method=POST") {
		t.Fatalf("missing method in log: %s", logs)
	}
}
