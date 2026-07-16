package domain

import "time"

type Destination struct {
	ID               string
	Type             DestinationType
	RendererProfiles []string
	SelectedProfile  string
}

type ManagedDestination struct {
	ID               string
	Type             DestinationType
	WebhookURL       string
	BotToken         string
	ChatID           string
	APIBaseURL       string
	RendererProfiles []string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type RendererProfile struct {
	Source    Source                       `json:"source"`
	Key       string                       `json:"key"`
	Templates RendererDestinationTemplates `json:"templates"`
}

type RendererDestinationTemplates struct {
	Slack    *SlackTemplate    `json:"slack,omitempty"`
	Telegram *TelegramTemplate `json:"telegram,omitempty"`
	Teams    *TeamsTemplate    `json:"teams,omitempty"`
	Email    *EmailTemplate    `json:"email,omitempty"`
}

type SlackTemplate struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

type TelegramTemplate struct {
	Text string `json:"text,omitempty"`
}

type TeamsTemplate struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

type EmailTemplate struct {
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`
}

type ManagedRendererProfile struct {
	ID        string
	Profile   RendererProfile
	CreatedAt time.Time
	UpdatedAt time.Time
}
