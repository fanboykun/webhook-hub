package watcher

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/ingress"
)

func TestVerifyAndNormalizeStandardWebhookContract(t *testing.T) {
	adapter := NewAdapter()
	now := time.Now().UTC()
	body := []byte(`{"schema_version":"v1","event_id":"evt_01j0deployfail","event_type":"watcher.deployment_failed","occurred_at":"2026-06-17T02:21:17Z","watcher":{"id":12,"name":"api-prod"},"attempt":{"id":302,"kind":"deploy","reason":"new_version_found","status":"failed","triggered_by":"agent","target_version":"v1.4.3","from_version":"v1.4.2","failed_target_version":"","failure_phase":"health_check","error":"health check returned 503","parent_attempt_id":null,"root_attempt_id":302},"summary":"Deployment of api-prod to v1.4.3 failed during health_check"}`)
	ts := now.Unix()
	webhookID := "evt_01j0deployfail"
	signingSecret := "whsec_c2VjcmV0"
	signature := standardWebhookSignature(signingSecret, webhookID, ts, body)

	req := ingress.InboundRequest{
		IntegrationID: "watcher-production",
		Headers:       http.Header{},
		RawBody:       body,
		ReceivedAt:    now,
	}
	req.Headers.Set(headerWebhookID, webhookID)
	req.Headers.Set(headerWebhookTimestamp, strconv.FormatInt(ts, 10))
	req.Headers.Set(headerWebhookSignature, signature)
	req.Headers.Set(headerEventType, "watcher.deployment_failed")

	cfg := config.IntegrationConfig{
		Source:         domain.SourceWatcher,
		ResolvedSecret: signingSecret,
		ReplayWindow:   5 * time.Minute,
	}

	if err := adapter.Verify(context.Background(), cfg, req); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	result, err := adapter.Normalize(context.Background(), "watcher-production", cfg, req)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if result.SourceDeliveryID != webhookID || len(result.Events) != 1 {
		t.Fatalf("unexpected normalize result: %+v", result)
	}
	if result.Events[0].Type != "watcher.deployment.failed" {
		t.Fatalf("unexpected event type: %s", result.Events[0].Type)
	}
	if result.Events[0].Severity != domain.SeverityError {
		t.Fatalf("unexpected severity: %s", result.Events[0].Severity)
	}
}

func TestNormalizeHealthChangedEvent(t *testing.T) {
	adapter := NewAdapter()
	now := time.Now().UTC()
	body := []byte(`{"schema_version":"v1","event_id":"evt_01j0healthchanged","event_type":"service.health_changed","occurred_at":"2026-06-17T03:02:11Z","watcher":{"id":12,"name":"api-prod"},"service":{"id":87,"name":"api-prod-web","service_type":"nssm","health_check_url":"https://api.example.com/health"},"health":{"previous_status":"healthy","current_status":"unhealthy","http_status":503,"error":"unexpected status code 503","checked_at":"2026-06-17T03:02:11Z","source":"manual"},"summary":"Service api-prod-web health changed from healthy to unhealthy"}`)
	ts := now.Unix()
	webhookID := "evt_01j0healthchanged"
	signingSecret := "whsec_c2VjcmV0"
	signature := standardWebhookSignature(signingSecret, webhookID, ts, body)

	req := ingress.InboundRequest{
		IntegrationID: "watcher-production",
		Headers:       http.Header{},
		RawBody:       body,
		ReceivedAt:    now,
	}
	req.Headers.Set(headerWebhookID, webhookID)
	req.Headers.Set(headerWebhookTimestamp, strconv.FormatInt(ts, 10))
	req.Headers.Set(headerWebhookSignature, signature)

	cfg := config.IntegrationConfig{
		Source:         domain.SourceWatcher,
		ResolvedSecret: signingSecret,
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
	if result.Events[0].Type != "service.health.changed" {
		t.Fatalf("unexpected event type: %s", result.Events[0].Type)
	}
	if result.Events[0].Lifecycle != domain.LifecycleUpdated {
		t.Fatalf("unexpected lifecycle: %s", result.Events[0].Lifecycle)
	}
}

func standardWebhookSignature(secret, webhookID string, timestamp int64, body []byte) string {
	signingKey := watcherSigningKey(secret)
	if len(signingKey) == 0 {
		signingKey = []byte(secret)
	}
	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte(webhookID))
	mac.Write([]byte("."))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
