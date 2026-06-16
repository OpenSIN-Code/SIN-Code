// SPDX-License-Identifier: MIT
// Purpose: autonomous git commit/push/PR for verified goals (loop-007) so the
// loop never stops at the mechanical `git add && commit && push` step. Safe by
// default: no-op on a clean tree, refuses to push to main/master without an
// explicit opt-in, and every failure is the caller's to treat as non-fatal.
package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CommitOptions configures one AutoCommit call.
type CommitOptions struct {
	Workspace  string
	Message    string
	Author     string // "Name <email>" — default: SIN-Code Agent <agent@sin-code.local>
	PushRemote string // "origin" when non-empty triggers a push
	PushBranch string // current branch when empty
	CreatePR   bool   // open a GitHub PR after push (requires GH_TOKEN)
	PRTitle    string
	PRBody     string
	PRBase     string // base branch for the PR (default: main)
}

// AutoCommit stages all changes, commits, and optionally pushes + opens a PR.
// It is a no-op (returns nil) when the working tree is clean.
func AutoCommit(ctx context.Context, opts CommitOptions) error {
	if opts.Workspace == "" {
		return fmt.Errorf("gitops: workspace must be set")
	}
	if opts.Author == "" {
		opts.Author = "SIN-Code Agent <agent@sin-code.local>"
	}

	status, err := runGit(ctx, opts.Workspace, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("gitops: status: %w", err)
	}
	if strings.TrimSpace(status) == "" {
		return nil // nothing to commit
	}

	if _, err := runGit(ctx, opts.Workspace, "add", "-A"); err != nil {
		return fmt.Errorf("gitops: add: %w", err)
	}

	msg := opts.Message
	if msg == "" {
		msg = "chore(agent): autonomous goal completion [sin-code]"
	}
	if _, err := runGit(ctx, opts.Workspace,
		"commit", "--author="+opts.Author, "-m", msg, "--no-verify"); err != nil {
		return fmt.Errorf("gitops: commit: %w", err)
	}
	fmt.Fprintf(os.Stderr, "gitops: committed: %s\n", truncate(firstLine(msg), 72))

	if opts.PushRemote == "" {
		return nil
	}

	branch := opts.PushBranch
	if branch == "" {
		branch, err = currentBranch(ctx, opts.Workspace)
		if err != nil {
			return fmt.Errorf("gitops: current branch: %w", err)
		}
	}
	if (branch == "main" || branch == "master") && !isMainPushAllowed() {
		return fmt.Errorf("gitops: refusing to push directly to %s; "+
			"set SIN_ALLOW_MAIN_PUSH=1 or use a feature branch", branch)
	}
	if _, err := runGit(ctx, opts.Workspace, "push", opts.PushRemote, branch); err != nil {
		return fmt.Errorf("gitops: push: %w", err)
	}
	fmt.Fprintf(os.Stderr, "gitops: pushed %s -> %s/%s\n", branch, opts.PushRemote, branch)

	if opts.CreatePR && os.Getenv("GH_TOKEN") != "" {
		if err := openPR(ctx, opts, branch); err != nil {
			return fmt.Errorf("gitops: open PR: %w", err)
		}
	}
	return nil
}

// githubAPIBase is the REST root, overridable in tests.
var githubAPIBase = "https://api.github.com"

// openPR opens a GitHub pull request from branch into PRBase (default main).
func openPR(ctx context.Context, opts CommitOptions, branch string) error {
	owner, repo, err := detectOwnerRepo(ctx, opts.Workspace)
	if err != nil || owner == "" || repo == "" {
		return fmt.Errorf("could not determine owner/repo: %w", err)
	}
	base := opts.PRBase
	if base == "" {
		base = "main"
	}
	title := opts.PRTitle
	if title == "" {
		title = firstLine(opts.Message)
	}
	payload := map[string]string{
		"title": title,
		"head":  branch,
		"base":  base,
		"body":  opts.PRBody,
	}
	buf, _ := json.Marshal(payload)
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls", githubAPIBase, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(buf)))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("GH_TOKEN"))
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github PR create returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var pr struct {
		HTMLURL string `json:"html_url"`
	}
	_ = json.Unmarshal(body, &pr)
	fmt.Fprintf(os.Stderr, "gitops: opened PR %s\n", pr.HTMLURL)
	return nil
}

func runGit(ctx context.Context, workspace string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func currentBranch(ctx context.Context, workspace string) (string, error) {
	out, err := runGit(ctx, workspace, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

// detectOwnerRepo parses owner/repo from the origin remote URL.
func detectOwnerRepo(ctx context.Context, workspace string) (owner, repo string, err error) {
	out, err := runGit(ctx, workspace, "remote", "get-url", "origin")
	if err != nil {
		return "", "", err
	}
	s := strings.TrimSpace(out)
	s = strings.TrimSuffix(s, ".git")
	for _, sep := range []string{"github.com/", "github.com:"} {
		if i := strings.Index(s, sep); i >= 0 {
			parts := strings.SplitN(s[i+len(sep):], "/", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				return parts[0], parts[1], nil
			}
		}
	}
	return "", "", fmt.Errorf("not a github remote: %s", s)
}

func isMainPushAllowed() bool { return os.Getenv("SIN_ALLOW_MAIN_PUSH") == "1" }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
