// SPDX-License-Identifier: MIT
// Purpose: SelfReviewReflector — automatic adversarial self-review that
// runs after the verify-gate passes but BEFORE the stop-gate. Scans
// changed files for TODO/FIXME/dummy data/dead code/placeholders and
// returns issues as a Reflection, forcing the loop to continue working.
// This is the programmatic equivalent of the skill-process-self-review
// skill, baked into the loop so every agent self-reviews automatically
// without the user ever having to ask.
package agentloop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SelfReviewConfig controls the automatic self-review reflector.
type SelfReviewConfig struct {
	// Workspace is the root directory for git operations.
	Workspace string
	// MaxFiles limits how many changed files to scan (0 = unlimited).
	MaxFiles int
	// MaxIssues caps the number of issues returned (0 = unlimited).
	MaxIssues int
}

// markerPatterns are the adversarial scan patterns. Each matches a
// common "not actually done" signal in source code.
var markerPatterns = []struct {
	pattern *regexp.Regexp
	label   string
}{
	{regexp.MustCompile(`(?i)\bTODO\b`), "TODO marker"},
	{regexp.MustCompile(`(?i)\bFIXME\b`), "FIXME marker"},
	{regexp.MustCompile(`(?i)\bHACK\b`), "HACK marker"},
	{regexp.MustCompile(`(?i)\bXXX\b`), "XXX marker"},
	{regexp.MustCompile(`(?i)\bplaceholder\b`), "placeholder reference"},
	{regexp.MustCompile(`(?i)\bdummy\b`), "dummy data"},
	{regexp.MustCompile(`(?i)\bmock\b`), "mock data (verify this is test-only)"},
	{regexp.MustCompile(`(?i)\bstub\b`), "stub implementation"},
	{regexp.MustCompile(`(?i)\bnot\s+implemented\b`), "not implemented"},
	{regexp.MustCompile(`(?i)\bcoming\s+soon\b`), "coming soon placeholder"},
	{regexp.MustCompile(`(?i)\bspäter\b`), "später marker (German TODO)"},
	{regexp.MustCompile(`(?i)\bnoch\s+offen\b`), "noch offen marker (German TODO)"},
	{regexp.MustCompile(`(?i)\bTBD\b`), "TBD marker"},
	{regexp.MustCompile(`(?i)\bTBA\b`), "TBA marker"},
	{regexp.MustCompile(`(?i)console\.log\s*\(`), "console.log left in code"},
	{regexp.MustCompile(`(?i)print\s*\(\s*["']DEBUG`), "debug print left in code"},
	{regexp.MustCompile(`(?i)\bfmt\.Println\s*\(\s*["']DEBUG`), "debug fmt.Println left in code"},
}

// excludedExtensions are file types we don't scan (binary, generated, etc).
var excludedExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
	".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".bz2": true,
	".so": true, ".dylib": true, ".dll": true, ".exe": true, ".bin": true,
	".lock": true, ".sum": true,
}

// NewSelfReviewReflector creates a Reflector that automatically scans
// changed files for incomplete-work markers. It runs after the verify-gate
// passes but before the stop-gate, forcing the loop to fix issues before
// reporting completion. This is the "Ultra-CEO" doctrine baked into the loop.
func NewSelfReviewReflector(cfg SelfReviewConfig) Reflector {
	return func(ctx context.Context, snap StopSnapshot) Reflection {
		issues := scanChangedFiles(cfg)
		var notes string
		if len(issues) == 0 {
			return Reflection{}
		}
		notes = "Self-review found incomplete-work markers. Fix all BLOCKER-level issues before claiming completion."
		return Reflection{Issues: issues, Notes: notes}
	}
}

// scanChangedFiles runs git diff to find changed files, then scans each
// for markers that indicate incomplete work.
func scanChangedFiles(cfg SelfReviewConfig) []string {
	ws := cfg.Workspace
	if ws == "" {
		ws = "."
	}

	// Get list of changed files (staged + unstaged + untracked).
	files := gitChangedFiles(ws)
	if len(files) == 0 {
		return nil
	}

	maxFiles := cfg.MaxFiles
	if maxFiles > 0 && len(files) > maxFiles {
		files = files[:maxFiles]
	}

	var issues []string
	for _, f := range files {
		if isExcludedFile(f) {
			continue
		}
		absPath := f
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(ws, f)
		}
		content, err := readFileForScan(absPath)
		if err != nil {
			continue
		}
		fileIssues := scanContent(f, content)
		issues = append(issues, fileIssues...)
		if cfg.MaxIssues > 0 && len(issues) >= cfg.MaxIssues {
			issues = issues[:cfg.MaxIssues]
			break
		}
	}
	return issues
}

// gitChangedFiles returns the list of files that are new, modified, or
// staged in the working tree.
func gitChangedFiles(workspace string) []string {
	// --name-only gives us just filenames; we combine staged + unstaged.
	cmd := exec.Command("git", "-C", workspace, "diff", "--name-only", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		// Not a git repo or no commits — try diff --cached + status.
		return nil
	}
	files := strings.Split(strings.TrimSpace(string(out)), "\n")

	// Also check untracked files.
	cmd2 := exec.Command("git", "-C", workspace, "ls-files", "--others", "--exclude-standard")
	out2, err2 := cmd2.Output()
	if err2 == nil {
		untracked := strings.Split(strings.TrimSpace(string(out2)), "\n")
		files = append(files, untracked...)
	}

	var result []string
	seen := map[string]bool{}
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		result = append(result, f)
	}
	return result
}

// scanContent scans a single file's content for marker patterns.
func scanContent(filename, content string) []string {
	lines := strings.Split(content, "\n")
	var issues []string
	seenLine := map[int]bool{}

	for _, mp := range markerPatterns {
		for i, line := range lines {
			if seenLine[i] {
				continue
			}
			if mp.pattern.MatchString(line) {
				seenLine[i] = true
				trimmed := strings.TrimSpace(line)
				if len(trimmed) > 120 {
					trimmed = trimmed[:117] + "..."
				}
				issues = append(issues, formatIssue(filename, i+1, mp.label, trimmed))
			}
		}
	}
	return issues
}

// formatIssue renders a self-review finding as a one-liner.
func formatIssue(filename string, line int, label, snippet string) string {
	return filename + ":" + itoa(line) + " — " + label + " — " + snippet
}

// isExcludedFile returns true for binary/generated file types.
func isExcludedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if excludedExtensions[ext] {
		return true
	}
	// Also exclude common lock/generated files by name.
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "go.sum",
		"cargo.lock", "flake.lock", ".gitignore", ".gitattributes":
		return true
	}
	return false
}

// readFileForScan reads a file's content for scanning. Files larger than
// 512KB are skipped to avoid scanning generated/minified content.
func readFileForScan(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > 512*1024 {
		return "", nil
	}
	return string(data), nil
}
