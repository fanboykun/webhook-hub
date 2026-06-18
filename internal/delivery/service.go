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
	envelopes, err := s.store.ClaimDueDeliveries(ctx, domain.ClaimRequest{
		WorkerID:      workerID,
		BatchSize:     s.cfg.Workers.BatchSize,
		LeaseDuration: s.cfg.Workers.LeaseDuration,
		Now:           s.clock.Now(),
	})
	if err != nil {
		return 0, err
	}

	for _, envelope := range envelopes {
		if err := s.processEnvelope(ctx, workerID, envelope); err != nil {
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
		return s.store.CompleteAttempt(ctx, domain.AttemptResult{
			DeliveryID:   envelope.Delivery.ID,
			WorkerID:     workerID,
			StartedAt:    startedAt,
			CompletedAt:  s.clock.Now(),
			Outcome:      "permanent_failure",
			ErrorCode:    "render_failed",
			ErrorMessage: err.Error(),
			NextStatus:   domain.DeliveryDeadLetter,
		})
	}

	sendResult, err := s.sendMessage(ctx, envelope.Delivery.DestinationID, destination.Type, message)
	if err == nil {
		return s.store.CompleteAttempt(ctx, domain.AttemptResult{
			DeliveryID:        envelope.Delivery.ID,
			WorkerID:          workerID,
			StartedAt:         startedAt,
			CompletedAt:       s.clock.Now(),
			Outcome:           "sent",
			ResponseCode:      sendResult.ResponseCode,
			ProviderMessageID: sendResult.ProviderMessageID,
			NextStatus:        domain.DeliverySent,
		})
	}

	nextStatus := domain.DeliveryDeadLetter
	outcome := "permanent_failure"
	var nextAttempt *time.Time
	var errorCode string
	var responseCode int

	switch typedErr := err.(type) {
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

	return s.store.CompleteAttempt(ctx, domain.AttemptResult{
		DeliveryID:    envelope.Delivery.ID,
		WorkerID:      workerID,
		StartedAt:     startedAt,
		CompletedAt:   s.clock.Now(),
		Outcome:       outcome,
		ResponseCode:  responseCode,
		ErrorCode:     errorCode,
		ErrorMessage:  err.Error(),
		NextStatus:    nextStatus,
		NextAttemptAt: nextAttempt,
	})
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
