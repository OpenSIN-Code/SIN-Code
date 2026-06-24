// SPDX-License-Identifier: MIT
// Purpose: extract candidates from observations. Two implementations:
// heuristic (no model, runs inline) and LLM-backed (cheap background
// model, fall-back to heuristic on any error). Mirrors the
// "continuous-learning-v2" extraction pattern in a clean-room port.
// Docs: extract.doc.md
package instinct

import (
	"context"
	"path/filepath"
	"strings"
)

// Observation is a normalized signal emitted from a hook event.
type Observation struct {
	Tool    string            // "Bash", "Edit", "Write", ...
	Action  string            // short description of what happened
	Domain  string            // git, testing, code-style, security, ...
	Success bool              // did the step succeed
	Meta    map[string]string // arbitrary context (path, command, lang, ...)
}

// Candidate is a proposed instinct extracted from observations.
type Candidate struct {
	Trigger  string
	Domain   string
	Action   string
	Evidence []string
}

// Extractor turns a batch of observations into instinct candidates.
type Extractor interface {
	Extract(ctx context.Context, obs []Observation) ([]Candidate, error)
}

// HeuristicExtractor groups observations by (domain, normalized action)
// and emits a candidate when a pattern repeats. Deterministic,
// model-free — safe to run inline in the hook dispatcher.
type HeuristicExtractor struct {
	MinRepeats int // default 2
}

func (h HeuristicExtractor) Extract(_ context.Context, obs []Observation) ([]Candidate, error) {
	min := h.MinRepeats
	if min <= 0 {
		min = 2
	}
	type agg struct {
		count    int
		evidence []string
		domain   string
		action   string
	}
	groups := map[string]*agg{}
	for _, o := range obs {
		if !o.Success || strings.TrimSpace(o.Action) == "" {
			continue
		}
		key := o.Domain + "::" + normalizeAction(o.Action)
		g := groups[key]
		if g == nil {
			g = &agg{domain: o.Domain, action: o.Action}
			groups[key] = g
		}
		g.count++
		if len(g.evidence) < 5 {
			g.evidence = append(g.evidence, describe(o))
		}
	}
	var out []Candidate
	for _, g := range groups {
		if g.count < min {
			continue
		}
		out = append(out, Candidate{
			Trigger:  "when " + triggerFromAction(g.action, g.domain),
			Domain:   g.domain,
			Action:   g.action,
			Evidence: g.evidence,
		})
	}
	return out, nil
}

func normalizeAction(a string) string {
	return strings.ToLower(strings.Join(strings.Fields(a), " "))
}

func triggerFromAction(action, domain string) string {
	switch domain {
	case "git":
		return "making a commit"
	case "testing":
		return "adding or updating tests"
	case "code-style":
		return "writing or editing code"
	case "security":
		return "handling secrets or auth"
	default:
		return "working in " + domain
	}
}

func describe(o Observation) string {
	if p := o.Meta["path"]; p != "" {
		return o.Tool + " on " + p
	}
	if c := o.Meta["command"]; c != "" {
		return o.Tool + ": " + c
	}
	return o.Tool + " action"
}

// ── tool-event → domain classification ─────────────────────────────────

// Classify infers an instinct domain from a tool invocation.
func Classify(tool string, meta map[string]string) string {
	cmd := strings.ToLower(meta["command"])
	path := strings.ToLower(meta["path"])

	if cmd != "" {
		switch {
		case hasAnyPrefix(cmd, "git commit", "git push", "git rebase", "git merge", "git cherry"):
			return "git"
		case containsAny(cmd, "go test", "pytest", "jest", "vitest", "cargo test", "npm test", "go vet"):
			return "testing"
		case containsAny(cmd, "go build", "cargo build", "make", "npm run build", "tsc", "webpack"):
			return "build"
		case containsAny(cmd, "docker", "kubectl", "helm", "terraform", "ansible"):
			return "infra"
		case containsAny(cmd, "npm install", "pnpm add", "go get", "pip install", "cargo add"):
			return "dependencies"
		case containsAny(cmd, "curl", "wget", "http"):
			return "network"
		case containsAny(cmd, "psql", "sqlite", "migrate", "prisma", "drizzle"):
			return "database"
		}
	}

	if path != "" {
		if d := domainFromPath(path); d != "" {
			return d
		}
	}

	switch tool {
	case "Edit", "Write", "MultiEdit":
		return "code-style"
	case "Grep", "Glob", "Read":
		return "navigation"
	}
	return "general"
}

func domainFromPath(path string) string {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	switch {
	case strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") ||
		strings.Contains(lower, ".spec.") || strings.HasSuffix(lower, "_test.go"):
		return "testing"
	case base == "dockerfile" || strings.HasSuffix(base, ".tf") ||
		strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml"):
		return "infra"
	case strings.Contains(lower, "auth") || strings.Contains(lower, "secret") ||
		strings.Contains(lower, "credential") || strings.Contains(lower, ".env"):
		return "security"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	case ".ts", ".tsx", ".js", ".jsx":
		return "typescript"
	case ".sql":
		return "database"
	case ".md", ".mdx", ".txt":
		return "docs"
	}
	return ""
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
