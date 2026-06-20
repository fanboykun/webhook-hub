package message

import (
	"context"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

type Renderer interface {
	Render(ctx context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error)
}
