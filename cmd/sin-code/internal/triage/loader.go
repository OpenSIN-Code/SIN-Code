// SPDX-License-Identifier: MIT
// Purpose: gh-bridge wrapper for the triage CLI. Calls
// `gh issue list --json ...` once, parses the result into []triage.Issue.
// The wrapper hides the ghbridge.Tier machinery from the renderer
// (we only ever call read-only commands, but we go through the bridge
// to be future-proof and to keep one auth path).
package triage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
)

// Loader fetches open issues. The function is a variable so tests
// can inject a fixture without spawning gh.
var Loader = loadFromGH

// ghExec is a package-level hook for the ghbridge call so the
// loadFromGH error and parsing paths can be exercised without a live
// `gh` binary.
var ghExec = ghbridge.New().Execute

// loadFromGH runs `gh issue list --state open --json ...` via the
// bridge. The JSON fields are deliberately a strict subset of what
// gh can return — see the Issue struct for what we need.
func loadFromGH(ctx context.Context, repo string) ([]Issue, error) {
	args := []string{
		"issue", "list",
		"--state", "open",
		"--limit", "200",
		"--json",
		"number,title,body,state,author,labels,updatedAt,createdAt,url",
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, _, err := ghExec(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("gh issue list: %w", err)
	}
	var raw []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		State     string `json:"state"`
		Author    struct {
			Login string `json:"login"`
		} `json:"author"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		UpdatedAt string `json:"updatedAt"`
		CreatedAt string `json:"createdAt"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("gh issue list: parse json: %w", err)
	}
	issues := make([]Issue, 0, len(raw))
	for _, r := range raw {
		labels := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			labels = append(labels, l.Name)
		}
		issues = append(issues, Issue{
			Number:    r.Number,
			Title:     r.Title,
			Body:      r.Body,
			State:     r.State,
			Author:    r.Author.Login,
			Labels:    labels,
			UpdatedAt: r.UpdatedAt,
			CreatedAt: r.CreatedAt,
			URL:       r.URL,
		})
	}
	return issues, nil
}
