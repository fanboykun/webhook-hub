package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/ingress"
)

func TestVerifyAndNormalizeRelease(t *testing.T) {
	adapter := NewAdapter()
	now := time.Now().UTC()
	body := []byte(`{"action":"published","repository":{"full_name":"iweka-dev/webhook-hub","html_url":"https://github.com/iweka-dev/webhook-hub"},"sender":{"login":"joyy"},"release":{"tag_name":"v1.0.0","name":"v1.0.0","html_url":"https://github.com/iweka-dev/webhook-hub/releases/tag/v1.0.0"}}`)

	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := ingress.InboundRequest{
		IntegrationID: "github-main",
		Headers: http.Header{
			headerEvent:     []string{"release"},
			headerDelivery:  []string{"delivery-1"},
			headerSignature: []string{signature},
		},
		RawBody:    body,
		ReceivedAt: now,
	}

	cfg := config.IntegrationConfig{
		Source:         domain.SourceGitHub,
		ResolvedSecret: "secret",
	}

	if err := adapter.Verify(context.Background(), cfg, req); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	result, err := adapter.Normalize(context.Background(), "github-main", cfg, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("expected one event, got %+v", result)
	}
	if result.Events[0].Type != "github.release.published" {
		t.Fatalf("unexpected event type: %s", result.Events[0].Type)
	}
}

func TestNormalizeUnsupportedEventIgnored(t *testing.T) {
	adapter := NewAdapter()
	req := ingress.InboundRequest{
		IntegrationID: "github-main",
		Headers: http.Header{
			headerEvent:    []string{"push"},
			headerDelivery: []string{"delivery-2"},
		},
		RawBody:    []byte(`{"repository":{"full_name":"iweka-dev/webhook-hub"}}`),
		ReceivedAt: time.Now().UTC(),
	}

	result, err := adapter.Normalize(context.Background(), "github-main", config.IntegrationConfig{}, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if result.IgnoreReason == "" {
		t.Fatalf("expected ignore reason, got %+v", result)
	}
	if len(result.Events) != 0 {
		t.Fatalf("expected no events, got %+v", result.Events)
	}
}
