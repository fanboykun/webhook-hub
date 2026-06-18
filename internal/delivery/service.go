package delivery

import (
	"context"
	"log/slog"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/clock"
	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	slackrender "github.com/iweka-dev/webhook-hub/internal/message/slack"
	telegramrender "github.com/iweka-dev/webhook-hub/internal/message/telegram"
	sendpkg "github.com/iweka-dev/webhook-hub/internal/sender"
	slacksender "github.com/iweka-dev/webhook-hub/internal/sender/slack"
	telegramsender "github.com/iweka-dev/webhook-hub/internal/sender/telegram"
	"github.com/iweka-dev/webhook-hub/internal/storage"
)

type Service struct {
	store            storage.Store
	cfg              config.Config
	clock            clock.Clock
	logger           *slog.Logger
	slackRenderer    *slackrender.Renderer
	telegramRenderer *telegramrender.Renderer
	slackSender      *slacksender.Sender
	telegramSender   *telegramsender.Sender
}

func NewService(store storage.Store, cfg config.Config, clk clock.Clock, logger *slog.Logger) *Service {
	return &Service{
		store:            store,
		cfg:              cfg,
		clock:            clk,
		logger:           logger,
		slackRenderer:    slackrender.NewRenderer(),
		telegramRenderer: telegramrender.NewRenderer(),
		slackSender:      slacksender.New(cfg.Destinations),
		telegramSender:   telegramsender.New(cfg.Destinations),
	}
}

func (s *Service) ProcessOnce(ctx context.Context, workerID string) (int, error) {
	s.logDebug("delivery.poll", "worker_id", workerID, "batch_size", s.cfg.Workers.BatchSize)
	envelopes, err := s.store.ClaimDueDeliveries(ctx, domain.ClaimRequest{
		WorkerID:      workerID,
		BatchSize:     s.cfg.Workers.BatchSize,
		LeaseDuration: s.cfg.Workers.LeaseDuration,
		Now:           s.clock.Now(),
	})
	if err != nil {
		return 0, err
	}
	if len(envelopes) == 0 {
		s.logDebug("delivery.idle", "worker_id", workerID)
		return 0, nil
	}
	s.logInfo("delivery.claimed", "worker_id", workerID, "count", len(envelopes))

	for _, envelope := range envelopes {
		if err := s.processEnvelope(ctx, workerID, envelope); err != nil {
			s.logError("delivery.processing_failed", "worker_id", workerID, "delivery_id", envelope.Delivery.ID, "event_id", envelope.Event.ID, "destination_id", envelope.Delivery.DestinationID, "error", err)
			return 0, err
		}
	}

	return len(envelopes), nil
}

func (s *Service) processEnvelope(ctx context.Context, workerID string, envelope domain.DeliveryEnvelope) error {
	startedAt := s.clock.Now()
	destinationCfg := s.cfg.Destinations[envelope.Delivery.DestinationID]
	destination := domain.Destination{
		ID:      envelope.Delivery.DestinationID,
		Type:    destinationCfg.Type,
		Profile: destinationCfg.Profile,
	}

	message, err := s.renderMessage(ctx, envelope.Event, destination)
	if err != nil {
		renderErr := err
		err = s.store.CompleteAttempt(ctx, domain.AttemptResult{
			DeliveryID:   envelope.Delivery.ID,
			WorkerID:     workerID,
			StartedAt:    startedAt,
			CompletedAt:  s.clock.Now(),
			Outcome:      "permanent_failure",
			ErrorCode:    "render_failed",
			ErrorMessage: renderErr.Error(),
			NextStatus:   domain.DeliveryDeadLetter,
		})
		if err != nil {
			s.logError("delivery.dead_letter_update_failed", "worker_id", workerID, "delivery_id", envelope.Delivery.ID, "event_id", envelope.Event.ID, "destination_id", envelope.Delivery.DestinationID, "error", err)
			return err
		}
		s.logWarn("delivery.dead_lettered", "worker_id", workerID, "delivery_id", envelope.Delivery.ID, "event_id", envelope.Event.ID, "destination_id", envelope.Delivery.DestinationID, "error_code", "render_failed", "error", renderErr.Error())
		return nil
	}

	sendResult, sendErr := s.sendMessage(ctx, envelope.Delivery.DestinationID, destination.Type, message)
	if sendErr == nil {
		err = s.store.CompleteAttempt(ctx, domain.AttemptResult{
			DeliveryID:        envelope.Delivery.ID,
			WorkerID:          workerID,
			StartedAt:         startedAt,
			CompletedAt:       s.clock.Now(),
			Outcome:           "sent",
			ResponseCode:      sendResult.ResponseCode,
			ProviderMessageID: sendResult.ProviderMessageID,
			NextStatus:        domain.DeliverySent,
		})
		if err != nil {
			s.logError("delivery.sent_update_failed", "worker_id", workerID, "delivery_id", envelope.Delivery.ID, "event_id", envelope.Event.ID, "destination_id", envelope.Delivery.DestinationID, "error", err)
			return err
		}
		s.logInfo("delivery.sent", "worker_id", workerID, "delivery_id", envelope.Delivery.ID, "event_id", envelope.Event.ID, "destination_id", envelope.Delivery.DestinationID, "provider_message_id", sendResult.ProviderMessageID)
		return nil
	}

	nextStatus := domain.DeliveryDeadLetter
	outcome := "permanent_failure"
	var nextAttempt *time.Time
	var errorCode string
	var responseCode int

	switch typedErr := sendErr.(type) {
	case *slacksender.Error:
		errorCode = typedErr.Code
		responseCode = typedErr.ResponseCode
		if typedErr.Retryable && envelope.Delivery.AttemptCount+1 < envelope.Delivery.MaxAttempts {
			outcome = "retryable_failure"
			nextStatus = domain.DeliveryRetryWait
			when := NextAttempt(s.clock.Now(), envelope.Delivery.AttemptCount+1, s.cfg.Retry)
			nextAttempt = &when
		}
	case *telegramsender.Error:
		errorCode = typedErr.Code
		responseCode = typedErr.ResponseCode
		if typedErr.Retryable && envelope.Delivery.AttemptCount+1 < envelope.Delivery.MaxAttempts {
			outcome = "retryable_failure"
			nextStatus = domain.DeliveryRetryWait
			when := NextAttempt(s.clock.Now(), envelope.Delivery.AttemptCount+1, s.cfg.Retry)
			nextAttempt = &when
		}
	}

	err = s.store.CompleteAttempt(ctx, domain.AttemptResult{
		DeliveryID:    envelope.Delivery.ID,
		WorkerID:      workerID,
		StartedAt:     startedAt,
		CompletedAt:   s.clock.Now(),
		Outcome:       outcome,
		ResponseCode:  responseCode,
		ErrorCode:     errorCode,
		ErrorMessage:  sendErr.Error(),
		NextStatus:    nextStatus,
		NextAttemptAt: nextAttempt,
	})
	if err != nil {
		s.logError("delivery.failure_update_failed", "worker_id", workerID, "delivery_id", envelope.Delivery.ID, "event_id", envelope.Event.ID, "destination_id", envelope.Delivery.DestinationID, "error", err)
		return err
	}
	switch nextStatus {
	case domain.DeliveryRetryWait:
		s.logWarn("delivery.retry_scheduled", "worker_id", workerID, "delivery_id", envelope.Delivery.ID, "event_id", envelope.Event.ID, "destination_id", envelope.Delivery.DestinationID, "error_code", errorCode, "response_code", responseCode, "error", sendErr.Error())
	default:
		s.logWarn("delivery.dead_lettered", "worker_id", workerID, "delivery_id", envelope.Delivery.ID, "event_id", envelope.Event.ID, "destination_id", envelope.Delivery.DestinationID, "error_code", errorCode, "response_code", responseCode, "error", sendErr.Error())
	}
	return nil
}

func (s *Service) renderMessage(ctx context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error) {
	switch destination.Type {
	case domain.DestinationSlack:
		return s.slackRenderer.Render(ctx, event, destination)
	case domain.DestinationTelegram:
		return s.telegramRenderer.Render(ctx, event, destination)
	default:
		return domain.RenderedMessage{}, slogError("unsupported destination type")
	}
}

func (s *Service) sendMessage(ctx context.Context, destinationID string, destinationType domain.DestinationType, message domain.RenderedMessage) (sendpkg.SendResult, error) {
	switch destinationType {
	case domain.DestinationSlack:
		return s.slackSender.Send(ctx, destinationID, message)
	case domain.DestinationTelegram:
		return s.telegramSender.Send(ctx, destinationID, message)
	default:
		return sendpkg.SendResult{}, slogError("unsupported destination type")
	}
}

type slogError string

func (e slogError) Error() string {
	return string(e)
}

func (s *Service) logDebug(msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Debug(msg, attrs...)
	}
}

func (s *Service) logInfo(msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Info(msg, attrs...)
	}
}

func (s *Service) logWarn(msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Warn(msg, attrs...)
	}
}

func (s *Service) logError(msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Error(msg, attrs...)
	}
}
