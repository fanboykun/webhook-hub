package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	ghcatalog "github.com/fanboykun/webhook-hub/internal/eventcatalog/github"
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

	event, ignoreReason, err := ghcatalog.Project(ghcatalog.ProjectionInput{
		IntegrationID: integrationID,
		ReceivedAt:    req.ReceivedAt,
		DeliveryID:    deliveryID,
		EventName:     eventName,
		Action:        body.Action,
		Repository: ghcatalog.Repository{
			FullName: body.Repository.FullName,
			HTMLURL:  body.Repository.HTMLURL,
		},
		Sender: ghcatalog.Sender{
			Login: body.Sender.Login,
		},
		PullRequest: ghcatalog.PullRequest{
			Number:     body.PullRequest.Number,
			Title:      body.PullRequest.Title,
			HTMLURL:    body.PullRequest.HTMLURL,
			Merged:     body.PullRequest.Merged,
			HeadBranch: body.PullRequest.Head.Ref,
			HeadSHA:    body.PullRequest.Head.SHA,
			BaseBranch: body.PullRequest.Base.Ref,
			Action:     body.Action,
		},
		WorkflowRun: ghcatalog.WorkflowRun{
			Name:       body.WorkflowRun.Name,
			HTMLURL:    body.WorkflowRun.HTMLURL,
			Conclusion: body.WorkflowRun.Conclusion,
			HeadBranch: body.WorkflowRun.HeadBranch,
			HeadSHA:    body.WorkflowRun.HeadSHA,
			RunNumber:  body.WorkflowRun.RunNumber,
			Action:     body.Action,
		},
		Release: ghcatalog.Release{
			TagName: body.Release.TagName,
			Name:    body.Release.Name,
			HTMLURL: body.Release.HTMLURL,
		},
	})
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
		Events:           []domain.EventCandidate{event},
	}, nil
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
