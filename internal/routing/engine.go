package routing

import (
	"slices"
	"sync"

	"github.com/iweka-dev/webhook-hub/internal/domain"
)

type Engine struct {
	mu     sync.RWMutex
	routes []domain.Route
}

func New(routes []domain.Route) *Engine {
	return &Engine{routes: routes}
}

func (e *Engine) Replace(routes []domain.Route) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.routes = append([]domain.Route(nil), routes...)
}

func (e *Engine) Snapshot() []domain.Route {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]domain.Route(nil), e.routes...)
}

func (e *Engine) Destinations(event domain.Event) []domain.RouteMatch {
	e.mu.RLock()
	routes := append([]domain.Route(nil), e.routes...)
	e.mu.RUnlock()

	var matches []domain.RouteMatch
	seen := map[string]struct{}{}

	for _, route := range routes {
		if !matchRoute(route.Match, event) {
			continue
		}
		for _, destinationID := range route.Destinations {
			key := event.ID + ":" + destinationID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			matches = append(matches, domain.RouteMatch{
				RouteID:       route.ID,
				DestinationID: destinationID,
			})
		}
	}

	return matches
}

func matchRoute(match domain.RouteMatchCriteria, event domain.Event) bool {
	if len(match.Sources) > 0 && !slices.Contains(match.Sources, event.Source) {
		return false
	}
	if len(match.Types) > 0 && !slices.Contains(match.Types, event.Type) {
		return false
	}
	if len(match.Severities) > 0 && !slices.Contains(match.Severities, event.Severity) {
		return false
	}
	if len(match.Environments) > 0 && !slices.Contains(match.Environments, event.Environment) {
		return false
	}
	return true
}
