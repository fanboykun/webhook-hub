package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/eventcatalog"
	eventdefaults "github.com/fanboykun/webhook-hub/internal/eventcatalog/defaults"
	"github.com/fanboykun/webhook-hub/internal/renderprofile"
	"github.com/spf13/viper"
)

type Config struct {
	Server           ServerConfig                 `mapstructure:"server"`
	API              APIConfig                    `mapstructure:"api"`
	Logging          LoggingConfig                `mapstructure:"logging"`
	Database         DatabaseConfig               `mapstructure:"database"`
	Workers          WorkersConfig                `mapstructure:"workers"`
	Retry            RetryConfig                  `mapstructure:"retry"`
	Integrations     map[string]IntegrationConfig `mapstructure:"integrations"`
	Destinations     map[string]DestinationConfig `mapstructure:"destinations"`
	RendererProfiles map[string]ProfileConfig     `mapstructure:"renderer_profiles"`
	Routes           []RouteConfig                `mapstructure:"routes"`
}

type RouteConfig struct {
	ID           string           `mapstructure:"id"`
	Description  string           `mapstructure:"description"`
	Match        RouteMatchConfig `mapstructure:"match"`
	Destinations []string         `mapstructure:"destinations"`
}

type RouteMatchConfig struct {
	Sources      []domain.Source   `mapstructure:"sources"`
	Types        []string          `mapstructure:"types"`
	Severities   []domain.Severity `mapstructure:"severities"`
	Environments []string          `mapstructure:"environments"`
}

type ProfileConfig struct {
	Source    domain.Source        `mapstructure:"source"`
	Key       string               `mapstructure:"key"`
	Templates DestinationTemplates `mapstructure:"templates"`
}

type DestinationTemplates struct {
	Slack    *SlackTemplateConfig    `mapstructure:"slack"`
	Telegram *TelegramTemplateConfig `mapstructure:"telegram"`
	Teams    *TeamsTemplateConfig    `mapstructure:"teams"`
	Email    *EmailTemplateConfig    `mapstructure:"email"`
}

type SlackTemplateConfig struct {
	Title string `mapstructure:"title"`
	Body  string `mapstructure:"body"`
}

type TelegramTemplateConfig struct {
	Text string `mapstructure:"text"`
}

type TeamsTemplateConfig struct {
	Title string `mapstructure:"title"`
	Body  string `mapstructure:"body"`
}

type EmailTemplateConfig struct {
	Subject string `mapstructure:"subject"`
	Body    string `mapstructure:"body"`
}

type ServerConfig struct {
	Address             string        `mapstructure:"address"`
	ReadHeaderTimeout   time.Duration `mapstructure:"read_header_timeout"`
	ReadTimeout         time.Duration `mapstructure:"read_timeout"`
	WriteTimeout        time.Duration `mapstructure:"write_timeout"`
	IdleTimeout         time.Duration `mapstructure:"idle_timeout"`
	ShutdownTimeout     time.Duration `mapstructure:"shutdown_timeout"`
	MaxWebhookBodyBytes int64         `mapstructure:"max_webhook_body_bytes"`
	TrustedProxies      []string      `mapstructure:"trusted_proxies"`
}

type APIConfig struct {
	DocsEnabled        bool   `mapstructure:"docs_enabled"`
	AdminTokenEnv      string `mapstructure:"admin_token_env"`
	ResolvedAdminToken string `mapstructure:"-"`
}

type LoggingConfig struct {
	Level     string `mapstructure:"level"`
	Format    string `mapstructure:"format"`
	AddSource bool   `mapstructure:"add_source"`
}

type DatabaseConfig struct {
	Path                  string        `mapstructure:"path"`
	EncryptionKeyEnv      string        `mapstructure:"encryption_key_env"`
	BusyTimeout           time.Duration `mapstructure:"busy_timeout"`
	MaxOpenConnections    int           `mapstructure:"max_open_connections"`
	RetainRawPayloads     bool          `mapstructure:"retain_raw_payloads"`
	ResolvedEncryptionKey string        `mapstructure:"-"`
}

type WorkersConfig struct {
	PollInterval     time.Duration `mapstructure:"poll_interval"`
	BatchSize        int           `mapstructure:"batch_size"`
	Concurrency      int           `mapstructure:"concurrency"`
	LeaseDuration    time.Duration `mapstructure:"lease_duration"`
	RecoveryInterval time.Duration `mapstructure:"recovery_interval"`
}

type RetryConfig struct {
	MaxAttempts int           `mapstructure:"max_attempts"`
	BaseDelay   time.Duration `mapstructure:"base_delay"`
	MaxDelay    time.Duration `mapstructure:"max_delay"`
	Jitter      float64       `mapstructure:"jitter"`
}

type IntegrationConfig struct {
	Source          domain.Source `mapstructure:"source"`
	SecretEnv       string        `mapstructure:"secret_env"`
	ClientSecretEnv string        `mapstructure:"client_secret_env"`
	ReplayWindow    time.Duration `mapstructure:"replay_window"`
	ResolvedSecret  string        `mapstructure:"-"`
}

type DestinationConfig struct {
	Type             domain.DestinationType `mapstructure:"type"`
	WebhookURLEnv    string                 `mapstructure:"webhook_url_env"`
	BotTokenEnv      string                 `mapstructure:"bot_token_env"`
	ChatID           string                 `mapstructure:"chat_id"`
	APIBaseURL       string                 `mapstructure:"api_base_url"`
	RendererProfiles []string               `mapstructure:"renderer_profiles"`
	ResolvedURL      string                 `mapstructure:"-"`
	ResolvedToken    string                 `mapstructure:"-"`
}

var (
	envRefPattern     = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	resourceIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

func Load(path string) (Config, error) {
	var cfg Config

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	if err := cfg.resolveSecrets(); err != nil {
		return cfg, err
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func LoadFromEnv() (Config, error) {
	path := os.Getenv("GATEWAY_CONFIG")
	if path == "" {
		path = DefaultPath()
	}
	return Load(path)
}

func DefaultPath() string {
	const (
		localPath   = "config.yaml"
		examplePath = "configs/config.example.yaml"
	)

	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	if _, err := os.Stat(examplePath); err == nil {
		return examplePath
	}

	return filepath.Clean(localPath)
}

func (c *Config) resolveSecrets() error {
	adminToken, key, err := resolveSecretValue(c.API.AdminTokenEnv)
	if err != nil {
		return fmt.Errorf("api admin token %q: %w", key, err)
	}
	c.API.ResolvedAdminToken = adminToken

	encryptionKey, key, err := resolveSecretValue(c.Database.EncryptionKeyEnv)
	if err != nil {
		return fmt.Errorf("database encryption key %q: %w", key, err)
	}
	c.Database.ResolvedEncryptionKey = encryptionKey

	for id, integration := range c.Integrations {
		value, key, err := resolveSecretValue(integration.SecretEnv)
		if integration.Source == domain.SourceSentry && integration.ClientSecretEnv != "" {
			value, key, err = resolveSecretValue(integration.ClientSecretEnv)
		}
		if err != nil {
			return fmt.Errorf("integration %q secret env %q: %w", id, key, err)
		}
		if value == "" {
			continue
		}
		if integration.Source == domain.SourceWatcher && !strings.HasPrefix(value, "whsec_") {
			return fmt.Errorf("integration %q secret must start with whsec_", id)
		}
		integration.ResolvedSecret = value
		c.Integrations[id] = integration
	}

	for id, destination := range c.Destinations {
		switch destination.Type {
		case domain.DestinationSlack, domain.DestinationTeams:
			value, key, err := resolveSecretValue(destination.WebhookURLEnv)
			if err != nil {
				return fmt.Errorf("destination %q webhook env %q: %w", id, key, err)
			}
			if value == "" {
				return fmt.Errorf("destination %q webhook env %q is empty", id, destination.WebhookURLEnv)
			}
			destination.ResolvedURL = value
		case domain.DestinationTelegram:
			value, key, err := resolveSecretValue(destination.BotTokenEnv)
			if err != nil {
				return fmt.Errorf("destination %q bot token env %q: %w", id, key, err)
			}
			if value == "" {
				return fmt.Errorf("destination %q bot token env %q is empty", id, destination.BotTokenEnv)
			}
			destination.ResolvedToken = value
		}
		c.Destinations[id] = destination
	}

	return nil
}

func (c Config) Validate() error {
	var errs []error

	if strings.TrimSpace(c.Server.Address) == "" {
		errs = append(errs, errors.New("server.address is required"))
	}
	if c.Server.MaxWebhookBodyBytes <= 0 {
		errs = append(errs, errors.New("server.max_webhook_body_bytes must be > 0"))
	}
	if strings.TrimSpace(c.Database.Path) == "" {
		errs = append(errs, errors.New("database.path is required"))
	}
	if strings.TrimSpace(c.Database.EncryptionKeyEnv) == "" {
		errs = append(errs, errors.New("database.encryption_key_env is required"))
	}
	if c.Workers.BatchSize <= 0 {
		errs = append(errs, errors.New("workers.batch_size must be > 0"))
	}
	if c.Workers.Concurrency <= 0 {
		errs = append(errs, errors.New("workers.concurrency must be > 0"))
	}
	if c.Workers.PollInterval <= 0 {
		errs = append(errs, errors.New("workers.poll_interval must be > 0"))
	}
	if c.Workers.LeaseDuration <= 0 {
		errs = append(errs, errors.New("workers.lease_duration must be > 0"))
	}
	if c.Workers.RecoveryInterval <= 0 {
		errs = append(errs, errors.New("workers.recovery_interval must be > 0"))
	}
	if c.Retry.MaxAttempts <= 0 {
		errs = append(errs, errors.New("retry.max_attempts must be > 0"))
	}

	for id, integration := range c.Integrations {
		if !resourceIDPattern.MatchString(id) || strings.TrimSpace(id) != id {
			errs = append(errs, fmt.Errorf("integration id %q must be a lowercase slug without surrounding whitespace", id))
		}
		if integration.Source == "" {
			errs = append(errs, fmt.Errorf("integration %q source is required", id))
		}
		if integration.ReplayWindow < 0 {
			errs = append(errs, fmt.Errorf("integration %q replay_window must be >= 0", id))
		}
	}

	for id, destination := range c.Destinations {
		if !resourceIDPattern.MatchString(id) || strings.TrimSpace(id) != id {
			errs = append(errs, fmt.Errorf("destination id %q must be a lowercase slug without surrounding whitespace", id))
		}
		switch destination.Type {
		case domain.DestinationSlack, domain.DestinationTeams:
			if strings.TrimSpace(destination.WebhookURLEnv) == "" {
				errs = append(errs, fmt.Errorf("destination %q webhook_url_env is required", id))
			}
		case domain.DestinationTelegram:
			if strings.TrimSpace(destination.BotTokenEnv) == "" {
				errs = append(errs, fmt.Errorf("destination %q bot_token_env is required", id))
			}
			if strings.TrimSpace(destination.ChatID) == "" {
				errs = append(errs, fmt.Errorf("destination %q chat_id is required", id))
			} else if !IsTelegramChatID(destination.ChatID) {
				errs = append(errs, fmt.Errorf("destination %q chat_id %q is invalid", id, destination.ChatID))
			}
		default:
			errs = append(errs, fmt.Errorf("destination %q type %q is not yet supported in this slice", id, destination.Type))
		}
		if err := validateDestinationProfileRefs(id, destination, c.RendererProfiles); err != nil {
			errs = append(errs, err)
		}
	}

	if err := c.ValidateRendererProfiles(); err != nil {
		errs = append(errs, err)
	}
	if err := c.validateRoutes(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (c Config) validateRoutes() error {
	seenRoutes := make(map[string]struct{}, len(c.Routes))
	var errs []error
	for _, route := range c.Routes {
		if !resourceIDPattern.MatchString(route.ID) || strings.TrimSpace(route.ID) != route.ID {
			errs = append(errs, fmt.Errorf("route id %q must be a lowercase slug without surrounding whitespace", route.ID))
			continue
		}
		if _, exists := seenRoutes[route.ID]; exists {
			errs = append(errs, fmt.Errorf("route id %q is duplicated", route.ID))
			continue
		}
		seenRoutes[route.ID] = struct{}{}
		if err := validateRouteMatchConfig(route.ID, route.Match); err != nil {
			errs = append(errs, err)
		}
		if err := validateRouteDestinations(route.ID, route.Destinations, c.Destinations); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func validateRouteMatchConfig(routeID string, match RouteMatchConfig) error {
	var errs []error
	seenSources := make(map[domain.Source]struct{}, len(match.Sources))
	for _, source := range match.Sources {
		if strings.TrimSpace(string(source)) != string(source) || !domain.IsKnownSource(source) {
			errs = append(errs, fmt.Errorf("route %q has unknown source %q", routeID, source))
			continue
		}
		if _, exists := seenSources[source]; exists {
			errs = append(errs, fmt.Errorf("route %q repeats source %q", routeID, source))
		}
		seenSources[source] = struct{}{}
	}
	seenTypes := make(map[string]struct{}, len(match.Types))
	for _, eventType := range match.Types {
		if strings.TrimSpace(eventType) != eventType || !eventdefaults.Registry().IsKnown(eventType) {
			errs = append(errs, fmt.Errorf("route %q has unknown event type %q", routeID, eventType))
			continue
		}
		if _, exists := seenTypes[eventType]; exists {
			errs = append(errs, fmt.Errorf("route %q repeats event type %q", routeID, eventType))
		}
		seenTypes[eventType] = struct{}{}
	}
	seenSeverities := make(map[domain.Severity]struct{}, len(match.Severities))
	for _, severity := range match.Severities {
		if strings.TrimSpace(string(severity)) != string(severity) || !domain.IsKnownSeverity(severity) {
			errs = append(errs, fmt.Errorf("route %q has unknown severity %q", routeID, severity))
			continue
		}
		if _, exists := seenSeverities[severity]; exists {
			errs = append(errs, fmt.Errorf("route %q repeats severity %q", routeID, severity))
		}
		seenSeverities[severity] = struct{}{}
	}
	if err := validateRouteStrings(routeID, "environment", match.Environments); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func validateRouteDestinations(routeID string, values []string, destinations map[string]DestinationConfig) error {
	if len(values) == 0 {
		return fmt.Errorf("route %q destinations must not be empty", routeID)
	}
	if err := validateRouteStrings(routeID, "destination", values); err != nil {
		return err
	}
	var errs []error
	for _, destinationID := range values {
		if _, exists := destinations[destinationID]; !exists {
			errs = append(errs, fmt.Errorf("route %q references undefined destination %q", routeID, destinationID))
		}
	}
	return errors.Join(errs...)
}

func validateRouteStrings(routeID, label string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	var errs []error
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) != value {
			errs = append(errs, fmt.Errorf("route %q has invalid %s %q", routeID, label, value))
			continue
		}
		if _, exists := seen[value]; exists {
			errs = append(errs, fmt.Errorf("route %q repeats %s %q", routeID, label, value))
		}
		seen[value] = struct{}{}
	}
	return errors.Join(errs...)
}

func (c *Config) ValidateRendererProfiles() error {
	var errs []error
	registry := eventdefaults.Registry()
	for profileName, profile := range c.RendererProfiles {
		if !resourceIDPattern.MatchString(profileName) {
			errs = append(errs, fmt.Errorf("renderer profile name %q must be a lowercase slug", profileName))
			continue
		}
		if profile.Source == "" {
			errs = append(errs, fmt.Errorf("profile %q source is required", profileName))
			continue
		}
		key := strings.TrimSpace(profile.Key)
		if key == "" {
			errs = append(errs, fmt.Errorf("profile %q key is required", profileName))
			continue
		}
		if key != profile.Key {
			errs = append(errs, fmt.Errorf("profile %q key must not contain surrounding whitespace", profileName))
			continue
		}
		definition, ok := registry.Resolve(profile.Source, key)
		if !ok {
			if definition, exists := registry.Get(eventcatalog.Key(key)); exists && definition.Source != profile.Source {
				errs = append(errs, fmt.Errorf("profile %q event %q belongs to source %q, not %q", profileName, key, definition.Source, profile.Source))
			} else {
				errs = append(errs, fmt.Errorf("profile %q references unknown event %q for source %q", profileName, key, profile.Source))
			}
			continue
		}
		if err := validateDestinationTemplates(profile.Templates, definition); err != nil {
			errs = append(errs, fmt.Errorf("profile %q %s:%s: %w", profileName, profile.Source, key, err))
		}
	}
	return errors.Join(errs...)
}

func validateDestinationProfileRefs(destinationID string, destination DestinationConfig, profiles map[string]ProfileConfig) error {
	seenIDs := make(map[string]struct{}, len(destination.RendererProfiles))
	seenEvents := make(map[string]string, len(destination.RendererProfiles))
	var errs []error
	for _, profileID := range destination.RendererProfiles {
		if profileID != strings.TrimSpace(profileID) || !resourceIDPattern.MatchString(profileID) {
			errs = append(errs, fmt.Errorf("destination %q renderer profile %q must be a lowercase slug without surrounding whitespace", destinationID, profileID))
			continue
		}
		if _, exists := seenIDs[profileID]; exists {
			errs = append(errs, fmt.Errorf("destination %q repeats renderer profile %q", destinationID, profileID))
			continue
		}
		seenIDs[profileID] = struct{}{}
		profile, exists := profiles[profileID]
		if !exists {
			errs = append(errs, fmt.Errorf("destination %q references undefined renderer profile %q", destinationID, profileID))
			continue
		}
		contract := string(profile.Source) + ":" + profile.Key
		if previous, exists := seenEvents[contract]; exists {
			errs = append(errs, fmt.Errorf("destination %q renderer profiles %q and %q both target %s", destinationID, previous, profileID, contract))
			continue
		}
		seenEvents[contract] = profileID
		if !configProfileSupportsDestination(profile, destination.Type) {
			errs = append(errs, fmt.Errorf("destination %q renderer profile %q has no %s template", destinationID, profileID, destination.Type))
		}
	}
	return errors.Join(errs...)
}

func configProfileSupportsDestination(profile ProfileConfig, destinationType domain.DestinationType) bool {
	switch destinationType {
	case domain.DestinationSlack:
		return profile.Templates.Slack != nil
	case domain.DestinationTelegram:
		return profile.Templates.Telegram != nil
	case domain.DestinationTeams:
		return profile.Templates.Teams != nil
	case domain.DestinationEmail:
		return profile.Templates.Email != nil
	default:
		return false
	}
}

func validateDestinationTemplates(dt DestinationTemplates, definition eventcatalog.Definition) error {
	var errs []error
	if dt.Slack == nil && dt.Telegram == nil && dt.Teams == nil && dt.Email == nil {
		errs = append(errs, errors.New("at least one destination template is required"))
	}
	if dt.Slack != nil {
		if _, err := renderprofile.CompileTemplate("slack_title", dt.Slack.Title, definition); err != nil {
			errs = append(errs, fmt.Errorf("slack title: %w", err))
		}
		if _, err := renderprofile.CompileTemplate("slack_body", dt.Slack.Body, definition); err != nil {
			errs = append(errs, fmt.Errorf("slack body: %w", err))
		}
	}
	if dt.Telegram != nil {
		if _, err := renderprofile.CompileTemplate("telegram_text", dt.Telegram.Text, definition); err != nil {
			errs = append(errs, fmt.Errorf("telegram text: %w", err))
		}
	}
	if dt.Teams != nil {
		if _, err := renderprofile.CompileTemplate("teams_title", dt.Teams.Title, definition); err != nil {
			errs = append(errs, fmt.Errorf("teams title: %w", err))
		}
		if _, err := renderprofile.CompileTemplate("teams_body", dt.Teams.Body, definition); err != nil {
			errs = append(errs, fmt.Errorf("teams body: %w", err))
		}
	}
	if dt.Email != nil {
		if _, err := renderprofile.CompileTemplate("email_subject", dt.Email.Subject, definition); err != nil {
			errs = append(errs, fmt.Errorf("email subject: %w", err))
		}
		if _, err := renderprofile.CompileTemplate("email_body", dt.Email.Body, definition); err != nil {
			errs = append(errs, fmt.Errorf("email body: %w", err))
		}
	}
	return errors.Join(errs...)
}

func resolveSecretValue(ref string) (string, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", nil
	}

	if !envRefPattern.MatchString(ref) {
		return "", ref, fmt.Errorf("must be an environment variable name")
	}

	value := strings.TrimSpace(os.Getenv(ref))
	if value == "" {
		return "", ref, fmt.Errorf("referenced environment variable is empty")
	}

	return value, ref, nil
}

func IsTelegramChatID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "@") {
		remainder := strings.TrimPrefix(value, "@")
		if len(remainder) < 5 || len(remainder) > 32 {
			return false
		}
		for _, r := range remainder {
			if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
				return false
			}
		}
		return true
	}
	if after, ok := strings.CutPrefix(value, "-"); ok {
		value = after
	}
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
