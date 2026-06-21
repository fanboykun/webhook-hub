package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	eventdefaults "github.com/fanboykun/webhook-hub/internal/eventcatalog/defaults"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
)

func (s *Service) validateRoute(route domain.Route) error {
	if strings.TrimSpace(route.ID) == "" {
		return errors.New("route id is required")
	}
	if len(route.Destinations) == 0 {
		return errors.New("route destinations must not be empty")
	}
	seenDestinations := make(map[string]struct{}, len(route.Destinations))
	for _, destinationID := range route.Destinations {
		destinationID = strings.TrimSpace(destinationID)
		if destinationID == "" {
			return errors.New("route destinations must not contain blank values")
		}
		if _, exists := seenDestinations[destinationID]; exists {
			return fmt.Errorf("route destinations must be unique: %s", destinationID)
		}
		seenDestinations[destinationID] = struct{}{}
		if s.destinations == nil {
			return errors.New("destination registry is not configured")
		}
		if _, exists := s.destinations.Get(destinationID); !exists {
			return errors.New("route references unknown destination " + destinationID)
		}
	}
	return validateRouteMatch(route.Match)
}

func (s *Service) validateIntegration(integration domain.ManagedIntegration) error {
	if strings.TrimSpace(integration.ID) == "" {
		return errors.New("integration id is required")
	}
	switch integration.Source {
	case domain.SourceWatcher:
		if !strings.HasPrefix(integration.Secret, "whsec_") {
			return errors.New("watcher integration secret must start with whsec_")
		}
	case domain.SourceGitHub:
		if strings.TrimSpace(integration.Secret) == "" {
			return errors.New("github integration secret is required")
		}
	default:
		return fmt.Errorf("integration source %q is not supported in this slice", integration.Source)
	}
	if integration.ReplayWindow < 0 {
		return errors.New("integration replay window must be >= 0")
	}
	return nil
}

func (s *Service) validateDestination(destination domain.ManagedDestination) error {
	if strings.TrimSpace(destination.ID) == "" {
		return errors.New("destination id is required")
	}
	switch destination.Type {
	case domain.DestinationSlack:
		if strings.TrimSpace(destination.WebhookURL) == "" {
			return errors.New("slack destination webhook_url is required")
		}
	case domain.DestinationTelegram:
		if strings.TrimSpace(destination.BotToken) == "" {
			return errors.New("telegram destination bot_token is required")
		}
		if !config.IsTelegramChatID(destination.ChatID) {
			return errors.New("telegram destination chat_id is invalid")
		}
	default:
		return fmt.Errorf("destination type %q is not supported in this slice", destination.Type)
	}
	if destination.Profile != "" {
		if s.profiles == nil {
			return errors.New("renderer profile registry is not configured")
		}
		if _, ok := s.profiles.Get(destination.Profile); !ok {
			return fmt.Errorf("destination references undefined renderer profile %q", destination.Profile)
		}
	}
	return nil
}

func (s *Service) validateRendererProfile(profile domain.ManagedRendererProfile) error {
	if strings.TrimSpace(profile.ID) == "" {
		return errors.New("renderer profile id is required")
	}
	if len(profile.Profile) == 0 {
		return errors.New("renderer profile must contain at least one event binding")
	}
	cfg := config.Config{
		RendererProfiles: runtimeconfig.RendererProfilesToConfig(runtimeconfig.MapRendererProfiles([]domain.ManagedRendererProfile{profile})),
	}
	return cfg.ValidateRendererProfiles()
}

func normalizeRoute(route domain.Route) domain.Route {
	route.Match.Sources = normalizeSources(route.Match.Sources)
	route.Match.Types = normalizeStrings(route.Match.Types)
	route.Match.Severities = normalizeSeverities(route.Match.Severities)
	route.Match.Environments = normalizeStrings(route.Match.Environments)
	route.Destinations = normalizeStrings(route.Destinations)
	return route
}

func normalizeSources(values []domain.Source) []domain.Source {
	out := make([]domain.Source, 0, len(values))
	for _, value := range values {
		value = domain.Source(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func normalizeSeverities(values []domain.Severity) []domain.Severity {
	out := make([]domain.Severity, 0, len(values))
	for _, value := range values {
		value = domain.Severity(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func validateRouteMatch(match domain.RouteMatchCriteria) error {
	if err := validateRouteSources(match.Sources); err != nil {
		return err
	}
	if err := validateRouteEventTypes(match.Types); err != nil {
		return err
	}
	if err := validateRouteSeverities(match.Severities); err != nil {
		return err
	}
	return validateRouteEnvironments(match.Environments)
}

func validateRouteSources(values []domain.Source) error {
	seen := map[domain.Source]struct{}{}
	for _, value := range values {
		value = domain.Source(strings.TrimSpace(string(value)))
		if value == "" {
			return errors.New("route sources must not contain blank values")
		}
		if !domain.IsKnownSource(value) {
			return fmt.Errorf("route sources contains unknown value %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("route sources must be unique: %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateRouteEventTypes(values []string) error {
	registry := eventdefaults.Registry()
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return errors.New("route types must not contain blank values")
		}
		if !registry.IsKnown(value) {
			return fmt.Errorf("route types contains unknown value %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("route types must be unique: %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateRouteSeverities(values []domain.Severity) error {
	seen := map[domain.Severity]struct{}{}
	for _, value := range values {
		value = domain.Severity(strings.TrimSpace(string(value)))
		if value == "" {
			return errors.New("route severities must not contain blank values")
		}
		if !domain.IsKnownSeverity(value) {
			return fmt.Errorf("route severities contains unknown value %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("route severities must be unique: %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateRouteEnvironments(values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return errors.New("route environments must not contain blank values")
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("route environments must be unique: %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
