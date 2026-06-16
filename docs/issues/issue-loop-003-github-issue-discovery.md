# [loop-003] Discovery must scan GitHub Issues, not just local files

**Labels:** `loop-engineering` `autonomy` `p1`
**Branch:** `sincode-loop-system`
**Status:** DONE
**Affects:** `autonomy/discover.go`, `autonomy/discover_github.go` (new), `autonomy/triggers.go`

---

## Problem

`Discover()` today only reads local files: source TODO/FIXME markers and
`MASTER_TODO.md`. But the real backlog lives in GitHub Issues — bug reports,
feature requests, `help wanted` labels. An agent that only reads local files
misses 80% of its actual work queue. The daemon should autonomously drain
GitHub Issues just like it drains the local TODO list.

---

## Root cause

```go
// autonomy/discover.go — current Discover function
func Discover(cfg DiscoverConfig) ([]Finding, error) {
    if cfg.ScanComments {
        if err := scanComments(cfg.Workspace, add); err != nil { return nil, err }
    }
    if cfg.ScanMaster {
        if err := scanMasterTodo(cfg.Workspace, add); err != nil { return nil, err }
    }
    return out, nil
    // MISSING: ScanGitHubIssues, ScanCIChecks
}
```

There is no `ScanGitHub` path. The `DiscoverConfig` struct has no GitHub fields.
The dedup key design already supports external sources (arbitrary string), so
adding a GitHub source is purely additive.

---

## Proposed solution

### 1. Extend `DiscoverConfig`

```go
// autonomy/discover.go
type DiscoverConfig struct {
    Workspace    string
    ScanComments bool
    ScanMaster   bool
    MaxFindings  int
    MaxRetries   int

    // NEW: GitHub-native discovery (loop-003)
    ScanGitHubIssues bool     // drain open issues labelled help-wanted / bug
    GitHubOwner      string   // derived from git remote when empty
    GitHubRepo       string
    GitHubToken      string   // GH_TOKEN / GITHUB_TOKEN env fallback
    GitHubLabels     []string // empty = ["bug","help wanted","good first issue"]
    GitHubMaxIssues  int      // cap per scan, default 20

    // NEW: CI failure discovery (loop-008)
    ScanCIChecks  bool
    CIBranch      string
    CIMaxFailures int
}
```

### 2. `scanGitHubIssues` — GitHub REST API, no SDK dependency

```go
// autonomy/discover_github.go (new file)
// The base URL is a package-level var so tests can point at a mock server.
var githubAPIBase = "https://api.github.com"

func scanGitHubIssues(ctx context.Context, cfg DiscoverConfig, add func(Finding) bool) error {
    owner, repo := cfg.GitHubOwner, cfg.GitHubRepo
    if owner == "" || repo == "" {
        var err error
        owner, repo, err = detectGitHubOwnerRepo(cfg.Workspace)
        if err != nil || owner == "" {
            return nil // not a GitHub repo; skip silently
        }
    }
    token := firstNonEmpty(cfg.GitHubToken,
        os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN"))

    labels := cfg.GitHubLabels
    if len(labels) == 0 {
        labels = []string{"bug", "help wanted", "good first issue"}
    }
    maxIssues := cfg.GitHubMaxIssues
    if maxIssues <= 0 { maxIssues = 20 }

    // Pull requests appear in the issues endpoint with a "pull_request" key.
    // We filter them out in the loop below.
    url := fmt.Sprintf(
        "%s/repos/%s/%s/issues?state=open&labels=%s&per_page=%d",
        githubAPIBase, owner, repo,
        strings.Join(labels, ","), maxIssues)

    // ... HTTP call, JSON parse, dedup ...

    for _, iss := range issues {
        if iss.PullRequest != nil { continue } // skip PRs
        add(Finding{
            Source:   "github_issue",
            Prompt:   buildIssuePrompt(iss, owner, repo),
            DedupKey: fmt.Sprintf("gh_issue:%s/%s:%d", owner, repo, iss.Number),
            Priority: issuePriority(iss.Labels),
        })
    }
    return nil
}

// detectGitHubOwnerRepo parses the `origin` remote URL to extract owner/repo.
// Handles both HTTPS (https://github.com/owner/repo.git) and
// SSH (git@github.com:owner/repo.git) formats.
func detectGitHubOwnerRepo(workspace string) (owner, repo string, err error) {
    out, err := runCmd(workspace, "git", "remote", "get-url", "origin")
    if err != nil { return "", "", err }
    s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(out), ".git"))
    for _, prefix := range []string{"github.com/", "github.com:"} {
        if i := strings.Index(s, prefix); i >= 0 {
            parts := strings.SplitN(s[i+len(prefix):], "/", 2)
            if len(parts) == 2 { return parts[0], parts[1], nil }
        }
    }
    return "", "", nil
}
```

### 3. Trigger config example (`.sin-code/triggers.json`)

```json
[
  {
    "type": "discover",
    "every": "30m",
    "scan_comments": true,
    "scan_master": true,
    "scan_github_issues": true,
    "github_labels": ["bug", "help wanted"]
  }
]
```

### 4. Trigger struct fields (additive, backward-compatible)

```go
// autonomy/triggers.go — Trigger struct extension
type Trigger struct {
    // ... existing fields ...
    ScanComments     *bool    `json:"scan_comments,omitempty"`
    ScanMaster       *bool    `json:"scan_master,omitempty"`
    ScanGitHubIssues bool     `json:"scan_github_issues,omitempty"`
    GitHubLabels     []string `json:"github_labels,omitempty"`
    ScanCIChecks     bool     `json:"scan_ci_checks,omitempty"`
    CIBranch         string   `json:"ci_branch,omitempty"`
}
```

Note: `ScanComments` and `ScanMaster` use `*bool` (pointer) rather than `bool`
so the zero value (nil) means "use the default" (true), not "explicitly off".
This preserves backward compatibility for existing trigger configs that omit
these fields.

---

## Implementation notes (added after build)

**Pull request filtering:**
GitHub's Issues API returns both issues and PRs under the same endpoint (PRs
are a subtype of issues). Without filtering, a PR comment like "fix this bug"
would generate a duplicate goal alongside any issue it closes. The filter
`if iss.PullRequest != nil { continue }` eliminates all PRs from the scan.

**`githubAPIBase` package-level var for testability:**
Rather than hard-coding `https://api.github.com`, the base URL is a package-
level variable. Unit tests swap it to point at an `httptest.NewServer` mock
without touching production code or needing an interface. This is the simplest
testable pattern for an HTTP-based scanner in Go.

**Fail-open and non-fatal:**
GitHub issues scan errors are non-fatal: a network timeout, a 401 (bad token),
or a 404 (private repo without auth) all produce a `fmt.Fprintf(os.Stderr)`
warning and return the local findings collected so far. The loop keeps running.
CI failure discovery follows the same pattern (see loop-008).

**Priority mapping:**
`issuePriority` maps label names to integers. The priority range used (0–10)
aligns with the queue's internal priority scale. GitHub issues generally rank
lower than explicit `goal add` goals (priority 0 by default) unless they carry
a `critical` or `p0` label. This prevents the issue scanner from starving
manually-enqueued goals.

**Dedup key format `gh_issue:owner/repo:number`:**
The queue dedup mechanism hashes the key and stores it in the `goals` table.
Re-running `goal discover --github-issues` every 30 min will never create
duplicate goals for the same issue number. Once a goal with that dedup key
is in any non-exhausted state, the finding is silently skipped.

**`GitHubMaxIssues` default of 20:**
This is intentionally conservative. Dumping 200 open issues into the queue at
once would flood the worker pool and starve other work. The operator can raise
this via the config field. In practice, filtering to specific labels (bug,
help wanted) already reduces the count significantly.

---

## Acceptance criteria

- [x] `DiscoverConfig` extended with GitHub fields
- [x] `scanGitHubIssues` fetches open issues by label, no SDK dependency
- [x] auto-detects owner/repo from git remote when not explicit
- [x] dedup key is `gh_issue:owner/repo:number` — re-scans never duplicate
- [x] fails silently (logs warning) when no token or repo is not GitHub
- [x] PRs filtered out from the issues endpoint
- [x] `goal discover --github-issues` flag on CLI
- [x] `scan_github_issues` field in trigger config
- [x] unit tests with HTTP mock server
- [x] `go test -race` green
