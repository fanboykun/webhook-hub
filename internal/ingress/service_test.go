package ingress

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/clock"
	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/observability"
	"github.com/iweka-dev/webhook-hub/internal/routing"
	"github.com/iweka-dev/webhook-hub/internal/storage/sqlite"
)

func TestHandleUnroutedWatcherWebhookDoesNotCreateDeliveries(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")

	cfg := config.Config{
		Server: config.ServerConfig{
			MaxWebhookBodyBytes: 1 << 20,
		},
		API: config.APIConfig{
			ResolvedAdminToken: "admin-secret",
		},
		Logging: config.LoggingConfig{Format: "text"},
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
		},
		Integrations: map[string]config.IntegrationConfig{
			"watcher-production": {
				Source:         domain.SourceWatcher,
				SecretEnv:      "WATCHER_WEBHOOK_SECRET",
				ResolvedSecret: "secret",
				ReplayWindow:   5 * time.Minute,
			},
		},
		Destinations: map[string]config.DestinationConfig{
			"slack-deployments": {
				Type:        domain.DestinationSlack,
				ResolvedURL: "https://example.invalid",
			},
		},
	}

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service := NewService(store, cfg, NewRegistry(fakeWatcherAdapter{}), routing.New(nil), clock.Real{}, observability.NewLogger(cfg.Logging))

	now := time.Now().UTC()
	body := []byte(`{"schema_version":1,"id":"deploy_01","event":"deployment.failed","occurred_at":"2026-06-18T08:42:10Z","service":"auth-service","environment":"production","version":"v2.4.1","commit_sha":"abc123","actor":"joyy","url":"https://watcher.example/deployments/deploy_01","error":{"message":"Health check failed","stage":"verify"},"labels":{"team":"platform"}}`)
	ts := now.Format(time.RFC3339)

	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(ts))
	mac.Write([]byte(":"))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	result, err := service.Handle(context.Background(), domain.SourceWatcher, InboundRequest{
		IntegrationID: "watcher-production",
		Headers: http.Header{
			"X-Watcher-Event-ID":  []string{"deploy_01:deployment.failed"},
			"X-Watcher-Timestamp": []string{ts},
			"X-Watcher-Signature": []string{signature},
		},
		RawBody:    bytes.Clone(body),
		ReceivedAt: now,
	})
	if err != nil {
		t.Fatalf("handle failed: %v", err)
	}
	if result.Status != domain.ReceiptUnrouted {
		t.Fatalf("expected unrouted receipt, got %s", result.Status)
	}
	if result.DeliveryCount != 0 {
		t.Fatalf("expected zero deliveries, got %d", result.DeliveryCount)
	}

	deliveries, err := store.ListDeliveries(context.Background(), domain.DeliveryFilter{})
	if err != nil {
		t.Fatalf("list deliveries failed: %v", err)
	}
	if len(deliveries.Items) != 0 {
		t.Fatalf("expected no delivery rows, got %+v", deliveries.Items)
	}

	claimed, err := store.ClaimDueDeliveries(context.Background(), domain.ClaimRequest{
		WorkerID:      "worker-1",
		BatchSize:     10,
		LeaseDuration: time.Minute,
		Now:           now,
	})
	if err != nil {
		t.Fatalf("claim due deliveries failed: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("expected no claimable deliveries, got %+v", claimed)
	}
}

type fakeWatcherAdapter struct{}

func (fakeWatcherAdapter) Source() domain.Source {
	return domain.SourceWatcher
}

func (fakeWatcherAdapter) Verify(_ context.Context, integration config.IntegrationConfig, req InboundRequest) error {
	mac := hmac.New(sha256.New, []byte(integration.ResolvedSecret))
	mac.Write([]byte(req.Headers.Get("X-Watcher-Timestamp")))
	mac.Write([]byte(":"))
	mac.Write(req.RawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	if expected != req.Headers.Get("X-Watcher-Signature") {
		return ErrUnauthorized
	}
	return nil
}

func (fakeWatcherAdapter) Normalize(_ context.Context, integrationID string, _ config.IntegrationConfig, req InboundRequest) (AdapterResult, error) {
	event := domain.Event{
		Source:        domain.SourceWatcher,
		IntegrationID: integrationID,
		Type:          "watcher.deployment.failed",
		Action:        "deployment.failed",
		Lifecycle:     domain.LifecycleFailed,
		Severity:      domain.SeverityError,
		Title:         "deployment failed",
		OccurredAt:    req.ReceivedAt,
	}
	return AdapterResult{
		SourceDeliveryID: req.Headers.Get("X-Watcher-Event-ID"),
		SourceEventType:  "deployment.failed",
		Events:           []domain.Event{event},
	}, nil
}
