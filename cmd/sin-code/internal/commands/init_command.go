// SPDX-License-Identifier: MIT
// Purpose: /init built-in slash command. Analyzes the current project and
// generates a draft AGENTS.md file with auto-detected build/test/lint
// commands, architecture overview, and conventions. Modeled on Claude
// Code's /init command. The generated content is returned as a string for
// the user to review before saving — the workspace is never modified.
package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

type ProjectInfo struct {
	Type          string
	RootDir       string
	BuildCmd      string
	TestCmd       string
	LintCmd       string
	ModulePath    string
	KeyDirs       []string
	TestFramework string
	CI            string
	Tools         []string
}

type InitCommand struct {
	rootDir string
}

func NewInitCommand() *InitCommand {
	return &InitCommand{}
}

func (c *InitCommand) Name() string { return "init" }

func (c *InitCommand) Description() string {
	return "Analyze project and generate AGENTS.md"
}

func (c *InitCommand) Execute(ctx context.Context, args string, sess *session.Session) (string, error) {
	root := c.rootDir
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("/init: cannot determine working directory: %w", err)
		}
	}
	info, err := c.analyzeProject(root)
	if err != nil {
		return "", fmt.Errorf("/init: %w", err)
	}
	existing := ""
	if data, err := os.ReadFile(filepath.Join(root, "AGENTS.md")); err == nil {
		existing = string(data)
	}
	generated := c.GenerateAGENTS(info)
	if existing != "" {
		generated = c.mergeWithExisting(generated, existing)
	}
	header := fmt.Sprintf("# AGENTS.md (draft generated %s)\n\n> Review and save to AGENTS.md in the project root.\n\n", time.Now().Format("2006-01-02"))
	return header + generated, nil
}

func (c *InitCommand) analyzeProject(root string) (*ProjectInfo, error) {
	info := &ProjectInfo{
		RootDir: root,
		Type:    c.DetectProjectType(root),
	}
	info.Tools = c.DetectTools(root)
	info.KeyDirs = c.detectKeyDirs(root)
	info.CI = c.detectCI(root)
	info.TestFramework = c.detectTestFramework(info.Type, root)
	c.fillCommands(info, root)
	return info, nil
}

func (c *InitCommand) DetectProjectType(root string) string {
	checks := []struct {
		file string
		typ  string
	}{
		{"go.mod", "go"},
		{"pyproject.toml", "python"},
		{"requirements.txt", "python"},
		{"setup.py", "python"},
		{"package.json", "node"},
		{"Cargo.toml", "rust"},
	}
	for _, ch := range checks {
		if fileExists(filepath.Join(root, ch.file)) {
			return ch.typ
		}
	}
	return "generic"
}

func (c *InitCommand) DetectTools(root string) []string {
	seen := map[string]bool{}
	var tools []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			tools = append(tools, name)
		}
	}
	if fileExists(filepath.Join(root, ".golangci.yml")) ||
		fileExists(filepath.Join(root, ".golangci.yaml")) ||
		fileExists(filepath.Join(root, ".golangci.toml")) {
		add("golangci-lint")
	}
	if fileExists(filepath.Join(root, "ruff.toml")) ||
		fileExists(filepath.Join(root, ".ruff.toml")) ||
		fileExistsInDir(root, "pyproject.toml", "ruff") {
		add("ruff")
	}
	if fileExists(filepath.Join(root, ".eslintrc.js")) ||
		fileExists(filepath.Join(root, ".eslintrc.json")) ||
		fileExists(filepath.Join(root, ".eslintrc.yml")) ||
		fileExists(filepath.Join(root, "eslint.config.js")) ||
		fileExists(filepath.Join(root, "eslint.config.mjs")) {
		add("eslint")
	}
	if fileExists(filepath.Join(root, ".prettierrc")) ||
		fileExists(filepath.Join(root, ".prettierrc.json")) ||
		fileExists(filepath.Join(root, ".prettierrc.js")) {
		add("prettier")
	}
	if fileExists(filepath.Join(root, "clippy.toml")) ||
		fileExists(filepath.Join(root, "rustfmt.toml")) {
		add("clippy")
	}
	if dirExists(filepath.Join(root, ".github", "workflows")) {
		add("github-actions")
	}
	if fileExists(filepath.Join(root, ".gitlab-ci.yml")) {
		add("gitlab-ci")
	}
	if fileExists(filepath.Join(root, "Makefile")) {
		add("make")
	}
	if fileExists(filepath.Join(root, "Taskfile.yml")) ||
		fileExists(filepath.Join(root, "Taskfile.yaml")) {
		add("task")
	}
	if fileExists(filepath.Join(root, "Dockerfile")) {
		add("docker")
	}
	if fileExists(filepath.Join(root, "docker-compose.yml")) ||
		fileExists(filepath.Join(root, "docker-compose.yaml")) {
		add("docker-compose")
	}
	if fileExists(filepath.Join(root, ".sin-code")) || fileExists(filepath.Join(root, ".sin")) {
		add("sin-code")
	}
	sort.Strings(tools)
	return tools
}

func (c *InitCommand) detectKeyDirs(root string) []string {
	commonDirs := []struct {
		name string
		tags []string
	}{
		{"cmd", []string{"cmd/"}},
		{"internal", []string{"internal/"}},
		{"src", []string{"src/"}},
		{"lib", []string{"lib/"}},
		{"pkg", []string{"pkg/"}},
		{"tests", []string{"tests/"}},
		{"test", []string{"test/"}},
		{"docs", []string{"docs/"}},
		{"scripts", []string{"scripts/"}},
		{"skills", []string{"skills/"}},
		{"api", []string{"api/"}},
		{"web", []string{"web/"}},
		{"configs", []string{"configs/"}},
		{"templates", []string{"templates/"}},
	}
	var dirs []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return dirs
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if isIgnoredDir(name) {
			continue
		}
		for _, cd := range commonDirs {
			if name == cd.name {
				dirs = append(dirs, cd.tags[0])
				break
			}
		}
	}
	sort.Strings(dirs)
	return dirs
}

func (c *InitCommand) detectCI(root string) string {
	if dirExists(filepath.Join(root, ".github", "workflows")) {
		return "GitHub Actions"
	}
	if fileExists(filepath.Join(root, ".gitlab-ci.yml")) {
		return "GitLab CI"
	}
	if dirExists(filepath.Join(root, ".circleci")) {
		return "CircleCI"
	}
	if fileExists(filepath.Join(root, "Jenkinsfile")) {
		return "Jenkins"
	}
	if fileExists(filepath.Join(root, ".drone.yml")) {
		return "Drone"
	}
	return ""
}

func (c *InitCommand) detectTestFramework(projectType, root string) string {
	switch projectType {
	case "go":
		return "go test"
	case "python":
		if fileExists(filepath.Join(root, "pytest.ini")) ||
			fileExists(filepath.Join(root, "conftest.py")) ||
			fileExistsInDir(root, "pyproject.toml", "pytest") ||
			fileExistsInDir(root, "setup.cfg", "pytest") {
			return "pytest"
		}
		if fileExists(filepath.Join(root, "tox.ini")) {
			return "tox"
		}
		return "unittest"
	case "node":
		pkg := readPackageJSON(root)
		if pkg != nil {
			if _, ok := pkg["jest"]; ok {
				return "jest"
			}
			if _, ok := pkg["vitest"]; ok {
				return "vitest"
			}
			if _, ok := pkg["mocha"]; ok {
				return "mocha"
			}
			if scripts, ok := pkg["scripts"].(map[string]any); ok {
				if _, ok := scripts["test"]; ok {
					return "npm test"
				}
			}
		}
		return "npm test"
	case "rust":
		return "cargo test"
	default:
		return ""
	}
}

func (c *InitCommand) fillCommands(info *ProjectInfo, root string) {
	switch info.Type {
	case "go":
		info.BuildCmd = "go build ./..."
		info.TestCmd = "go test ./... -race -count=1"
		info.LintCmd = "go vet ./..."
		if contains(info.Tools, "golangci-lint") {
			info.LintCmd = "golangci-lint run"
		}
		info.ModulePath = c.extractGoModule(root)
	case "python":
		if fileExists(filepath.Join(root, "pyproject.toml")) {
			info.BuildCmd = "pip install -e ."
		} else if fileExists(filepath.Join(root, "setup.py")) {
			info.BuildCmd = "python setup.py build"
		}
		info.TestCmd = "pytest"
		if info.TestFramework == "unittest" {
			info.TestCmd = "python -m pytest"
		}
		info.LintCmd = "ruff check ."
		if !contains(info.Tools, "ruff") {
			info.LintCmd = "python -m pyflakes ."
		}
	case "node":
		info.BuildCmd = "npm run build"
		info.TestCmd = "npm test"
		info.LintCmd = "npm run lint"
		if contains(info.Tools, "eslint") {
			info.LintCmd = "npx eslint ."
		}
		pkg := readPackageJSON(root)
		if pkg != nil {
			if name, ok := pkg["name"].(string); ok {
				info.ModulePath = name
			}
		}
	case "rust":
		info.BuildCmd = "cargo build"
		info.TestCmd = "cargo test"
		info.LintCmd = "cargo clippy"
		info.ModulePath = c.extractCargoPackage(root)
	default:
		if fileExists(filepath.Join(root, "Makefile")) {
			info.BuildCmd = "make build"
			info.TestCmd = "make test"
			info.LintCmd = "make lint"
		}
	}
}

func (c *InitCommand) extractGoModule(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func (c *InitCommand) extractCargoPackage(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "Cargo.toml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name = ") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "name = "))
			val = strings.Trim(val, "\"'")
			return val
		}
	}
	return ""
}

func (c *InitCommand) GenerateAGENTS(info *ProjectInfo) string {
	var b strings.Builder
	b.WriteString("## Project Description\n\n")
	b.WriteString(fmt.Sprintf("This is a **%s** project", info.Type))
	if info.ModulePath != "" {
		b.WriteString(fmt.Sprintf(" (`%s`)", info.ModulePath))
	}
	b.WriteString(".\n\n")

	b.WriteString("## Build / Test / Lint\n\n")
	b.WriteString("| Command | Description |\n")
	b.WriteString("|---|---|\n")
	if info.BuildCmd != "" {
		b.WriteString(fmt.Sprintf("| `%s` | Build | |\n", info.BuildCmd))
	}
	if info.TestCmd != "" {
		b.WriteString(fmt.Sprintf("| `%s` | Test | |\n", info.TestCmd))
	}
	if info.LintCmd != "" {
		b.WriteString(fmt.Sprintf("| `%s` | Lint | |\n", info.LintCmd))
	}
	b.WriteString("\n")

	if info.TestFramework != "" {
		b.WriteString(fmt.Sprintf("**Test framework:** %s\n\n", info.TestFramework))
	}

	if len(info.KeyDirs) > 0 {
		b.WriteString("## Architecture\n\n")
		b.WriteString("Key directories:\n\n")
		for _, d := range info.KeyDirs {
			b.WriteString(fmt.Sprintf("- `%s`\n", d))
		}
		b.WriteString("\n")
	}

	if len(info.Tools) > 0 {
		b.WriteString("## Conventions & Tools\n\n")
		b.WriteString("Detected tools:\n\n")
		for _, t := range info.Tools {
			b.WriteString(fmt.Sprintf("- %s\n", t))
		}
		b.WriteString("\n")
	}

	if info.CI != "" {
		b.WriteString(fmt.Sprintf("## CI/CD\n\nCI: %s\n\n", info.CI))
	}

	b.WriteString("## Development Workflow\n\n")
	b.WriteString("- Follow existing code conventions in the codebase.\n")
	b.WriteString("- Run lint and tests before committing.\n")
	b.WriteString("- Use conventional commits (`feat:`, `fix:`, `docs:`).\n")
	b.WriteString("\n")

	return b.String()
}

func (c *InitCommand) mergeWithExisting(generated, existing string) string {
	if existing == "" {
		return generated
	}
	var b strings.Builder
	b.WriteString(generated)
	b.WriteString("\n---\n\n")
	b.WriteString("## Preserved from existing AGENTS.md\n\n")
	b.WriteString(strings.TrimSpace(existing))
	b.WriteString("\n")
	return b.String()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExistsInDir(root, filename, substring string) bool {
	data, err := os.ReadFile(filepath.Join(root, filename))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substring)
}

func readPackageJSON(root string) map[string]any {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	pkg := make(map[string]any)
	// Simple parse: look for key patterns without importing encoding/json
	// to keep the function self-contained and testable.
	raw := string(data)
	if strings.Contains(raw, "\"jest\"") {
		pkg["jest"] = true
	}
	if strings.Contains(raw, "\"vitest\"") {
		pkg["vitest"] = true
	}
	if strings.Contains(raw, "\"mocha\"") {
		pkg["mocha"] = true
	}
	if strings.Contains(raw, "\"name\"") {
		start := strings.Index(raw, "\"name\"")
		if start >= 0 {
			rest := raw[start:]
			colon := strings.Index(rest, ":")
			if colon >= 0 {
				afterColon := rest[colon+1:]
				quoteStart := strings.Index(afterColon, "\"")
				if quoteStart >= 0 {
					afterQuote := afterColon[quoteStart+1:]
					quoteEnd := strings.Index(afterQuote, "\"")
					if quoteEnd >= 0 {
						pkg["name"] = afterQuote[:quoteEnd]
					}
				}
			}
		}
	}
	if strings.Contains(raw, "\"scripts\"") {
		pkg["scripts"] = map[string]any{}
		if strings.Contains(raw, "\"test\"") {
			pkg["scripts"] = map[string]any{"test": true}
		}
	}
	return pkg
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func isIgnoredDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "__pycache__", ".sin-code",
		".sin", "dist", "build", "target", ".cache", "tmp", ".idea", ".vscode":
		return true
	}
	return false
}
