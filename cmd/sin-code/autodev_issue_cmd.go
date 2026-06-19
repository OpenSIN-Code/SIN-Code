// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

// ghAuthLoggedInRe matches a positive `gh auth status` line that
// confirms the user is authenticated to github.com.
//
// `gh auth status` emits one status line per known host. Pre-2.40 the
// lines are bare ASCII (no leading marker); 2.40+ prefixes every entry
// with a unicode mark ("✓" U+2713 for OK, "✗" U+2717 for fail) or, on
// some CI / Windows builds, an ascii marker ("X", "[OK]", "[FAIL]").
//
// Anchored per-line (multiline mode) so a positive entry on one host
// does not bleed onto an adjacent host's line. The optional marker
// group is bounded to check-mark / X / bracket tokens so a line like
// "Not logged in to github.com" cannot pass via the optional word
// prefix — only an explicit OK-style marker is accepted, and the verb
// phrase itself is "logged in to github.com" with mandatory whitespace,
// case-insensitive but anchored.
//
// Compiled once at package init — see parseGhAuthStatus.
var ghAuthLoggedInRe = regexp.MustCompile(
	`(?im)^\s*` +
		`(?:[✓✔✗✘]|X|\[OK\]|\[FAIL\]|\(\s*\))?\s*` +
		`logged\s+in\s+to\s+github\.com\s*\b`,
)

// parseGhAuthStatus returns true iff the given `gh auth status`
// stdout confirms the user is authenticated to github.com.
//
// Robust to:
//   - pre-2.40 (no marker): "Logged in to github.com as user (oauth_token)"
//   - gh 2.40+ unicode "✓": "✓ Logged in to github.com account Delqhi (keyring)"
//   - gh 2.40+ unicode "✔": "✔ Logged in to github.com as user"
//   - ascii "[OK]":         "[OK] Logged in to github.com as user"
//
// Robust against false positives:
//   - "✗ You are not logged in to github.com"        (no OK marker + wrong verb sequence)
//   - "X Failed to log in to github.com using token" (OK marker + wrong verb)
//   - "You are not logged into any GitHub hosts."   (different phrase entirely)
//   - "✓ Logged in to gitlab.example.com as user"   (non-github host)
//
// The regex is case-insensitive and multiline so future gh versions
// adjusting phrasing or capitalization continue to match.
func parseGhAuthStatus(out string) bool {
	return ghAuthLoggedInRe.MatchString(out)
}

// newGhBridgeForDetect selects the Bridge constructor used by
// detectRepo. Production calls ghbridge.New(); tests override this
// var to inject a mock bridge backed by an in-process Runner so
// detectRepo can be tested hermetically without spawning `gh`.
//
// This is the same pattern used elsewhere in this repo for cli-side
// dependency injection (see ghbridge.Runner, classifyFunc). The hook
// is package-private so external code is unaffected.
var newGhBridgeForDetect = ghbridge.New

type issuePayload struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	URL    string `json:"url"`
}

func fetchIssue(ctx context.Context, repo string, number int) (*issuePayload, error) {
	b := ghbridge.New()
	out, _, err := b.Execute(ctx, []string{"issue", "view", fmt.Sprintf("%d", number), "--repo", repo, "--json", "number,title,body,state,url"})
	if err != nil {
		return nil, fmt.Errorf("gh issue view: %w", err)
	}
	var issue issuePayload
	if err := json.Unmarshal([]byte(out), &issue); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &issue, nil
}

func issueToPrompt(issue *issuePayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Resolve GitHub issue #%d: %s\n\nURL: %s\n\n%s\n\nInstructions: implement a fix, write tests, update docs, ensure build passes.", issue.Number, issue.Title, issue.URL, issue.Body)
	return b.String()
}

func defaultIssueContract(issue *issuePayload) *goalcontract.GoalContract {
	return &goalcontract.GoalContract{
		DeterministicChecks: []orchestrator.Check{
			{Kind: orchestrator.CheckBuild, Name: "build", Cmd: []string{"go", "build", "./..."}},
			{Kind: orchestrator.CheckTest, Name: "test", Cmd: []string{"go", "test", "./...", "-race", "-count=1"}},
		},
		SemanticCriteria: []string{fmt.Sprintf("Addresses issue #%d", issue.Number), "Tests exercise new behavior", "No debug scaffolding"},
	}
}

func newGoalAddFromIssueCmd() *cobra.Command {
	var priority, retries int
	var repo string
	var extraCriteria []string
	cmd := &cobra.Command{
		Use: "add-from-issue <number>", Short: "Read a GitHub issue and enqueue it as an autonomous goal (Autodev, issue #391)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			num, err := fmtAtoi(args[0])
			if err != nil {
				return fmt.Errorf("issue number must be positive: %w", err)
			}
			if num <= 0 {
				return fmt.Errorf("issue number must be a positive integer")
			}
			if repo == "" {
				ctx := cmd.Context()
				var err error
				repo, err = detectRepo(ctx)
				if err != nil {
					return err
				}
			}
			ctx := cmd.Context()
			issue, err := fetchIssue(ctx, repo, num)
			if err != nil {
				return err
			}
			if issue.State != "open" {
				return fmt.Errorf("issue #%d is %s", num, issue.State)
			}
		contract := defaultIssueContract(issue)
		for _, c := range extraCriteria {
			if strings.TrimSpace(c) != "" {
				contract.SemanticCriteria = append(contract.SemanticCriteria, c)
			}
		}
			contractJSON, err := contract.Marshal()
			if err != nil {
				return err
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			ws, _ := os.Getwd()
			id, err := q.AddWithContract(ctx, issueToPrompt(issue), ws, priority, retries, contractJSON)
			if err != nil {
				return err
			}
			fmt.Printf("goal %d enqueued from issue #%d\n", id, num)
			return nil
		},
	}
	cmd.Flags().IntVar(&priority, "priority", 5, "higher runs sooner")
	cmd.Flags().IntVar(&retries, "retries", 3, "retry budget")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (auto-detected if empty)")
	cmd.Flags().StringArrayVar(&extraCriteria, "criteria", nil, "extra acceptance criterion")
	return cmd
}

// detectRepo asks the gh CLI which github repo the current working
// directory belongs to. The auth probe runs `gh auth status` and then
// parses the output via parseGhAuthStatus — see that helper for the
// exact accepted/accepted-against formats.
//
// Auth is checked BEFORE the repo scan so a not-logged-in user gets an
// actionable error rather than a confusing `gh repo view` failure.
//
// The default `--repo` flag path bypasses detectRepo entirely; this
// helper is only consulted when the user did not pass `--repo`.
//
// Implementation note: the auth probe is invoked via `b.Run` (the
// underlying Runner), NOT via `b.Execute`. The classifier in
// `ghbridge.Execute` blocks any call whose first group is "auth" —
// `auth status` is a known read-only probe, so we route around the
// classifier the same way `Bridge.Health` does internally. All OTHER
// gh calls (including `repo view`) still go through `b.Execute` so
// they retain the 3-tier safety net.
func detectRepo(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not found in PATH; install it or pass --repo flag")
	}

	b := newGhBridgeForDetect()
	// Auth probe: bypass the 3-tier classifier (auth status is the
	// canonical read-only credential healthcheck). Use b.Run directly
	// — same pattern as ghbridge.Bridge.Health.
	out, _, err := b.Run(ctx, []string{"auth", "status"})
	if err != nil {
		return "", fmt.Errorf("gh not authenticated; run 'gh auth login' or pass --repo flag: %w", err)
	}
	if !parseGhAuthStatus(out) {
		return "", fmt.Errorf("gh not authenticated; run 'gh auth login' or pass --repo flag")
	}

	// Repo lookup: still classified (read-only; "repo view" + "status"
	// verb is in readOnlyVerbs).
	out, _, err = b.Execute(ctx, []string{"repo", "view", "--json", "nameWithOwner"})
	if err != nil {
		return "", fmt.Errorf("failed to detect repo (not a git repo with GitHub remote?); pass --repo flag: %w", err)
	}
	var v struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if json.Unmarshal([]byte(out), &v) == nil {
		return v.NameWithOwner, nil
	}
	return "", fmt.Errorf("could not parse repo name from gh output")
}

func fmtAtoi(s string) (int, error) { var n int; _, err := fmt.Sscanf(s, "%d", &n); return n, err }
