package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
)

func TestSendIncludesTelegramErrorBody(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"chat not found"}`))
	}))
	defer api.Close()

	sender := New(map[string]config.DestinationConfig{
		"telegram-group": {
			Type:          domain.DestinationTelegram,
			ResolvedToken: "bot-token",
			ChatID:        "-100123456789",
			APIBaseURL:    api.URL,
		},
	})

	_, err := sender.Send(context.Background(), "telegram-group", domain.RenderedMessage{
		ContentType: "text/html; charset=utf-8",
		Body:        []byte("<b>hello</b>"),
	})
	if err == nil {
		t.Fatal("expected telegram upstream rejection")
	}
	if got := err.Error(); got == "" || got == "telegram returned 400" {
		t.Fatalf("expected response body in error, got %q", got)
	}
}
