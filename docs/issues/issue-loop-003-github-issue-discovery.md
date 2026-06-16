# [loop-003] Discovery must scan GitHub Issues + PRs, not just local files

**Labels:** `loop-engineering` `autonomy` `p1`
**Branch:** `agent-loop-engineering`
**Affects:** `autonomy/discover.go`, `autonomy/triggers.go`

---

## Problem

`Discover()` today only reads local files: source TODO/FIXME markers and
`MASTER_TODO.md`. But the real backlog lives in GitHub Issues — bug reports,
feature requests, `help wanted` labels. An agent that only reads local files
misses 80 % of its actual work queue. The daemon should autonomously drain
GitHub Issues just like it drains the local TODO list.

---

## Root cause

```go
// autonomy/discover.go — current Discover function
func Discover(cfg DiscoverConfig) ([]Finding, error) {
    // ...
    if cfg.ScanComments {
        if err := scanComments(cfg.Workspace, add); err != nil { return nil, err }
    }
    if cfg.ScanMaster {
        if err := scanMasterTodo(cfg.Workspace, add); err != nil { return nil, err }
    }
    return out, nil
    // MISSING: ScanGitHubIssues, ScanGitHubPRReviews, ScanFailingCIChecks
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

    // NEW: GitHub-native discovery sources
    ScanGitHubIssues bool   // drain open issues labelled help-wanted / bug
    ScanGitHubPRs    bool   // turn open PR review comments into fix-goals
    GitHubOwner      string // derived from git remote when empty
    GitHubRepo       string
    GitHubToken      string // GITHUB_TOKEN / GH_TOKEN env fallback
    // Labels filters issues; empty = ["bug","help wanted","good first issue"]
    GitHubLabels     []string
    GitHubMaxIssues  int // cap per scan, default 20
}
```

### 2. `scanGitHubIssues` via the REST API (no SDK dependency)

```go
// autonomy/discover_github.go (new file)

package autonomy

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strings"
    "time"
)

type ghIssue struct {
    Number int    `json:"number"`
    Title  string `json:"title"`
    Body   string `json:"body"`
    HTMLURL string `json:"html_url"`
    Labels []struct{ Name string `json:"name"` } `json:"labels"`
}

func scanGitHubIssues(ctx context.Context, cfg DiscoverConfig, add func(Finding) bool) error {
    owner, repo := cfg.GitHubOwner, cfg.GitHubRepo
    if owner == "" || repo == "" {
        var err error
        owner, repo, err = detectGitHubOwnerRepo(cfg.Workspace)
        if err != nil || owner == "" {
            return nil // not a GitHub repo; skip silently
        }
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

    labelStr := strings.Join(labels, ",")
    url := fmt.Sprintf(
        "https://api.github.com/repos/%s/%s/issues?state=open&labels=%s&per_page=%d",
        owner, repo, labelStr, maxIssues)

    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    req.Header.Set("Accept", "application/vnd.github+json")
    req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
    if token != "" {
        req.Header.Set("Authorization", "Bearer "+token)
    }

    client := &http.Client{Timeout: 15 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("github issues fetch: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized {
        return nil // repo not found or no token — skip silently
    }
    body, _ := io.ReadAll(resp.Body)
    var issues []ghIssue
    if err := json.Unmarshal(body, &issues); err != nil {
        return nil
    }

    for _, iss := range issues {
        labelsStr := ""
        for _, l := range iss.Labels {
            labelsStr += " [" + l.Name + "]"
        }
        prompt := fmt.Sprintf(
            "Resolve GitHub Issue #%d in %s/%s:%s\n\nTitle: %s\n\n%s\n\n"+
                "When done: the issue's acceptance criteria are satisfied, "+
                "all tests pass, and the code is merged-ready.",
            iss.Number, owner, repo, labelsStr, iss.Title, iss.Body)

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

func issuePriority(labels []struct{ Name string `json:"name"` }) int {
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

// detectGitHubOwnerRepo extracts owner/repo from the git remote URL.
func detectGitHubOwnerRepo(workspace string) (owner, repo string, err error) {
    out, err := runCmd(workspace, "git", "remote", "get-url", "origin")
    if err != nil {
        return "", "", err
    }
    // Handles both https://github.com/owner/repo.git and git@github.com:owner/repo.git
    s := strings.TrimSpace(out)
    s = strings.TrimSuffix(s, ".git")
    if i := strings.Index(s, "github.com/"); i >= 0 {
        parts := strings.SplitN(s[i+len("github.com/"):], "/", 2)
        if len(parts) == 2 {
            return parts[0], parts[1], nil
        }
    }
    if i := strings.Index(s, "github.com:"); i >= 0 {
        parts := strings.SplitN(s[i+len("github.com:"):], "/", 2)
        if len(parts) == 2 {
            return parts[0], parts[1], nil
        }
    }
    return "", "", nil
}
```

### 3. Wire into `Discover` and the `discover` trigger

```go
// autonomy/discover.go — extended Discover
func Discover(cfg DiscoverConfig) ([]Finding, error) {
    // ... existing code ...
    if cfg.ScanGitHubIssues {
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        if err := scanGitHubIssues(ctx, cfg, add); err != nil {
            fmt.Fprintf(os.Stderr, "warn: github issues scan failed: %v\n", err)
            // non-fatal: local findings still returned
        }
    }
    return out, nil
}
```

### 4. Trigger config example (`.sin-code/triggers.json`)

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

---

## Acceptance criteria

- [ ] `DiscoverConfig` extended with GitHub fields
- [ ] `scanGitHubIssues` fetches open issues by label, no SDK dependency
- [ ] auto-detects owner/repo from git remote when not explicit
- [ ] dedup key is `gh_issue:owner/repo:number` — re-scans never duplicate
- [ ] fails silently (logs warning) when no token or repo is not GitHub
- [ ] `goal discover --github-issues` flag on CLI
- [ ] `scan_github_issues` field in trigger config
- [ ] unit tests with HTTP mock server
- [ ] `go test -race` green
