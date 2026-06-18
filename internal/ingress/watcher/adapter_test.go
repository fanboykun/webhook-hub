package watcher

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

func TestVerifyAndNormalize(t *testing.T) {
	adapter := NewAdapter()
	now := time.Now().UTC()
	body := []byte(`{"schema_version":1,"id":"deploy_01","event":"deployment.failed","occurred_at":"2026-06-18T08:42:10Z","service":"auth-service","environment":"production","version":"v2.4.1","commit_sha":"abc123","actor":"joyy","url":"https://watcher.example/deployments/deploy_01","error":{"message":"Health check failed","stage":"verify"},"labels":{"team":"platform"}}`)
	ts := now.Format(time.RFC3339)

	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(ts))
	mac.Write([]byte(":"))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	req := ingress.InboundRequest{
		IntegrationID: "watcher-production",
		Headers: http.Header{
			headerEventID:   []string{"deploy_01:deployment.failed"},
			headerTimestamp: []string{ts},
			headerSignature: []string{signature},
		},
		RawBody:    body,
		ReceivedAt: now,
	}

	cfg := config.IntegrationConfig{
		Source:         domain.SourceWatcher,
		ResolvedSecret: "secret",
		ReplayWindow:   5 * time.Minute,
	}

	if err := adapter.Verify(context.Background(), cfg, req); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	result, err := adapter.Normalize(context.Background(), "watcher-production", cfg, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if result.SourceDeliveryID == "" || len(result.Events) != 1 {
		t.Fatalf("unexpected normalize result: %+v", result)
	}
	if result.Events[0].Type != "watcher.deployment.failed" {
		t.Fatalf("unexpected event type: %s", result.Events[0].Type)
	}
}

func TestNormalizeCurrentWatcherContract(t *testing.T) {
	adapter := NewAdapter()
	now := time.Now().UTC()
	body := []byte(`{"schema_version":"v1","event_id":"evt_01j0healthchanged","event_type":"service.health_changed","occurred_at":"2026-06-17T03:02:11Z","watcher":{"id":12,"name":"api-prod"},"service":{"id":87,"name":"api-prod-web","service_type":"nssm","health_check_url":"https://api.example.com/health"},"health":{"previous_status":"healthy","current_status":"unhealthy","http_status":503,"error":"unexpected status code 503","checked_at":"2026-06-17T03:02:11Z","source":"manual"},"summary":"Service api-prod-web health changed from healthy to unhealthy"}`)
	ts := now.Format(time.RFC3339)

	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(ts))
	mac.Write([]byte(":"))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	req := ingress.InboundRequest{
		IntegrationID: "watcher-production",
		Headers: http.Header{
			headerDeliveryID: []string{"dlv_01j0healthchanged"},
			headerEventType:  []string{"service.health_changed"},
			headerTimestamp:  []string{ts},
			headerSignature:  []string{signature},
		},
		RawBody:    body,
		ReceivedAt: now,
	}

	cfg := config.IntegrationConfig{
		Source:         domain.SourceWatcher,
		ResolvedSecret: "secret",
		ReplayWindow:   5 * time.Minute,
	}

	if err := adapter.Verify(context.Background(), cfg, req); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	result, err := adapter.Normalize(context.Background(), "watcher-production", cfg, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("unexpected normalize result: %+v", result)
	}
	if result.Events[0].Type != "service.health_changed" {
		t.Fatalf("unexpected event type: %s", result.Events[0].Type)
	}
	if result.Events[0].Service != "api-prod-web" {
		t.Fatalf("unexpected service: %s", result.Events[0].Service)
	}
}
