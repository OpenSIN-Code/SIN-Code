// SPDX-License-Identifier: MIT
// Purpose: Intent Contract — machine-enforceable mandate compiled from a task.
// Pre-flight gate on every edit (instant) + post-hoc diff-scope check in
// the Verifier (authoritative). Agents cannot drift outside their scope.
package orchestrator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type Contract struct {
	TaskID             string
	AllowedGlobs       []string
	FrozenGlobs        []string
	ForbiddenPatterns  []ForbiddenPattern
	MaxFilesChanged    int
	MaxLinesChanged    int
	RequiredInvariants []Check
}

type ForbiddenPattern struct {
	Name        string
	Pattern     *regexp.Regexp
	OnlyNewCode bool
}

func DefaultForbidden() []ForbiddenPattern {
	return []ForbiddenPattern{
		{Name: "hardcoded-secret", OnlyNewCode: true,
			Pattern: regexp.MustCompile(`(?i)(api[_-]?key|secret|password|token)\s*[:=]+\s*["'][A-Za-z0-9+/_\-]{16,}["']`)},
		{Name: "private-key-block", OnlyNewCode: true,
			Pattern: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
		{Name: "disabled-test", OnlyNewCode: true,
			Pattern: regexp.MustCompile(`t\.Skip\([^)]*\)|@pytest\.mark\.skip\b|it\.skip\(|describe\.skip\(`)},
		{Name: "debug-leftover", OnlyNewCode: true,
			Pattern: regexp.MustCompile(`console\.log\(["']\[v0\]|fmt\.Println\(["']DEBUG`)},
	}
}

type Violation struct {
	Kind   string
	Path   string
	Line   int
	Detail string
}

func (v Violation) String() string {
	if v.Line > 0 {
		return fmt.Sprintf("[%s] %s:%d — %s", v.Kind, v.Path, v.Line, v.Detail)
	}
	return fmt.Sprintf("[%s] %s — %s", v.Kind, v.Path, v.Detail)
}

func (c *Contract) CheckEdit(path string, addedLines []string) []Violation {
	var out []Violation

	for _, g := range c.FrozenGlobs {
		if matched, _ := filepath.Match(g, path); matched {
			out = append(out, Violation{Kind: "frozen-path", Path: path,
				Detail: fmt.Sprintf("matches frozen glob %q", g)})
			continue
		}
		prefix := strings.TrimSuffix(g, "/*")
		if prefix != g && strings.HasPrefix(path, prefix+"/") {
			out = append(out, Violation{Kind: "frozen-path", Path: path,
				Detail: fmt.Sprintf("matches frozen prefix %q", g)})
		}
	}

	if len(c.AllowedGlobs) > 0 && !c.pathAllowed(path) {
		out = append(out, Violation{Kind: "out-of-scope", Path: path,
			Detail: fmt.Sprintf("not covered by allowed globs %v", c.AllowedGlobs)})
	}

	for i, line := range addedLines {
		for _, fp := range c.ForbiddenPatterns {
			if fp.Pattern.MatchString(line) {
				out = append(out, Violation{Kind: "forbidden-content", Path: path, Line: i + 1,
					Detail: fmt.Sprintf("matches forbidden pattern %q", fp.Name)})
			}
		}
	}
	return out
}

func (c *Contract) CheckDiffStats(filesChanged, linesChanged int) []Violation {
	var out []Violation
	if c.MaxFilesChanged > 0 && filesChanged > c.MaxFilesChanged {
		out = append(out, Violation{Kind: "blast-radius",
			Detail: fmt.Sprintf("%d files changed, contract allows %d", filesChanged, c.MaxFilesChanged)})
	}
	if c.MaxLinesChanged > 0 && linesChanged > c.MaxLinesChanged {
		out = append(out, Violation{Kind: "blast-radius",
			Detail: fmt.Sprintf("%d lines changed, contract allows %d", linesChanged, c.MaxLinesChanged)})
	}
	return out
}

func (c *Contract) pathAllowed(path string) bool {
	for _, g := range c.AllowedGlobs {
		if matched, _ := filepath.Match(g, path); matched {
			return true
		}
		prefix := strings.TrimSuffix(g, "/*")
		if prefix != g && strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func (c *Contract) AsChecks() []Check {
	checks := make([]Check, 0, 1+len(c.RequiredInvariants))
	if len(c.AllowedGlobs) > 0 {
		checks = append(checks, Check{
			Kind:         CheckDiffScope,
			Name:         "contract-diff-scope",
			AllowedPaths: c.AllowedGlobs,
			Cmd:          []string{"sh", "-c", diffScopeScript(c.AllowedGlobs)},
		})
	}
	checks = append(checks, c.RequiredInvariants...)
	return checks
}

func diffScopeScript(globs []string) string {
	var conds []string
	for _, g := range globs {
		conds = append(conds, fmt.Sprintf(`case "$f" in %s) ok=1;; esac`, g))
	}
	return fmt.Sprintf(
		`set -e; bad=0; for f in $(git diff --name-only HEAD); do ok=0; %s; `+
			`if [ "$ok" = 0 ]; then echo "OUT OF SCOPE: $f"; bad=1; fi; done; exit $bad`,
		strings.Join(conds, "; "))
}

func CompileContract(task *Task) *Contract {
	c := &Contract{
		TaskID:            task.ID,
		ForbiddenPatterns: DefaultForbidden(),
		MaxFilesChanged:   25,
		MaxLinesChanged:   2000,
	}
	lower := strings.ToLower(task.Title + " " + task.Description)
	if !strings.Contains(lower, "lockfile") && !strings.Contains(lower, "dependency") &&
		!strings.Contains(lower, "dependencies") {
		c.FrozenGlobs = append(c.FrozenGlobs, "go.sum", "package-lock.json", "pnpm-lock.yaml", "poetry.lock")
	}
	if !strings.Contains(lower, "ci") && !strings.Contains(lower, "workflow") &&
		!strings.Contains(lower, "pipeline") {
		c.FrozenGlobs = append(c.FrozenGlobs, ".github/workflows/*")
	}
	return c
}
