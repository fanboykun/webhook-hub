package github

import (
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/eventcatalog"
)

const (
	EventPullRequestOpened Key = "github.pull_request.opened"
	EventPullRequestMerged Key = "github.pull_request.merged"
	EventPullRequestClosed Key = "github.pull_request.closed"
	EventWorkflowSucceeded Key = "github.workflow.succeeded"
	EventWorkflowFailed    Key = "github.workflow.failed"
	EventWorkflowCancelled Key = "github.workflow.cancelled"
	EventReleasePublished  Key = "github.release.published"
	PayloadVersionV1           = 1
)

type Key string

type Repository struct {
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

type Sender struct {
	Login string `json:"login"`
}

type PullRequest struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	HTMLURL    string `json:"html_url"`
	Merged     bool   `json:"merged"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	BaseBranch string `json:"base_branch"`
	Action     string `json:"action"`
}

type WorkflowRun struct {
	Name       string `json:"name"`
	HTMLURL    string `json:"html_url"`
	Conclusion string `json:"conclusion"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	RunNumber  int    `json:"run_number"`
	Action     string `json:"action"`
}

type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url"`
}

type ProjectionInput struct {
	IntegrationID string
	ReceivedAt    time.Time
	DeliveryID    string
	EventName     string
	Action        string
	Repository    Repository
	Sender        Sender
	PullRequest   PullRequest
	WorkflowRun   WorkflowRun
	Release       Release
}

func Definitions() []eventcatalog.Definition {
	return []eventcatalog.Definition{
		{
			Key:            eventcatalog.Key(EventPullRequestOpened),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub pull request opened event.",
			TemplatePaths:  []string{".Payload.repository", ".Payload.pull_request", ".Payload.sender"},
			Fields: []eventcatalog.Field{
				{Path: "repository", Type: "object", Required: true, Description: "Repository identity and canonical URLs."},
				{Path: "pull_request", Type: "object", Required: true, Description: "Pull request number, title, branches, and status."},
				{Path: "sender", Type: "object", Description: "Actor identity from the webhook."},
			},
		},
		{
			Key:            eventcatalog.Key(EventPullRequestMerged),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub pull request merged event.",
			TemplatePaths:  []string{".Payload.repository", ".Payload.pull_request", ".Payload.sender"},
			Fields: []eventcatalog.Field{
				{Path: "repository", Type: "object", Required: true},
				{Path: "pull_request", Type: "object", Required: true},
				{Path: "sender", Type: "object"},
			},
		},
		{
			Key:            eventcatalog.Key(EventPullRequestClosed),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub pull request closed without merge.",
			TemplatePaths:  []string{".Payload.repository", ".Payload.pull_request", ".Payload.sender"},
			Fields: []eventcatalog.Field{
				{Path: "repository", Type: "object", Required: true},
				{Path: "pull_request", Type: "object", Required: true},
				{Path: "sender", Type: "object"},
			},
		},
		{
			Key:            eventcatalog.Key(EventWorkflowSucceeded),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub workflow run completed successfully.",
			TemplatePaths:  []string{".Payload.repository", ".Payload.workflow_run", ".Payload.sender"},
			Fields: []eventcatalog.Field{
				{Path: "repository", Type: "object", Required: true},
				{Path: "workflow_run", Type: "object", Required: true},
				{Path: "sender", Type: "object"},
			},
		},
		{
			Key:            eventcatalog.Key(EventWorkflowFailed),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub workflow run failed.",
			TemplatePaths:  []string{".Payload.repository", ".Payload.workflow_run", ".Payload.sender"},
			Fields: []eventcatalog.Field{
				{Path: "repository", Type: "object", Required: true},
				{Path: "workflow_run", Type: "object", Required: true},
				{Path: "sender", Type: "object"},
			},
		},
		{
			Key:            eventcatalog.Key(EventWorkflowCancelled),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub workflow run was cancelled.",
			TemplatePaths:  []string{".Payload.repository", ".Payload.workflow_run", ".Payload.sender"},
			Fields: []eventcatalog.Field{
				{Path: "repository", Type: "object", Required: true},
				{Path: "workflow_run", Type: "object", Required: true},
				{Path: "sender", Type: "object"},
			},
		},
		{
			Key:            eventcatalog.Key(EventReleasePublished),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub release published event.",
			TemplatePaths:  []string{".Payload.repository", ".Payload.release", ".Payload.sender"},
			Fields: []eventcatalog.Field{
				{Path: "repository", Type: "object", Required: true},
				{Path: "release", Type: "object", Required: true},
				{Path: "sender", Type: "object"},
			},
		},
	}
}
