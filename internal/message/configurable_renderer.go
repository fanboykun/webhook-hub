package message

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	slackrender "github.com/fanboykun/webhook-hub/internal/message/slack"
	teamsrender "github.com/fanboykun/webhook-hub/internal/message/teams"
	telegramrender "github.com/fanboykun/webhook-hub/internal/message/telegram"
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
	if destination.Profile == "" {
		return r.fallback(ctx, event, destination)
	}
	if r.profiles == nil {
		return r.fallback(ctx, event, destination)
	}
	templates, ok := r.profiles.Resolve(destination.Profile, event)
	if !ok {
		return r.fallback(ctx, event, destination)
	}

	switch destination.Type {
	case domain.DestinationSlack:
		slackTpl := templates.Slack
		if slackTpl == nil {
			return r.slackFallback.Render(ctx, event, destination)
		}

		return r.renderSlack(event, destination, slackTpl)

	case domain.DestinationTelegram:
		telegramTpl := templates.Telegram
		if telegramTpl == nil {
			return r.telegramFallback.Render(ctx, event, destination)
		}

		return r.renderTelegram(event, destination, telegramTpl)

	case domain.DestinationTeams:
		teamsTpl := templates.Teams
		if teamsTpl == nil {
			return r.teamsFallback.Render(ctx, event, destination)
		}

		return r.renderTeams(event, destination, teamsTpl)

	default:
		return r.fallback(ctx, event, destination)
	}
}

func (r *ConfigurableRenderer) fallback(ctx context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error) {
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

func (r *ConfigurableRenderer) renderSlack(event domain.Event, destination domain.Destination, tpl *domain.SlackTemplate) (domain.RenderedMessage, error) {
	ctxVal := config.TemplateContext{
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
		Payload:     eventPayload(event),
		Metadata:    eventMetadata(event),
	}

	escapedCtx := escapeContext(ctxVal, escapeSlack)

	var renderedTitle string
	var err error
	if tpl.Title != "" {
		renderedTitle, err = executeGoTemplate("slack_title", tpl.Title, escapedCtx)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render slack title: %w", err)
		}
	} else {
		renderedTitle = fmt.Sprintf("[%s] %s", strings.ToUpper(string(event.Severity)), event.Title)
	}

	var renderedBody string
	if tpl.Body != "" {
		renderedBody, err = executeGoTemplate("slack_body", tpl.Body, escapedCtx)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render slack body: %w", err)
		}
	} else {
		renderedBody = fmt.Sprintf("*%s*\n%s", event.Title, event.Summary)
	}

	payload := map[string]any{
		"text": renderedTitle,
		"blocks": []map[string]any{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": renderedBody,
				},
			},
			{
				"type": "context",
				"elements": []map[string]string{
					{"type": "mrkdwn", "text": fmt.Sprintf("service=%s env=%s profile=%s", event.Scope.Service, event.Scope.Environment, destination.Profile)},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.RenderedMessage{}, err
	}

	return domain.RenderedMessage{
		ContentType: "application/json",
		Body:        body,
	}, nil
}

func (r *ConfigurableRenderer) renderTelegram(event domain.Event, destination domain.Destination, tpl *domain.TelegramTemplate) (domain.RenderedMessage, error) {
	ctxVal := config.TemplateContext{
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
		Payload:     eventPayload(event),
		Metadata:    eventMetadata(event),
	}

	escapedCtx := escapeContext(ctxVal, escapeTelegram)

	var renderedText string
	var err error
	if tpl.Text != "" {
		renderedText, err = executeGoTemplate("telegram_text", tpl.Text, escapedCtx)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render telegram text: %w", err)
		}
	} else {
		text := fmt.Sprintf(
			"<b>%s</b>\n%s\n\nseverity=%s\nenv=%s\nservice=%s\nprofile=%s",
			escapeTelegram(event.Title),
			escapeTelegram(event.Summary),
			escapeTelegram(string(event.Severity)),
			escapeTelegram(event.Scope.Environment),
			escapeTelegram(event.Scope.Service),
			escapeTelegram(destination.Profile),
		)
		if event.SourceURL != "" {
			text += fmt.Sprintf("\n<a href=\"%s\">Open source event</a>", escapeTelegram(event.SourceURL))
		}
		renderedText = text
	}

	return domain.RenderedMessage{
		ContentType: "text/html; charset=utf-8",
		Body:        []byte(renderedText),
	}, nil
}

func (r *ConfigurableRenderer) renderTeams(event domain.Event, destination domain.Destination, tpl *domain.TeamsTemplate) (domain.RenderedMessage, error) {
	ctxVal := config.TemplateContext{
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
		Payload:     eventPayload(event),
		Metadata:    eventMetadata(event),
	}

	var renderedTitle string
	var err error
	if tpl.Title != "" {
		renderedTitle, err = executeGoTemplate("teams_title", tpl.Title, ctxVal)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render teams title: %w", err)
		}
	} else {
		renderedTitle = fmt.Sprintf("[%s] %s", strings.ToUpper(string(event.Severity)), event.Title)
	}

	var renderedBody string
	if tpl.Body != "" {
		renderedBody, err = executeGoTemplate("teams_body", tpl.Body, ctxVal)
		if err != nil {
			return domain.RenderedMessage{}, fmt.Errorf("render teams body: %w", err)
		}
	} else {
		renderedBody = event.Summary
	}

	return teamsrender.RenderMessageCard(event, destination, renderedTitle, renderedBody)
}

func escapeSlack(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(s)
}

func escapeTelegram(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return replacer.Replace(s)
}

func escapeContext(ctxVal config.TemplateContext, escapeFn func(string) string) config.TemplateContext {
	ctxVal.Source = escapeFn(ctxVal.Source)
	ctxVal.EventKey = escapeFn(ctxVal.EventKey)
	ctxVal.EventType = escapeFn(ctxVal.EventType)
	ctxVal.Title = escapeFn(ctxVal.Title)
	ctxVal.Summary = escapeFn(ctxVal.Summary)
	ctxVal.Severity = escapeFn(ctxVal.Severity)
	ctxVal.Lifecycle = escapeFn(ctxVal.Lifecycle)
	ctxVal.Service = escapeFn(ctxVal.Service)
	ctxVal.Environment = escapeFn(ctxVal.Environment)
	ctxVal.SourceURL = escapeFn(ctxVal.SourceURL)
	return ctxVal
}

func eventPayload(event domain.Event) map[string]any {
	if len(event.PayloadJSON) == 0 {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(event.PayloadJSON, &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func eventMetadata(event domain.Event) map[string]any {
	if len(event.MetadataJSON) == 0 {
		return map[string]any{}
	}
	var metadata map[string]any
	if err := json.Unmarshal(event.MetadataJSON, &metadata); err != nil {
		return map[string]any{}
	}
	return metadata
}

func executeGoTemplate(name, templateStr string, ctx config.TemplateContext) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(templateStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
