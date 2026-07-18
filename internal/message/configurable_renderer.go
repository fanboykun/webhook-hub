package message

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/fanboykun/webhook-hub/internal/domain"
	eventdefaults "github.com/fanboykun/webhook-hub/internal/eventcatalog/defaults"
	slackrender "github.com/fanboykun/webhook-hub/internal/message/slack"
	teamsrender "github.com/fanboykun/webhook-hub/internal/message/teams"
	telegramrender "github.com/fanboykun/webhook-hub/internal/message/telegram"
	"github.com/fanboykun/webhook-hub/internal/renderprofile"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
)

type ConfigurableRenderer struct {
	profiles         *runtimeconfig.RendererProfileRegistry
	slackFallback    Renderer
	telegramFallback Renderer
	teamsFallback    Renderer
}

func NewConfigurableRenderer(profiles *runtimeconfig.RendererProfileRegistry, slackFallback Renderer, telegramFallback Renderer) *ConfigurableRenderer {
	if slackFallback == nil {
		slackFallback = slackrender.NewRenderer()
	}
	if telegramFallback == nil {
		telegramFallback = telegramrender.NewRenderer()
	}
	return &ConfigurableRenderer{
		profiles:         profiles,
		slackFallback:    slackFallback,
		telegramFallback: telegramFallback,
		teamsFallback:    teamsrender.NewRenderer(),
	}
}

func (r *ConfigurableRenderer) Render(ctx context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error) {
	if len(destination.RendererProfiles) == 0 || r.profiles == nil {
		return r.fallback(ctx, event, destination)
	}
	profile, profileID, err := r.profiles.Resolve(destination.RendererProfiles, event)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	if profileID == "" {
		return r.fallback(ctx, event, destination)
	}
	destination.SelectedProfile = profileID

	switch destination.Type {
	case domain.DestinationSlack:
		if profile.Slack == nil {
			return r.slackFallback.Render(ctx, event, destination)
		}
		return r.renderSlack(event, destination, profile.Slack)
	case domain.DestinationTelegram:
		if profile.Telegram == nil {
			return r.telegramFallback.Render(ctx, event, destination)
		}
		return r.renderTelegram(event, destination, profile.Telegram)
	case domain.DestinationTeams:
		if profile.Teams == nil {
			return r.teamsFallback.Render(ctx, event, destination)
		}
		return r.renderTeams(event, destination, profile.Teams)
	default:
		return r.fallback(ctx, event, destination)
	}
}

func (r *ConfigurableRenderer) fallback(ctx context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error) {
	destination.SelectedProfile = ""
	switch destination.Type {
	case domain.DestinationSlack:
		return r.slackFallback.Render(ctx, event, destination)
	case domain.DestinationTelegram:
		return r.telegramFallback.Render(ctx, event, destination)
	case domain.DestinationTeams:
		return r.teamsFallback.Render(ctx, event, destination)
	default:
		return domain.RenderedMessage{}, fmt.Errorf("unsupported destination type: %s", destination.Type)
	}
}

func (r *ConfigurableRenderer) renderSlack(event domain.Event, destination domain.Destination, templates *renderprofile.CompiledPair) (domain.RenderedMessage, error) {
	contextValue, err := templateContext(event)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	escaped := escapeContext(contextValue, escapeSlack)

	title := fmt.Sprintf("[%s] %s", strings.ToUpper(string(event.Severity)), escapeSlack(event.Title))
	if templates.Title != nil {
		title, err = renderprofile.Execute(templates.Title, escaped)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render slack title: %w", err)
		}
	}
	bodyText := fmt.Sprintf("*%s*\n%s", escapeSlack(event.Title), escapeSlack(event.Summary))
	if templates.Body != nil {
		bodyText, err = renderprofile.Execute(templates.Body, escaped)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render slack body: %w", err)
		}
	}
	if err := enforceRuneLimit("slack title", title, 3000); err != nil {
		return domain.RenderedMessage{}, err
	}
	if err := enforceRuneLimit("slack body", bodyText, 3000); err != nil {
		return domain.RenderedMessage{}, err
	}

	payload := map[string]any{
		"text": title,
		"blocks": []map[string]any{
			{"type": "section", "text": map[string]string{"type": "mrkdwn", "text": bodyText}},
			{
				"type": "context",
				"elements": []map[string]string{{
					"type": "mrkdwn",
					"text": fmt.Sprintf("service=%s env=%s profile=%s", escapeSlack(event.Scope.Service), escapeSlack(event.Scope.Environment), escapeSlack(destination.SelectedProfile)),
				}},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	return domain.RenderedMessage{ContentType: "application/json", Body: encoded}, nil
}

func (r *ConfigurableRenderer) renderTelegram(event domain.Event, destination domain.Destination, compiled *renderprofile.CompiledTemplate) (domain.RenderedMessage, error) {
	contextValue, err := templateContext(event)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	text, err := renderprofile.Execute(compiled, escapeContext(contextValue, escapeTelegram))
	if err != nil {
		return domain.RenderedMessage{}, fmt.Errorf("render telegram text: %w", err)
	}
	if err := enforceRuneLimit("telegram text", text, 4096); err != nil {
		return domain.RenderedMessage{}, err
	}
	return domain.RenderedMessage{ContentType: "text/html; charset=utf-8", Body: []byte(text)}, nil
}

func (r *ConfigurableRenderer) renderTeams(event domain.Event, destination domain.Destination, templates *renderprofile.CompiledPair) (domain.RenderedMessage, error) {
	contextValue, err := templateContext(event)
	if err != nil {
		return domain.RenderedMessage{}, err
	}
	escaped := escapeContext(contextValue, escapeTeamsMarkdown)

	title := fmt.Sprintf("[%s] %s", strings.ToUpper(string(event.Severity)), escapeTeamsMarkdown(event.Title))
	if templates.Title != nil {
		title, err = renderprofile.Execute(templates.Title, escaped)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render teams title: %w", err)
		}
	}
	bodyText := escapeTeamsMarkdown(event.Summary)
	if templates.Body != nil {
		bodyText, err = renderprofile.Execute(templates.Body, escaped)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render teams body: %w", err)
		}
	}
	if err := enforceRuneLimit("teams title", title, 3000); err != nil {
		return domain.RenderedMessage{}, err
	}
	if err := enforceRuneLimit("teams body", bodyText, 24000); err != nil {
		return domain.RenderedMessage{}, err
	}
	return teamsrender.RenderMessageCard(event, destination, title, bodyText)
}

func templateContext(event domain.Event) (renderprofile.Context, error) {
	payload, err := eventdefaults.Registry().MaterializePayload(event.Source, event.Key, event.PayloadVersion, event.PayloadJSON)
	if err != nil {
		return renderprofile.Context{}, fmt.Errorf("materialize renderer payload: %w", err)
	}
	return renderprofile.Context{
		Source:      string(event.Source),
		EventKey:    event.Key,
		EventType:   event.Key,
		Title:       event.Title,
		Summary:     event.Summary,
		Severity:    string(event.Severity),
		Lifecycle:   string(event.Lifecycle),
		Service:     event.Scope.Service,
		Environment: event.Scope.Environment,
		SourceURL:   event.SourceURL,
		OccurredAt:  event.OccurredAt,
		Payload:     payload,
	}, nil
}

func escapeSlack(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}

func escapeTelegram(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(value)
}

func escapeTeamsMarkdown(value string) string {
	return strings.NewReplacer("&", "&amp;", "\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)", "<", "&lt;", ">", "&gt;").Replace(value)
}

func escapeContext(contextValue renderprofile.Context, escapeFn func(string) string) renderprofile.Context {
	contextValue.Source = escapeFn(contextValue.Source)
	contextValue.EventKey = escapeFn(contextValue.EventKey)
	contextValue.EventType = escapeFn(contextValue.EventType)
	contextValue.Title = escapeFn(contextValue.Title)
	contextValue.Summary = escapeFn(contextValue.Summary)
	contextValue.Severity = escapeFn(contextValue.Severity)
	contextValue.Lifecycle = escapeFn(contextValue.Lifecycle)
	contextValue.Service = escapeFn(contextValue.Service)
	contextValue.Environment = escapeFn(contextValue.Environment)
	contextValue.SourceURL = escapeFn(contextValue.SourceURL)
	contextValue.Payload = escapePayload(contextValue.Payload, escapeFn)
	return contextValue
}

func escapePayload(payload map[string]any, escapeFn func(string) string) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		switch typed := value.(type) {
		case string:
			out[key] = escapeFn(typed)
		case map[string]any:
			out[key] = escapePayload(typed, escapeFn)
		default:
			out[key] = typed
		}
	}
	return out
}

func enforceRuneLimit(label, value string, limit int) error {
	if utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("%s exceeds %d characters", label, limit)
	}
	return nil
}
