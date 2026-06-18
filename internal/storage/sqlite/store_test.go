package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/observability"
)

func TestIngestDuplicateReceipt(t *testing.T) {
	t.Setenv("TZ", "UTC")
	store := openTestStore(t)
	now := time.Now().UTC()

	batch := domain.IngestBatch{
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
	}

	first, err := store.Ingest(context.Background(), batch)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	second, err := store.Ingest(context.Background(), batch)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if first.ReceiptID != second.ReceiptID || !second.Duplicate {
		t.Fatalf("expected duplicate receipt, got first=%+v second=%+v", first, second)
	}
}

func TestMigrationsRepeatable(t *testing.T) {
	path := t.TempDir() + "/gateway.db"
	logger := observability.NewLogger(config.LoggingConfig{Format: "text"})

	store1, err := Open(config.DatabaseConfig{
		Path:               path,
		BusyTimeout:        5 * time.Second,
		MaxOpenConnections: 1,
		RetainRawPayloads:  true,
	}, logger)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = store1.Close()

	store2, err := Open(config.DatabaseConfig{
		Path:               path,
		BusyTimeout:        5 * time.Second,
		MaxOpenConnections: 1,
		RetainRawPayloads:  true,
	}, logger)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	t.Cleanup(func() { _ = store2.Close() })
}

func TestRouteCRUD(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()

	created := domain.Route{
		ID:           "deployment-failed",
		Description:  "Send failed deployments to Slack",
		Match:        domain.RouteMatchCriteria{Types: []string{"watcher.deployment.failed"}},
		Destinations: []string{"slack-deployments"},
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := store.CreateRoute(context.Background(), created); err != nil {
		t.Fatalf("create route failed: %v", err)
	}

	listed, err := store.ListRoutes(context.Background())
	if err != nil {
		t.Fatalf("list routes failed: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("unexpected listed routes: %+v", listed)
	}

	got, err := store.GetRoute(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get route failed: %v", err)
	}
	if got.Description != created.Description {
		t.Fatalf("unexpected route description: %q", got.Description)
	}

	created.Description = "Send failed deployments to Telegram bot"
	created.Destinations = []string{"telegram-bot"}
	created.UpdatedAt = now.Add(time.Minute)
	if err := store.UpdateRoute(context.Background(), created); err != nil {
		t.Fatalf("update route failed: %v", err)
	}

	updated, err := store.GetRoute(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get updated route failed: %v", err)
	}
	if len(updated.Destinations) != 1 || updated.Destinations[0] != "telegram-bot" {
		t.Fatalf("unexpected updated route: %+v", updated)
	}

	if err := store.DeleteRoute(context.Background(), created.ID); err != nil {
		t.Fatalf("delete route failed: %v", err)
	}

	remaining, err := store.ListRoutes(context.Background())
	if err != nil {
		t.Fatalf("list remaining routes failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no routes, got %+v", remaining)
	}
}

func TestGetEventAndDeliveryAndRetry(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()

	batch := domain.IngestBatch{
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
				ID:            "e1",
				ReceiptID:     "r1",
				Source:        domain.SourceWatcher,
				IntegrationID: "watcher-production",
				Type:          "watcher.deployment.failed",
				Action:        "deployment.failed",
				Lifecycle:     domain.LifecycleFailed,
				Severity:      domain.SeverityError,
				Title:         "deployment failed",
				Summary:       "health check failed",
				Service:       "auth-service",
				Environment:   "production",
				OccurredAt:    now,
				CreatedAt:     now,
			},
		},
		DeliveryByEvent: map[string][]domain.Delivery{
			"e1": {
				{
					ID:              "d1",
					EventID:         "e1",
					DestinationID:   "slack-deployments",
					DestinationType: domain.DestinationSlack,
					Status:          domain.DeliveryDeadLetter,
					MaxAttempts:     3,
					NextAttemptAt:   now,
					CreatedAt:       now,
					UpdatedAt:       now,
				},
			},
		},
	}

	if _, err := store.Ingest(context.Background(), batch); err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	event, err := store.GetEvent(context.Background(), "e1")
	if err != nil {
		t.Fatalf("get event failed: %v", err)
	}
	if event.Type != "watcher.deployment.failed" {
		t.Fatalf("unexpected event type: %s", event.Type)
	}

	delivery, err := store.GetDelivery(context.Background(), "d1")
	if err != nil {
		t.Fatalf("get delivery failed: %v", err)
	}
	if delivery.Status != domain.DeliveryDeadLetter {
		t.Fatalf("unexpected delivery status: %s", delivery.Status)
	}

	retryAt := now.Add(time.Minute)
	if err := store.RetryDelivery(context.Background(), "d1", retryAt); err != nil {
		t.Fatalf("retry delivery failed: %v", err)
	}

	retried, err := store.GetDelivery(context.Background(), "d1")
	if err != nil {
		t.Fatalf("get retried delivery failed: %v", err)
	}
	if retried.Status != domain.DeliveryRetryWait {
		t.Fatalf("expected retry_wait status, got %s", retried.Status)
	}
	if !retried.NextAttemptAt.Equal(retryAt) {
		t.Fatalf("expected next attempt at %v, got %v", retryAt, retried.NextAttemptAt)
	}
}

func TestIngestBatchValidation(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()

	_, err := store.Ingest(context.Background(), domain.IngestBatch{
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
				ID:            "e1",
				ReceiptID:     "r1",
				Source:        domain.SourceWatcher,
				IntegrationID: "watcher-production",
				Type:          "watcher.deployment.failed",
				OccurredAt:    now,
				CreatedAt:     now,
			},
		},
		DeliveryByEvent: map[string][]domain.Delivery{
			"e1": {
				{
					ID:              "d1",
					EventID:         "wrong-event",
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
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := t.TempDir() + "/gateway.db"
	store, err := Open(config.DatabaseConfig{
		Path:               path,
		BusyTimeout:        5 * time.Second,
		MaxOpenConnections: 1,
		RetainRawPayloads:  true,
	}, observability.NewLogger(config.LoggingConfig{Format: "text"}))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
