package delivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/observability"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
)

func TestRunnerProcessesPendingDelivery(t *testing.T) {
	delivered := make(chan struct{}, 1)
	slack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case delivered <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer slack.Close()

	cfg := deliveryTestConfig(t, slack.URL)
	cfg.Workers.PollInterval = 10 * time.Millisecond
	cfg.Workers.RecoveryInterval = 10 * time.Millisecond

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

	service := NewService(store, cfg, runtimeconfig.NewDestinationRegistry(cfg.Destinations), runtimeconfig.NewRendererProfileRegistry(cfg.RendererProfiles), clock.Real{}, observability.NewLogger(cfg.Logging))
	runner := NewRunner(service)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx, "worker-1")

	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runner delivery")
	}

	cancel()
	runner.Wait()
}
