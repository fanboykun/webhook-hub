package github

import (
	"encoding/json"
	"fmt"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

func Project(input ProjectionInput) (domain.EventCandidate, string, error) {
	base := domain.EventCandidate{
		EventEnvelope: domain.EventEnvelope{
			Source:         domain.SourceGitHub,
			IntegrationID:  input.IntegrationID,
			OccurredAt:     input.ReceivedAt.UTC(),
			PayloadVersion: PayloadVersionV1,
		},
	}

	switch input.EventName {
	case "pull_request":
		return projectPullRequest(base, input)
	case "workflow_run":
		return projectWorkflowRun(base, input)
	case "release":
		return projectRelease(base, input)
	default:
		return domain.EventCandidate{}, fmt.Sprintf("github event %q is not supported", input.EventName), nil
	}
}

func projectPullRequest(base domain.EventCandidate, input ProjectionInput) (domain.EventCandidate, string, error) {
	event := base
	event.SourceEventID = fmt.Sprintf("%d", input.PullRequest.Number)
	event.SourceURL = firstNonEmpty(input.PullRequest.HTMLURL, input.Repository.HTMLURL)
	event.Scope = domain.EventScope{Service: input.Repository.FullName}
	event.MetadataJSON = mustJSON(map[string]any{
		"repository":          input.Repository.FullName,
		"pull_request_number": input.PullRequest.Number,
		"head_branch":         input.PullRequest.HeadBranch,
		"base_branch":         input.PullRequest.BaseBranch,
		"actor":               input.Sender.Login,
		"commit_sha":          input.PullRequest.HeadSHA,
	})
	event.PayloadJSON = mustJSON(struct {
		Repository  Repository  `json:"repository"`
		PullRequest PullRequest `json:"pull_request"`
		Sender      Sender      `json:"sender"`
	}{
		Repository:  input.Repository,
		PullRequest: input.PullRequest,
		Sender:      input.Sender,
	})
	event.Fingerprint = fmt.Sprintf("%s:pr:%d:%s", input.Repository.FullName, input.PullRequest.Number, input.Action)

	switch input.Action {
	case "opened":
		event.Key = string(EventPullRequestOpened)
		event.Action = input.Action
		event.Lifecycle = domain.LifecycleTriggered
		event.Severity = domain.SeverityInfo
		event.Title = fmt.Sprintf("Pull request opened: #%d %s", input.PullRequest.Number, input.PullRequest.Title)
		event.Summary = fmt.Sprintf("%s opened pull request #%d in %s", input.Sender.Login, input.PullRequest.Number, input.Repository.FullName)
	case "closed":
		if input.PullRequest.Merged {
			event.Key = string(EventPullRequestMerged)
			event.Action = "merged"
			event.Lifecycle = domain.LifecycleSucceeded
			event.Severity = domain.SeverityInfo
			event.Title = fmt.Sprintf("Pull request merged: #%d %s", input.PullRequest.Number, input.PullRequest.Title)
			event.Summary = fmt.Sprintf("%s merged pull request #%d in %s", input.Sender.Login, input.PullRequest.Number, input.Repository.FullName)
		} else {
			event.Key = string(EventPullRequestClosed)
			event.Action = input.Action
			event.Lifecycle = domain.LifecycleCancelled
			event.Severity = domain.SeverityInfo
			event.Title = fmt.Sprintf("Pull request closed: #%d %s", input.PullRequest.Number, input.PullRequest.Title)
			event.Summary = fmt.Sprintf("%s closed pull request #%d in %s", input.Sender.Login, input.PullRequest.Number, input.Repository.FullName)
		}
	default:
		return domain.EventCandidate{}, "github pull_request action is not routed in this slice", nil
	}

	return event, "", nil
}

func projectWorkflowRun(base domain.EventCandidate, input ProjectionInput) (domain.EventCandidate, string, error) {
	if input.Action != "completed" {
		return domain.EventCandidate{}, "github workflow_run action is not routed in this slice", nil
	}

	event := base
	event.SourceEventID = fmt.Sprintf("%s:%d", input.WorkflowRun.Name, input.WorkflowRun.RunNumber)
	event.SourceURL = firstNonEmpty(input.WorkflowRun.HTMLURL, input.Repository.HTMLURL)
	event.Scope = domain.EventScope{Service: input.Repository.FullName}
	event.MetadataJSON = mustJSON(map[string]any{
		"repository":    input.Repository.FullName,
		"workflow_name": input.WorkflowRun.Name,
		"head_branch":   input.WorkflowRun.HeadBranch,
		"run_number":    input.WorkflowRun.RunNumber,
		"conclusion":    input.WorkflowRun.Conclusion,
		"actor":         input.Sender.Login,
		"commit_sha":    input.WorkflowRun.HeadSHA,
	})
	event.PayloadJSON = mustJSON(struct {
		Repository  Repository  `json:"repository"`
		WorkflowRun WorkflowRun `json:"workflow_run"`
		Sender      Sender      `json:"sender"`
	}{
		Repository:  input.Repository,
		WorkflowRun: input.WorkflowRun,
		Sender:      input.Sender,
	})
	event.Fingerprint = fmt.Sprintf("%s:workflow:%s:%d", input.Repository.FullName, input.WorkflowRun.Name, input.WorkflowRun.RunNumber)

	switch input.WorkflowRun.Conclusion {
	case "success":
		event.Key = string(EventWorkflowSucceeded)
		event.Action = "succeeded"
		event.Lifecycle = domain.LifecycleSucceeded
		event.Severity = domain.SeverityInfo
	case "failure", "timed_out", "startup_failure":
		event.Key = string(EventWorkflowFailed)
		event.Action = "failed"
		event.Lifecycle = domain.LifecycleFailed
		event.Severity = domain.SeverityError
	case "cancelled":
		event.Key = string(EventWorkflowCancelled)
		event.Action = "cancelled"
		event.Lifecycle = domain.LifecycleCancelled
		event.Severity = domain.SeverityWarning
	default:
		return domain.EventCandidate{}, "github workflow_run conclusion is not routed in this slice", nil
	}

	event.Title = fmt.Sprintf("Workflow %s: %s", event.Action, input.WorkflowRun.Name)
	event.Summary = fmt.Sprintf("Workflow %s #%d %s in %s", input.WorkflowRun.Name, input.WorkflowRun.RunNumber, event.Action, input.Repository.FullName)
	return event, "", nil
}

func projectRelease(base domain.EventCandidate, input ProjectionInput) (domain.EventCandidate, string, error) {
	if input.Action != "published" {
		return domain.EventCandidate{}, "github release action is not routed in this slice", nil
	}

	event := base
	event.SourceEventID = input.Release.TagName
	event.Key = string(EventReleasePublished)
	event.Action = "published"
	event.Lifecycle = domain.LifecycleSucceeded
	event.Severity = domain.SeverityInfo
	event.SourceURL = firstNonEmpty(input.Release.HTMLURL, input.Repository.HTMLURL)
	event.Scope = domain.EventScope{Service: input.Repository.FullName}
	event.Title = fmt.Sprintf("Release published: %s", input.Release.TagName)
	event.Summary = fmt.Sprintf("%s published release %s in %s", input.Sender.Login, input.Release.TagName, input.Repository.FullName)
	event.MetadataJSON = mustJSON(map[string]any{
		"repository":   input.Repository.FullName,
		"release_name": input.Release.Name,
		"release":      input.Release.TagName,
		"release_tag":  input.Release.TagName,
		"actor":        input.Sender.Login,
	})
	event.PayloadJSON = mustJSON(struct {
		Repository Repository `json:"repository"`
		Release    Release    `json:"release"`
		Sender     Sender     `json:"sender"`
	}{
		Repository: input.Repository,
		Release:    input.Release,
		Sender:     input.Sender,
	})
	event.Fingerprint = fmt.Sprintf("%s:release:%s", input.Repository.FullName, input.Release.TagName)
	return event, "", nil
}

func mustJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
