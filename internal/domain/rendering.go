package domain

import "time"

type Destination struct {
	ID      string
	Type    DestinationType
	Profile string
}

type ManagedDestination struct {
	ID         string
	Type       DestinationType
	WebhookURL string
	BotToken   string
	ChatID     string
	APIBaseURL string
	Profile    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type EventRef struct {
	Source Source `json:"source"`
	Key    string `json:"key"`
}

type RendererBinding struct {
	Event     EventRef                     `json:"event"`
	Templates RendererDestinationTemplates `json:"templates"`
}

type RendererProfile struct {
	Bindings []RendererBinding `json:"bindings"`
}

type RendererDestinationTemplates struct {
	Slack    *SlackTemplate    `json:"slack,omitempty"`
	Telegram *TelegramTemplate `json:"telegram,omitempty"`
	Email    *EmailTemplate    `json:"email,omitempty"`
}

type SlackTemplate struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

type TelegramTemplate struct {
	Text string `json:"text,omitempty"`
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
