// SPDX-License-Identifier: MIT
// Purpose: GitHubSync converts between GitHub issues and todos (issue #324).
// ImportIssues turns GitHub issues into todos; ExportTodos turns todos back
// into the GitHub issue shape. Label/priority mapping is bidirectional.
package todo

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// GitHubIssue is the subset of a GitHub issue the sync layer cares about.
type GitHubIssue struct {
	Number   int      `json:"number"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Labels   []string `json:"labels"`
	State    string   `json:"state"`
	Assignee string   `json:"assignee"`
}

// GitHubSync converts between GitHub issues and todos. All methods are safe
// for concurrent use (M7).
type GitHubSync struct {
	mu sync.RWMutex
}

// NewGitHubSync creates a new sync instance.
func NewGitHubSync() *GitHubSync {
	return &GitHubSync{}
}

// ImportIssues converts GitHub issues into todos. Issue numbers are stored in
// ExternalRef as "issue:<n>". Labels are mapped to priority/type via MapLabel.
func (s *GitHubSync) ImportIssues(issues []GitHubIssue) []*Todo {
	todos := make([]*Todo, 0, len(issues))
	for _, iss := range issues {
		td := &Todo{
			Title:       iss.Title,
			Description: iss.Body,
			ExternalRef: fmt.Sprintf("issue:%d", iss.Number),
			Assignee:    iss.Assignee,
			Priority:    PriorityP2,
			Type:        TypeTask,
		}
		if strings.EqualFold(iss.State, "closed") {
			td.Status = StatusDone
		} else {
			td.Status = StatusOpen
		}
		for _, label := range iss.Labels {
			mapped := s.MapLabel(label)
			if Priority(mapped).Valid() {
				td.Priority = Priority(mapped)
			} else if TodoType(mapped).Valid() {
				td.Type = TodoType(mapped)
			} else if mapped != label {
				td.Tags = append(td.Tags, mapped)
			} else {
				td.Tags = append(td.Tags, label)
			}
		}
		td.Tags = normalizeTags(td.Tags)
		todos = append(todos, td)
	}
	return todos
}

// ExportTodos converts todos into GitHub issue format. Priority is expanded to
// labels via MapPriority; type and tags are added as labels.
func (s *GitHubSync) ExportTodos(todos []*Todo) []GitHubIssue {
	issues := make([]GitHubIssue, 0, len(todos))
	for _, td := range todos {
		iss := GitHubIssue{
			Title:    td.Title,
			Body:     td.Description,
			Assignee: td.Assignee,
			Number:   extractIssueNumber(td.ExternalRef),
		}
		if td.IsClosed() {
			iss.State = "closed"
		} else {
			iss.State = "open"
		}
		labels := s.MapPriority(string(td.Priority))
		if td.Type != "" {
			labels = append(labels, string(td.Type))
		}
		labels = append(labels, td.Tags...)
		iss.Labels = labels
		issues = append(issues, iss)
	}
	return issues
}

// MapLabel maps a GitHub label to a todo priority (P0-P3), todo type
// (task/bug/feature/...), or returns the label unchanged when no mapping
// exists.
func (s *GitHubSync) MapLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "bug", "defect":
		return string(TypeBug)
	case "enhancement", "feature":
		return string(TypeFeature)
	case "documentation", "docs":
		return string(TypeChore)
	case "question", "help wanted":
		return string(TypeQuestion)
	case "epic":
		return string(TypeEpic)
	case "critical", "urgent", "p0":
		return string(PriorityP0)
	case "high", "p1":
		return string(PriorityP1)
	case "medium", "p2":
		return string(PriorityP2)
	case "low", "p3":
		return string(PriorityP3)
	default:
		return label
	}
}

// MapPriority maps a todo priority string to a set of GitHub labels.
func (s *GitHubSync) MapPriority(priority string) []string {
	switch Priority(priority) {
	case PriorityP0:
		return []string{"P0", "critical"}
	case PriorityP1:
		return []string{"P1", "high"}
	case PriorityP2:
		return []string{"P2", "medium"}
	case PriorityP3:
		return []string{"P3", "low"}
	default:
		return nil
	}
}

// extractIssueNumber parses the issue number from an ExternalRef like
// "issue:42" or a GitHub URL ending in /issues/42. Returns 0 when no number
// is found.
func extractIssueNumber(ref string) int {
	if strings.HasPrefix(ref, "issue:") {
		n, _ := strconv.Atoi(strings.TrimPrefix(ref, "issue:"))
		return n
	}
	if idx := strings.LastIndex(ref, "/issues/"); idx >= 0 {
		n, _ := strconv.Atoi(ref[idx+len("/issues/"):])
		return n
	}
	return 0
}
