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
			Fields:         pullRequestFields(),
		},
		{
			Key:            eventcatalog.Key(EventPullRequestMerged),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub pull request merged event.",
			Fields:         pullRequestFields(),
		},
		{
			Key:            eventcatalog.Key(EventPullRequestClosed),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub pull request closed without merge.",
			Fields:         pullRequestFields(),
		},
		{
			Key:            eventcatalog.Key(EventWorkflowSucceeded),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub workflow run completed successfully.",
			Fields:         workflowFields(),
		},
		{
			Key:            eventcatalog.Key(EventWorkflowFailed),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub workflow run failed.",
			Fields:         workflowFields(),
		},
		{
			Key:            eventcatalog.Key(EventWorkflowCancelled),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub workflow run was cancelled.",
			Fields:         workflowFields(),
		},
		{
			Key:            eventcatalog.Key(EventReleasePublished),
			Source:         domain.SourceGitHub,
			PayloadVersion: PayloadVersionV1,
			Summary:        "GitHub release published event.",
			Fields:         releaseFields(),
		},
	}
}

func baseFields() []eventcatalog.Field {
	return []eventcatalog.Field{
		{Path: "repository", Type: "object", Required: true, Description: "Repository identity and canonical URLs."},
		{Path: "repository.full_name", Type: "string", Required: true, Description: "Repository full name."},
		{Path: "repository.html_url", Type: "string", Required: true, Description: "Repository URL."},
		{Path: "sender", Type: "object", Description: "Actor identity from the webhook."},
		{Path: "sender.login", Type: "string", Description: "Actor login."},
	}
}

func pullRequestFields() []eventcatalog.Field {
	return append(baseFields(),
		eventcatalog.Field{Path: "pull_request", Type: "object", Required: true, Description: "Pull request number, title, branches, and status."},
		eventcatalog.Field{Path: "pull_request.number", Type: "number", Required: true, Description: "Pull request number."},
		eventcatalog.Field{Path: "pull_request.title", Type: "string", Required: true, Description: "Pull request title."},
		eventcatalog.Field{Path: "pull_request.html_url", Type: "string", Required: true, Description: "Pull request URL."},
		eventcatalog.Field{Path: "pull_request.merged", Type: "boolean", Description: "Whether the pull request was merged."},
		eventcatalog.Field{Path: "pull_request.head_branch", Type: "string", Description: "Head branch."},
		eventcatalog.Field{Path: "pull_request.head_sha", Type: "string", Description: "Head SHA."},
		eventcatalog.Field{Path: "pull_request.base_branch", Type: "string", Description: "Base branch."},
		eventcatalog.Field{Path: "pull_request.action", Type: "string", Description: "Pull request action."},
	)
}

func workflowFields() []eventcatalog.Field {
	return append(baseFields(),
		eventcatalog.Field{Path: "workflow_run", Type: "object", Required: true, Description: "Workflow run metadata."},
		eventcatalog.Field{Path: "workflow_run.name", Type: "string", Required: true, Description: "Workflow name."},
		eventcatalog.Field{Path: "workflow_run.html_url", Type: "string", Required: true, Description: "Workflow run URL."},
		eventcatalog.Field{Path: "workflow_run.conclusion", Type: "string", Required: true, Description: "Workflow conclusion."},
		eventcatalog.Field{Path: "workflow_run.head_branch", Type: "string", Description: "Head branch."},
		eventcatalog.Field{Path: "workflow_run.head_sha", Type: "string", Description: "Head SHA."},
		eventcatalog.Field{Path: "workflow_run.run_number", Type: "number", Description: "Run number."},
		eventcatalog.Field{Path: "workflow_run.action", Type: "string", Description: "Workflow action."},
	)
}

func releaseFields() []eventcatalog.Field {
	return append(baseFields(),
		eventcatalog.Field{Path: "release", Type: "object", Required: true, Description: "Release metadata."},
		eventcatalog.Field{Path: "release.tag_name", Type: "string", Required: true, Description: "Release tag name."},
		eventcatalog.Field{Path: "release.name", Type: "string", Description: "Release display name."},
		eventcatalog.Field{Path: "release.html_url", Type: "string", Required: true, Description: "Release URL."},
	)
}
