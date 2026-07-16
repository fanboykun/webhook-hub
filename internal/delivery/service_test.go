package delivery

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/observability"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
)

func TestProcessOnceSent(t *testing.T) {
	slack := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			ID:        "e1",
			ReceiptID: "r1",
			EventEnvelope: domain.EventEnvelope{
				Source:        domain.SourceWatcher,
				IntegrationID: "watcher-production",
				Key:           "watcher.deployment.failed",
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

	service := NewService(store, cfg, runtimeconfig.NewDestinationRegistry(cfg.Destinations), runtimeconfig.NewRendererProfileRegistry(runtimeconfig.RendererProfilesFromConfig(cfg.RendererProfiles)), clock.Real{}, observability.NewLogger(cfg.Logging))
	claimed, err := service.ProcessOnce(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("process once: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("expected 1 claimed delivery, got %d", claimed)
	}
}

func TestProcessOnceTelegramSent(t *testing.T) {
	telegramAPI := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			ID:        "e1",
			ReceiptID: "r1",
			EventEnvelope: domain.EventEnvelope{
				Source:        domain.SourceWatcher,
				IntegrationID: "watcher-production",
				Key:           "watcher.deployment.failed",
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

	service := NewService(store, cfg, runtimeconfig.NewDestinationRegistry(cfg.Destinations), runtimeconfig.NewRendererProfileRegistry(runtimeconfig.RendererProfilesFromConfig(cfg.RendererProfiles)), clock.Real{}, observability.NewLogger(cfg.Logging))
	claimed, err := service.ProcessOnce(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("process once: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("expected 1 claimed delivery, got %d", claimed)
	}
}

func TestProcessOnceTeamsSent(t *testing.T) {
	teams := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected teams content type: %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer teams.Close()

	cfg := deliveryTeamsTestConfig(t, teams.URL)
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
			ID:        "e1",
			ReceiptID: "r1",
			EventEnvelope: domain.EventEnvelope{
				Source:        domain.SourceWatcher,
				IntegrationID: "watcher-production",
				Key:           "watcher.deployment.failed",
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
		}},
		DeliveryByEvent: map[string][]domain.Delivery{
			"e1": {{
				ID:              "d1",
				EventID:         "e1",
				DestinationID:   "teams-oncall",
				DestinationType: domain.DestinationTeams,
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

	service := NewService(store, cfg, runtimeconfig.NewDestinationRegistry(cfg.Destinations), runtimeconfig.NewRendererProfileRegistry(runtimeconfig.RendererProfilesFromConfig(cfg.RendererProfiles)), clock.Real{}, observability.NewLogger(cfg.Logging))
	claimed, err := service.ProcessOnce(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("process once: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("expected 1 claimed delivery, got %d", claimed)
	}
}

func TestProcessOnceUsesHotReloadedDestinationRegistry(t *testing.T) {
	firstHits := make(chan struct{}, 1)
	first := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case firstHits <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer first.Close()

	secondHits := make(chan struct{}, 1)
	second := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case secondHits <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer second.Close()

	cfg := deliveryTestConfig(t, first.URL)
	store, err := sqlite.Open(cfg.Database, observability.NewLogger(cfg.Logging))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Now().UTC()
	_, err = store.Ingest(context.Background(), domain.IngestBatch{
		Receipt: domain.Receipt{
			ID:               "r-hot",
			Source:           domain.SourceWatcher,
			IntegrationID:    "watcher-production",
			SourceDeliveryID: "evt-hot",
			SourceEventType:  "deployment.failed",
			PayloadSHA256:    "abc",
			ReceivedAt:       now,
			Status:           domain.ReceiptAccepted,
			CreatedAt:        now,
		},
		Events: []domain.Event{{
			ID:        "e-hot",
			ReceiptID: "r-hot",
			EventEnvelope: domain.EventEnvelope{
				Source:        domain.SourceWatcher,
				IntegrationID: "watcher-production",
				Key:           "watcher.deployment.failed",
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
		}},
		DeliveryByEvent: map[string][]domain.Delivery{
			"e-hot": {{
				ID:              "d-hot",
				EventID:         "e-hot",
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

	registry := runtimeconfig.NewDestinationRegistry(cfg.Destinations)
	registry.Replace(map[string]config.DestinationConfig{
		"slack-deployments": {
			Type:        domain.DestinationSlack,
			ResolvedURL: second.URL,
		},
	})

	service := NewService(store, cfg, registry, runtimeconfig.NewRendererProfileRegistry(runtimeconfig.RendererProfilesFromConfig(cfg.RendererProfiles)), clock.Real{}, observability.NewLogger(cfg.Logging))
	if _, err := service.ProcessOnce(context.Background(), "worker-1"); err != nil {
		t.Fatalf("process once: %v", err)
	}

	select {
	case <-secondHits:
	default:
		t.Fatal("expected reloaded destination to receive delivery")
	}

	select {
	case <-firstHits:
		t.Fatal("expected original destination not to receive delivery after reload")
	default:
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
			},
		},
	}
}

type loopbackTestServer struct {
	URL    string
	server *http.Server
}

func newLoopbackTestServer(t *testing.T, handler http.Handler) *loopbackTestServer {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback listener unavailable in this environment: %v", err)
	}

	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()

	return &loopbackTestServer{
		URL:    fmt.Sprintf("http://%s", listener.Addr().String()),
		server: server,
	}
}

func (s *loopbackTestServer) Close() {
	_ = s.server.Shutdown(context.Background())
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
			},
		},
	}
}

func deliveryTeamsTestConfig(t *testing.T, webhookURL string) config.Config {
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
			"teams-oncall": {
				Type:        domain.DestinationTeams,
				ResolvedURL: webhookURL,
			},
		},
	}
}
