package teams

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/sender"
)

type Sender struct {
	client       *http.Client
	destinations *runtimeconfig.DestinationRegistry
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

func New(destinations *runtimeconfig.DestinationRegistry) *Sender {
	return &Sender{
		client:       &http.Client{Timeout: 10 * time.Second},
		destinations: destinations,
	}
}

func (s *Sender) Type() domain.DestinationType {
	return domain.DestinationTeams
}

func (s *Sender) Send(ctx context.Context, destinationID string, message domain.RenderedMessage) (sender.SendResult, error) {
	destination, ok := s.destinations.Get(destinationID)
	if !ok || destination.ResolvedURL == "" {
		return sender.SendResult{}, &Error{Code: "missing_destination", Err: fmt.Errorf("unknown teams destination %q", destinationID)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destination.ResolvedURL, strings.NewReader(string(message.Body)))
	if err != nil {
		return sender.SendResult{}, &Error{Code: "request_build_failed", Err: err}
	}
	req.Header.Set("Content-Type", message.ContentType)

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
			return result, &Error{Retryable: true, Code: "upstream_retryable", ResponseCode: resp.StatusCode, Err: fmt.Errorf("teams returned %d: %s", resp.StatusCode, bodyText)}
		}
		return result, &Error{Retryable: true, Code: "upstream_retryable", ResponseCode: resp.StatusCode, Err: fmt.Errorf("teams returned %d", resp.StatusCode)}
	}
	if bodyText != "" {
		return result, &Error{Retryable: false, Code: "upstream_rejected", ResponseCode: resp.StatusCode, Err: fmt.Errorf("teams returned %d: %s", resp.StatusCode, bodyText)}
	}
	return result, &Error{Retryable: false, Code: "upstream_rejected", ResponseCode: resp.StatusCode, Err: fmt.Errorf("teams returned %d", resp.StatusCode)}
}
