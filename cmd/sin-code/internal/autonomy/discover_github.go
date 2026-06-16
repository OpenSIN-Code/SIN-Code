// SPDX-License-Identifier: MIT
// Purpose: GitHub-native backlog discovery (loop-003) — drain open issues into
// deduplicated goals so the daemon works the real backlog, not just local
// TODO markers. Uses the GitHub REST API directly (no SDK dependency, M2).
package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// githubAPIBase is the REST API root. Overridable in tests to point at a mock
// server.
var githubAPIBase = "https://api.github.com"

type ghLabel struct {
	Name string `json:"name"`
}

type ghIssue struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PullRequest *struct{} `json:"pull_request,omitempty"` // present => it's a PR, skip
	Labels      []ghLabel `json:"labels"`
}

// scanGitHubIssues fetches open issues by label and turns each into a goal.
// It fails silently (returns nil) when the repo is not a GitHub repo or no
// token is available — local discovery still proceeds.
func scanGitHubIssues(ctx context.Context, cfg DiscoverConfig, add func(Finding) bool) error {
	owner, repo := resolveOwnerRepo(cfg)
	if owner == "" || repo == "" {
		return nil // not a GitHub repo; skip silently
	}
	token := firstNonEmpty(cfg.GitHubToken, os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN"))

	labels := cfg.GitHubLabels
	if len(labels) == 0 {
		labels = []string{"bug", "help wanted", "good first issue"}
	}
	maxIssues := cfg.GitHubMaxIssues
	if maxIssues <= 0 {
		maxIssues = 20
	}

	endpoint := fmt.Sprintf("%s/repos/%s/%s/issues?state=open&labels=%s&per_page=%d",
		githubAPIBase, owner, repo, url.QueryEscape(strings.Join(labels, ",")), maxIssues)

	body, status, err := githubGet(ctx, endpoint, token)
	if err != nil {
		return fmt.Errorf("github issues fetch: %w", err)
	}
	if status == http.StatusNotFound || status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil // repo not found, or no/insufficient token — skip silently
	}
	var issues []ghIssue
	if err := json.Unmarshal(body, &issues); err != nil {
		return nil
	}

	for _, iss := range issues {
		if iss.PullRequest != nil {
			continue // the issues endpoint includes PRs; we only want issues
		}
		var labelStr strings.Builder
		for _, l := range iss.Labels {
			fmt.Fprintf(&labelStr, " [%s]", l.Name)
		}
		prompt := fmt.Sprintf(
			"Resolve GitHub Issue #%d in %s/%s:%s\n\nTitle: %s\n\n%s\n\n"+
				"When done: the issue's acceptance criteria are satisfied, all tests "+
				"pass, and the code is merge-ready.",
			iss.Number, owner, repo, labelStr.String(), iss.Title, iss.Body)

		if !add(Finding{
			Source:   "github_issue",
			Prompt:   prompt,
			DedupKey: fmt.Sprintf("gh_issue:%s/%s:%d", owner, repo, iss.Number),
			Priority: issuePriority(iss.Labels),
		}) {
			return nil
		}
	}
	return nil
}

// issuePriority maps issue labels to a queue priority.
func issuePriority(labels []ghLabel) int {
	for _, l := range labels {
		switch strings.ToLower(l.Name) {
		case "critical", "p0":
			return 10
		case "bug", "p1":
			return 5
		case "help wanted", "good first issue":
			return 2
		}
	}
	return 0
}

// resolveOwnerRepo returns the configured owner/repo, falling back to the git
// remote when either is empty.
func resolveOwnerRepo(cfg DiscoverConfig) (owner, repo string) {
	owner, repo = cfg.GitHubOwner, cfg.GitHubRepo
	if owner != "" && repo != "" {
		return owner, repo
	}
	o, r, err := detectGitHubOwnerRepo(cfg.Workspace)
	if err != nil {
		return owner, repo
	}
	if owner == "" {
		owner = o
	}
	if repo == "" {
		repo = r
	}
	return owner, repo
}

// detectGitHubOwnerRepo extracts owner/repo from the origin remote URL,
// handling both https and ssh forms.
func detectGitHubOwnerRepo(workspace string) (owner, repo string, err error) {
	out, err := runCmd(workspace, "git", "remote", "get-url", "origin")
	if err != nil {
		return "", "", err
	}
	return parseGitHubRemote(out)
}

// parseGitHubRemote parses owner/repo out of a github remote URL string.
func parseGitHubRemote(s string) (owner, repo string, err error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".git")
	for _, sep := range []string{"github.com/", "github.com:"} {
		if i := strings.Index(s, sep); i >= 0 {
			parts := strings.SplitN(s[i+len(sep):], "/", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				return parts[0], parts[1], nil
			}
		}
	}
	return "", "", nil
}

// githubGet performs an authenticated GET and returns the body and status.
func githubGet(ctx context.Context, endpoint, token string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode, nil
}
