package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/domain"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func (s *Store) Ingest(ctx context.Context, batch domain.IngestBatch) (domain.IngestResult, error) {
	if err := validateIngestBatch(batch); err != nil {
		return domain.IngestResult{}, err
	}

	var result domain.IngestResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rm := toReceiptModel(batch.Receipt)
		if err := tx.Create(&rm).Error; err != nil {
			if isUniqueConstraint(err) {
				var existing receiptModel
				if lookupErr := tx.Where("source = ? AND integration_id = ? AND source_delivery_id = ?", rm.Source, rm.IntegrationID, rm.SourceDeliveryID).First(&existing).Error; lookupErr != nil {
					return lookupErr
				}
				result = domain.IngestResult{
					ReceiptID: existing.ID,
					Duplicate: true,
					Status:    domain.ReceiptDuplicate,
				}
				return nil
			}
			return err
		}

		deliveryCount := 0
		for _, event := range batch.Events {
			em := toEventModel(event)
			if err := tx.Create(&em).Error; err != nil {
				return err
			}
			for _, delivery := range batch.DeliveryByEvent[event.ID] {
				dm := toDeliveryModel(delivery)
				if err := tx.Create(&dm).Error; err != nil {
					return err
				}
				deliveryCount++
			}
		}

		result = domain.IngestResult{
			ReceiptID:     batch.Receipt.ID,
			Duplicate:     false,
			Status:        batch.Receipt.Status,
			EventCount:    len(batch.Events),
			DeliveryCount: deliveryCount,
		}
		return nil
	})
	return result, err
}

func (s *Store) ListRoutes(ctx context.Context) ([]domain.Route, error) {
	var models []routeModel
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	routes := make([]domain.Route, 0, len(models))
	for _, model := range models {
		routes = append(routes, toDomainRoute(model))
	}
	return routes, nil
}

func (s *Store) GetRoute(ctx context.Context, id string) (domain.Route, error) {
	var model routeModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Route{}, err
	}
	return toDomainRoute(model), nil
}

func (s *Store) CreateRoute(ctx context.Context, route domain.Route) error {
	model := toRouteModel(route)
	return s.db.WithContext(ctx).Create(&model).Error
}

func (s *Store) UpdateRoute(ctx context.Context, route domain.Route) error {
	model := toRouteModel(route)
	return s.db.WithContext(ctx).Model(&routeModel{}).
		Where("id = ?", route.ID).
		Updates(map[string]any{
			"description":       model.Description,
			"match_json":        model.MatchJSON,
			"destinations_json": model.DestinationsJSON,
			"updated_at":        route.UpdatedAt,
		}).Error
}

func (s *Store) DeleteRoute(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&routeModel{}, "id = ?", id).Error
}

func (s *Store) GetReceipt(ctx context.Context, id string) (domain.Receipt, error) {
	var model receiptModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Receipt{}, err
	}
	return toDomainReceipt(model), nil
}

func (s *Store) ListReceipts(ctx context.Context, filter domain.ReceiptFilter) (domain.ReceiptPage, error) {
	query := s.db.WithContext(ctx).Model(&receiptModel{}).Order("created_at DESC").Order("id DESC")
	if filter.Status != "" {
		query = query.Where("status = ?", string(filter.Status))
	}
	if filter.Source != "" {
		query = query.Where("source = ?", string(filter.Source))
	}
	if filter.IntegrationID != "" {
		query = query.Where("integration_id = ?", filter.IntegrationID)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", filter.From.UTC())
	}
	if filter.To != nil {
		query = query.Where("created_at <= ?", filter.To.UTC())
	}
	if filter.Cursor != "" {
		var cursor receiptModel
		if err := s.db.WithContext(ctx).First(&cursor, "id = ?", filter.Cursor).Error; err != nil {
			return domain.ReceiptPage{}, err
		}
		query = query.Where("(created_at < ?) OR (created_at = ? AND id < ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	var models []receiptModel
	if err := query.Limit(limit + 1).Find(&models).Error; err != nil {
		return domain.ReceiptPage{}, err
	}

	page := domain.ReceiptPage{
		Items: make([]domain.Receipt, 0, minInt(len(models), limit)),
	}
	for i, model := range models {
		if i == limit {
			page.NextCursor = model.ID
			break
		}
		page.Items = append(page.Items, toDomainReceipt(model))
	}
	return page, nil
}

func (s *Store) GetEvent(ctx context.Context, id string) (domain.Event, error) {
	var model eventModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Event{}, err
	}
	return toDomainEvent(model), nil
}

func (s *Store) ListEventsByReceipt(ctx context.Context, receiptID string) ([]domain.Event, error) {
	var models []eventModel
	if err := s.db.WithContext(ctx).Order("created_at ASC").Find(&models, "receipt_id = ?", receiptID).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Event, 0, len(models))
	for _, model := range models {
		out = append(out, toDomainEvent(model))
	}
	return out, nil
}

func (s *Store) ListDeliveries(ctx context.Context, filter domain.DeliveryFilter) (domain.DeliveryPage, error) {
	query := s.db.WithContext(ctx).Model(&deliveryModel{}).Order("created_at DESC").Order("id DESC")

	if filter.Status != "" {
		query = query.Where("status = ?", string(filter.Status))
	}
	if filter.DestinationID != "" {
		query = query.Where("destination_id = ?", filter.DestinationID)
	}
	if filter.EventID != "" {
		query = query.Where("event_id = ?", filter.EventID)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", filter.From.UTC())
	}
	if filter.To != nil {
		query = query.Where("created_at <= ?", filter.To.UTC())
	}
	if filter.Cursor != "" {
		var cursor deliveryModel
		if err := s.db.WithContext(ctx).First(&cursor, "id = ?", filter.Cursor).Error; err != nil {
			return domain.DeliveryPage{}, err
		}
		query = query.Where("(created_at < ?) OR (created_at = ? AND id < ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	var models []deliveryModel
	if err := query.Limit(limit + 1).Find(&models).Error; err != nil {
		return domain.DeliveryPage{}, err
	}

	page := domain.DeliveryPage{
		Items: make([]domain.Delivery, 0, minInt(len(models), limit)),
	}
	for i, model := range models {
		if i == limit {
			page.NextCursor = model.ID
			break
		}
		page.Items = append(page.Items, toDomainDelivery(model))
	}
	return page, nil
}

func (s *Store) GetDelivery(ctx context.Context, id string) (domain.Delivery, error) {
	var model deliveryModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Delivery{}, err
	}
	return toDomainDelivery(model), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Store) ClaimDueDeliveries(ctx context.Context, claim domain.ClaimRequest) ([]domain.DeliveryEnvelope, error) {
	var claimed []deliveryModel
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("(status = ? OR status = ?) AND next_attempt_at <= ? AND (locked_until IS NULL OR locked_until < ?)", domain.DeliveryPending, domain.DeliveryRetryWait, claim.Now, claim.Now).
			Order("next_attempt_at ASC").
			Limit(claim.BatchSize).
			Find(&claimed).Error; err != nil {
			return err
		}

		lockedUntil := claim.Now.Add(claim.LeaseDuration)
		for i := range claimed {
			if err := tx.Model(&deliveryModel{}).
				Where("id = ?", claimed[i].ID).
				Updates(map[string]any{
					"status":       string(domain.DeliveryProcessing),
					"locked_by":    claim.WorkerID,
					"locked_until": &lockedUntil,
					"updated_at":   claim.Now,
				}).Error; err != nil {
				return err
			}
			claimed[i].Status = string(domain.DeliveryProcessing)
			claimed[i].LockedBy = claim.WorkerID
			claimed[i].LockedUntil = &lockedUntil
			claimed[i].UpdatedAt = claim.Now
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]domain.DeliveryEnvelope, 0, len(claimed))
	for _, delivery := range claimed {
		var event eventModel
		if err := s.db.WithContext(ctx).First(&event, "id = ?", delivery.EventID).Error; err != nil {
			return nil, err
		}
		out = append(out, domain.DeliveryEnvelope{
			Delivery: toDomainDelivery(delivery),
			Event:    toDomainEvent(event),
		})
	}
	return out, nil
}

func (s *Store) CompleteAttempt(ctx context.Context, result domain.AttemptResult) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var delivery deliveryModel
		if err := tx.First(&delivery, "id = ?", result.DeliveryID).Error; err != nil {
			return err
		}

		attempt := deliveryAttemptModel{
			ID:            result.DeliveryID + fmt.Sprintf("-%d", delivery.AttemptCount+1),
			DeliveryID:    result.DeliveryID,
			AttemptNumber: delivery.AttemptCount + 1,
			WorkerID:      result.WorkerID,
			StartedAt:     result.StartedAt,
			CompletedAt:   result.CompletedAt,
			Outcome:       result.Outcome,
			ResponseCode:  result.ResponseCode,
			ErrorCode:     result.ErrorCode,
			ErrorMessage:  result.ErrorMessage,
			DurationMS:    result.CompletedAt.Sub(result.StartedAt).Milliseconds(),
		}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}

		updates := map[string]any{
			"attempt_count":       delivery.AttemptCount + 1,
			"status":              string(result.NextStatus),
			"locked_by":           "",
			"locked_until":        nil,
			"provider_message_id": result.ProviderMessageID,
			"last_error_code":     result.ErrorCode,
			"last_error":          result.ErrorMessage,
			"updated_at":          result.CompletedAt,
		}
		if result.NextAttemptAt != nil {
			updates["next_attempt_at"] = *result.NextAttemptAt
		}
		if result.NextStatus == domain.DeliverySent {
			updates["sent_at"] = result.CompletedAt
		}

		return tx.Model(&deliveryModel{}).Where("id = ?", result.DeliveryID).Updates(updates).Error
	})
}

func (s *Store) RecoverExpiredLeases(ctx context.Context, now time.Time) (int64, error) {
	result := s.db.WithContext(ctx).Model(&deliveryModel{}).
		Where("status = ? AND locked_until IS NOT NULL AND locked_until < ?", domain.DeliveryProcessing, now).
		Updates(map[string]any{
			"status":          string(domain.DeliveryRetryWait),
			"locked_by":       "",
			"locked_until":    nil,
			"updated_at":      now,
			"next_attempt_at": now,
		})
	return result.RowsAffected, result.Error
}

func (s *Store) RetryDelivery(ctx context.Context, id string, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var delivery deliveryModel
		if err := tx.First(&delivery, "id = ?", id).Error; err != nil {
			return err
		}

		if delivery.Status != string(domain.DeliveryDeadLetter) && delivery.Status != string(domain.DeliveryRetryWait) {
			return fmt.Errorf("delivery %q is not retryable from status %q", id, delivery.Status)
		}

		return tx.Model(&deliveryModel{}).Where("id = ?", id).Updates(map[string]any{
			"status":          string(domain.DeliveryRetryWait),
			"next_attempt_at": now,
			"locked_by":       "",
			"locked_until":    nil,
			"updated_at":      now,
		}).Error
	})
}
