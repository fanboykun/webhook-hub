package sqlite

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/config"
	configcrypto "github.com/fanboykun/webhook-hub/internal/config/crypto"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/observability"
	"github.com/fanboykun/webhook-hub/internal/storage"
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

func TestRendererProfileCRUD(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()

	profile := domain.ManagedRendererProfile{
		ID: "detailed",
		Profile: domain.RendererProfile{
			"watcher.deployment.failed": {
				Slack: &domain.SlackTemplate{
					Title: "{{.Title}}",
					Body:  "{{.Summary}}",
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.CreateRendererProfile(context.Background(), profile); err != nil {
		t.Fatalf("create renderer profile failed: %v", err)
	}

	listed, err := store.ListRendererProfiles(context.Background())
	if err != nil {
		t.Fatalf("list renderer profiles failed: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != profile.ID {
		t.Fatalf("unexpected listed renderer profiles: %+v", listed)
	}

	got, err := store.GetRendererProfile(context.Background(), profile.ID)
	if err != nil {
		t.Fatalf("get renderer profile failed: %v", err)
	}
	if got.Profile["watcher.deployment.failed"].Slack == nil {
		t.Fatalf("expected slack template in stored profile, got %+v", got.Profile)
	}

	profile.Profile["watcher.deployment.failed"] = domain.RendererDestinationTemplates{
		Telegram: &domain.TelegramTemplate{Text: "<b>{{.Title}}</b>"},
	}
	profile.UpdatedAt = now.Add(time.Minute)
	if err := store.UpdateRendererProfile(context.Background(), profile); err != nil {
		t.Fatalf("update renderer profile failed: %v", err)
	}

	updated, err := store.GetRendererProfile(context.Background(), profile.ID)
	if err != nil {
		t.Fatalf("get updated renderer profile failed: %v", err)
	}
	if updated.Profile["watcher.deployment.failed"].Telegram == nil {
		t.Fatalf("expected telegram template after update, got %+v", updated.Profile)
	}

	if err := store.DeleteRendererProfile(context.Background(), profile.ID); err != nil {
		t.Fatalf("delete renderer profile failed: %v", err)
	}

	remaining, err := store.ListRendererProfiles(context.Background())
	if err != nil {
		t.Fatalf("list remaining renderer profiles failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no renderer profiles, got %+v", remaining)
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

func TestIngestBatchValidationRejectsDuplicateEventIDs(t *testing.T) {
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
			{ID: "e1", ReceiptID: "r1", Source: domain.SourceWatcher, Type: "watcher.deployment.failed", OccurredAt: now, CreatedAt: now},
			{ID: "e1", ReceiptID: "r1", Source: domain.SourceWatcher, Type: "watcher.deployment.failed", OccurredAt: now, CreatedAt: now},
		},
	})
	if err == nil || err.Error() != `duplicate event id "e1" in ingest batch` {
		t.Fatalf("expected duplicate event id error, got %v", err)
	}
}

func TestIngestBatchValidationRejectsDuplicateDeliveryTargets(t *testing.T) {
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
			{ID: "e1", ReceiptID: "r1", Source: domain.SourceWatcher, Type: "watcher.deployment.failed", OccurredAt: now, CreatedAt: now},
		},
		DeliveryByEvent: map[string][]domain.Delivery{
			"e1": {
				{ID: "d1", EventID: "e1", DestinationID: "slack-deployments", DestinationType: domain.DestinationSlack, Status: domain.DeliveryPending, MaxAttempts: 3, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now},
				{ID: "d2", EventID: "e1", DestinationID: "slack-deployments", DestinationType: domain.DestinationSlack, Status: domain.DeliveryPending, MaxAttempts: 3, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now},
			},
		},
	})
	if err == nil || err.Error() != `duplicate delivery target "slack-deployments" for event "e1"` {
		t.Fatalf("expected duplicate delivery target error, got %v", err)
	}
}

func TestClaimDueDeliveriesSkipsRowsThatLoseEligibility(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()
	if err := seedDeliveryForClaimTests(store, now, domain.DeliveryPending); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}

	var model deliveryModel
	if err := store.db.WithContext(context.Background()).First(&model, "id = ?", "d1").Error; err != nil {
		t.Fatalf("load seeded delivery: %v", err)
	}
	if err := store.db.WithContext(context.Background()).Model(&deliveryModel{}).
		Where("id = ?", model.ID).
		Updates(map[string]any{
			"status":       string(domain.DeliveryProcessing),
			"locked_by":    "other-worker",
			"locked_until": now.Add(time.Minute),
			"updated_at":   now,
		}).Error; err != nil {
		t.Fatalf("update delivery before claim: %v", err)
	}

	claimed, err := store.ClaimDueDeliveries(context.Background(), domain.ClaimRequest{
		WorkerID:      "worker-1",
		BatchSize:     10,
		LeaseDuration: time.Minute,
		Now:           now,
	})
	if err != nil {
		t.Fatalf("claim deliveries: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("expected no claimed deliveries, got %+v", claimed)
	}
}

func TestCompleteAttemptReturnsLeaseLostWhenWorkerNoLongerOwnsDelivery(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()
	if err := seedDeliveryForClaimTests(store, now, domain.DeliveryProcessing); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}

	if err := store.db.WithContext(context.Background()).Model(&deliveryModel{}).
		Where("id = ?", "d1").
		Updates(map[string]any{
			"locked_by":    "other-worker",
			"locked_until": now.Add(time.Minute),
			"updated_at":   now,
		}).Error; err != nil {
		t.Fatalf("reassign lease: %v", err)
	}

	err := store.CompleteAttempt(context.Background(), domain.AttemptResult{
		DeliveryID:   "d1",
		WorkerID:     "worker-1",
		StartedAt:    now,
		CompletedAt:  now.Add(time.Second),
		Outcome:      "sent",
		NextStatus:   domain.DeliverySent,
		ResponseCode: 200,
	})
	if !errors.Is(err, storage.ErrDeliveryLeaseLost) {
		t.Fatalf("expected lease lost error, got %v", err)
	}

	delivery, getErr := store.GetDelivery(context.Background(), "d1")
	if getErr != nil {
		t.Fatalf("get delivery: %v", getErr)
	}
	if delivery.Status != domain.DeliveryProcessing {
		t.Fatalf("expected delivery to remain processing, got %s", delivery.Status)
	}
	if delivery.LockedBy != "other-worker" {
		t.Fatalf("expected lease owner to remain other-worker, got %q", delivery.LockedBy)
	}
}

func TestListDeliveriesByEventIDsReturnsAllDeliveries(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()
	if err := seedReceiptWithManyDeliveries(store, now, 105); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}

	deliveriesByEvent, err := store.ListDeliveriesByEventIDs(context.Background(), []string{"e-many"})
	if err != nil {
		t.Fatalf("list deliveries by event ids: %v", err)
	}
	if got := len(deliveriesByEvent["e-many"]); got != 105 {
		t.Fatalf("expected 105 deliveries, got %d", got)
	}
}

func TestDynamicConfigCRUD(t *testing.T) {
	store := openEncryptedTestStore(t)
	now := time.Now().UTC()

	integration := domain.ManagedIntegration{
		ID:           "github-main",
		Source:       domain.SourceGitHub,
		Secret:       "github-secret",
		ReplayWindow: 5 * time.Minute,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.CreateIntegration(context.Background(), integration); err != nil {
		t.Fatalf("create integration failed: %v", err)
	}

	gotIntegration, err := store.GetIntegration(context.Background(), integration.ID)
	if err != nil {
		t.Fatalf("get integration failed: %v", err)
	}
	if gotIntegration.Secret != integration.Secret {
		t.Fatalf("integration secret mismatch: got %q", gotIntegration.Secret)
	}

	destination := domain.ManagedDestination{
		ID:         "slack-deployments",
		Type:       domain.DestinationSlack,
		WebhookURL: "https://hooks.slack.test/services/abc",
		Profile:    "detailed",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := store.CreateDestination(context.Background(), destination); err != nil {
		t.Fatalf("create destination failed: %v", err)
	}

	gotDestination, err := store.GetDestination(context.Background(), destination.ID)
	if err != nil {
		t.Fatalf("get destination failed: %v", err)
	}
	if gotDestination.WebhookURL != destination.WebhookURL {
		t.Fatalf("destination webhook mismatch: got %q", gotDestination.WebhookURL)
	}

	var rawIntegration integrationModel
	if err := store.db.WithContext(context.Background()).First(&rawIntegration, "id = ?", integration.ID).Error; err != nil {
		t.Fatalf("load raw integration row failed: %v", err)
	}
	if string(rawIntegration.ConfigCiphertext) == integration.Secret {
		t.Fatal("expected encrypted integration secret at rest")
	}

	var rawDestination destinationModel
	if err := store.db.WithContext(context.Background()).First(&rawDestination, "id = ?", destination.ID).Error; err != nil {
		t.Fatalf("load raw destination row failed: %v", err)
	}
	if string(rawDestination.ConfigCiphertext) == destination.WebhookURL {
		t.Fatal("expected encrypted destination secret at rest")
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

func openEncryptedTestStore(t *testing.T) *Store {
	t.Helper()
	path := t.TempDir() + "/gateway.db"
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	cipher, err := configcrypto.NewFromString(hex.EncodeToString(key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	store, err := OpenWithCipher(config.DatabaseConfig{
		Path:               path,
		BusyTimeout:        5 * time.Second,
		MaxOpenConnections: 1,
		RetainRawPayloads:  true,
	}, observability.NewLogger(config.LoggingConfig{Format: "text"}), cipher)
	if err != nil {
		t.Fatalf("open encrypted store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func seedDeliveryForClaimTests(store *Store, now time.Time, status domain.DeliveryStatus) error {
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
					Status:          status,
					MaxAttempts:     3,
					NextAttemptAt:   now,
					CreatedAt:       now,
					UpdatedAt:       now,
				},
			},
		},
	})
	return err
}

func seedReceiptWithManyDeliveries(store *Store, now time.Time, count int) error {
	deliveries := make([]domain.Delivery, 0, count)
	for i := 0; i < count; i++ {
		deliveries = append(deliveries, domain.Delivery{
			ID:              "d" + hex.EncodeToString([]byte{byte(i / 16), byte(i % 16)}),
			EventID:         "e-many",
			DestinationID:   "dest-" + hex.EncodeToString([]byte{byte(i / 16), byte(i % 16)}),
			DestinationType: domain.DestinationSlack,
			Status:          domain.DeliveryPending,
			MaxAttempts:     3,
			NextAttemptAt:   now,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}

	_, err := store.Ingest(context.Background(), domain.IngestBatch{
		Receipt: domain.Receipt{
			ID:               "r-many",
			Source:           domain.SourceWatcher,
			IntegrationID:    "watcher-production",
			SourceDeliveryID: "event-many",
			SourceEventType:  "deployment.failed",
			PayloadSHA256:    "abc",
			ReceivedAt:       now,
			Status:           domain.ReceiptAccepted,
			CreatedAt:        now,
		},
		Events: []domain.Event{
			{
				ID:            "e-many",
				ReceiptID:     "r-many",
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
			"e-many": deliveries,
		},
	})
	return err
}
