package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	configcrypto "github.com/fanboykun/webhook-hub/internal/config/crypto"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/storage"
	"gorm.io/gorm"
)

type Store struct {
	db     *gorm.DB
	cipher *configcrypto.Cipher
}

var ErrEncryptionUnavailable = errors.New("dynamic config encryption is not configured")

func (s *Store) SeedDynamicConfig(ctx context.Context, integrations []domain.ManagedIntegration, destinations []domain.ManagedDestination, profiles []domain.ManagedRendererProfile, routes []domain.Route) error {
	integrationModels := make([]integrationModel, 0, len(integrations))
	for _, integration := range integrations {
		model, err := s.toIntegrationModel(integration)
		if err != nil {
			return err
		}
		integrationModels = append(integrationModels, model)
	}
	destinationModels := make([]destinationModel, 0, len(destinations))
	for _, destination := range destinations {
		model, err := s.toDestinationModel(destination)
		if err != nil {
			return err
		}
		destinationModels = append(destinationModels, model)
	}
	profileModels := make([]rendererProfileModel, 0, len(profiles))
	for _, profile := range profiles {
		model, err := s.toRendererProfileModel(profile)
		if err != nil {
			return err
		}
		profileModels = append(profileModels, model)
	}
	routeModels := make([]routeModel, 0, len(routes))
	for _, route := range routes {
		model, err := toRouteModel(route)
		if err != nil {
			return err
		}
		routeModels = append(routeModels, model)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := seedModelsIfEmpty(tx, &integrationModel{}, integrationModels); err != nil {
			return err
		}
		if err := seedModelsIfEmpty(tx, &destinationModel{}, destinationModels); err != nil {
			return err
		}
		if err := seedModelsIfEmpty(tx, &rendererProfileModel{}, profileModels); err != nil {
			return err
		}
		return seedModelsIfEmpty(tx, &routeModel{}, routeModels)
	})
}

func seedModelsIfEmpty[T any](tx *gorm.DB, model *T, seeds []T) error {
	var count int64
	if err := tx.Model(model).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 || len(seeds) == 0 {
		return nil
	}
	return tx.Create(&seeds).Error
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

func (s *Store) ListIntegrations(ctx context.Context) ([]domain.ManagedIntegration, error) {
	if s.cipher == nil {
		return nil, ErrEncryptionUnavailable
	}
	var models []integrationModel
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]domain.ManagedIntegration, 0, len(models))
	for _, model := range models {
		item, err := s.toDomainIntegration(model)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) GetIntegration(ctx context.Context, id string) (domain.ManagedIntegration, error) {
	if s.cipher == nil {
		return domain.ManagedIntegration{}, ErrEncryptionUnavailable
	}
	var model integrationModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.ManagedIntegration{}, err
	}
	return s.toDomainIntegration(model)
}

func (s *Store) CreateIntegration(ctx context.Context, integration domain.ManagedIntegration) error {
	model, err := s.toIntegrationModel(integration)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(&model).Error
}

func (s *Store) UpdateIntegration(ctx context.Context, integration domain.ManagedIntegration) error {
	model, err := s.toIntegrationModel(integration)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&integrationModel{}).
		Where("id = ?", integration.ID).
		Updates(map[string]any{
			"source":            model.Source,
			"config_ciphertext": model.ConfigCiphertext,
			"updated_at":        integration.UpdatedAt,
		}).Error
}

func (s *Store) DeleteIntegration(ctx context.Context, id string) error {
	if s.cipher == nil {
		return ErrEncryptionUnavailable
	}
	return s.db.WithContext(ctx).Delete(&integrationModel{}, "id = ?", id).Error
}

func (s *Store) ListDestinations(ctx context.Context) ([]domain.ManagedDestination, error) {
	if s.cipher == nil {
		return nil, ErrEncryptionUnavailable
	}
	var models []destinationModel
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]domain.ManagedDestination, 0, len(models))
	for _, model := range models {
		item, err := s.toDomainDestination(model)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) GetDestination(ctx context.Context, id string) (domain.ManagedDestination, error) {
	if s.cipher == nil {
		return domain.ManagedDestination{}, ErrEncryptionUnavailable
	}
	var model destinationModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.ManagedDestination{}, err
	}
	return s.toDomainDestination(model)
}

func (s *Store) CreateDestination(ctx context.Context, destination domain.ManagedDestination) error {
	model, err := s.toDestinationModel(destination)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(&model).Error
}

func (s *Store) UpdateDestination(ctx context.Context, destination domain.ManagedDestination) error {
	model, err := s.toDestinationModel(destination)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&destinationModel{}).
		Where("id = ?", destination.ID).
		Updates(map[string]any{
			"type":              model.Type,
			"config_ciphertext": model.ConfigCiphertext,
			"updated_at":        destination.UpdatedAt,
		}).Error
}

func (s *Store) DeleteDestination(ctx context.Context, id string) error {
	if s.cipher == nil {
		return ErrEncryptionUnavailable
	}
	return s.db.WithContext(ctx).Delete(&destinationModel{}, "id = ?", id).Error
}

func (s *Store) HasActiveDeliveriesForDestination(ctx context.Context, id string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&deliveryModel{}).
		Where("destination_id = ? AND status IN ?", id, []string{
			string(domain.DeliveryPending),
			string(domain.DeliveryProcessing),
			string(domain.DeliveryRetryWait),
		}).Count(&count).Error
	return count > 0, err
}

func (s *Store) ListRendererProfiles(ctx context.Context) ([]domain.ManagedRendererProfile, error) {
	var models []rendererProfileModel
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]domain.ManagedRendererProfile, 0, len(models))
	for _, model := range models {
		item, err := s.toDomainRendererProfile(model)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) GetRendererProfile(ctx context.Context, id string) (domain.ManagedRendererProfile, error) {
	var model rendererProfileModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	return s.toDomainRendererProfile(model)
}

func (s *Store) CreateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) error {
	model, err := s.toRendererProfileModel(profile)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(&model).Error
}

func (s *Store) UpdateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) error {
	model, err := s.toRendererProfileModel(profile)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&rendererProfileModel{}).
		Where("id = ?", profile.ID).
		Updates(map[string]any{
			"profile_json": model.ProfileJSON,
			"updated_at":   profile.UpdatedAt,
		}).Error
}

func (s *Store) DeleteRendererProfile(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&rendererProfileModel{}, "id = ?", id).Error
}

func (s *Store) ListRoutes(ctx context.Context) ([]domain.Route, error) {
	var models []routeModel
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	routes := make([]domain.Route, 0, len(models))
	for _, model := range models {
		route, err := toDomainRoute(model)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, nil
}

func (s *Store) GetRoute(ctx context.Context, id string) (domain.Route, error) {
	var model routeModel
	if err := s.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return domain.Route{}, err
	}
	return toDomainRoute(model)
}

func (s *Store) CreateRoute(ctx context.Context, route domain.Route) error {
	model, err := toRouteModel(route)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(&model).Error
}

func (s *Store) UpdateRoute(ctx context.Context, route domain.Route) error {
	model, err := toRouteModel(route)
	if err != nil {
		return err
	}
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

func (s *Store) ListDeliveriesByEventIDs(ctx context.Context, eventIDs []string) (map[string][]domain.Delivery, error) {
	out := make(map[string][]domain.Delivery, len(eventIDs))
	if len(eventIDs) == 0 {
		return out, nil
	}

	var models []deliveryModel
	if err := s.db.WithContext(ctx).
		Where("event_id IN ?", eventIDs).
		Order("created_at ASC").
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	for _, eventID := range eventIDs {
		out[eventID] = nil
	}
	for _, model := range models {
		delivery := toDomainDelivery(model)
		out[delivery.EventID] = append(out[delivery.EventID], delivery)
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

type integrationPayload struct {
	Secret       string        `json:"secret"`
	ClientSecret string        `json:"client_secret,omitempty"`
	ReplayWindow time.Duration `json:"replay_window"`
}

func (s *Store) toIntegrationModel(in domain.ManagedIntegration) (integrationModel, error) {
	if s.cipher == nil {
		return integrationModel{}, ErrEncryptionUnavailable
	}
	payload, err := json.Marshal(integrationPayload{
		Secret:       in.Secret,
		ClientSecret: in.ClientSecret,
		ReplayWindow: in.ReplayWindow,
	})
	if err != nil {
		return integrationModel{}, err
	}
	ciphertext, err := s.cipher.Encrypt(payload)
	if err != nil {
		return integrationModel{}, err
	}
	return integrationModel{
		ID:               in.ID,
		Source:           string(in.Source),
		ConfigCiphertext: ciphertext,
		CreatedAt:        in.CreatedAt,
		UpdatedAt:        in.UpdatedAt,
	}, nil
}

func (s *Store) toDomainIntegration(in integrationModel) (domain.ManagedIntegration, error) {
	plaintext, err := s.cipher.Decrypt(in.ConfigCiphertext)
	if err != nil {
		return domain.ManagedIntegration{}, err
	}
	var payload integrationPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return domain.ManagedIntegration{}, err
	}
	return domain.ManagedIntegration{
		ID:           in.ID,
		Source:       domain.Source(in.Source),
		Secret:       payload.Secret,
		ClientSecret: payload.ClientSecret,
		ReplayWindow: payload.ReplayWindow,
		CreatedAt:    in.CreatedAt,
		UpdatedAt:    in.UpdatedAt,
	}, nil
}

type destinationPayload struct {
	WebhookURL       string   `json:"webhook_url,omitempty"`
	BotToken         string   `json:"bot_token,omitempty"`
	ChatID           string   `json:"chat_id,omitempty"`
	APIBaseURL       string   `json:"api_base_url,omitempty"`
	RendererProfiles []string `json:"renderer_profiles,omitempty"`
}

func (s *Store) toDestinationModel(in domain.ManagedDestination) (destinationModel, error) {
	if s.cipher == nil {
		return destinationModel{}, ErrEncryptionUnavailable
	}
	payload, err := json.Marshal(destinationPayload{
		WebhookURL:       in.WebhookURL,
		BotToken:         in.BotToken,
		ChatID:           in.ChatID,
		APIBaseURL:       in.APIBaseURL,
		RendererProfiles: append([]string(nil), in.RendererProfiles...),
	})
	if err != nil {
		return destinationModel{}, err
	}
	ciphertext, err := s.cipher.Encrypt(payload)
	if err != nil {
		return destinationModel{}, err
	}
	return destinationModel{
		ID:               in.ID,
		Type:             string(in.Type),
		ConfigCiphertext: ciphertext,
		CreatedAt:        in.CreatedAt,
		UpdatedAt:        in.UpdatedAt,
	}, nil
}

func (s *Store) toDomainDestination(in destinationModel) (domain.ManagedDestination, error) {
	plaintext, err := s.cipher.Decrypt(in.ConfigCiphertext)
	if err != nil {
		return domain.ManagedDestination{}, err
	}
	var payload destinationPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return domain.ManagedDestination{}, err
	}
	return domain.ManagedDestination{
		ID:               in.ID,
		Type:             domain.DestinationType(in.Type),
		WebhookURL:       payload.WebhookURL,
		BotToken:         payload.BotToken,
		ChatID:           payload.ChatID,
		APIBaseURL:       payload.APIBaseURL,
		RendererProfiles: append([]string(nil), payload.RendererProfiles...),
		CreatedAt:        in.CreatedAt,
		UpdatedAt:        in.UpdatedAt,
	}, nil
}

func (s *Store) toRendererProfileModel(in domain.ManagedRendererProfile) (rendererProfileModel, error) {
	profileJSON, err := json.Marshal(rendererProfilePayload{
		Version: 1,
		Profile: in.Profile,
	})
	if err != nil {
		return rendererProfileModel{}, err
	}
	return rendererProfileModel{
		ID:          in.ID,
		ProfileJSON: profileJSON,
		CreatedAt:   in.CreatedAt,
		UpdatedAt:   in.UpdatedAt,
	}, nil
}

func (s *Store) toDomainRendererProfile(in rendererProfileModel) (domain.ManagedRendererProfile, error) {
	var payload rendererProfilePayload
	if err := json.Unmarshal(in.ProfileJSON, &payload); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	if payload.Version != 1 {
		return domain.ManagedRendererProfile{}, fmt.Errorf("renderer profile %q has unsupported payload version %d", in.ID, payload.Version)
	}
	return domain.ManagedRendererProfile{
		ID:        in.ID,
		Profile:   payload.Profile,
		CreatedAt: in.CreatedAt,
		UpdatedAt: in.UpdatedAt,
	}, nil
}

type rendererProfilePayload struct {
	Version int                    `json:"version"`
	Profile domain.RendererProfile `json:"profile"`
}

func (s *Store) ClaimDueDeliveries(ctx context.Context, claim domain.ClaimRequest) ([]domain.DeliveryEnvelope, error) {
	var claimed []deliveryModel
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidates []deliveryModel
		if err := tx.
			Where("(status = ? OR status = ?) AND next_attempt_at <= ? AND (locked_until IS NULL OR locked_until < ?)", domain.DeliveryPending, domain.DeliveryRetryWait, claim.Now, claim.Now).
			Order("next_attempt_at ASC").
			Limit(claim.BatchSize).
			Find(&candidates).Error; err != nil {
			return err
		}

		lockedUntil := claim.Now.Add(claim.LeaseDuration)
		for i := range candidates {
			result := tx.Model(&deliveryModel{}).
				Where("id = ? AND (status = ? OR status = ?) AND next_attempt_at <= ? AND (locked_until IS NULL OR locked_until < ?)",
					candidates[i].ID,
					string(domain.DeliveryPending),
					string(domain.DeliveryRetryWait),
					claim.Now,
					claim.Now).
				Updates(map[string]any{
					"status":       string(domain.DeliveryProcessing),
					"locked_by":    claim.WorkerID,
					"locked_until": &lockedUntil,
					"updated_at":   claim.Now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			candidates[i].Status = string(domain.DeliveryProcessing)
			candidates[i].LockedBy = claim.WorkerID
			candidates[i].LockedUntil = &lockedUntil
			candidates[i].UpdatedAt = claim.Now
			claimed = append(claimed, candidates[i])
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

		updateResult := tx.Model(&deliveryModel{}).
			Where("id = ? AND locked_by = ? AND status = ?", result.DeliveryID, result.WorkerID, string(domain.DeliveryProcessing)).
			Updates(updates)
		if updateResult.Error != nil {
			return updateResult.Error
		}
		if updateResult.RowsAffected == 0 {
			return storage.ErrDeliveryLeaseLost
		}
		return nil
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

		return tx.Model(&deliveryModel{}).
			Where("id = ? AND status IN ? AND (locked_until IS NULL OR locked_until < ?)", id, []string{string(domain.DeliveryDeadLetter), string(domain.DeliveryRetryWait)}, now).
			Updates(map[string]any{
				"status":          string(domain.DeliveryRetryWait),
				"next_attempt_at": now,
				"locked_by":       "",
				"locked_until":    nil,
				"updated_at":      now,
			}).Error
	})
}
