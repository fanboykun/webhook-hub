package ingress

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/observability"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
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

	service := NewService(
		store,
		cfg,
		NewRegistry(fakeWatcherAdapter{}),
		routing.New(nil),
		runtimeconfig.NewIntegrationRegistry(cfg.Integrations),
		runtimeconfig.NewDestinationRegistry(cfg.Destinations),
		clock.Real{},
		observability.NewLogger(cfg.Logging),
	)

	now := time.Now().UTC()
	body := []byte(`{"schema_version":"v1","event_id":"deploy_01","event_type":"watcher.deployment_failed","occurred_at":"2026-06-18T08:42:10Z","watcher":{"id":12,"name":"api-prod"},"attempt":{"id":302,"kind":"deploy","reason":"new_version_found","status":"failed","triggered_by":"agent","target_version":"v1.4.3","from_version":"v1.4.2","failed_target_version":"","failure_phase":"health_check","error":"health check returned 503","parent_attempt_id":null,"root_attempt_id":302},"summary":"Deployment of api-prod to v1.4.3 failed during health_check"}`)
	ts := now.Unix()

	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte("deploy_01"))
	mac.Write([]byte("."))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	signature := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))

	headers := http.Header{}
	headers.Set("webhook-id", "deploy_01")
	headers.Set("webhook-timestamp", strconv.FormatInt(ts, 10))
	headers.Set("webhook-signature", signature)

	result, err := service.Handle(context.Background(), domain.SourceWatcher, InboundRequest{
		IntegrationID: "watcher-production",
		Headers:       headers,
		RawBody:       bytes.Clone(body),
		ReceivedAt:    now,
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

func TestHandleUsesHotReloadedIntegrationRegistry(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "secret")

	cfg := config.Config{
		Server:  config.ServerConfig{MaxWebhookBodyBytes: 1 << 20},
		API:     config.APIConfig{ResolvedAdminToken: "admin-secret"},
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
		Retry: config.RetryConfig{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute},
		Destinations: map[string]config.DestinationConfig{
			"slack-deployments": {Type: domain.DestinationSlack, ResolvedURL: "https://example.invalid"},
		},
	}

	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	integrations := runtimeconfig.NewIntegrationRegistry(nil)
	service := NewService(
		store,
		cfg,
		NewRegistry(fakeWatcherAdapter{}),
		routing.New([]domain.Route{{ID: "watcher-all", Match: domain.RouteMatchCriteria{Sources: []domain.Source{domain.SourceWatcher}}, Destinations: []string{"slack-deployments"}}}),
		integrations,
		runtimeconfig.NewDestinationRegistry(cfg.Destinations),
		clock.Real{},
		observability.NewLogger(cfg.Logging),
	)

	now := time.Now().UTC()
	body := []byte(`{"schema_version":"v1","event_id":"deploy_02","event_type":"watcher.deployment_failed"}`)
	ts := now.Unix()
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte("deploy_02"))
	mac.Write([]byte("."))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	signature := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	headers := http.Header{}
	headers.Set("webhook-id", "deploy_02")
	headers.Set("webhook-timestamp", strconv.FormatInt(ts, 10))
	headers.Set("webhook-signature", signature)
	req := InboundRequest{
		IntegrationID: "watcher-production",
		Headers:       headers,
		RawBody:       body,
		ReceivedAt:    now,
	}

	if _, err := service.Handle(context.Background(), domain.SourceWatcher, req); err != ErrUnknownIntegration {
		t.Fatalf("expected unknown integration before reload, got %v", err)
	}

	integrations.Replace(map[string]config.IntegrationConfig{
		"watcher-production": {
			Source:         domain.SourceWatcher,
			ResolvedSecret: "secret",
			ReplayWindow:   5 * time.Minute,
		},
	})

	result, err := service.Handle(context.Background(), domain.SourceWatcher, req)
	if err != nil {
		t.Fatalf("handle after reload failed: %v", err)
	}
	if result.DeliveryCount != 1 {
		t.Fatalf("expected 1 delivery after reload, got %d", result.DeliveryCount)
	}
}

type fakeWatcherAdapter struct{}

func (fakeWatcherAdapter) Source() domain.Source {
	return domain.SourceWatcher
}

func (fakeWatcherAdapter) Verify(_ context.Context, integration config.IntegrationConfig, req InboundRequest) error {
	mac := hmac.New(sha256.New, []byte(integration.ResolvedSecret))
	mac.Write([]byte(req.Headers.Get("webhook-id")))
	mac.Write([]byte("."))
	mac.Write([]byte(req.Headers.Get("webhook-timestamp")))
	mac.Write([]byte("."))
	mac.Write(req.RawBody)
	expected := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if expected != req.Headers.Get("webhook-signature") {
		return ErrUnauthorized
	}
	return nil
}

func (fakeWatcherAdapter) Normalize(_ context.Context, integrationID string, _ config.IntegrationConfig, req InboundRequest) (AdapterResult, error) {
	event := domain.EventCandidate{
		EventEnvelope: domain.EventEnvelope{
			Source:        domain.SourceWatcher,
			IntegrationID: integrationID,
			Key:           "watcher.deployment.failed",
			Action:        "failed",
			Lifecycle:     domain.LifecycleFailed,
			Severity:      domain.SeverityError,
			Title:         "deployment failed",
			OccurredAt:    req.ReceivedAt,
		},
	}
	return AdapterResult{
		SourceDeliveryID: req.Headers.Get("webhook-id"),
		SourceEventType:  "watcher.deployment.failed",
		Events:           []domain.EventCandidate{event},
	}, nil
}
