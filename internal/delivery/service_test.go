package delivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/observability"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
)

func TestProcessOnceSent(t *testing.T) {
	slack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer slack.Close()

	cfg := deliveryTestConfig(t, slack.URL)
	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	_, err = store.Ingest(context.Background(), domain.IngestBatch{
		Receipt: domain.Receipt{
			ID:               "r1",
			Source:           domain.SourceWatcher,
			IntegrationID:    "watcher-production",
			SourceDeliveryID: "evt-1",
			SourceEventType:  "deployment.failed",
			PayloadSHA256:    "abc",
			ReceivedAt:       now,
			Status:           domain.ReceiptAccepted,
			CreatedAt:        now,
		},
		Events: []domain.Event{{
			ID:            "e1",
			ReceiptID:     "r1",
			Source:        domain.SourceWatcher,
			IntegrationID: "watcher-production",
			Type:          "watcher.deployment.failed",
			Severity:      domain.SeverityError,
			Title:         "deployment failed",
			Summary:       "health check failed",
			Service:       "auth-service",
			Environment:   "production",
			Release:       "v1.2.3",
			OccurredAt:    now,
			CreatedAt:     now,
		}},
		DeliveryByEvent: map[string][]domain.Delivery{
			"e1": {{
				ID:              "d1",
				EventID:         "e1",
				DestinationID:   "slack-deployments",
				DestinationType: domain.DestinationSlack,
				Status:          domain.DeliveryPending,
				MaxAttempts:     3,
				NextAttemptAt:   now,
				CreatedAt:       now,
				UpdatedAt:       now,
			}},
		},
	})
	if err != nil {
		t.Fatalf("seed ingest: %v", err)
	}

	service := NewService(store, cfg, clock.Real{}, observability.NewLogger(cfg.Logging))
	claimed, err := service.ProcessOnce(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("process once: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("expected 1 claimed delivery, got %d", claimed)
	}
}

func TestProcessOnceTelegramSent(t *testing.T) {
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/sendMessage" {
			t.Fatalf("unexpected telegram path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer telegramAPI.Close()

	cfg := deliveryTelegramTestConfig(t, telegramAPI.URL)
	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	_, err = store.Ingest(context.Background(), domain.IngestBatch{
		Receipt: domain.Receipt{
			ID:               "r1",
			Source:           domain.SourceWatcher,
			IntegrationID:    "watcher-production",
			SourceDeliveryID: "evt-1",
			SourceEventType:  "deployment.failed",
			PayloadSHA256:    "abc",
			ReceivedAt:       now,
			Status:           domain.ReceiptAccepted,
			CreatedAt:        now,
		},
		Events: []domain.Event{{
			ID:            "e1",
			ReceiptID:     "r1",
			Source:        domain.SourceWatcher,
			IntegrationID: "watcher-production",
			Type:          "watcher.deployment.failed",
			Severity:      domain.SeverityError,
			Title:         "deployment failed",
			Summary:       "health check failed",
			Service:       "auth-service",
			Environment:   "production",
			Release:       "v1.2.3",
			OccurredAt:    now,
			CreatedAt:     now,
		}},
		DeliveryByEvent: map[string][]domain.Delivery{
			"e1": {{
				ID:              "d1",
				EventID:         "e1",
				DestinationID:   "telegram-bot",
				DestinationType: domain.DestinationTelegram,
				Status:          domain.DeliveryPending,
				MaxAttempts:     3,
				NextAttemptAt:   now,
				CreatedAt:       now,
				UpdatedAt:       now,
			}},
		},
	})
	if err != nil {
		t.Fatalf("seed ingest: %v", err)
	}

	service := NewService(store, cfg, clock.Real{}, observability.NewLogger(cfg.Logging))
	claimed, err := service.ProcessOnce(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("process once: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("expected 1 claimed delivery, got %d", claimed)
	}
}

func deliveryTestConfig(t *testing.T, slackURL string) config.Config {
	t.Helper()
	return config.Config{
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
			Jitter:      0,
		},
		Destinations: map[string]config.DestinationConfig{
			"slack-deployments": {
				Type:        domain.DestinationSlack,
				ResolvedURL: slackURL,
				Profile:     "detailed",
			},
		},
	}
}

func deliveryTelegramTestConfig(t *testing.T, apiBaseURL string) config.Config {
	t.Helper()
	return config.Config{
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
			Jitter:      0,
		},
		Destinations: map[string]config.DestinationConfig{
			"telegram-bot": {
				Type:          domain.DestinationTelegram,
				BotTokenEnv:   "TELEGRAM_ONCALL_BOT_TOKEN",
				ResolvedToken: "test-token",
				ChatID:        "-100123456789",
				APIBaseURL:    apiBaseURL,
				Profile:       "compact",
			},
		},
	}
}
