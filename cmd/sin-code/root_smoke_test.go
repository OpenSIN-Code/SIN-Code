// SPDX-License-Identifier: MIT
// Purpose: Root-package smoke tests for the `sin-code` CLI surface.
// Complements the subprocess-fork tests in main_version_test.go and the
// per-subcommand unit tests scattered under internal/. Targets coverage
// gaps on the Cmd constructors and the root cobra wiring itself.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
)

// runSinSubprocess execs the current test binary in a forked process so that
// rootCmd.Execute() does not mutate global cobra state shared with sibling
// tests in this package. The trigger env key (caller-defined) is the only
// switch the child looks at; everything else behaves like a real invocation.
func runSinSubprocess(t *testing.T, testFilter, triggerKey string, extraEnv map[string]string) string {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(bin, "-test.run="+testFilter)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		triggerKey+"=1",
		"SIN_CODE_NO_UPDATE_CHECK=1",
		"HOME="+t.TempDir(),
	)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess %s failed: %v\noutput: %s", testFilter, err, out)
	}
	return string(out)
}

// TestRoot_Help: `sin-code --help` must print the cobra "Available Commands:"
// section so users can discover subcommands. Forked because rootCmd is a
// shared package singleton; running Execute() in-process mutates flags.
func TestRoot_Help(t *testing.T) {
	if os.Getenv("TEST_ROOT_HELP") == "1" {
		os.Args = []string{"sin-code", "--help"}
		main()
		return
	}
	out := runSinSubprocess(t, "^TestRoot_Help$", "TEST_ROOT_HELP", nil)
	if !strings.Contains(out, "Available Commands:") {
		t.Errorf("--help should contain 'Available Commands:', got: %q", out)
	}
	if !strings.Contains(out, "sin-code") {
		t.Errorf("--help should contain binary name 'sin-code', got: %q", out)
	}
	if strings.Contains(out, "unknown command") {
		t.Errorf("--help output should not contain error text, got: %q", out)
	}
}

// TestRoot_VersionFlag: `sin-code --version` prints the build-time version
// string built by internal.versionString() (Version, commit, date).
func TestRoot_VersionFlag(t *testing.T) {
	if os.Getenv("TEST_ROOT_VERSION") == "1" {
		os.Args = []string{"sin-code", "--version"}
		main()
		return
	}
	out := runSinSubprocess(t, "^TestRoot_VersionFlag$", "TEST_ROOT_VERSION", nil)
	if !strings.Contains(out, "sin-code") {
		t.Errorf("--version should contain binary name, got: %q", out)
	}
	if !strings.Contains(out, "commit") {
		t.Errorf("--version should contain 'commit' annotation, got: %q", out)
	}
	if !strings.Contains(out, internal.Version) {
		t.Errorf("--version should contain Version=%q, got: %q", internal.Version, out)
	}
}

// TestRoot_UnknownCommand: invoking an unregistered subcommand must print
// "unknown command" to stderr and produce a non-zero exit. Cobra routes the
// error through internal.PrintError -> os.Exit(1); the subprocess lets us
// observe both the exit code AND the stderr text.
func TestRoot_UnknownCommand(t *testing.T) {
	if os.Getenv("TEST_ROOT_UNKNOWN") == "1" {
		os.Args = []string{"sin-code", "foo-bar-baz"}
		main()
		return
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(bin, "-test.run=^TestRoot_UnknownCommand$")
	cmd.Env = append(os.Environ(),
		"TEST_ROOT_UNKNOWN=1",
		"SIN_CODE_NO_UPDATE_CHECK=1",
		"HOME="+t.TempDir(),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown command, err=nil\noutput: %s", out)
	}
	if !strings.Contains(string(out), "unknown command") {
		t.Errorf("stderr should contain 'unknown command', got: %q", out)
	}
	if !strings.Contains(string(out), "foo-bar-baz") {
		t.Errorf("stderr should reference the offending command name, got: %q", out)
	}
	// PrintError prefixes with "sin-code:" — verify the branded error format.
	if !strings.Contains(string(out), "sin-code:") {
		t.Errorf("expected 'sin-code:' prefix from PrintError, got: %q", out)
	}
}

// findConfigSub is a tiny helper that locates a leaf sub-cmd by name on
// internal.ConfigCmd. We can't access the private configInitCmd / configShowCmd
// vars from package main, but ConfigCmd is exported and exposes its children
// via .Commands().
func findConfigSub(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, c := range internal.ConfigCmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("config subcommand %q not registered on internal.ConfigCmd", name)
	return nil
}

// TestConfig_Init: `sin-code config init` writes the default TOML config to
// the user-level path ($HOME/.config/sin/sin-code.toml).
// Calls the leaf RunE directly (same pattern as config_coverage_test.go for
// the internal package) because cmd.SetArgs on the global ConfigCmd does not
// reliably route to the registered subcommand — the public ConfigCmd tree
// shares flag-parser state with rootCmd.
func TestConfig_Init(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	getStdout := captureStdout(t)
	initCmd := findConfigSub(t, "init")
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("config init RunE: %v", err)
	}
	stdout := getStdout()

	want := filepath.Join(home, ".config", "sin", "sin-code.toml")
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("expected user config at %s, not found: %v", want, err)
	}
	body := string(got)
	if !strings.Contains(body, "theme =") {
		t.Errorf("config should contain theme key, got:\n%s", body)
	}
	if !strings.Contains(body, "llm.base_url") {
		t.Errorf("config should contain llm.base_url key, got:\n%s", body)
	}
	if !strings.Contains(stdout, "Created") {
		t.Errorf("init should print 'Created' confirmation, got: %q", stdout)
	}
}

// TestConfig_Show_WithSecretMasking: `config show` must mask llm.api_key by
// default and reveal it when --plain is passed. Seeds a config file with a
// recognizable plaintext secret, then exercises both modes. Calls Show's
// RunE directly with --plain toggled via cmd.Flags() Set (same convention
// as config_coverage_test.go in the internal package).
func TestConfig_Show_WithSecretMasking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgDir := filepath.Join(home, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "sin-code.toml")
	// Keep the secret ≤8 chars so maskSecret() emits the literal "***"
	// sentinel; longer secrets get the partial redacted form `head...tail`
	// (4+4) instead (see internal/config.go maskSecret()).
	const secret = "shortpwd"
	if err := os.WriteFile(cfgPath, []byte(
		"theme = \"dark\"\n"+
			"llm.base_url = \"https://example.invalid\"\n"+
			"llm.api_key = \""+secret+"\"\n"+
			"llm.model = \"stub\"\n",
	), 0o644); err != nil {
		t.Fatalf("write seed config: %v", err)
	}

	showCmd := findConfigSub(t, "show")

	// Default (masked) mode: secret must NOT appear; "***" marker must.
	getMaskedOut := captureStdout(t)
	if err := showCmd.Flags().Set("plain", "false"); err != nil {
		t.Fatalf("flag set plain=false: %v", err)
	}
	if err := showCmd.RunE(showCmd, nil); err != nil {
		t.Fatalf("config show (masked) RunE: %v", err)
	}
	masked := getMaskedOut()
	if strings.Contains(masked, secret) {
		t.Errorf("config show should mask llm.api_key, but plaintext leaked:\n%s", masked)
	}
	if !strings.Contains(masked, "***") {
		t.Errorf("config show should contain '***' masking marker, got:\n%s", masked)
	}

	// --plain mode: secret must appear verbatim.
	getPlainOut := captureStdout(t)
	if err := showCmd.Flags().Set("plain", "true"); err != nil {
		t.Fatalf("flag set plain=true: %v", err)
	}
	if err := showCmd.RunE(showCmd, nil); err != nil {
		t.Fatalf("config show --plain RunE: %v", err)
	}
	plain := getPlainOut()
	if !strings.Contains(plain, secret) {
		t.Errorf("config show --plain should reveal secret, got:\n%s", plain)
	}
}

// TestHub_List: `sin-code hub list` must emit at least one tool row from
// the static catalog (hub.DefaultCatalog()). Operating on a freshly built
// cobra command avoids rootCmd state pollution across sibling tests.
// Uses os.Stdout capture because the hub subcommand writes with fmt.Print
// (not cmd.OutOrStdout) so cobra.SetOut does not redirect the output.
func TestHub_List(t *testing.T) {
	getOut := captureStdout(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("hub list: %v", err)
	}
	body := getOut()
	if strings.TrimSpace(body) == "" {
		t.Fatal("hub list produced empty output")
	}
	rows := strings.Count(body, "\n")
	if rows < 2 {
		t.Errorf("hub list should have multiple rows, got %d lines:\n%s", rows, body)
	}
	// Every static catalog contains well-known tools; assert at least one.
	known := []string{"discover", "execute", "map", "grasp", "chat"}
	matched := 0
	for _, k := range known {
		if strings.Contains(body, k) {
			matched++
		}
	}
	if matched == 0 {
		t.Errorf("hub list should mention at least one well-known tool (%v), got:\n%s", known, body)
	}
}

// TestLedger_List_SinCodeLedgerEnv: `sin-code ledger tools --heatmap --json`
// must honor SIN_CODE_LEDGER, open the SQLite store there, and emit JSON
// whose per-tool entries carry both `total` and `by_outcome` keys (the
// UsageCount schema; size=0 is fine — we just want byte-valid JSON shape).
func TestLedger_List_SinCodeLedgerEnv(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ledger.db")
	t.Setenv("SIN_CODE_LEDGER", dbPath)
	t.Setenv("SIN_CODE_HOME", tmpDir)

	getOut := captureStdout(t)
	cmd := NewLedgerCmd()
	cmd.SetArgs([]string{"tools", "--heatmap", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ledger tools --heatmap --json: %v", err)
	}

	// The output must be a JSON array (possibly empty) that is round-trippable.
	body := strings.TrimSpace(getOut())
	if body == "" {
		t.Fatal("ledger tools --json emitted empty body")
	}
	var probe []map[string]any
	if err := json.Unmarshal([]byte(body), &probe); err != nil {
		t.Fatalf("output is not valid JSON array: %v\nbody: %s", err, body)
	}
	// Empty array is acceptable here — we only assert the schema is intact
	// and the env override was honored (file got created) — but if any
	// entry exists, every entry must have `total` and `by_outcome` keys.
	for i, entry := range probe {
		if _, ok := entry["total"]; !ok {
			t.Errorf("entry[%d] missing required key 'total': %v", i, entry)
		}
		if _, ok := entry["by_outcome"]; !ok {
			t.Errorf("entry[%d] missing required key 'by_outcome': %v", i, entry)
		}
	}
	// SIN_CODE_LEDGER must have been honored — the file exists and is non-empty.
	if info, err := os.Stat(dbPath); err != nil || info.Size() == 0 {
		t.Errorf("expected non-empty ledger.db at SIN_CODE_LEDGER path, stat=%v err=%v", info, err)
	}
}

// TestSessions_List_Empty: `sin-code sessions list` against a fresh SIN_CODE_HOME
// must print 'no sessions' rather than erroring. DefaultPath respects
// SIN_CODE_HOME so we never touch the real user filesystem.
func TestSessions_List_Empty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIN_CODE_HOME", home)

	getOut := captureStdout(t)
	cmd := NewSessionsCmd()
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sessions list: %v", err)
	}
	body := strings.TrimSpace(getOut())
	if body != "no sessions" {
		t.Errorf("expected 'no sessions' for empty DB, got: %q", body)
	}

	// --json on an empty DB should marshal to '[]' (the empty Info slice).
	getOut2 := captureStdout(t)
	cmd2 := NewSessionsCmd()
	cmd2.SetArgs([]string{"list", "--json"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("sessions list --json: %v", err)
	}
	body2 := strings.TrimSpace(getOut2())
	var probe []any
	if err := json.Unmarshal([]byte(body2), &probe); err != nil {
		t.Errorf("sessions list --json not valid JSON: %v\nbody: %s", err, body2)
	}
	if len(probe) != 0 {
		t.Errorf("expected empty JSON array, got %d entries:\n%s", len(probe), body2)
	}
}
