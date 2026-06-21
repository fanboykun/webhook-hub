package sqlite

import (
	"encoding/json"
	"fmt"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

func toReceiptModel(in domain.Receipt) receiptModel {
	return receiptModel{
		ID:               in.ID,
		Source:           string(in.Source),
		IntegrationID:    in.IntegrationID,
		SourceDeliveryID: in.SourceDeliveryID,
		SourceEventType:  in.SourceEventType,
		PayloadSHA256:    in.PayloadSHA256,
		RawPayload:       in.RawPayload,
		HeadersJSON:      in.HeadersJSON,
		ReceivedAt:       in.ReceivedAt,
		Status:           string(in.Status),
		IgnoreReason:     in.IgnoreReason,
		CreatedAt:        in.CreatedAt,
	}
}

func toEventModel(in domain.Event) eventModel {
	payloadVersion := in.PayloadVersion
	if payloadVersion <= 0 {
		payloadVersion = 1
	}
	return eventModel{
		ID:               in.ID,
		ReceiptID:        in.ReceiptID,
		Source:           string(in.Source),
		IntegrationID:    in.IntegrationID,
		SourceEventID:    in.SourceEventID,
		Key:              in.Key,
		Action:           in.Action,
		Lifecycle:        string(in.Lifecycle),
		Severity:         string(in.Severity),
		Title:            in.Title,
		Summary:          in.Summary,
		ScopeService:     in.Scope.Service,
		ScopeEnvironment: in.Scope.Environment,
		LegacyRelease:    legacyMetadataString(in.MetadataJSON, "release"),
		LegacyCommitSHA:  legacyMetadataString(in.MetadataJSON, "commit_sha"),
		LegacyActor:      legacyMetadataString(in.MetadataJSON, "actor"),
		Fingerprint:      in.Fingerprint,
		GroupKey:         in.GroupKey,
		SourceURL:        in.SourceURL,
		OccurredAt:       in.OccurredAt,
		LabelsJSON:       in.LabelsJSON,
		MetadataJSON:     in.MetadataJSON,
		RouteTraceJSON:   in.RouteTraceJSON,
		PayloadVersion:   payloadVersion,
		PayloadJSON:      in.PayloadJSON,
		CreatedAt:        in.CreatedAt,
	}
}

func toDeliveryModel(in domain.Delivery) deliveryModel {
	return deliveryModel{
		ID:              in.ID,
		EventID:         in.EventID,
		DestinationID:   in.DestinationID,
		DestinationType: string(in.DestinationType),
		Status:          string(in.Status),
		AttemptCount:    in.AttemptCount,
		MaxAttempts:     in.MaxAttempts,
		NextAttemptAt:   in.NextAttemptAt,
		LockedBy:        in.LockedBy,
		LockedUntil:     in.LockedUntil,
		CreatedAt:       in.CreatedAt,
		UpdatedAt:       in.UpdatedAt,
	}
}

func toDomainEvent(in eventModel) domain.Event {
	return domain.Event{
		ID:        in.ID,
		ReceiptID: in.ReceiptID,
		EventEnvelope: domain.EventEnvelope{
			Source:        domain.Source(in.Source),
			IntegrationID: in.IntegrationID,
			SourceEventID: in.SourceEventID,
			Key:           in.Key,
			Action:        in.Action,
			Lifecycle:     domain.Lifecycle(in.Lifecycle),
			Severity:      domain.Severity(in.Severity),
			Title:         in.Title,
			Summary:       in.Summary,
			Scope: domain.EventScope{
				Service:     in.ScopeService,
				Environment: in.ScopeEnvironment,
			},
			Fingerprint:    in.Fingerprint,
			GroupKey:       in.GroupKey,
			SourceURL:      in.SourceURL,
			OccurredAt:     in.OccurredAt,
			LabelsJSON:     in.LabelsJSON,
			MetadataJSON:   in.MetadataJSON,
			PayloadVersion: in.PayloadVersion,
			PayloadJSON:    in.PayloadJSON,
		},
		RouteTraceJSON: in.RouteTraceJSON,
		CreatedAt:      in.CreatedAt,
	}
}

func toDomainReceipt(in receiptModel) domain.Receipt {
	return domain.Receipt{
		ID:               in.ID,
		Source:           domain.Source(in.Source),
		IntegrationID:    in.IntegrationID,
		SourceDeliveryID: in.SourceDeliveryID,
		SourceEventType:  in.SourceEventType,
		PayloadSHA256:    in.PayloadSHA256,
		RawPayload:       in.RawPayload,
		HeadersJSON:      in.HeadersJSON,
		ReceivedAt:       in.ReceivedAt,
		Status:           domain.ReceiptStatus(in.Status),
		IgnoreReason:     in.IgnoreReason,
		CreatedAt:        in.CreatedAt,
	}
}

func toDomainDelivery(in deliveryModel) domain.Delivery {
	return domain.Delivery{
		ID:                in.ID,
		EventID:           in.EventID,
		DestinationID:     in.DestinationID,
		DestinationType:   domain.DestinationType(in.DestinationType),
		Status:            domain.DeliveryStatus(in.Status),
		AttemptCount:      in.AttemptCount,
		MaxAttempts:       in.MaxAttempts,
		NextAttemptAt:     in.NextAttemptAt,
		LockedBy:          in.LockedBy,
		LockedUntil:       in.LockedUntil,
		ProviderMessageID: in.ProviderMessageID,
		LastErrorCode:     in.LastErrorCode,
		LastError:         in.LastError,
		SentAt:            in.SentAt,
		CreatedAt:         in.CreatedAt,
		UpdatedAt:         in.UpdatedAt,
	}
}

func toRouteModel(in domain.Route) (routeModel, error) {
	matchJSON, err := json.Marshal(in.Match)
	if err != nil {
		return routeModel{}, fmt.Errorf("marshal route match: %w", err)
	}
	destinationsJSON, err := json.Marshal(in.Destinations)
	if err != nil {
		return routeModel{}, fmt.Errorf("marshal route destinations: %w", err)
	}
	return routeModel{
		ID:               in.ID,
		Description:      in.Description,
		MatchJSON:        matchJSON,
		DestinationsJSON: destinationsJSON,
	}, nil
}

func toDomainRoute(in routeModel) (domain.Route, error) {
	var match domain.RouteMatchCriteria
	if err := json.Unmarshal(in.MatchJSON, &match); err != nil {
		return domain.Route{}, fmt.Errorf("unmarshal route match: %w", err)
	}

	var destinations []string
	if err := json.Unmarshal(in.DestinationsJSON, &destinations); err != nil {
		return domain.Route{}, fmt.Errorf("unmarshal route destinations: %w", err)
	}

	return domain.Route{
		ID:           in.ID,
		Description:  in.Description,
		Match:        match,
		Destinations: destinations,
		CreatedAt:    in.CreatedAt,
		UpdatedAt:    in.UpdatedAt,
	}, nil
}

func legacyMetadataString(metadataJSON []byte, key string) string {
	if len(metadataJSON) == 0 {
		return ""
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}
