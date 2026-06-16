// SPDX-License-Identifier: MIT
// Purpose: autonomous backlog discovery — turn latent work in a repository
// into queued goals so the daemon finds its own work instead of waiting for a
// human prompt. Sources: TODO/FIXME markers in source, and unchecked items in
// MASTER_TODO.md. Each finding is enqueued with a stable dedup key so repeated
// scans never pile up duplicates of the same item.
package autonomy

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Finding is one unit of discovered work.
type Finding struct {
	Source   string // "todo" | "fixme" | "master_todo" | "github_issue" | "ci_check"
	Prompt   string // instruction handed to a worker
	DedupKey string // stable identity so re-scans don't duplicate
	Priority int
}

// DiscoverConfig tunes a scan.
type DiscoverConfig struct {
	Workspace    string
	ScanComments bool // scan source files for TODO/FIXME
	ScanMaster   bool // scan MASTER_TODO.md for unchecked items
	MaxFindings  int  // cap per scan (0 = default 50)
	MaxRetries   int  // retry budget for enqueued goals (0 = default 3)

	// GitHub-native discovery (loop-003): drain open issues as goals.
	ScanGitHubIssues bool
	GitHubLabels     []string // empty = ["bug","help wanted","good first issue"]
	GitHubMaxIssues  int      // cap per scan (0 = default 20)

	// CI failure discovery (loop-008): turn failing check runs into fix-goals.
	ScanCIChecks  bool
	CIBranch      string // branch to inspect (empty = current branch)
	CIMaxFailures int    // cap per scan (0 = default 5)

	// Shared GitHub connection fields (auto-detected from git remote / env
	// when empty). Used by both the issue and CI scanners.
	GitHubOwner string
	GitHubRepo  string
	GitHubToken string // GH_TOKEN / GITHUB_TOKEN env fallback
}

// codeExts is the set of source extensions scanned for TODO/FIXME markers.
var codeExts = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".py": true, ".rs": true, ".java": true, ".rb": true, ".c": true,
	".cc": true, ".cpp": true, ".h": true, ".hpp": true, ".sh": true,
}

// Discover scans the workspace and returns deduplicated findings. It performs
// no enqueueing itself so it is trivially testable; EnqueueFindings persists.
func Discover(cfg DiscoverConfig) ([]Finding, error) {
	if cfg.MaxFindings <= 0 {
		cfg.MaxFindings = 50
	}
	var out []Finding
	seen := map[string]bool{}
	add := func(f Finding) bool {
		if f.DedupKey == "" || seen[f.DedupKey] {
			return len(out) < cfg.MaxFindings
		}
		seen[f.DedupKey] = true
		out = append(out, f)
		return len(out) < cfg.MaxFindings
	}

	if cfg.ScanComments {
		if err := scanComments(cfg.Workspace, add); err != nil {
			return nil, err
		}
	}
	if cfg.ScanMaster {
		if err := scanMasterTodo(cfg.Workspace, add); err != nil {
			return nil, err
		}
	}
	if cfg.ScanGitHubIssues {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := scanGitHubIssues(ctx, cfg, add); err != nil {
			fmt.Fprintf(os.Stderr, "warn: github issues scan failed: %v\n", err)
			// non-fatal: local findings are still returned
		}
		cancel()
	}
	if cfg.ScanCIChecks {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := scanCICheckRuns(ctx, cfg, add); err != nil {
			fmt.Fprintf(os.Stderr, "warn: CI check scan failed: %v\n", err)
			// non-fatal
		}
		cancel()
	}
	return out, nil
}

// EnqueueFindings persists findings as goals via AddDiscovered, skipping any
// that already have a live goal with the same dedup key. Returns how many were
// newly enqueued.
func EnqueueFindings(ctx context.Context, q *Queue, workspace string, findings []Finding, maxRetries int) (int, error) {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	n := 0
	for _, f := range findings {
		id, added, err := q.AddDiscovered(ctx, f.Prompt, workspace, f.DedupKey, f.Priority, maxRetries, "")
		if err != nil {
			return n, err
		}
		if added {
			n++
			fmt.Fprintf(os.Stderr, "discover: enqueued goal %d from %s\n", id, f.Source)
		}
	}
	return n, nil
}

func scanComments(workspace string, add func(Finding) bool) error {
	cont := true
	return filepath.WalkDir(workspace, func(path string, d os.DirEntry, err error) error {
		if err != nil || !cont {
			if !cont {
				return filepath.SkipAll
			}
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == ".sin-code" ||
				base == "vendor" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if !codeExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		rel, _ := filepath.Rel(workspace, path)
		f, ferr := os.Open(path)
		if ferr != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		line := 0
		for sc.Scan() {
			line++
			text := sc.Text()
			marker, note := todoMarker(text)
			if marker == "" {
				continue
			}
			prompt := fmt.Sprintf(
				"Resolve the %s at %s:%d: %q. Implement the fix or improvement it describes, "+
					"then remove the marker. Ensure the build and tests still pass.",
				marker, rel, line, note)
			if !add(Finding{
				Source:   strings.ToLower(marker),
				Prompt:   prompt,
				DedupKey: fmt.Sprintf("%s:%s:%s", strings.ToLower(marker), rel, note),
				Priority: priorityFor(marker),
			}) {
				cont = false
				return filepath.SkipAll
			}
		}
		return nil
	})
}

// todoMarker extracts a TODO/FIXME/XXX/HACK marker and its trailing note from a
// source line, or ("","") if none. Only matches markers inside comments to
// avoid false positives on string literals containing the word.
func todoMarker(line string) (marker, note string) {
	for _, m := range []string{"TODO", "FIXME", "XXX", "HACK"} {
		for _, prefix := range []string{"// " + m, "//" + m, "# " + m, "#" + m, "/* " + m, "* " + m} {
			if idx := strings.Index(line, prefix); idx >= 0 {
				rest := line[idx+len(prefix):]
				rest = strings.TrimLeft(rest, ":() ")
				rest = strings.TrimRight(rest, "*/ ")
				return m, strings.TrimSpace(rest)
			}
		}
	}
	return "", ""
}

func priorityFor(marker string) int {
	switch marker {
	case "FIXME", "XXX":
		return 2
	case "HACK":
		return 1
	default:
		return 0
	}
}

// scanMasterTodo reads MASTER_TODO.md and enqueues unchecked GitHub-style
// checklist items ("- [ ] ...").
func scanMasterTodo(workspace string, add func(Finding) bool) error {
	path := filepath.Join(workspace, "MASTER_TODO.md")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		low := strings.ToLower(text)
		if !strings.HasPrefix(low, "- [ ]") && !strings.HasPrefix(low, "* [ ]") {
			continue
		}
		item := strings.TrimSpace(text[5:])
		if item == "" {
			continue
		}
		prompt := fmt.Sprintf(
			"Complete this MASTER_TODO item: %q. When done, check it off in MASTER_TODO.md "+
				"and ensure the build and tests pass.", item)
		if !add(Finding{
			Source:   "master_todo",
			Prompt:   prompt,
			DedupKey: "master_todo:" + normalizeKey(item),
			Priority: 1,
		}) {
			return filepath.SkipAll
		}
	}
	return nil
}

// normalizeKey lowercases and collapses whitespace so trivial edits to a TODO
// line still map to the same dedup identity.
func normalizeKey(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
