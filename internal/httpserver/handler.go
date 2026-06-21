package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/app"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/ingress"
)

type Handler struct {
	service *app.Service
}

func NewHandler(service *app.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) handleWatcherWebhook(ctx context.Context, input *watcherWebhookInput) (*watcherWebhookOutput, error) {
	headers := http.Header{}
	headers.Set("webhook-id", input.WebhookID)
	headers.Set("webhook-timestamp", input.Timestamp)
	headers.Set("webhook-signature", input.Signature)
	headers.Set("X-Watcher-Event", input.Event)
	headers.Set("X-Watcher-Event-ID", input.WebhookID)
	headers.Set("X-Watcher-Delivery-ID", input.DeliveryID)
	headers.Set("Content-Type", "application/json")

	return h.handleWebhook(ctx, domain.SourceWatcher, ingress.InboundRequest{
		IntegrationID: input.IntegrationID,
		Headers:       headers,
		RawBody:       input.RawBody,
		ReceivedAt:    time.Now().UTC(),
	})
}

func (h *Handler) handleGitHubWebhook(ctx context.Context, input *githubWebhookInput) (*watcherWebhookOutput, error) {
	return h.handleWebhook(ctx, domain.SourceGitHub, ingress.InboundRequest{
		IntegrationID: input.IntegrationID,
		Headers: http.Header{
			"X-GitHub-Event":      []string{input.Event},
			"X-GitHub-Delivery":   []string{input.DeliveryID},
			"X-Hub-Signature-256": []string{input.Signature},
			"Content-Type":        []string{"application/json"},
		},
		RawBody:    input.RawBody,
		ReceivedAt: time.Now().UTC(),
	})
}

func (h *Handler) handleWebhook(ctx context.Context, source domain.Source, req ingress.InboundRequest) (*watcherWebhookOutput, error) {
	result, err := h.service.HandleWebhook(ctx, source, req)
	if err != nil {
		switch err {
		case ingress.ErrUnknownIntegration:
			return nil, huma.Error404NotFound("unknown integration")
		case ingress.ErrWrongSource, ingress.ErrMalformedPayload:
			return nil, huma.Error400BadRequest(err.Error())
		case ingress.ErrUnauthorized:
			return nil, huma.Error401Unauthorized("invalid signature")
		default:
			return nil, huma.Error503ServiceUnavailable("ingest failed")
		}
	}

	out := &watcherWebhookOutput{Status: http.StatusAccepted}
	out.Body.ReceiptID = result.ReceiptID
	out.Body.Status = string(result.Status)
	out.Body.Duplicate = result.Duplicate
	out.Body.EventCount = result.EventCount
	out.Body.DeliveryCount = result.DeliveryCount
	return out, nil
}

func (h *Handler) healthLive(ctx context.Context, input *struct{}) (*healthResponse, error) {
	return &healthResponse{Body: map[string]string{"status": "ok"}}, nil
}

func (h *Handler) healthReady(ctx context.Context, input *struct{}) (*healthResponse, error) {
	if err := h.service.CheckReady(ctx); err != nil {
		return nil, huma.Error503ServiceUnavailable("database unavailable")
	}
	return &healthResponse{Body: map[string]string{"status": "ready"}}, nil
}
