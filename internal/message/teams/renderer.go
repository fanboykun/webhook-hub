package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Render(_ context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error) {
	title := fmt.Sprintf("[%s] %s", strings.ToUpper(string(event.Severity)), event.Title)
	body := event.Summary
	return RenderMessageCard(event, destination, title, body)
}

func RenderMessageCard(event domain.Event, destination domain.Destination, title, body string) (domain.RenderedMessage, error) {
	themeColor := themeColorForSeverity(event.Severity)
	payload := map[string]any{
		"@type":      "MessageCard",
		"@context":   "https://schema.org/extensions",
		"summary":    title,
		"themeColor": themeColor,
		"title":      title,
		"text":       body,
		"sections": []map[string]any{
			{
				"facts": []map[string]string{
					{"name": "Severity", "value": string(event.Severity)},
					{"name": "Lifecycle", "value": string(event.Lifecycle)},
					{"name": "Service", "value": event.Scope.Service},
					{"name": "Environment", "value": event.Scope.Environment},
					{"name": "Profile", "value": destination.Profile},
				},
				"markdown": true,
			},
		},
	}
	if event.SourceURL != "" {
		payload["potentialAction"] = []map[string]any{
			{
				"@type": "OpenUri",
				"name":  "Open source event",
				"targets": []map[string]string{
					{"os": "default", "uri": event.SourceURL},
				},
			},
		}
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	return domain.RenderedMessage{ContentType: "application/json", Body: encoded}, nil
}

func themeColorForSeverity(severity domain.Severity) string {
	switch severity {
	case domain.SeverityCritical, domain.SeverityError:
		return "D13438"
	case domain.SeverityWarning:
		return "FFB900"
	case domain.SeverityDebug:
		return "8A8886"
	default:
		return "107C10"
	}
}
