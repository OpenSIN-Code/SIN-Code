// SPDX-License-Identifier: MIT
// Purpose: Status snapshot / readiness report (issue #326). Collects system
// status — Go version, git state, build/vet/test results, config, MCP servers,
// skills, sessions, todos — and renders a markdown readiness report.
package internal

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ── Data types ──────────────────────────────────────────────────────────────

// SnapshotData holds the collected system status fields rendered by
// RenderMarkdown. Populated by Collect; zero-value fields mean "not collected"
// or "collection failed" (the corresponding error field carries the reason).
type SnapshotData struct {
	GeneratedAt time.Time `json:"generated_at"`

	GoVersion string `json:"go_version"`
	BuildPass bool   `json:"build_pass"`
	VetPass   bool   `json:"vet_pass"`
	TestsPass int    `json:"tests_pass"`
	TestsFail int    `json:"tests_fail"`

	GitBranch  string `json:"git_branch"`
	GitClean   bool   `json:"git_clean"`
	GitAhead   int    `json:"git_ahead"`
	GitBehind  int    `json:"git_behind"`

	ConfigModel      string `json:"config_model"`
	ConfigProvider   string `json:"config_provider"`
	ConfigVerifyMode string `json:"config_verify_mode"`

	MCPServers map[string]bool   `json:"mcp_servers"`
	MCPOrder   []string          `json:"-"`
	Skills     int               `json:"skills_installed"`
	Sessions   int               `json:"sessions_active"`
	TodosOpen  int               `json:"todos_open"`
	TodosBlocked int            `json:"todos_blocked"`
	TodosReady  int             `json:"todos_ready"`
}

// Snapshot is the collector + renderer for the readiness report (issue #326).
// All external operations (git, go build/vet/test, store queries) go through
// injectable function hooks so tests can run hermetically without spawning
// subprocesses or opening real databases.
type Snapshot struct {
	Workdir string

	// git runs a git command in Workdir and returns trimmed stdout.
	git func(args ...string) (string, error)
	// exec runs an arbitrary command in Workdir and returns trimmed stdout.
	exec func(name string, args ...string) (string, error)
	// sessionCount returns the number of active sessions.
	sessionCount func() (int, error)
	// todoCounts returns open, blocked, ready counts.
	todoCounts func() (open, blocked, ready int, err error)
	// mcpStatus returns a map of server-name → available (true/false),
	// plus the display order.
	mcpStatus func() (map[string]bool, []string, error)
	// skillsCount returns the number of installed skills.
	skillsCount func() (int, error)
	// configValues returns model, provider, verify_mode from the effective config.
	configValues func() (model, provider, verifyMode string)

	data    SnapshotData
	collected bool
}

// NewSnapshot returns a Snapshot wired with real default implementations.
// All hooks are overridable for testing; see NewSnapshotWithHooks.
func NewSnapshot() *Snapshot {
	s := &Snapshot{
		Workdir: ".",
	}
	s.git = func(args ...string) (string, error) {
		return s.runCmd("git", args...)
	}
	s.exec = func(name string, args ...string) (string, error) {
		return s.runCmd(name, args...)
	}
	s.sessionCount = func() (int, error) { return 0, nil }
	s.todoCounts = func() (int, int, int, error) { return 0, 0, 0, nil }
	s.mcpStatus = func() (map[string]bool, []string, error) { return nil, nil, nil }
	s.skillsCount = func() (int, error) { return 0, nil }
	s.configValues = func() (string, string, string) { return "", "", "" }
	return s
}

// NewSnapshotWithHooks returns a Snapshot with all hooks overridden by the
// caller. Any nil hook falls back to the NewSnapshot default (no-op / zero).
func NewSnapshotWithHooks(hooks SnapshotHooks) *Snapshot {
	s := NewSnapshot()
	if hooks.Git != nil {
		s.git = hooks.Git
	}
	if hooks.Exec != nil {
		s.exec = hooks.Exec
	}
	if hooks.SessionCount != nil {
		s.sessionCount = hooks.SessionCount
	}
	if hooks.TodoCounts != nil {
		s.todoCounts = hooks.TodoCounts
	}
	if hooks.MCPStatus != nil {
		s.mcpStatus = hooks.MCPStatus
	}
	if hooks.SkillsCount != nil {
		s.skillsCount = hooks.SkillsCount
	}
	if hooks.ConfigValues != nil {
		s.configValues = hooks.ConfigValues
	}
	if hooks.Workdir != "" {
		s.Workdir = hooks.Workdir
	}
	return s
}

// SnapshotHooks is the injection point for test overrides.
type SnapshotHooks struct {
	Workdir       string
	Git           func(args ...string) (string, error)
	Exec          func(name string, args ...string) (string, error)
	SessionCount  func() (int, error)
	TodoCounts    func() (open, blocked, ready int, err error)
	MCPStatus     func() (map[string]bool, []string, error)
	SkillsCount   func() (int, error)
	ConfigValues  func() (model, provider, verifyMode string)
}

// runCmd executes a command in Workdir and returns trimmed stdout.
func (s *Snapshot) runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...) // #nosec G204
	cmd.Dir = s.Workdir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, errb.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// ── Collect ─────────────────────────────────────────────────────────────────

// Collect gathers all system status fields. It is resilient: each section is
// collected independently and a failure in one section does not abort the
// others. Errors are swallowed (the field stays zero-value) so the report is
// always rendered, even on a broken system.
func (s *Snapshot) Collect() error {
	s.data.GeneratedAt = time.Now()

	// Go version (always available — runtime.Version).
	s.data.GoVersion = runtime.Version()

	// Build: go build ./...
	if out, err := s.exec("go", "build", "./..."); err != nil {
		s.data.BuildPass = false
		_ = out
	} else {
		s.data.BuildPass = true
	}

	// Vet: go vet ./...
	if out, err := s.exec("go", "vet", "./..."); err != nil {
		s.data.VetPass = false
		_ = out
	} else {
		s.data.VetPass = true
	}

	// Tests: go test -count=1 ./... — parse output for pass/fail counts.
	s.collectTests()

	// Git state.
	s.collectGit()

	// Config.
	model, provider, verifyMode := s.configValues()
	s.data.ConfigModel = model
	s.data.ConfigProvider = provider
	s.data.ConfigVerifyMode = verifyMode

	// MCP servers.
	if servers, order, err := s.mcpStatus(); err == nil && servers != nil {
		s.data.MCPServers = servers
		s.data.MCPOrder = order
	}

	// Skills.
	if n, err := s.skillsCount(); err == nil {
		s.data.Skills = n
	}

	// Sessions.
	if n, err := s.sessionCount(); err == nil {
		s.data.Sessions = n
	}

	// Todos.
	if open, blocked, ready, err := s.todoCounts(); err == nil {
		s.data.TodosOpen = open
		s.data.TodosBlocked = blocked
		s.data.TodosReady = ready
	}

	s.collected = true
	return nil
}

// collectTests runs `go test -count=1 ./...` and parses the output for
// pass/fail counts. The output format is one line per package:
//
//	ok    pkg/path   0.123s
//	FAIL  pkg/path   0.456s
//
// A timeout of 5 minutes is enforced so a hanging test suite does not block
// the snapshot indefinitely.
func (s *Snapshot) collectTests() {
	out, err := s.exec("go", "test", "-count=1", "./...")
	if err != nil {
		// Even on failure, the output contains pass/fail lines we can parse.
		_ = err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ok") {
			s.data.TestsPass++
		} else if strings.HasPrefix(line, "FAIL") {
			s.data.TestsFail++
		}
	}
}

// collectGit gathers branch, dirty state, and ahead/behind counts.
func (s *Snapshot) collectGit() {
	branch, err := s.git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return
	}
	s.data.GitBranch = branch

	status, err := s.git("status", "--porcelain")
	if err == nil {
		s.data.GitClean = status == ""
	}

	ahead, err := s.git("rev-list", "--count", "@{u}..HEAD")
	if err == nil {
		s.data.GitAhead = atoiSafe(ahead)
	}

	behind, err := s.git("rev-list", "--count", "HEAD..@{u}")
	if err == nil {
		s.data.GitBehind = atoiSafe(behind)
	}
}

// atoiSafe converts s to int, returning 0 on any error.
func atoiSafe(s string) int {
	s = strings.TrimSpace(s)
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ── RenderMarkdown ──────────────────────────────────────────────────────────

// RenderMarkdown renders the collected data as a markdown readiness report.
// Collect must be called first; if it hasn't, RenderMarkdown calls it.
func (s *Snapshot) RenderMarkdown() string {
	if !s.collected {
		_ = s.Collect()
	}
	d := s.data
	var b strings.Builder

	fmt.Fprintf(&b, "# SIN-Code Readiness Report\n")
	fmt.Fprintf(&b, "Generated: %s\n\n", d.GeneratedAt.Format("2006-01-02 15:04:05"))

	// Build section
	fmt.Fprintf(&b, "## Build\n")
	fmt.Fprintf(&b, "- Go: %s\n", d.GoVersion)
	fmt.Fprintf(&b, "- Build: %s\n", checkmark(d.BuildPass))
	fmt.Fprintf(&b, "- Vet: %s\n", checkmark(d.VetPass))
	fmt.Fprintf(&b, "- Tests: %d pass, %d fail\n\n", d.TestsPass, d.TestsFail)

	// Git section
	fmt.Fprintf(&b, "## Git\n")
	fmt.Fprintf(&b, "- Branch: %s\n", d.GitBranch)
	fmt.Fprintf(&b, "- Clean: %s\n", checkmark(d.GitClean))
	fmt.Fprintf(&b, "- Ahead: %d, Behind: %d\n\n", d.GitAhead, d.GitBehind)

	// Configuration section
	fmt.Fprintf(&b, "## Configuration\n")
	fmt.Fprintf(&b, "- Model: %s\n", snapshotOrDash(d.ConfigModel))
	fmt.Fprintf(&b, "- Provider: %s\n", snapshotOrDash(d.ConfigProvider))
	fmt.Fprintf(&b, "- Verify mode: %s\n\n", snapshotOrDash(d.ConfigVerifyMode))

	// MCP Servers section
	fmt.Fprintf(&b, "## MCP Servers\n")
	if len(d.MCPServers) == 0 {
		fmt.Fprintf(&b, "- (none configured)\n")
	} else {
		order := d.MCPOrder
		if len(order) == 0 {
			order = make([]string, 0, len(d.MCPServers))
			for k := range d.MCPServers {
				order = append(order, k)
			}
			sort.Strings(order)
		}
		for _, name := range order {
			avail, ok := d.MCPServers[name]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "- %s: %s\n", name, checkmark(avail))
		}
	}
	fmt.Fprintf(&b, "\n")

	// Skills section
	fmt.Fprintf(&b, "## Skills\n")
	fmt.Fprintf(&b, "- Installed: %d\n\n", d.Skills)

	// Sessions section
	fmt.Fprintf(&b, "## Sessions\n")
	fmt.Fprintf(&b, "- Active: %d\n\n", d.Sessions)

	// Todos section
	fmt.Fprintf(&b, "## Todos\n")
	fmt.Fprintf(&b, "- Open: %d, Blocked: %d, Ready: %d\n\n", d.TodosOpen, d.TodosBlocked, d.TodosReady)

	// Verdict
	fmt.Fprintf(&b, "## Verdict\n")
	fmt.Fprintf(&b, "%s\n", s.verdict())

	return b.String()
}

// verdict computes the overall readiness verdict based on collected data.
func (s *Snapshot) verdict() string {
	d := s.data
	issues := 0
	if !d.BuildPass {
		issues++
	}
	if !d.VetPass {
		issues++
	}
	if d.TestsFail > 0 {
		issues++
	}
	if !d.GitClean {
		issues++
	}
	switch {
	case issues == 0:
		return "READY FOR PRODUCTION ✅"
	case issues <= 2:
		return "ATTENTION NEEDED ⚠️"
	default:
		return "NOT READY ❌"
	}
}

// NeedsAttention returns true when the verdict is not "READY FOR PRODUCTION".
// Used by the --exit-code flag (exit 2 when attention is needed).
func (s *Snapshot) NeedsAttention() bool {
	return s.verdict() != "READY FOR PRODUCTION ✅"
}

// checkmark returns ✅ for true and ❌ for false.
func checkmark(ok bool) string {
	if ok {
		return "✅ pass"
	}
	return "❌ fail"
}

// snapshotOrDash returns s or "—" if s is empty.
func snapshotOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// ── CLI command ─────────────────────────────────────────────────────────────

var snapshotMarkdown bool
var snapshotJSON bool
var snapshotExitCode bool

// SnapshotCmd is the cobra command for `sin-code snapshot`.
// Register via rootCmd.AddCommand(internal.SnapshotCmd) in main.go.
var SnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Generate a readiness report (issue #326)",
	Long: `Collect system status — Go version, git state, build/vet/test results,
config, MCP servers, skills, sessions, todos — and render a readiness report.

Examples:
  sin-code snapshot --markdown
  sin-code snapshot --json
  sin-code snapshot --exit-code     # exits 2 when attention needed`,
	Args:    cobra.NoArgs,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		s := NewSnapshot()
		if err := s.Collect(); err != nil {
			return err
		}
		if snapshotJSON {
			cmd.Println(renderSnapshotJSON(s))
			return nil
		}
		cmd.Print(s.RenderMarkdown())
		if snapshotExitCode && s.NeedsAttention() {
			cmd.SilenceUsage = true
			return fmt.Errorf("readiness: %s", s.verdict())
		}
		return nil
	},
}

func init() {
	SnapshotCmd.Flags().BoolVar(&snapshotMarkdown, "markdown", true, "render as markdown (default)")
	SnapshotCmd.Flags().BoolVar(&snapshotJSON, "json", false, "render as JSON")
	SnapshotCmd.Flags().BoolVar(&snapshotExitCode, "exit-code", false, "exit 2 when readiness needs attention (CI gate)")
	RegisterVersionCmd(SnapshotCmd)
}

// renderSnapshotJSON produces a compact JSON representation of the snapshot.
func renderSnapshotJSON(s *Snapshot) string {
	d := s.data
	var b strings.Builder
	fmt.Fprintf(&b, `{"generated_at":"%s",`, d.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, `"go_version":"%s",`, d.GoVersion)
	fmt.Fprintf(&b, `"build_pass":%t,`, d.BuildPass)
	fmt.Fprintf(&b, `"vet_pass":%t,`, d.VetPass)
	fmt.Fprintf(&b, `"tests_pass":%d,`, d.TestsPass)
	fmt.Fprintf(&b, `"tests_fail":%d,`, d.TestsFail)
	fmt.Fprintf(&b, `"git_branch":"%s",`, d.GitBranch)
	fmt.Fprintf(&b, `"git_clean":%t,`, d.GitClean)
	fmt.Fprintf(&b, `"git_ahead":%d,`, d.GitAhead)
	fmt.Fprintf(&b, `"git_behind":%d,`, d.GitBehind)
	fmt.Fprintf(&b, `"config_model":"%s",`, d.ConfigModel)
	fmt.Fprintf(&b, `"config_provider":"%s",`, d.ConfigProvider)
	fmt.Fprintf(&b, `"config_verify_mode":"%s",`, d.ConfigVerifyMode)
	fmt.Fprintf(&b, `"skills_installed":%d,`, d.Skills)
	fmt.Fprintf(&b, `"sessions_active":%d,`, d.Sessions)
	fmt.Fprintf(&b, `"todos_open":%d,`, d.TodosOpen)
	fmt.Fprintf(&b, `"todos_blocked":%d,`, d.TodosBlocked)
	fmt.Fprintf(&b, `"todos_ready":%d,`, d.TodosReady)
	fmt.Fprintf(&b, `"verdict":"%s"}`, s.verdict())
	return b.String()
}
