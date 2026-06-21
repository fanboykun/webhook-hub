package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/eventcatalog"
	eventdefaults "github.com/fanboykun/webhook-hub/internal/eventcatalog/defaults"
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
}

type ProfileConfig struct {
	Bindings []ProfileBindingConfig `mapstructure:"bindings"`
}

type ProfileBindingConfig struct {
	Event     EventBindingConfig   `mapstructure:"event"`
	Templates DestinationTemplates `mapstructure:"templates"`
}

type EventBindingConfig struct {
	Source domain.Source `mapstructure:"source"`
	Key    string        `mapstructure:"key"`
}

type DestinationTemplates struct {
	Slack    *SlackTemplateConfig    `mapstructure:"slack"`
	Telegram *TelegramTemplateConfig `mapstructure:"telegram"`
	Email    *EmailTemplateConfig    `mapstructure:"email"`
}

type SlackTemplateConfig struct {
	Title string `mapstructure:"title"`
	Body  string `mapstructure:"body"`
}

type TelegramTemplateConfig struct {
	Text string `mapstructure:"text"`
}

type EmailTemplateConfig struct {
	Subject string `mapstructure:"subject"`
	Body    string `mapstructure:"body"`
}

type TemplateContext struct {
	Source      string
	EventKey    string
	EventType   string
	Title       string
	Summary     string
	Severity    string
	Lifecycle   string
	Service     string
	Environment string
	SourceURL   string
	OccurredAt  time.Time
	Payload     map[string]any
	Metadata    map[string]any
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
	Type          domain.DestinationType `mapstructure:"type"`
	WebhookURLEnv string                 `mapstructure:"webhook_url_env"`
	BotTokenEnv   string                 `mapstructure:"bot_token_env"`
	ChatID        string                 `mapstructure:"chat_id"`
	APIBaseURL    string                 `mapstructure:"api_base_url"`
	Profile       string                 `mapstructure:"profile"`
	ResolvedURL   string                 `mapstructure:"-"`
	ResolvedToken string                 `mapstructure:"-"`
}

var envRefPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

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
		case domain.DestinationSlack:
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
	if c.Retry.MaxAttempts <= 0 {
		errs = append(errs, errors.New("retry.max_attempts must be > 0"))
	}

	for id, integration := range c.Integrations {
		if integration.Source == "" {
			errs = append(errs, fmt.Errorf("integration %q source is required", id))
		}
		if integration.ReplayWindow < 0 {
			errs = append(errs, fmt.Errorf("integration %q replay_window must be >= 0", id))
		}
	}

	for id, destination := range c.Destinations {
		switch destination.Type {
		case domain.DestinationSlack:
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

	}

	if err := c.ValidateRendererProfiles(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func (c *Config) ValidateRendererProfiles() error {
	var errs []error
	registry := eventdefaults.Registry()
	for profileName, profile := range c.RendererProfiles {
		if len(profile.Bindings) == 0 {
			errs = append(errs, fmt.Errorf("profile %q must contain at least one binding", profileName))
			continue
		}
		seen := make(map[string]struct{}, len(profile.Bindings))
		for _, binding := range profile.Bindings {
			if binding.Event.Source == "" {
				errs = append(errs, fmt.Errorf("profile %q binding source is required", profileName))
				continue
			}
			key := strings.TrimSpace(binding.Event.Key)
			if key == "" {
				errs = append(errs, fmt.Errorf("profile %q binding key is required", profileName))
				continue
			}
			composite := string(binding.Event.Source) + ":" + key
			if _, exists := seen[composite]; exists {
				errs = append(errs, fmt.Errorf("profile %q contains duplicate binding %s", profileName, composite))
				continue
			}
			seen[composite] = struct{}{}
			if _, ok := registry.Resolve(binding.Event.Source, key); !ok {
				if definition, exists := registry.Get(eventcatalog.Key(key)); exists && definition.Source != binding.Event.Source {
					errs = append(errs, fmt.Errorf("profile %q binding %q belongs to source %q, not %q", profileName, key, definition.Source, binding.Event.Source))
				} else {
					errs = append(errs, fmt.Errorf("profile %q binding: unknown event %q for source %q", profileName, key, binding.Event.Source))
				}
				continue
			}
			if err := validateDestinationTemplates(binding.Templates); err != nil {
				errs = append(errs, fmt.Errorf("profile %q binding %s: %w", profileName, composite, err))
			}
		}
	}
	return errors.Join(errs...)
}

func validateDestinationTemplates(dt DestinationTemplates) error {
	var errs []error
	if dt.Slack == nil && dt.Telegram == nil && dt.Email == nil {
		errs = append(errs, errors.New("at least one destination template is required"))
	}
	if dt.Slack != nil {
		if err := ValidateTemplate(dt.Slack.Title); err != nil {
			errs = append(errs, fmt.Errorf("slack title: %w", err))
		}
		if err := ValidateTemplate(dt.Slack.Body); err != nil {
			errs = append(errs, fmt.Errorf("slack body: %w", err))
		}
	}
	if dt.Telegram != nil {
		if err := ValidateTemplate(dt.Telegram.Text); err != nil {
			errs = append(errs, fmt.Errorf("telegram text: %w", err))
		}
	}
	if dt.Email != nil {
		if err := ValidateTemplate(dt.Email.Subject); err != nil {
			errs = append(errs, fmt.Errorf("email subject: %w", err))
		}
		if err := ValidateTemplate(dt.Email.Body); err != nil {
			errs = append(errs, fmt.Errorf("email body: %w", err))
		}
	}
	return errors.Join(errs...)
}

func ValidateTemplate(tplStr string) error {
	if tplStr == "" {
		return nil
	}
	tmpl, err := template.New("test").Option("missingkey=error").Parse(tplStr)
	if err != nil {
		return fmt.Errorf("invalid template syntax: %w", err)
	}

	var dummy TemplateContext
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, &dummy)
	if err != nil {
		return fmt.Errorf("invalid template fields: %w", err)
	}
	return nil
}

func resolveSecretValue(ref string) (string, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", nil
	}

	if !envRefPattern.MatchString(ref) {
		return ref, ref, nil
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
