// SPDX-License-Identifier: MIT
// Purpose: tests for GitHub issue (loop-003) and CI check (loop-008)
// discovery — remote parsing, dedup keys, label priority, PR filtering, and
// silent-skip behaviour, all against an httptest mock GitHub API.
package autonomy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func collect(cfg DiscoverConfig) []Finding {
	var out []Finding
	add := func(f Finding) bool { out = append(out, f); return true }
	switch {
	case cfg.ScanCIChecks:
		_ = scanCICheckRuns(context.Background(), cfg, add)
	default:
		_ = scanGitHubIssues(context.Background(), cfg, add)
	}
	return out
}

func TestParseGitHubRemote(t *testing.T) {
	cases := map[string][2]string{
		"https://github.com/OpenSIN-Code/SIN-Code.git": {"OpenSIN-Code", "SIN-Code"},
		"git@github.com:OpenSIN-Code/SIN-Code.git":      {"OpenSIN-Code", "SIN-Code"},
		"https://github.com/foo/bar":                    {"foo", "bar"},
		"https://gitlab.com/foo/bar.git":                {"", ""},
	}
	for url, want := range cases {
		o, r, _ := parseGitHubRemote(url)
		if o != want[0] || r != want[1] {
			t.Errorf("%q: got (%q,%q) want (%q,%q)", url, o, r, want[0], want[1])
		}
	}
}

func TestScanGitHubIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
          {"number":1,"title":"crash on start","body":"it crashes","labels":[{"name":"bug"}]},
          {"number":2,"title":"a PR","body":"x","pull_request":{},"labels":[{"name":"bug"}]},
          {"number":3,"title":"help me","body":"y","labels":[{"name":"good first issue"}]}
        ]`))
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	findings := collect(DiscoverConfig{
		Workspace: t.TempDir(), GitHubOwner: "o", GitHubRepo: "r",
		ScanGitHubIssues: true,
	})
	if len(findings) != 2 {
		t.Fatalf("expected 2 issues (PR filtered out), got %d: %+v", len(findings), findings)
	}
	if findings[0].DedupKey != "gh_issue:o/r:1" {
		t.Fatalf("unexpected dedup key %q", findings[0].DedupKey)
	}
	if findings[0].Priority != 5 {
		t.Fatalf("bug label should be priority 5, got %d", findings[0].Priority)
	}
	if findings[1].Priority != 2 {
		t.Fatalf("good-first-issue should be priority 2, got %d", findings[1].Priority)
	}
}

func TestScanGitHubIssuesSilentSkipNoRepo(t *testing.T) {
	// No owner/repo and a temp dir with no git remote => silent skip, no error.
	findings := collect(DiscoverConfig{Workspace: t.TempDir(), ScanGitHubIssues: true})
	if len(findings) != 0 {
		t.Fatalf("expected no findings without a GitHub repo, got %d", len(findings))
	}
}

func TestScanGitHubIssuesUnauthorizedSkips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()
	findings := collect(DiscoverConfig{
		Workspace: t.TempDir(), GitHubOwner: "o", GitHubRepo: "r", ScanGitHubIssues: true,
	})
	if len(findings) != 0 {
		t.Fatalf("401 must skip silently, got %d findings", len(findings))
	}
}

func TestScanCICheckRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"check_runs":[
          {"id":11,"name":"go-test","status":"completed","conclusion":"failure","html_url":"u","output":{"summary":"TestFoo failed"}},
          {"id":12,"name":"go-vet","status":"completed","conclusion":"success"},
          {"id":13,"name":"lint","status":"completed","conclusion":"timed_out","output":{"text":"timeout"}},
          {"id":14,"name":"build","status":"in_progress","conclusion":""}
        ]}`))
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	findings := collect(DiscoverConfig{
		Workspace: t.TempDir(), GitHubOwner: "o", GitHubRepo: "r",
		ScanCIChecks: true, CIBranch: "main",
	})
	if len(findings) != 2 {
		t.Fatalf("expected 2 failing checks (failure+timed_out), got %d: %+v", len(findings), findings)
	}
	for _, f := range findings {
		if f.Source != "ci_check" {
			t.Fatalf("unexpected source %q", f.Source)
		}
		if f.Priority != 8 {
			t.Fatalf("CI findings must be priority 8, got %d", f.Priority)
		}
	}
	if findings[0].DedupKey != "ci_check:o/r:11" {
		t.Fatalf("unexpected dedup key %q", findings[0].DedupKey)
	}
}

func TestScanCIMaxFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"check_runs":[
          {"id":1,"name":"a","status":"completed","conclusion":"failure"},
          {"id":2,"name":"b","status":"completed","conclusion":"failure"},
          {"id":3,"name":"c","status":"completed","conclusion":"failure"}
        ]}`))
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()
	findings := collect(DiscoverConfig{
		Workspace: t.TempDir(), GitHubOwner: "o", GitHubRepo: "r",
		ScanCIChecks: true, CIBranch: "main", CIMaxFailures: 2,
	})
	if len(findings) != 2 {
		t.Fatalf("CIMaxFailures=2 should cap at 2, got %d", len(findings))
	}
}
