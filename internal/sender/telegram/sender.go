package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/sender"
)

type Sender struct {
	client       *http.Client
	destinations map[string]config.DestinationConfig
}

type Error struct {
	Retryable    bool
	Code         string
	ResponseCode int
	Err          error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Err.Error()
}

func New(destinations map[string]config.DestinationConfig) *Sender {
	return &Sender{
		client:       &http.Client{Timeout: 10 * time.Second},
		destinations: destinations,
	}
}

func (s *Sender) Type() domain.DestinationType {
	return domain.DestinationTelegram
}

func (s *Sender) Send(ctx context.Context, destinationID string, message domain.RenderedMessage) (sender.SendResult, error) {
	destination, ok := s.destinations[destinationID]
	if !ok || destination.ResolvedToken == "" || destination.ChatID == "" {
		return sender.SendResult{}, &Error{Code: "missing_destination", Err: fmt.Errorf("unknown telegram destination %q", destinationID)}
	}

	payload, err := json.Marshal(map[string]any{
		"chat_id":                  destination.ChatID,
		"text":                     string(message.Body),
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	if err != nil {
		return sender.SendResult{}, &Error{Code: "payload_encode_failed", Err: err}
	}

	baseURL := strings.TrimRight(destination.APIBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", baseURL, destination.ResolvedToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return sender.SendResult{}, &Error{Code: "request_build_failed", Err: err}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return sender.SendResult{}, &Error{Retryable: true, Code: "transport_error", Err: err}
	}
	defer resp.Body.Close()

	result := sender.SendResult{ResponseCode: resp.StatusCode}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return result, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	bodyText := strings.TrimSpace(string(body))
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		if bodyText != "" {
			return result, &Error{Retryable: true, Code: "upstream_retryable", ResponseCode: resp.StatusCode, Err: fmt.Errorf("telegram returned %d: %s", resp.StatusCode, bodyText)}
		}
		return result, &Error{Retryable: true, Code: "upstream_retryable", ResponseCode: resp.StatusCode, Err: fmt.Errorf("telegram returned %d", resp.StatusCode)}
	}
	if bodyText != "" {
		return result, &Error{Retryable: false, Code: "upstream_rejected", ResponseCode: resp.StatusCode, Err: fmt.Errorf("telegram returned %d: %s", resp.StatusCode, bodyText)}
	}
	return result, &Error{Retryable: false, Code: "upstream_rejected", ResponseCode: resp.StatusCode, Err: fmt.Errorf("telegram returned %d", resp.StatusCode)}
}
