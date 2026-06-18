package sender

import (
	"context"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/domain"
)

type SendResult struct {
	ProviderMessageID string
	ResponseCode      int
	RetryAfter        time.Duration
}

type Sender interface {
	Type() domain.DestinationType
	Send(ctx context.Context, destinationID string, message domain.RenderedMessage) (SendResult, error)
}
