package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iweka-dev/webhook-hub/internal/domain"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Render(_ context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error) {
	payload := map[string]any{
		"text": fmt.Sprintf("[%s] %s", strings.ToUpper(string(event.Severity)), event.Title),
		"blocks": []map[string]any{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*%s*\n%s", event.Title, event.Summary),
				},
			},
			{
				"type": "context",
				"elements": []map[string]string{
					{"type": "mrkdwn", "text": fmt.Sprintf("service=%s env=%s release=%s profile=%s", event.Service, event.Environment, event.Release, destination.Profile)},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	return domain.RenderedMessage{ContentType: "application/json", Body: body}, nil
}
