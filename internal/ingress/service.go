package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/id"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage"
)

type Service struct {
	store        storage.Store
	cfg          config.Config
	adapters     *Registry
	router       *routing.Engine
	clock        clock.Clock
	logger       *slog.Logger
	integrations *runtimeconfig.IntegrationRegistry
	destinations *runtimeconfig.DestinationRegistry
}

func NewService(store storage.Store, cfg config.Config, adapters *Registry, router *routing.Engine, integrations *runtimeconfig.IntegrationRegistry, destinations *runtimeconfig.DestinationRegistry, clk clock.Clock, logger *slog.Logger) *Service {
	return &Service{
		store:        store,
		cfg:          cfg,
		adapters:     adapters,
		router:       router,
		clock:        clk,
		logger:       logger,
		integrations: integrations,
		destinations: destinations,
	}
}

func (s *Service) Handle(ctx context.Context, source domain.Source, req InboundRequest) (domain.IngestResult, error) {
	integration, ok := s.integrations.Get(req.IntegrationID)
	if !ok {
		return domain.IngestResult{}, ErrUnknownIntegration
	}
	if integration.Source != source {
		return domain.IngestResult{}, ErrWrongSource
	}

	adapter, ok := s.adapters.Get(source)
	if !ok {
		return domain.IngestResult{}, fmt.Errorf("no adapter for source %s", source)
	}
	if err := adapter.Verify(ctx, integration, req); err != nil {
		return domain.IngestResult{}, err
	}

	normalized, err := adapter.Normalize(ctx, req.IntegrationID, integration, req)
	if err != nil {
		return domain.IngestResult{}, err
	}

	now := s.clock.Now()
	payloadHash := sha256.Sum256(req.RawBody)
	headersJSON, _ := json.Marshal(sanitizedHeaders(source, req.Headers))

	receiptID := id.New(now)
	receipt := domain.Receipt{
		ID:               receiptID,
		Source:           source,
		IntegrationID:    req.IntegrationID,
		SourceDeliveryID: normalized.SourceDeliveryID,
		SourceEventType:  normalized.SourceEventType,
		PayloadSHA256:    hex.EncodeToString(payloadHash[:]),
		RawPayload:       req.RawBody,
		HeadersJSON:      headersJSON,
		ReceivedAt:       req.ReceivedAt.UTC(),
		Status:           domain.ReceiptAccepted,
		IgnoreReason:     normalized.IgnoreReason,
		CreatedAt:        now,
	}

	if len(normalized.Events) == 0 {
		receipt.Status = domain.ReceiptIgnored
		return s.store.Ingest(ctx, domain.IngestBatch{Receipt: receipt})
	}

	batch := domain.IngestBatch{
		Receipt:         receipt,
		Events:          make([]domain.Event, 0, len(normalized.Events)),
		DeliveryByEvent: make(map[string][]domain.Delivery, len(normalized.Events)),
	}

	totalMatches := 0
	for _, candidate := range normalized.Events {
		event := domain.Event{
			ID:            id.New(now),
			ReceiptID:     receiptID,
			EventEnvelope: candidate.EventEnvelope,
			CreatedAt:     now,
		}

		matches := s.router.Destinations(event)
		matchedRouteIDs := make([]string, 0, len(matches))
		matchedDestinationIDs := make([]string, 0, len(matches))
		deliveries := make([]domain.Delivery, 0, len(matches))
		for _, match := range matches {
			destination, ok := s.destinations.Get(match.DestinationID)
			if !ok {
				continue
			}
			matchedRouteIDs = append(matchedRouteIDs, match.RouteID)
			matchedDestinationIDs = append(matchedDestinationIDs, match.DestinationID)
			deliveries = append(deliveries, domain.Delivery{
				ID:              id.New(now),
				EventID:         event.ID,
				DestinationID:   match.DestinationID,
				DestinationType: destination.Type,
				Status:          domain.DeliveryPending,
				MaxAttempts:     s.cfg.Retry.MaxAttempts,
				NextAttemptAt:   now,
				CreatedAt:       now,
				UpdatedAt:       now,
			})
		}
		traceJSON, _ := json.Marshal(domain.RouteTrace{
			RouteMatchCount: len(matches),
			RouteIDs:        matchedRouteIDs,
			DestinationIDs:  matchedDestinationIDs,
		})
		event.RouteTraceJSON = traceJSON
		totalMatches += len(deliveries)
		batch.Events = append(batch.Events, event)
		batch.DeliveryByEvent[event.ID] = deliveries

		eventFields := []any{
			"source", event.Source,
			"event_id", event.ID,
			"event_key", event.Key,
			"severity", event.Severity,
			"route_match_count", len(matches),
			"route_ids", matchedRouteIDs,
			"destination_ids", matchedDestinationIDs,
			"delivery_count", len(deliveries),
		}
		if len(matches) == 0 && s.logger != nil {
			s.logger.Info("webhook.route_unmatched", eventFields...)
		} else if s.logger != nil {
			s.logger.Info("webhook.route_matched", eventFields...)
		}
	}

	if totalMatches == 0 {
		receipt.Status = domain.ReceiptUnrouted
		batch.Receipt = receipt
	}

	result, err := s.store.Ingest(ctx, batch)
	if err == nil && s.logger != nil {
		fields := []any{
			"source", source,
			"integration_id", req.IntegrationID,
			"receipt_id", result.ReceiptID,
			"event_count", result.EventCount,
			"delivery_count", result.DeliveryCount,
			"status", result.Status,
		}
		if receipt.IgnoreReason != "" {
			fields = append(fields, "ignore_reason", receipt.IgnoreReason)
		}
		switch {
		case result.Duplicate:
			s.logger.Info("webhook.duplicate", fields...)
		case result.Status == domain.ReceiptIgnored:
			s.logger.Info("webhook.ignored", fields...)
		case result.Status == domain.ReceiptUnrouted:
			s.logger.Info("webhook.unrouted", fields...)
		default:
			s.logger.Info("webhook.accepted", fields...)
		}
	}
	return result, err
}

func sanitizedHeaders(source domain.Source, headers http.Header) map[string]string {
	out := map[string]string{
		"Content-Type": headers.Get("Content-Type"),
	}

	switch source {
	case domain.SourceWatcher:
		out["webhook-id"] = headers.Get("webhook-id")
		out["webhook-timestamp"] = headers.Get("webhook-timestamp")
		out["X-Watcher-Event"] = headers.Get("X-Watcher-Event")
		out["X-Watcher-Delivery-ID"] = headers.Get("X-Watcher-Delivery-ID")
		out["X-Watcher-Event-ID"] = headers.Get("X-Watcher-Event-ID")
	case domain.SourceGitHub:
		out["X-GitHub-Event"] = headers.Get("X-GitHub-Event")
		out["X-GitHub-Delivery"] = headers.Get("X-GitHub-Delivery")
	}

	return out
}
