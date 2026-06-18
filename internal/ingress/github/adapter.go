package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/ingress"
)

const (
	headerEvent     = "X-GitHub-Event"
	headerDelivery  = "X-GitHub-Delivery"
	headerSignature = "X-Hub-Signature-256"
	signaturePrefix = "sha256="
)

type Adapter struct{}

type payload struct {
	Action     string `json:"action"`
	Repository struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
	PullRequest struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		Merged  bool   `json:"merged"`
		Head    struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
	WorkflowRun struct {
		Name       string `json:"name"`
		HTMLURL    string `json:"html_url"`
		Conclusion string `json:"conclusion"`
		HeadBranch string `json:"head_branch"`
		HeadSHA    string `json:"head_sha"`
		RunNumber  int    `json:"run_number"`
	} `json:"workflow_run"`
	Release struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
	} `json:"release"`
}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Source() domain.Source {
	return domain.SourceGitHub
}

func (a *Adapter) Verify(_ context.Context, integration config.IntegrationConfig, req ingress.InboundRequest) error {
	signature := headerValue(req.Headers, headerSignature)
	if signature == "" || integration.ResolvedSecret == "" {
		return ingress.ErrUnauthorized
	}

	mac := hmac.New(sha256.New, []byte(integration.ResolvedSecret))
	mac.Write(req.RawBody)
	expected := signaturePrefix + hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(strings.ToLower(expected))) != 1 {
		return ingress.ErrUnauthorized
	}

	return nil
}

func (a *Adapter) Normalize(_ context.Context, integrationID string, _ config.IntegrationConfig, req ingress.InboundRequest) (ingress.AdapterResult, error) {
	var body payload
	if err := json.Unmarshal(req.RawBody, &body); err != nil {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	eventName := strings.TrimSpace(headerValue(req.Headers, headerEvent))
	deliveryID := strings.TrimSpace(headerValue(req.Headers, headerDelivery))
	if eventName == "" || deliveryID == "" {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	event, ignoreReason, err := normalizeEvent(integrationID, eventName, body, req.ReceivedAt)
	if err != nil {
		return ingress.AdapterResult{}, err
	}
	if ignoreReason != "" {
		return ingress.AdapterResult{
			SourceDeliveryID: deliveryID,
			SourceEventType:  eventName,
			IgnoreReason:     ignoreReason,
		}, nil
	}

	return ingress.AdapterResult{
		SourceDeliveryID: deliveryID,
		SourceEventType:  eventName,
		Events:           []domain.Event{event},
	}, nil
}

func normalizeEvent(integrationID, eventName string, body payload, receivedAt time.Time) (domain.Event, string, error) {
	base := domain.Event{
		Source:        domain.SourceGitHub,
		IntegrationID: integrationID,
		Actor:         body.Sender.Login,
		OccurredAt:    receivedAt.UTC(),
	}

	switch eventName {
	case "pull_request":
		return normalizePullRequest(base, body)
	case "workflow_run":
		return normalizeWorkflowRun(base, body)
	case "release":
		return normalizeRelease(base, body)
	default:
		return domain.Event{}, fmt.Sprintf("github event %q is not supported", eventName), nil
	}
}

func normalizePullRequest(base domain.Event, body payload) (domain.Event, string, error) {
	event := base
	event.CommitSHA = body.PullRequest.Head.SHA
	event.URL = firstNonEmpty(body.PullRequest.HTMLURL, body.Repository.HTMLURL)
	event.FieldsJSON = mustJSON(map[string]any{
		"repository":          body.Repository.FullName,
		"pull_request_number": body.PullRequest.Number,
		"head_branch":         body.PullRequest.Head.Ref,
		"base_branch":         body.PullRequest.Base.Ref,
	})
	event.Fingerprint = fmt.Sprintf("%s:pr:%d:%s", body.Repository.FullName, body.PullRequest.Number, body.Action)

	switch body.Action {
	case "opened":
		event.Type = "github.pull_request.opened"
		event.Action = body.Action
		event.Lifecycle = domain.LifecycleTriggered
		event.Severity = domain.SeverityInfo
		event.Title = fmt.Sprintf("Pull request opened: #%d %s", body.PullRequest.Number, body.PullRequest.Title)
		event.Summary = fmt.Sprintf("%s opened pull request #%d in %s", body.Sender.Login, body.PullRequest.Number, body.Repository.FullName)
	case "closed":
		if body.PullRequest.Merged {
			event.Type = "github.pull_request.merged"
			event.Action = "merged"
			event.Lifecycle = domain.LifecycleSucceeded
			event.Severity = domain.SeverityInfo
			event.Title = fmt.Sprintf("Pull request merged: #%d %s", body.PullRequest.Number, body.PullRequest.Title)
			event.Summary = fmt.Sprintf("%s merged pull request #%d in %s", body.Sender.Login, body.PullRequest.Number, body.Repository.FullName)
		} else {
			event.Type = "github.pull_request.closed"
			event.Action = body.Action
			event.Lifecycle = domain.LifecycleCancelled
			event.Severity = domain.SeverityInfo
			event.Title = fmt.Sprintf("Pull request closed: #%d %s", body.PullRequest.Number, body.PullRequest.Title)
			event.Summary = fmt.Sprintf("%s closed pull request #%d in %s", body.Sender.Login, body.PullRequest.Number, body.Repository.FullName)
		}
	default:
		return domain.Event{}, "github pull_request action is not routed in this slice", nil
	}

	return event, "", nil
}

func normalizeWorkflowRun(base domain.Event, body payload) (domain.Event, string, error) {
	if body.Action != "completed" {
		return domain.Event{}, "github workflow_run action is not routed in this slice", nil
	}

	event := base
	event.CommitSHA = body.WorkflowRun.HeadSHA
	event.URL = firstNonEmpty(body.WorkflowRun.HTMLURL, body.Repository.HTMLURL)
	event.FieldsJSON = mustJSON(map[string]any{
		"repository":    body.Repository.FullName,
		"workflow_name": body.WorkflowRun.Name,
		"head_branch":   body.WorkflowRun.HeadBranch,
		"run_number":    body.WorkflowRun.RunNumber,
		"conclusion":    body.WorkflowRun.Conclusion,
	})
	event.Fingerprint = fmt.Sprintf("%s:workflow:%s:%d", body.Repository.FullName, body.WorkflowRun.Name, body.WorkflowRun.RunNumber)

	switch body.WorkflowRun.Conclusion {
	case "success":
		event.Type = "github.workflow.succeeded"
		event.Action = "succeeded"
		event.Lifecycle = domain.LifecycleSucceeded
		event.Severity = domain.SeverityInfo
	case "failure", "timed_out", "startup_failure":
		event.Type = "github.workflow.failed"
		event.Action = "failed"
		event.Lifecycle = domain.LifecycleFailed
		event.Severity = domain.SeverityError
	case "cancelled":
		event.Type = "github.workflow.cancelled"
		event.Action = "cancelled"
		event.Lifecycle = domain.LifecycleCancelled
		event.Severity = domain.SeverityWarning
	default:
		return domain.Event{}, "github workflow_run conclusion is not routed in this slice", nil
	}

	event.Title = fmt.Sprintf("Workflow %s: %s", event.Action, body.WorkflowRun.Name)
	event.Summary = fmt.Sprintf("Workflow %s #%d %s in %s", body.WorkflowRun.Name, body.WorkflowRun.RunNumber, event.Action, body.Repository.FullName)
	return event, "", nil
}

func normalizeRelease(base domain.Event, body payload) (domain.Event, string, error) {
	if body.Action != "published" {
		return domain.Event{}, "github release action is not routed in this slice", nil
	}

	event := base
	event.Type = "github.release.published"
	event.Action = "published"
	event.Lifecycle = domain.LifecycleSucceeded
	event.Severity = domain.SeverityInfo
	event.Release = body.Release.TagName
	event.URL = firstNonEmpty(body.Release.HTMLURL, body.Repository.HTMLURL)
	event.Title = fmt.Sprintf("Release published: %s", body.Release.TagName)
	event.Summary = fmt.Sprintf("%s published release %s in %s", body.Sender.Login, body.Release.TagName, body.Repository.FullName)
	event.FieldsJSON = mustJSON(map[string]any{
		"repository":   body.Repository.FullName,
		"release_name": body.Release.Name,
	})
	event.Fingerprint = fmt.Sprintf("%s:release:%s", body.Repository.FullName, body.Release.TagName)
	return event, "", nil
}

func mustJSON(value map[string]any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func headerValue(headers http.Header, key string) string {
	if value := headers.Get(key); value != "" {
		return value
	}
	for headerKey, values := range headers {
		if strings.EqualFold(headerKey, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
