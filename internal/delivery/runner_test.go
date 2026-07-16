package delivery

import (
	"context"
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

func TestRunnerProcessesPendingDelivery(t *testing.T) {
	delivered := make(chan struct{}, 1)
	slack := newLoopbackTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestRunnerStartsConfiguredSchedulerConcurrency(t *testing.T) {
	store := &observingRunnerStore{claims: make(chan string, 8)}
	cfg := config.Config{Workers: config.WorkersConfig{
		BatchSize:        1,
		Concurrency:      3,
		PollInterval:     time.Hour,
		LeaseDuration:    time.Minute,
		RecoveryInterval: time.Hour,
	}}
	service := NewService(store, cfg, runtimeconfig.NewDestinationRegistry(nil), runtimeconfig.NewRendererProfileRegistry(nil), clock.Real{}, nil)
	runner := NewRunner(service)
	ctx, cancel := context.WithCancel(context.Background())
	runner.Start(ctx, "worker")

	workerIDs := make(map[string]struct{}, cfg.Workers.Concurrency)
	deadline := time.After(time.Second)
	for len(workerIDs) < cfg.Workers.Concurrency {
		select {
		case workerID := <-store.claims:
			workerIDs[workerID] = struct{}{}
		case <-deadline:
			cancel()
			runner.Wait()
			t.Fatalf("started %d distinct schedulers, want %d: %+v", len(workerIDs), cfg.Workers.Concurrency, workerIDs)
		}
	}
	cancel()
	runner.Wait()

	for _, want := range []string{"worker-1", "worker-2", "worker-3"} {
		if _, ok := workerIDs[want]; !ok {
			t.Fatalf("scheduler %q was not started: %+v", want, workerIDs)
		}
	}
}

type observingRunnerStore struct {
	claims chan string
}

func (s *observingRunnerStore) ClaimDueDeliveries(_ context.Context, claim domain.ClaimRequest) ([]domain.DeliveryEnvelope, error) {
	s.claims <- claim.WorkerID
	return nil, nil
}

func (*observingRunnerStore) CompleteAttempt(context.Context, domain.AttemptResult) error {
	return nil
}

func (*observingRunnerStore) RecoverExpiredLeases(context.Context, time.Time) (int64, error) {
	return 0, nil
}
