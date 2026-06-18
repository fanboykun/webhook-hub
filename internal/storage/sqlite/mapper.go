package sqlite

import (
	"encoding/json"

	"github.com/iweka-dev/webhook-hub/internal/domain"
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
	return eventModel{
		ID:            in.ID,
		ReceiptID:     in.ReceiptID,
		Source:        string(in.Source),
		IntegrationID: in.IntegrationID,
		SourceEventID: in.SourceEventID,
		Type:          in.Type,
		Action:        in.Action,
		Lifecycle:     string(in.Lifecycle),
		Severity:      string(in.Severity),
		Title:         in.Title,
		Summary:       in.Summary,
		Service:       in.Service,
		Environment:   in.Environment,
		Release:       in.Release,
		CommitSHA:     in.CommitSHA,
		Actor:         in.Actor,
		Fingerprint:   in.Fingerprint,
		GroupKey:      in.GroupKey,
		URL:           in.URL,
		OccurredAt:    in.OccurredAt,
		LabelsJSON:    in.LabelsJSON,
		FieldsJSON:    in.FieldsJSON,
		CreatedAt:     in.CreatedAt,
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
		ID:            in.ID,
		ReceiptID:     in.ReceiptID,
		Source:        domain.Source(in.Source),
		IntegrationID: in.IntegrationID,
		SourceEventID: in.SourceEventID,
		Type:          in.Type,
		Action:        in.Action,
		Lifecycle:     domain.Lifecycle(in.Lifecycle),
		Severity:      domain.Severity(in.Severity),
		Title:         in.Title,
		Summary:       in.Summary,
		Service:       in.Service,
		Environment:   in.Environment,
		Release:       in.Release,
		CommitSHA:     in.CommitSHA,
		Actor:         in.Actor,
		Fingerprint:   in.Fingerprint,
		GroupKey:      in.GroupKey,
		URL:           in.URL,
		OccurredAt:    in.OccurredAt,
		LabelsJSON:    in.LabelsJSON,
		FieldsJSON:    in.FieldsJSON,
		CreatedAt:     in.CreatedAt,
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

func toRouteModel(in domain.Route) routeModel {
	matchJSON, _ := json.Marshal(in.Match)
	destinationsJSON, _ := json.Marshal(in.Destinations)
	return routeModel{
		ID:               in.ID,
		Description:      in.Description,
		MatchJSON:        matchJSON,
		DestinationsJSON: destinationsJSON,
	}
}

func toDomainRoute(in routeModel) domain.Route {
	var match domain.RouteMatchCriteria
	_ = json.Unmarshal(in.MatchJSON, &match)

	var destinations []string
	_ = json.Unmarshal(in.DestinationsJSON, &destinations)

	return domain.Route{
		ID:           in.ID,
		Description:  in.Description,
		Match:        match,
		Destinations: destinations,
		CreatedAt:    in.CreatedAt,
		UpdatedAt:    in.UpdatedAt,
	}
}
