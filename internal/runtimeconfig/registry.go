package runtimeconfig

import (
	"sync"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
)

type IntegrationRegistry struct {
	mu    sync.RWMutex
	items map[string]config.IntegrationConfig
}

func NewIntegrationRegistry(items map[string]config.IntegrationConfig) *IntegrationRegistry {
	r := &IntegrationRegistry{}
	r.Replace(items)
	return r
}

func (r *IntegrationRegistry) Replace(items map[string]config.IntegrationConfig) {
	cloned := make(map[string]config.IntegrationConfig, len(items))
	for id, item := range items {
		cloned[id] = item
	}
	r.mu.Lock()
	r.items = cloned
	r.mu.Unlock()
}

func (r *IntegrationRegistry) Get(id string) (config.IntegrationConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	return item, ok
}

type DestinationRegistry struct {
	mu    sync.RWMutex
	items map[string]config.DestinationConfig
}

func NewDestinationRegistry(items map[string]config.DestinationConfig) *DestinationRegistry {
	r := &DestinationRegistry{}
	r.Replace(items)
	return r
}

func (r *DestinationRegistry) Replace(items map[string]config.DestinationConfig) {
	cloned := make(map[string]config.DestinationConfig, len(items))
	for id, item := range items {
		cloned[id] = item
	}
	r.mu.Lock()
	r.items = cloned
	r.mu.Unlock()
}

func (r *DestinationRegistry) Get(id string) (config.DestinationConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	return item, ok
}

type RendererProfileRegistry struct {
	mu    sync.RWMutex
	items map[string]config.ProfileConfig
}

func NewRendererProfileRegistry(items map[string]config.ProfileConfig) *RendererProfileRegistry {
	r := &RendererProfileRegistry{}
	r.Replace(items)
	return r
}

func (r *RendererProfileRegistry) Replace(items map[string]config.ProfileConfig) {
	cloned := make(map[string]config.ProfileConfig, len(items))
	for id, item := range items {
		cloned[id] = cloneProfileConfig(item)
	}
	r.mu.Lock()
	r.items = cloned
	r.mu.Unlock()
}

func (r *RendererProfileRegistry) Get(id string) (config.ProfileConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	return cloneProfileConfig(item), ok
}

func StaticIntegrations(cfg config.Config) []domain.ManagedIntegration {
	out := make([]domain.ManagedIntegration, 0, len(cfg.Integrations))
	for id, integration := range cfg.Integrations {
		out = append(out, domain.ManagedIntegration{
			ID:           id,
			Source:       integration.Source,
			Secret:       integration.ResolvedSecret,
			ReplayWindow: integration.ReplayWindow,
		})
	}
	return out
}

func StaticDestinations(cfg config.Config) []domain.ManagedDestination {
	out := make([]domain.ManagedDestination, 0, len(cfg.Destinations))
	for id, destination := range cfg.Destinations {
		out = append(out, domain.ManagedDestination{
			ID:         id,
			Type:       destination.Type,
			WebhookURL: destination.ResolvedURL,
			BotToken:   destination.ResolvedToken,
			ChatID:     destination.ChatID,
			APIBaseURL: destination.APIBaseURL,
			Profile:    destination.Profile,
		})
	}
	return out
}

func StaticRendererProfiles(cfg config.Config) []domain.ManagedRendererProfile {
	out := make([]domain.ManagedRendererProfile, 0, len(cfg.RendererProfiles))
	for id, profile := range cfg.RendererProfiles {
		out = append(out, domain.ManagedRendererProfile{
			ID:      id,
			Profile: profileConfigToDomain(profile),
		})
	}
	return out
}

func MapIntegrations(items []domain.ManagedIntegration) map[string]config.IntegrationConfig {
	out := make(map[string]config.IntegrationConfig, len(items))
	for _, item := range items {
		out[item.ID] = config.IntegrationConfig{
			Source:          item.Source,
			ResolvedSecret:  item.Secret,
			ClientSecretEnv: item.ClientSecret,
			ReplayWindow:    item.ReplayWindow,
		}
	}
	return out
}

func MapDestinations(items []domain.ManagedDestination) map[string]config.DestinationConfig {
	out := make(map[string]config.DestinationConfig, len(items))
	for _, item := range items {
		out[item.ID] = config.DestinationConfig{
			Type:          item.Type,
			ChatID:        item.ChatID,
			APIBaseURL:    item.APIBaseURL,
			Profile:       item.Profile,
			ResolvedURL:   item.WebhookURL,
			ResolvedToken: item.BotToken,
		}
	}
	return out
}

func MapRendererProfiles(items []domain.ManagedRendererProfile) map[string]config.ProfileConfig {
	out := make(map[string]config.ProfileConfig, len(items))
	for _, item := range items {
		out[item.ID] = domainProfileToConfig(item.Profile)
	}
	return out
}

func cloneProfileConfig(in config.ProfileConfig) config.ProfileConfig {
	out := make(config.ProfileConfig, len(in))
	for source, sourceConfig := range in {
		cloned := config.SourceConfig{
			Default: cloneDestinationTemplates(sourceConfig.Default),
		}
		if len(sourceConfig.Overrides) > 0 {
			cloned.Overrides = make(map[string]config.DestinationTemplates, len(sourceConfig.Overrides))
			for eventType, templates := range sourceConfig.Overrides {
				cloned.Overrides[eventType] = cloneDestinationTemplates(templates)
			}
		}
		out[source] = cloned
	}
	return out
}

func cloneDestinationTemplates(in config.DestinationTemplates) config.DestinationTemplates {
	out := config.DestinationTemplates{}
	if in.Slack != nil {
		slack := *in.Slack
		out.Slack = &slack
	}
	if in.Telegram != nil {
		telegram := *in.Telegram
		out.Telegram = &telegram
	}
	if in.Email != nil {
		email := *in.Email
		out.Email = &email
	}
	return out
}

func profileConfigToDomain(in config.ProfileConfig) domain.RendererProfile {
	out := make(domain.RendererProfile, len(in))
	for source, sourceConfig := range in {
		item := domain.RendererSourceConfig{
			Default:   destinationTemplatesToDomain(sourceConfig.Default),
			Overrides: make(map[string]domain.RendererDestinationTemplates, len(sourceConfig.Overrides)),
		}
		for eventType, templates := range sourceConfig.Overrides {
			item.Overrides[eventType] = destinationTemplatesToDomain(templates)
		}
		if len(item.Overrides) == 0 {
			item.Overrides = nil
		}
		out[source] = item
	}
	return out
}

func domainProfileToConfig(in domain.RendererProfile) config.ProfileConfig {
	out := make(config.ProfileConfig, len(in))
	for source, sourceConfig := range in {
		item := config.SourceConfig{
			Default:   destinationTemplatesFromDomain(sourceConfig.Default),
			Overrides: make(map[string]config.DestinationTemplates, len(sourceConfig.Overrides)),
		}
		for eventType, templates := range sourceConfig.Overrides {
			item.Overrides[eventType] = destinationTemplatesFromDomain(templates)
		}
		if len(item.Overrides) == 0 {
			item.Overrides = nil
		}
		out[source] = item
	}
	return out
}

func destinationTemplatesToDomain(in config.DestinationTemplates) domain.RendererDestinationTemplates {
	out := domain.RendererDestinationTemplates{}
	if in.Slack != nil {
		out.Slack = &domain.SlackTemplate{
			Title: in.Slack.Title,
			Body:  in.Slack.Body,
		}
	}
	if in.Telegram != nil {
		out.Telegram = &domain.TelegramTemplate{Text: in.Telegram.Text}
	}
	if in.Email != nil {
		out.Email = &domain.EmailTemplate{
			Subject: in.Email.Subject,
			Body:    in.Email.Body,
		}
	}
	return out
}

func destinationTemplatesFromDomain(in domain.RendererDestinationTemplates) config.DestinationTemplates {
	out := config.DestinationTemplates{}
	if in.Slack != nil {
		out.Slack = &config.SlackTemplateConfig{
			Title: in.Slack.Title,
			Body:  in.Slack.Body,
		}
	}
	if in.Telegram != nil {
		out.Telegram = &config.TelegramTemplateConfig{Text: in.Telegram.Text}
	}
	if in.Email != nil {
		out.Email = &config.EmailTemplateConfig{
			Subject: in.Email.Subject,
			Body:    in.Email.Body,
		}
	}
	return out
}
