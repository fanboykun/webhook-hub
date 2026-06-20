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
