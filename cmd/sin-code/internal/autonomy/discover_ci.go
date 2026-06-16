// SPDX-License-Identifier: MIT
// Purpose: CI failure discovery (loop-008) — poll GitHub check runs for the
// branch HEAD and turn each failing/timed-out run into a high-priority fix
// goal, so the loop self-heals CI without a human closing the feedback loop.
package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type ghCheckRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	Output     struct {
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Text    string `json:"text"`
	} `json:"output"`
}

// scanCICheckRuns polls the check runs on the branch HEAD and enqueues a fix
// goal per failing run. Fails silently on non-GitHub repos or without a token.
func scanCICheckRuns(ctx context.Context, cfg DiscoverConfig, add func(Finding) bool) error {
	owner, repo := resolveOwnerRepo(cfg)
	if owner == "" || repo == "" {
		return nil
	}
	token := firstNonEmpty(cfg.GitHubToken, os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN"))

	branch := cfg.CIBranch
	if branch == "" {
		b, err := currentGitBranch(cfg.Workspace)
		if err != nil || strings.TrimSpace(b) == "" {
			return nil
		}
		branch = strings.TrimSpace(b)
	}

	max := cfg.CIMaxFailures
	if max <= 0 {
		max = 5
	}

	endpoint := fmt.Sprintf("%s/repos/%s/%s/commits/%s/check-runs?per_page=50",
		githubAPIBase, owner, repo, branch)
	body, status, err := githubGet(ctx, endpoint, token)
	if err != nil {
		return fmt.Errorf("ci checks fetch: %w", err)
	}
	if status != http.StatusOK {
		return nil
	}
	var result struct {
		CheckRuns []ghCheckRun `json:"check_runs"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil
	}

	n := 0
	for _, cr := range result.CheckRuns {
		if cr.Status != "completed" {
			continue
		}
		if cr.Conclusion != "failure" && cr.Conclusion != "timed_out" {
			continue
		}
		n++
		if n > max {
			break
		}
		details := firstNonEmpty(cr.Output.Text, cr.Output.Summary, cr.Output.Title)
		if len(details) > 2000 {
			details = details[:2000] + "..."
		}
		prompt := fmt.Sprintf(
			"Fix the failing CI check %q on branch %q in %s/%s.\n\n"+
				"Check URL: %s\n\nFailure output:\n%s\n\n"+
				"When done: the check passes, all other tests still pass, and the fix "+
				"is committed.",
			cr.Name, branch, owner, repo, cr.HTMLURL, details)

		if !add(Finding{
			Source:   "ci_check",
			Prompt:   prompt,
			DedupKey: fmt.Sprintf("ci_check:%s/%s:%d", owner, repo, cr.ID),
			Priority: 8, // CI failures are high priority (above normal, below explicit goals)
		}) {
			return nil
		}
	}
	return nil
}

// currentGitBranch returns the current branch name for workspace.
func currentGitBranch(workspace string) (string, error) {
	out, err := runCmd(workspace, "git", "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}
