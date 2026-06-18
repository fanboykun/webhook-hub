package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/iweka-dev/webhook-hub/internal/domain"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Render(_ context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error) {
	text := fmt.Sprintf(
		"<b>%s</b>\n%s\n\nseverity=%s\nenv=%s\nservice=%s\nrelease=%s\nprofile=%s",
		escapeHTML(event.Title),
		escapeHTML(event.Summary),
		escapeHTML(string(event.Severity)),
		escapeHTML(event.Environment),
		escapeHTML(event.Service),
		escapeHTML(event.Release),
		escapeHTML(destination.Profile),
	)
	if event.URL != "" {
		text += fmt.Sprintf("\n<a href=\"%s\">Open source event</a>", escapeHTML(event.URL))
	}

	return domain.RenderedMessage{
		ContentType: "text/html; charset=utf-8",
		Body:        []byte(text),
	}, nil
}

func escapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return replacer.Replace(value)
}
