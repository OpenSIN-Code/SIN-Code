// SPDX-License-Identifier: MIT
// Purpose: `sin-code doctor` — unified health check that verifies the
// entire SIN-Code installation is healthy: Go toolchain, binary version,
// config file, SQLite databases, MCP/skill ecosystem, external tools,
// and mandate compliance (M5 module path, M2 CGO_ENABLED=0).
package main

// sin-debt: shrink, upgrade: inline DB check wrappers when doctor command grows

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
)

// CheckResult holds the outcome of a single doctor check.
type CheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pass", "warn", "fail"
	Detail string `json:"detail"`
}

// Check status constants.
const (
	statusPass = "pass"
	statusWarn = "warn"
	statusFail = "fail"
)

// Required module path per mandate M5.
const requiredModulePath = "github.com/OpenSIN-Code/SIN-Code"

// Minimum Go version per AGENTS.md §9.
const minGoMajor, minGoMinor = 1, 23

// External tools to probe on PATH.
var doctorExternalTools = []string{"git", "gh", "docker", "ruff", "python3", "node"}

// ── Test hooks (overridable in tests) ──────────────────────────────────

var doctorGoVersionHook = func() (string, error) {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

var doctorSinCodeVersionHook = func() string { return internal.Version }

var doctorConfigPathHook = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "sin", "sin-code.toml")
}

var doctorSessionDBPathHook = session.DefaultPath
var doctorLessonsDBPathHook = lessons.DefaultPath
var doctorGoalsDBPathHook = autonomy.DefaultPath
var doctorLedgerDBPathHook = ledger.DefaultPath

var doctorGoModPathHook = func() string {
	wd, err := os.Getwd()
	if err != nil {
		return "go.mod"
	}
	return filepath.Join(wd, "go.mod")
}

var doctorBuildInfoHook = debug.ReadBuildInfo

var doctorSkillStatusHook = func(ctx context.Context) []skillmgr.SkillStatus {
	return skillmgr.Status(ctx)
}

var doctorLookPathHook = exec.LookPath

// ── Check functions ────────────────────────────────────────────────────

// checkGoToolchain verifies Go 1.23+ is available on PATH.
func checkGoToolchain() CheckResult {
	ver, err := doctorGoVersionHook()
	if err != nil {
		return CheckResult{
			Name:   "go-toolchain",
			Status: statusFail,
			Detail: fmt.Sprintf("go version failed: %v", err),
		}
	}
	major, minor, ok := parseGoVersion(ver)
	if !ok {
		return CheckResult{
			Name:   "go-toolchain",
			Status: statusFail,
			Detail: fmt.Sprintf("cannot parse version from %q", ver),
		}
	}
	if major < minGoMajor || (major == minGoMajor && minor < minGoMinor) {
		return CheckResult{
			Name:   "go-toolchain",
			Status: statusFail,
			Detail: fmt.Sprintf("%s — need Go %d.%d+", ver, minGoMajor, minGoMinor),
		}
	}
	return CheckResult{
		Name:   "go-toolchain",
		Status: statusPass,
		Detail: ver,
	}
}

// checkSinCodeBinary verifies the sin-code binary is a versioned build (not "dev").
func checkSinCodeBinary() CheckResult {
	v := doctorSinCodeVersionHook()
	if v == "" || v == "dev" {
		return CheckResult{
			Name:   "sin-code-binary",
			Status: statusWarn,
			Detail: "running a dev build — use a tagged release for production",
		}
	}
	return CheckResult{
		Name:   "sin-code-binary",
		Status: statusPass,
		Detail: v,
	}
}

// checkConfigFile verifies the config file exists and is valid.
func checkConfigFile() CheckResult {
	path := doctorConfigPathHook()
	if path == "" {
		return CheckResult{
			Name:   "config-file",
			Status: statusWarn,
			Detail: "cannot resolve home directory",
		}
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:   "config-file",
				Status: statusWarn,
				Detail: fmt.Sprintf("not configured: %s — run 'sin-code config init'", path),
			}
		}
		return CheckResult{
			Name:   "config-file",
			Status: statusFail,
			Detail: fmt.Sprintf("stat %s: %v", path, err),
		}
	}
	_, err := internal.LoadMergedConfig()
	if err != nil {
		return CheckResult{
			Name:   "config-file",
			Status: statusFail,
			Detail: fmt.Sprintf("load config: %v", err),
		}
	}
	return CheckResult{
		Name:   "config-file",
		Status: statusPass,
		Detail: path,
	}
}

// checkDBFile is a generic helper for checking SQLite database files.
func checkDBFile(name, path string, missingDetail string) CheckResult {
	if path == "" {
		return CheckResult{
			Name:   name,
			Status: statusWarn,
			Detail: "cannot resolve path",
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:   name,
				Status: statusWarn,
				Detail: missingDetail,
			}
		}
		return CheckResult{
			Name:   name,
			Status: statusFail,
			Detail: fmt.Sprintf("stat %s: %v", path, err),
		}
	}
	if info.IsDir() {
		return CheckResult{
			Name:   name,
			Status: statusFail,
			Detail: fmt.Sprintf("%s is a directory, not a file", path),
		}
	}
	if info.Size() == 0 {
		return CheckResult{
			Name:   name,
			Status: statusWarn,
			Detail: fmt.Sprintf("%s is empty (0 bytes)", path),
		}
	}
	if info.Mode().Perm()&0o400 == 0 {
		return CheckResult{
			Name:   name,
			Status: statusFail,
			Detail: fmt.Sprintf("%s is not readable", path),
		}
	}
	return CheckResult{
		Name:   name,
		Status: statusPass,
		Detail: fmt.Sprintf("%s (%d bytes)", path, info.Size()),
	}
}

// checkSessionDB verifies the session SQLite DB exists and is readable.
func checkSessionDB() CheckResult {
	return checkDBFile("session-db", doctorSessionDBPathHook(), "no sessions yet — run 'sin-code chat' to create one")
}

// checkLessonsDB verifies the lessons SQLite DB exists.
func checkLessonsDB() CheckResult {
	return checkDBFile("lessons-db", doctorLessonsDBPathHook(), "no lessons DB yet — failures during agent runs create this")
}

// checkGoalsDB verifies the goal queue SQLite DB exists.
func checkGoalsDB() CheckResult {
	return checkDBFile("goals-db", doctorGoalsDBPathHook(), "no goals DB yet — run 'sin-code goal add' to create one")
}

// checkLedgerDB verifies the ledger SQLite DB exists.
func checkLedgerDB() CheckResult {
	return checkDBFile("ledger-db", doctorLedgerDBPathHook(), "no ledger DB yet — sessions create this automatically")
}

// checkMCPServers checks which ecosystem skills (MCP servers) are installed and runnable.
func checkMCPServers(ctx context.Context) CheckResult {
	statuses := doctorSkillStatusHook(ctx)
	if len(statuses) == 0 {
		return CheckResult{
			Name:   "mcp-servers",
			Status: statusWarn,
			Detail: "no ecosystem skills registered",
		}
	}
	var installed, runnable int
	var notRunnable []string
	for _, st := range statuses {
		if st.Installed {
			installed++
		}
		if st.Runnable {
			runnable++
		}
		if !st.Runnable {
			notRunnable = append(notRunnable, st.Name)
		}
	}
	total := len(statuses)
	if runnable == total {
		return CheckResult{
			Name:   "mcp-servers",
			Status: statusPass,
			Detail: fmt.Sprintf("%d/%d skills runnable", runnable, total),
		}
	}
	if runnable == 0 {
		return CheckResult{
			Name:   "mcp-servers",
			Status: statusWarn,
			Detail: fmt.Sprintf("0/%d skills runnable — run 'sin-code skill install all'", total),
		}
	}
	return CheckResult{
		Name:   "mcp-servers",
		Status: statusWarn,
		Detail: fmt.Sprintf("%d/%d runnable; not runnable: %s", runnable, total, strings.Join(notRunnable, ", ")),
	}
}

// checkExternalTools probes PATH for required external tools.
func checkExternalTools() CheckResult {
	var missing []string
	var found []string
	for _, tool := range doctorExternalTools {
		if _, err := doctorLookPathHook(tool); err != nil {
			missing = append(missing, tool)
		} else {
			found = append(found, tool)
		}
	}
	if len(missing) == 0 {
		return CheckResult{
			Name:   "external-tools",
			Status: statusPass,
			Detail: fmt.Sprintf("all %d tools on PATH: %s", len(found), strings.Join(found, ", ")),
		}
	}
	if len(found) == 0 {
		return CheckResult{
			Name:   "external-tools",
			Status: statusFail,
			Detail: fmt.Sprintf("none found on PATH; missing: %s", strings.Join(missing, ", ")),
		}
	}
	return CheckResult{
		Name:   "external-tools",
		Status: statusWarn,
		Detail: fmt.Sprintf("missing: %s (found: %s)", strings.Join(missing, ", "), strings.Join(found, ", ")),
	}
}

// checkModulePath verifies go.mod contains the correct module path (M5).
func checkModulePath() CheckResult {
	path := doctorGoModPathHook()
	data, err := os.ReadFile(path)
	if err != nil {
		return CheckResult{
			Name:   "module-path",
			Status: statusWarn,
			Detail: fmt.Sprintf("cannot read go.mod: %v", err),
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			modPath := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if modPath == requiredModulePath {
				return CheckResult{
					Name:   "module-path",
					Status: statusPass,
					Detail: modPath,
				}
			}
			return CheckResult{
				Name:   "module-path",
				Status: statusFail,
				Detail: fmt.Sprintf("got %q, want %q (mandate M5)", modPath, requiredModulePath),
			}
		}
	}
	return CheckResult{
		Name:   "module-path",
		Status: statusWarn,
		Detail: "no 'module' directive found in go.mod",
	}
}

// checkCGO verifies CGO_ENABLED=0 in the build settings (M2).
func checkCGO() CheckResult {
	info, ok := doctorBuildInfoHook()
	if !ok {
		return CheckResult{
			Name:   "cgo-enabled",
			Status: statusWarn,
			Detail: "build info unavailable (not a compiled binary?)",
		}
	}
	for _, s := range info.Settings {
		if s.Key == "CGO_ENABLED" {
			if s.Value == "0" {
				return CheckResult{
					Name:   "cgo-enabled",
					Status: statusPass,
					Detail: "CGO_ENABLED=0 (mandate M2)",
				}
			}
			return CheckResult{
				Name:   "cgo-enabled",
				Status: statusFail,
				Detail: fmt.Sprintf("CGO_ENABLED=%s — must be 0 for static binary (mandate M2)", s.Value),
			}
		}
	}
	return CheckResult{
		Name:   "cgo-enabled",
		Status: statusWarn,
		Detail: "CGO_ENABLED setting not recorded in build info",
	}
}

// runAllChecks executes every doctor check and returns the results in order.
func runAllChecks(ctx context.Context) []CheckResult {
	return []CheckResult{
		checkGoToolchain(),
		checkSinCodeBinary(),
		checkConfigFile(),
		checkSessionDB(),
		checkLessonsDB(),
		checkGoalsDB(),
		checkLedgerDB(),
		checkMCPServers(ctx),
		checkExternalTools(),
		checkModulePath(),
		checkCGO(),
	}
}

// hasFail returns true if any result has status "fail".
func hasFail(results []CheckResult) bool {
	for _, r := range results {
		if r.Status == statusFail {
			return true
		}
	}
	return false
}

// formatDoctorTable renders a human-readable table of check results.
func formatDoctorTable(results []CheckResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-20s %-6s %s\n", "CHECK", "STATUS", "DETAIL")
	fmt.Fprintf(&b, "%s\n", strings.Repeat("-", 60))
	for _, r := range results {
		icon := "OK"
		switch r.Status {
		case statusPass:
			icon = "PASS"
		case statusWarn:
			icon = "WARN"
		case statusFail:
			icon = "FAIL"
		}
		fmt.Fprintf(&b, "%-20s %-6s %s\n", r.Name, icon, r.Detail)
	}
	return b.String()
}

// ── helpers ────────────────────────────────────────────────────────────

// parseGoVersion extracts major and minor from a "go version goX.Y.Z ..." string.
func parseGoVersion(s string) (major, minor int, ok bool) {
	re := regexp.MustCompile(`go(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(s)
	if len(matches) < 3 {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(matches[1])
	minor, err2 := strconv.Atoi(matches[2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// ── cobra command ──────────────────────────────────────────────────────

// NewDoctorCmd builds the `doctor` cobra subcommand.
func NewDoctorCmd() *cobra.Command {
	var (
		jsonOut bool
		quiet   bool
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run a unified health check of the entire SIN-Code installation",
		Long: `sin-code doctor verifies that every component of your SIN-Code
installation is healthy: Go toolchain, binary version, config file,
SQLite databases, MCP/skill ecosystem, external tools, and mandate
compliance (M5 module path, M2 CGO_ENABLED=0).

Each check reports PASS, WARN, or FAIL. The exit code is 0 when no
check fails, 1 when any check returns FAIL.

Flags:
  --json   Emit a JSON array of check results (for CI / scripting)
  --quiet  Only show checks that are WARN or FAIL`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			results := runAllChecks(ctx)

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(results); err != nil {
					return fmt.Errorf("doctor: encode json: %w", err)
				}
			} else {
				shown := results
				if quiet {
					var filtered []CheckResult
					for _, r := range results {
						if r.Status != statusPass {
							filtered = append(filtered, r)
						}
					}
					shown = filtered
				}
				out := formatDoctorTable(shown)
				fmt.Fprint(cmd.OutOrStdout(), out)
			}

			if hasFail(results) {
				return fmt.Errorf("doctor: one or more checks failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit results as a JSON array")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Only show WARN and FAIL checks")
	return cmd
}
