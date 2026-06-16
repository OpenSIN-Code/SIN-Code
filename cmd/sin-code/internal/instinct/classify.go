// SPDX-License-Identifier: MIT
// Purpose: tool-event → domain classification. Heuristics derived from
// the continuous-learning-v2 domain taxonomy (git, testing, build, infra,
// dependencies, network, database, security, go, rust, python, typescript,
// docs, navigation, general). Cheap to run inline in the hook dispatcher.
// Docs: classify.doc.md
package instinct

import (
	"path/filepath"
	"strings"
)

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
