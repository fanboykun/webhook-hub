package httpserver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/app"
	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	configcrypto "github.com/fanboykun/webhook-hub/internal/config/crypto"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/ingress"
	ghingress "github.com/fanboykun/webhook-hub/internal/ingress/github"
	"github.com/fanboykun/webhook-hub/internal/ingress/watcher"
	"github.com/fanboykun/webhook-hub/internal/observability"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
)

func TestWatcherWebhookAccepted(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	server := newTestServer(t, cfg, store, nil)

	body := map[string]any{
		"schema_version": "v1",
		"event_id":       "deploy_01",
		"event_type":     "watcher.deployment_failed",
		"occurred_at":    "2026-06-18T08:42:10Z",
		"watcher":        map[string]any{"id": 12, "name": "api-prod"},
		"attempt": map[string]any{
			"id":                302,
			"kind":              "deploy",
			"reason":            "new_version_found",
			"status":            "failed",
			"triggered_by":      "agent",
			"target_version":    "v1.4.3",
			"from_version":      "v1.4.2",
			"failure_phase":     "health_check",
			"error":             "health check returned 503",
			"parent_attempt_id": nil,
			"root_attempt_id":   302,
		},
		"summary": "Deployment of api-prod to v1.4.3 failed during health_check",
	}
	rawBody, _ := json.Marshal(body)
	ts := time.Now().UTC().Unix()
	signature := watcherSignature("secret", "deploy_01", ts, rawBody)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/v1/watcher/watcher-production", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", "deploy_01")
	req.Header.Set("webhook-timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("webhook-signature", signature)
	req.Header.Set("X-Watcher-Event", "watcher.deployment_failed")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted && rec.Code != http.StatusOK {
		t.Fatalf("expected success, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWatcherWebhookUnauthorized(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	server := newTestServer(t, cfg, store, nil)

	body := map[string]any{
		"schema_version": "v1",
		"event_id":       "deploy_01",
		"event_type":     "watcher.deployment_failed",
		"occurred_at":    "2026-06-18T08:42:10Z",
		"watcher":        map[string]any{"id": 12, "name": "api-prod"},
		"attempt": map[string]any{
			"id":                302,
			"kind":              "deploy",
			"reason":            "new_version_found",
			"status":            "failed",
			"triggered_by":      "agent",
			"target_version":    "v1.4.3",
			"from_version":      "v1.4.2",
			"failure_phase":     "health_check",
			"error":             "health check returned 503",
			"parent_attempt_id": nil,
			"root_attempt_id":   302,
		},
		"summary": "Deployment of api-prod to v1.4.3 failed during health_check",
	}
	rawBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/v1/watcher/watcher-production", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", "deploy_01")
	req.Header.Set("webhook-timestamp", strconv.FormatInt(time.Now().UTC().Unix(), 10))
	req.Header.Set("webhook-signature", "bad")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGitHubWebhookAccepted(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	server := newTestServer(t, cfg, store, nil)

	rawBody, _ := json.Marshal(map[string]any{
		"action": "published",
		"repository": map[string]any{
			"full_name": "iweka-dev/webhook-hub",
			"html_url":  "https://github.com/fanboykun/webhook-hub",
		},
		"sender": map[string]any{
			"login": "joyy",
		},
		"release": map[string]any{
			"tag_name": "v1.0.0",
			"name":     "v1.0.0",
			"html_url": "https://github.com/fanboykun/webhook-hub/releases/tag/v1.0.0",
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/v1/github/github-main", bytes.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "release")
	req.Header.Set("X-GitHub-Delivery", "gh-delivery-1")
	req.Header.Set("X-Hub-Signature-256", githubSignature("github-secret", rawBody))

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted && rec.Code != http.StatusOK {
		t.Fatalf("expected success, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpenAPIIncludesWatcherWebhook(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	server := newTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Contains(body, []byte("/webhooks/v1/watcher/{integration_id}")) {
		t.Fatalf("openapi missing watcher webhook path: %s", string(body))
	}
	if !bytes.Contains(body, []byte("/webhooks/v1/github/{integration_id}")) {
		t.Fatalf("openapi missing github webhook path: %s", string(body))
	}
	if !bytes.Contains(body, []byte("/api/v1/deliveries/{delivery_id}")) {
		t.Fatalf("openapi missing delivery detail path: %s", string(body))
	}
	if !bytes.Contains(body, []byte("/api/v1/receipts/{receipt_id}")) {
		t.Fatalf("openapi missing receipt detail path: %s", string(body))
	}
	if !bytes.Contains(body, []byte("/api/v1/receipts")) {
		t.Fatalf("openapi missing receipt list path: %s", string(body))
	}
	if !bytes.Contains(body, []byte("/api/v1/renderer-profiles")) {
		t.Fatalf("openapi missing renderer profile path: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`"bearerAuth"`)) {
		t.Fatalf("openapi missing bearer auth scheme: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`"propertyName":"source"`)) {
		t.Fatalf("openapi missing integration source discriminator: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`"propertyName":"type"`)) {
		t.Fatalf("openapi missing destination type discriminator: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`"oneOf"`)) {
		t.Fatalf("openapi missing oneOf union schemas: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`#/components/schemas/WatcherIntegrationRequestModel`)) {
		t.Fatalf("openapi missing watcher integration request schema ref: %s", string(body))
	}
	if !bytes.Contains(body, []byte(`#/components/schemas/GithubIntegrationRequestModel`)) {
		t.Fatalf("openapi missing github integration request schema ref: %s", string(body))
	}
	if bytes.Contains(body, []byte(`"GithubIntegrationRequestModel":{"additionalProperties":false,"properties":{"id":{"type":"string"},"replay_window"`)) {
		t.Fatalf("openapi should not expose replay_window on github integration request schema: %s", string(body))
	}
}

func TestDocsUsesScalarRenderer(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	server := newTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("@scalar/api-reference")) {
		t.Fatalf("docs page is not using scalar: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("persistAuth: true")) {
		t.Fatalf("docs page does not persist auth: %s", rec.Body.String())
	}
}

func TestDynamicConfigEndpoints(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-secret")
	cfg := testConfig(t)

	store := openEncryptedHTTPTestStore(t, cfg)
	server := newEncryptedTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list integrations expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "[REDACTED]") {
		t.Fatalf("expected redacted integration secret, body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/integrations?source=github", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filtered integrations expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"source":"github"`) {
		t.Fatalf("expected github integration in filtered list, body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"source":"watcher"`) {
		t.Fatalf("did not expect watcher integration in filtered list, body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/integrations/github-main", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get integration expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"secret":"[REDACTED]"`) {
		t.Fatalf("expected integration detail to redact secret, body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/destinations", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list destinations expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"webhook_url":"[REDACTED]"`) {
		t.Fatalf("expected destination list to redact webhook url, body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/destinations?type=slack", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("filtered destinations expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"type":"slack"`) {
		t.Fatalf("expected slack destination in filtered list, body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"type":"telegram"`) {
		t.Fatalf("did not expect telegram destination in filtered list, body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/renderer-profiles", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list renderer profiles expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"detailed"`) {
		t.Fatalf("expected renderer profile in list response, body=%s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/renderer-profiles/detailed", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get renderer profile expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"key":"watcher.deployment.failed"`) {
		t.Fatalf("expected renderer profile detail to include event binding, body=%s", rec.Body.String())
	}

	updateIntegrationBody := []byte(`{"id":"github-main","source":"github","secret":"[REDACTED]"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/integrations/github-main", bytes.NewReader(updateIntegrationBody))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update integration expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	integration, err := store.GetIntegration(context.Background(), "github-main")
	if err != nil {
		t.Fatalf("get integration failed: %v", err)
	}
	if integration.Secret != "github-secret" {
		t.Fatalf("expected preserved integration secret, got %q", integration.Secret)
	}

	updateProfileBody := []byte(`{"id":"detailed","bindings":[{"event":{"source":"watcher","key":"watcher.deployment.failed"},"templates":{"slack":{"title":"{{.Title}}","body":"{{.Summary}}"},"telegram":{"text":"<b>{{.Title}}</b>"}}}]}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/renderer-profiles/detailed", bytes.NewReader(updateProfileBody))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update renderer profile expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	profile, err := store.GetRendererProfile(context.Background(), "detailed")
	if err != nil {
		t.Fatalf("get renderer profile failed: %v", err)
	}
	var found *domain.RendererDestinationTemplates
	for _, b := range profile.Profile.Bindings {
		if b.Event.Key == "watcher.deployment.failed" {
			found = &b.Templates
			break
		}
	}
	if found == nil || found.Telegram == nil {
		t.Fatalf("expected updated renderer profile to persist telegram template, got %+v", profile.Profile)
	}

	updateBody := []byte(`{"id":"slack-deployments","type":"slack","webhook_url":"[REDACTED]","profile":"detailed"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/destinations/slack-deployments", bytes.NewReader(updateBody))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update destination expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	item, err := store.GetDestination(context.Background(), "slack-deployments")
	if err != nil {
		t.Fatalf("get destination failed: %v", err)
	}
	if item.WebhookURL != "https://example.invalid" {
		t.Fatalf("expected preserved webhook url, got %q", item.WebhookURL)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/destinations/slack-deployments", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get destination expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"webhook_url":"[REDACTED]"`) {
		t.Fatalf("expected destination detail to redact webhook url, body=%s", rec.Body.String())
	}

	createIntegrationBody := []byte(`{"id":"github-secondary","source":"github","secret":"new-github-secret"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/integrations", bytes.NewReader(createIntegrationBody))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create integration expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/integrations/github-secondary", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete integration expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	createProfileBody := []byte(`{"id":"ops-compact","bindings":[{"event":{"source":"watcher","key":"watcher.deployment.failed"},"templates":{"slack":{"title":"{{.Title}}","body":"{{.Summary}}"},"telegram":{"text":"<b>{{.Title}}</b>\n{{.Summary}}"}}}]}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/renderer-profiles", bytes.NewReader(createProfileBody))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create renderer profile expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	createDestinationBody := []byte(`{"id":"telegram-ops","type":"telegram","bot_token":"bot-token","chat_id":"-100123456789","profile":"detailed"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/destinations", bytes.NewReader(createDestinationBody))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create destination expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	routeBody := []byte(`{"id":"telegram-route","match":{"sources":["watcher"]},"destinations":["telegram-ops"]}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewReader(routeBody))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create route expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/destinations/telegram-ops", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete destination expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewReader(routeBody))
	req.Header.Set("Authorization", "Bearer admin-secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected route validation failure after destination delete, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/renderer-profiles/ops-compact", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete renderer profile expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeliveryDetailAuthorized(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-secret")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	if _, err := store.Ingest(context.Background(), deliverySeedBatch(now)); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	server := newTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deliveries/d1", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"id":"d1"`)) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestReceiptDetailAuthorizedShowsUnroutedStatus(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-secret")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	if _, err := store.Ingest(context.Background(), domain.IngestBatch{
		Receipt: domain.Receipt{
			ID:               "r-unrouted",
			Source:           domain.SourceWatcher,
			IntegrationID:    "watcher-production",
			SourceDeliveryID: "deploy_01",
			SourceEventType:  "watcher.deployment.failed",
			PayloadSHA256:    "abc",
			ReceivedAt:       now,
			Status:           domain.ReceiptUnrouted,
			CreatedAt:        now,
		},
		Events: []domain.Event{
			{
				ID:        "e-unrouted",
				ReceiptID: "r-unrouted",
				EventEnvelope: domain.EventEnvelope{
					Source:        domain.SourceWatcher,
					IntegrationID: "watcher-production",
					Key:           "watcher.deployment.failed",
					Severity:      domain.SeverityError,
					Title:         "deployment failed",
					Summary:       "health check failed",
					OccurredAt:    now,
				},
				CreatedAt: now,
			},
		},
	}); err != nil {
		t.Fatalf("seed unrouted receipt: %v", err)
	}

	server := newTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/receipts/r-unrouted", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"unrouted"`)) {
		t.Fatalf("receipt status missing from response: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"delivery_count":0`)) {
		t.Fatalf("expected no deliveries in response: %s", rec.Body.String())
	}
}

func TestListReceiptsAuthorizedFiltersUnrouted(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-secret")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	if _, err := store.Ingest(context.Background(), domain.IngestBatch{
		Receipt: domain.Receipt{
			ID:               "r-list-1",
			Source:           domain.SourceWatcher,
			IntegrationID:    "watcher-production",
			SourceDeliveryID: "deploy_01",
			SourceEventType:  "watcher.deployment.failed",
			PayloadSHA256:    "abc",
			ReceivedAt:       now,
			Status:           domain.ReceiptUnrouted,
			CreatedAt:        now,
		},
	}); err != nil {
		t.Fatalf("seed unrouted receipt: %v", err)
	}

	server := newTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/receipts?status=unrouted&source=watcher&integration_id=watcher-production&limit=10", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"id":"r-list-1"`)) {
		t.Fatalf("expected receipt in list response: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"unrouted"`)) {
		t.Fatalf("expected unrouted status in list response: %s", rec.Body.String())
	}
}

func TestListDeliveriesAuthorized(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-secret")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	if _, err := store.Ingest(context.Background(), deliverySeedBatch(now)); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	server := newTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deliveries?status=pending&destination_id=slack-deployments&limit=1", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"items":[`)) {
		t.Fatalf("unexpected list response: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"id":"d1"`)) {
		t.Fatalf("delivery d1 missing from list response: %s", rec.Body.String())
	}
}

func TestDeliveryDetailUnauthorized(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-secret")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	server := newTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deliveries/d1", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRetryDeliveryAuthorized(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-secret")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	seed := deliverySeedBatch(now)
	seed.DeliveryByEvent["e1"][0].Status = domain.DeliveryDeadLetter
	if _, err := store.Ingest(context.Background(), seed); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	server := newTestServer(t, cfg, store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deliveries/d1/retry", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	delivery, err := store.GetDelivery(context.Background(), "d1")
	if err != nil {
		t.Fatalf("get delivery: %v", err)
	}
	if delivery.Status != domain.DeliveryRetryWait {
		t.Fatalf("expected retry_wait, got %s", delivery.Status)
	}
}

func TestRoutesListAndReplaceAuthorized(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid")
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-secret")
	cfg := testConfig(t)

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, route := range seededRoutes() {
		if err := store.CreateRoute(context.Background(), route); err != nil {
			t.Fatalf("seed route: %v", err)
		}
	}

	server := newTestServer(t, cfg, store, seededRoutes())

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	listReq.Header.Set("Authorization", "Bearer admin-secret")
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !bytes.Contains(listRec.Body.Bytes(), []byte(`"id":"deployment-failed"`)) {
		t.Fatalf("unexpected list response: %s", listRec.Body.String())
	}

	postBody := []byte(`{"id":"watcher-all-events","description":"Send all Watcher events to Telegram bot","match":{"sources":["watcher"]},"destinations":["slack-deployments"]}`)
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewReader(postBody))
	postReq.Header.Set("Authorization", "Bearer admin-secret")
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", postRec.Code, postRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/routes/watcher-all-events", nil)
	getReq.Header.Set("Authorization", "Bearer admin-secret")
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	putBody := []byte(`{"id":"watcher-all-events","description":"Updated route","match":{"sources":["watcher"],"types":["watcher.deployment.failed"]},"destinations":["slack-deployments"]}`)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/routes/watcher-all-events", bytes.NewReader(putBody))
	putReq.Header.Set("Authorization", "Bearer admin-secret")
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", putRec.Code, putRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/routes/watcher-all-events", nil)
	deleteReq.Header.Set("Authorization", "Bearer admin-secret")
	deleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	routes, err := store.ListRoutes(context.Background())
	if err != nil {
		t.Fatalf("list routes after replace: %v", err)
	}
	if len(routes) != len(seededRoutes()) {
		t.Fatalf("unexpected routes after delete: %+v", routes)
	}
}

func watcherSignature(secret, webhookID string, timestamp int64, body []byte) string {
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

func watcherSigningKey(secret string) []byte {
	secret = strings.TrimSpace(secret)
	if strings.HasPrefix(secret, "whsec_") {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_")); err == nil {
			return decoded
		}
	}
	return nil
}

func githubSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func testRegistry() *ingress.Registry {
	return ingress.NewRegistry(
		watcher.NewAdapter(),
		ghingress.NewAdapter(),
	)
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		Server: config.ServerConfig{
			Address:             ":0",
			ReadHeaderTimeout:   5 * time.Second,
			ReadTimeout:         15 * time.Second,
			WriteTimeout:        15 * time.Second,
			IdleTimeout:         60 * time.Second,
			ShutdownTimeout:     5 * time.Second,
			MaxWebhookBodyBytes: 1 << 20,
		},
		API: config.APIConfig{
			AdminTokenEnv:      "GATEWAY_ADMIN_TOKEN",
			ResolvedAdminToken: "admin-secret",
			DocsEnabled:        true,
		},
		Logging: config.LoggingConfig{Level: "debug", Format: "text"},
		Database: config.DatabaseConfig{
			Path:               t.TempDir() + "/gateway.db",
			BusyTimeout:        5 * time.Second,
			MaxOpenConnections: 1,
			RetainRawPayloads:  true,
		},
		Workers: config.WorkersConfig{
			BatchSize:     10,
			Concurrency:   1,
			LeaseDuration: time.Minute,
		},
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   time.Second,
			MaxDelay:    time.Minute,
			Jitter:      0,
		},
		Integrations: map[string]config.IntegrationConfig{
			"watcher-production": {
				Source:         "watcher",
				SecretEnv:      "WATCHER_WEBHOOK_SECRET",
				ResolvedSecret: "whsec_c2VjcmV0",
				ReplayWindow:   5 * time.Minute,
			},
			"github-main": {
				Source:         "github",
				SecretEnv:      "GITHUB_WEBHOOK_SECRET",
				ResolvedSecret: "github-secret",
			},
		},
		Destinations: map[string]config.DestinationConfig{
			"slack-deployments": {
				Type:          "slack",
				WebhookURLEnv: "SLACK_DEPLOYMENTS_WEBHOOK_URL",
				ResolvedURL:   "https://example.invalid",
				Profile:       "detailed",
			},
		},
		RendererProfiles: map[string]config.ProfileConfig{
			"detailed": {
				Bindings: []config.ProfileBindingConfig{
					{
						Event: config.EventBindingConfig{
							Source: domain.SourceWatcher,
							Key:    "watcher.deployment.failed",
						},
						Templates: config.DestinationTemplates{
							Slack: &config.SlackTemplateConfig{
								Title: "{{.Title}}",
								Body:  "{{.Summary}}",
							},
						},
					},
				},
			},
		},
	}
}

func seededRoutes() []domain.Route {
	return []domain.Route{
		{
			ID:           "deployment-failed",
			Match:        domain.RouteMatchCriteria{Types: []string{"watcher.deployment.failed"}},
			Destinations: []string{"slack-deployments"},
		},
		{
			ID:           "github-release",
			Match:        domain.RouteMatchCriteria{Types: []string{"github.release.published"}},
			Destinations: []string{"slack-deployments"},
		},
	}
}

func newTestServer(t *testing.T, cfg config.Config, store *sqlite.Store, routes []domain.Route) *Server {
	t.Helper()
	engine := routing.New(routes)
	integrations := runtimeconfig.NewIntegrationRegistry(cfg.Integrations)
	destinations := runtimeconfig.NewDestinationRegistry(cfg.Destinations)
	service := ingress.NewService(store, cfg, testRegistry(), engine, integrations, destinations, clock.Real{}, observability.NewLogger(cfg.Logging))
	appService := app.NewService(cfg, clock.Real{}, store, service, engine, integrations, destinations, runtimeconfig.NewRendererProfileRegistry(runtimeconfig.RendererProfilesFromConfig(cfg.RendererProfiles)))
	if routes != nil {
		engine.Replace(routes)
	}
	return New(cfg, appService, observability.NewLogger(cfg.Logging))
}

func newEncryptedTestServer(t *testing.T, cfg config.Config, store *sqlite.Store, routes []domain.Route) *Server {
	t.Helper()
	engine := routing.New(routes)
	integrations := runtimeconfig.NewIntegrationRegistry(cfg.Integrations)
	destinations := runtimeconfig.NewDestinationRegistry(cfg.Destinations)
	service := ingress.NewService(store, cfg, testRegistry(), engine, integrations, destinations, clock.Real{}, observability.NewLogger(cfg.Logging))
	appService := app.NewService(cfg, clock.Real{}, store, service, engine, integrations, destinations, runtimeconfig.NewRendererProfileRegistry(runtimeconfig.RendererProfilesFromConfig(cfg.RendererProfiles)))
	if err := appService.BootstrapDynamicConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap dynamic config: %v", err)
	}
	if routes != nil {
		engine.Replace(routes)
	}
	return New(cfg, appService, observability.NewLogger(cfg.Logging))
}

func openEncryptedHTTPTestStore(t *testing.T, cfg config.Config) *sqlite.Store {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	cipher, err := configcrypto.NewFromString(hex.EncodeToString(key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	store, err := sqlite.OpenWithCipher(cfg.Database, observability.NewLogger(cfg.Logging), cipher)
	if err != nil {
		t.Fatalf("open encrypted store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func deliverySeedBatch(now time.Time) domain.IngestBatch {
	return domain.IngestBatch{
		Receipt: domain.Receipt{
			ID:               "r1",
			Source:           domain.SourceWatcher,
			IntegrationID:    "watcher-production",
			SourceDeliveryID: "event-1",
			SourceEventType:  "deployment.failed",
			PayloadSHA256:    "abc",
			ReceivedAt:       now,
			Status:           domain.ReceiptAccepted,
			CreatedAt:        now,
		},
		Events: []domain.Event{
			{
				ID:        "e1",
				ReceiptID: "r1",
				EventEnvelope: domain.EventEnvelope{
					Source:        domain.SourceWatcher,
					IntegrationID: "watcher-production",
					Key:           "watcher.deployment.failed",
					Action:        "deployment.failed",
					Lifecycle:     domain.LifecycleFailed,
					Severity:      domain.SeverityError,
					Title:         "deployment failed",
					Summary:       "health check failed",
					Scope: domain.EventScope{
						Service:     "auth-service",
						Environment: "production",
					},
					OccurredAt: now,
				},
				CreatedAt: now,
			},
		},
		DeliveryByEvent: map[string][]domain.Delivery{
			"e1": {
				{
					ID:              "d1",
					EventID:         "e1",
					DestinationID:   "slack-deployments",
					DestinationType: domain.DestinationSlack,
					Status:          domain.DeliveryPending,
					MaxAttempts:     3,
					NextAttemptAt:   now,
					CreatedAt:       now,
					UpdatedAt:       now,
				},
			},
		},
	}
}
