# [loop-008] Failing CI checks must auto-enqueue as goals — loop closes the CI feedback loop

**Labels:** `loop-engineering` `autonomy` `p2`
**Branch:** `agent-loop-engineering`
**Affects:** `autonomy/discover.go`, `autonomy/triggers.go`

---

## Problem

When a CI check fails on a PR or branch, a human must read the output, open an
issue or comment, and wait for someone to fix it. In a fully autonomous loop,
a failing CI check should immediately spawn a goal: "fix the failing test /
lint error / build failure on branch X". The daemon should monitor CI and
self-heal without any human loop-closing.

---

## Root cause

`Discover` only reads local files. There is no `ScanCIChecks` path:

```go
// autonomy/discover.go — current DiscoverConfig
type DiscoverConfig struct {
    Workspace    string
    ScanComments bool
    ScanMaster   bool
    MaxFindings  int
    MaxRetries   int
    // MISSING: ScanCIChecks, ScanFailingPRChecks, ScanGitHubActions
}
```

The GitHub Actions / CI API is never polled. Failing check runs never become goals.

---

## Proposed solution

### 1. Extend `DiscoverConfig`

```go
// autonomy/discover.go
type DiscoverConfig struct {
    // ... existing fields ...

    // NEW: CI failure discovery
    ScanCIChecks    bool   // scan for failing GitHub Actions check runs
    CIBranch        string // branch to check (default: current branch)
    GitHubOwner     string // derived from git remote when empty
    GitHubRepo      string
    GitHubToken     string
    CIMaxFailures   int    // max failing checks to enqueue per scan (default 5)
}
```

### 2. `scanCICheckRuns` — poll GitHub Actions API

```go
// autonomy/discover_ci.go (new file)
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

func scanCICheckRuns(ctx context.Context, cfg DiscoverConfig, add func(Finding) bool) error {
    owner, repo := cfg.GitHubOwner, cfg.GitHubRepo
    if owner == "" || repo == "" {
        var err error
        owner, repo, err = detectGitHubOwnerRepo(cfg.Workspace)
        if err != nil || owner == "" {
            return nil
        }
    }
    token := firstNonEmpty(cfg.GitHubToken, os.Getenv("GH_TOKEN"), os.Getenv("GITHUB_TOKEN"))

    branch := cfg.CIBranch
    if branch == "" {
        var err error
        branch, err = currentGitBranch(cfg.Workspace)
        if err != nil || branch == "" {
            return nil
        }
    }

    max := cfg.CIMaxFailures
    if max <= 0 { max = 5 }

    // Get the latest commit SHA on the branch
    sha, err := latestCommitSHA(ctx, owner, repo, branch, token)
    if err != nil || sha == "" {
        return nil
    }

    url := fmt.Sprintf(
        "https://api.github.com/repos/%s/%s/commits/%s/check-runs?per_page=50",
        owner, repo, sha)

    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    req.Header.Set("Accept", "application/vnd.github+json")
    req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
    if token != "" {
        req.Header.Set("Authorization", "Bearer "+token)
    }

    client := &http.Client{Timeout: 15 * time.Second}
    resp, err := client.Do(req)
    if err != nil { return fmt.Errorf("ci checks fetch: %w", err) }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK { return nil }

    body, _ := io.ReadAll(resp.Body)
    var result struct {
        CheckRuns []ghCheckRun `json:"check_runs"`
    }
    if err := json.Unmarshal(body, &result); err != nil { return nil }

    n := 0
    for _, cr := range result.CheckRuns {
        if cr.Status != "completed" { continue }
        if cr.Conclusion != "failure" && cr.Conclusion != "timed_out" { continue }
        n++
        if n > max { break }

        details := cr.Output.Summary
        if cr.Output.Text != "" {
            details = cr.Output.Text
        }
        if len(details) > 2000 { details = details[:2000] + "..." }

        prompt := fmt.Sprintf(
            "Fix the failing CI check %q on branch %q in %s/%s.\n\n"+
                "Check URL: %s\n\nFailure output:\n%s\n\n"+
                "When done: the check passes, all other tests still pass, "+
                "and the fix is committed.",
            cr.Name, branch, owner, repo, cr.HTMLURL, details)

        dedupKey := fmt.Sprintf("ci_check:%s/%s:%d", owner, repo, cr.ID)
        if !add(Finding{
            Source:   "ci_check",
            Prompt:   prompt,
            DedupKey: dedupKey,
            Priority: 8, // CI failures are high priority
        }) {
            return nil
        }
    }
    return nil
}

func currentGitBranch(workspace string) (string, error) {
    out, err := runCmd(workspace, "git", "rev-parse", "--abbrev-ref", "HEAD")
    return strings.TrimSpace(out), err
}

func latestCommitSHA(ctx context.Context, owner, repo, branch, token string) (string, error) {
    url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", owner, repo, branch)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    req.Header.Set("Accept", "application/vnd.github+json")
    if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    var result struct{ SHA string `json:"sha"` }
    _ = json.Unmarshal(body, &result)
    return result.SHA, nil
}
```

### 3. Wire into `Discover` and the trigger config

```go
// autonomy/discover.go
if cfg.ScanCIChecks {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    if err := scanCICheckRuns(ctx, cfg, add); err != nil {
        fmt.Fprintf(os.Stderr, "warn: CI check scan failed: %v\n", err)
    }
}
```

```json
// .sin-code/triggers.json example
[
  {
    "type": "discover",
    "every": "5m",
    "scan_ci_checks": true,
    "ci_branch": "main",
    "github_labels": ["bug"]
  }
]
```

### 4. Auto-commit of CI-fix should include `Fixes CI: <check-name>`

When `SIN_AUTO_COMMIT=1`, CI-fix goals should include the check name in the
commit message trailer so the CI system can correlate:

```
fix(ci): resolve failing check "go-test" on branch main

All tests now pass. Root cause: missing mock in TestFoo.

Fixes CI: go-test
[sin-code goal-id: 77]
```

---

## Acceptance criteria

- [ ] `DiscoverConfig` extended with CI fields
- [ ] `scanCICheckRuns` polls GitHub Actions for `failure`/`timed_out` runs
- [ ] dedup key is `ci_check:owner/repo:check_run_id` — re-scans never pile up
- [ ] fails silently (logs warning) without token or on non-GitHub repos
- [ ] CI check findings have priority=8 (above normal, below explicit goals)
- [ ] `goal discover --ci-checks` flag
- [ ] `scan_ci_checks` and `ci_branch` fields in trigger config
- [ ] auto-commit message includes `Fixes CI: <name>` trailer when goal source is ci_check
- [ ] unit tests with HTTP mock server for check-runs API
- [ ] `go test -race` green
