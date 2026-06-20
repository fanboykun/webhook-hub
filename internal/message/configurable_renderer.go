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
	telegramrender "github.com/fanboykun/webhook-hub/internal/message/telegram"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
)

type ConfigurableRenderer struct {
	profiles         *runtimeconfig.RendererProfileRegistry
	slackFallback    Renderer
	telegramFallback Renderer
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
	}
}

func (r *ConfigurableRenderer) Render(ctx context.Context, event domain.Event, destination domain.Destination) (domain.RenderedMessage, error) {
	if destination.Profile == "" {
		return r.fallback(ctx, event, destination)
	}
	if r.profiles == nil {
		return r.fallback(ctx, event, destination)
	}

	profile, ok := r.profiles.Get(destination.Profile)
	if !ok {
		return r.fallback(ctx, event, destination)
	}

	sourceConfig, ok := profile[string(event.Source)]
	if !ok {
		return r.fallback(ctx, event, destination)
	}

	var dt config.DestinationTemplates
	hasOverride := false
	if sourceConfig.Overrides != nil {
		if o, ok := sourceConfig.Overrides[event.Type]; ok {
			dt = o
			hasOverride = true
		}
	}

	switch destination.Type {
	case domain.DestinationSlack:
		var slackTpl *config.SlackTemplateConfig
		if hasOverride && dt.Slack != nil {
			slackTpl = dt.Slack
		} else if sourceConfig.Default.Slack != nil {
			slackTpl = sourceConfig.Default.Slack
		}

		if slackTpl == nil {
			return r.slackFallback.Render(ctx, event, destination)
		}

		return r.renderSlack(event, destination, slackTpl)

	case domain.DestinationTelegram:
		var telegramTpl *config.TelegramTemplateConfig
		if hasOverride && dt.Telegram != nil {
			telegramTpl = dt.Telegram
		} else if sourceConfig.Default.Telegram != nil {
			telegramTpl = sourceConfig.Default.Telegram
		}

		if telegramTpl == nil {
			return r.telegramFallback.Render(ctx, event, destination)
		}

		return r.renderTelegram(event, destination, telegramTpl)

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
	default:
		return domain.RenderedMessage{}, fmt.Errorf("unsupported destination type: %s", destination.Type)
	}
}

func (r *ConfigurableRenderer) renderSlack(event domain.Event, destination domain.Destination, tpl *config.SlackTemplateConfig) (domain.RenderedMessage, error) {
	ctxVal := config.TemplateContext{
		Title:       event.Title,
		Summary:     event.Summary,
		Severity:    string(event.Severity),
		Lifecycle:   string(event.Lifecycle),
		Service:     event.Service,
		Environment: event.Environment,
		Release:     event.Release,
		CommitSHA:   event.CommitSHA,
		Actor:       event.Actor,
		URL:         event.URL,
		OccurredAt:  event.OccurredAt,
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
					{"type": "mrkdwn", "text": fmt.Sprintf("service=%s env=%s release=%s profile=%s", event.Service, event.Environment, event.Release, destination.Profile)},
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

func (r *ConfigurableRenderer) renderTelegram(event domain.Event, destination domain.Destination, tpl *config.TelegramTemplateConfig) (domain.RenderedMessage, error) {
	ctxVal := config.TemplateContext{
		Title:       event.Title,
		Summary:     event.Summary,
		Severity:    string(event.Severity),
		Lifecycle:   string(event.Lifecycle),
		Service:     event.Service,
		Environment: event.Environment,
		Release:     event.Release,
		CommitSHA:   event.CommitSHA,
		Actor:       event.Actor,
		URL:         event.URL,
		OccurredAt:  event.OccurredAt,
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
			"<b>%s</b>\n%s\n\nseverity=%s\nenv=%s\nservice=%s\nrelease=%s\nprofile=%s",
			escapeTelegram(event.Title),
			escapeTelegram(event.Summary),
			escapeTelegram(string(event.Severity)),
			escapeTelegram(event.Environment),
			escapeTelegram(event.Service),
			escapeTelegram(event.Release),
			escapeTelegram(destination.Profile),
		)
		if event.URL != "" {
			text += fmt.Sprintf("\n<a href=\"%s\">Open source event</a>", escapeTelegram(event.URL))
		}
		renderedText = text
	}

	return domain.RenderedMessage{
		ContentType: "text/html; charset=utf-8",
		Body:        []byte(renderedText),
	}, nil
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
	ctxVal.Title = escapeFn(ctxVal.Title)
	ctxVal.Summary = escapeFn(ctxVal.Summary)
	ctxVal.Severity = escapeFn(ctxVal.Severity)
	ctxVal.Lifecycle = escapeFn(ctxVal.Lifecycle)
	ctxVal.Service = escapeFn(ctxVal.Service)
	ctxVal.Environment = escapeFn(ctxVal.Environment)
	ctxVal.Release = escapeFn(ctxVal.Release)
	ctxVal.CommitSHA = escapeFn(ctxVal.CommitSHA)
	ctxVal.Actor = escapeFn(ctxVal.Actor)
	ctxVal.URL = escapeFn(ctxVal.URL)
	return ctxVal
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
