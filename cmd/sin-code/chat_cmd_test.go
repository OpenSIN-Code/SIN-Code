// SPDX-License-Identifier: MIT
// Purpose: race-safe tests for the chat command's autoactivate
// wiring (issue #176) and the headless-mode sandbox defaults
// (issue #420). Covers the helpers, the session-start invocation
// through the hooklife runner, the `.sin-code/autoactivate.toml`
// path under a temp workspace, and the M3/M4 sandbox policy helper.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife/autoactivate"
)

func TestParseActivateFlagEmpty(t *testing.T) {
	if got := parseActivateFlag(""); got != nil {
		t.Errorf("empty flag should be nil, got %v", got)
	}
}

func TestParseActivateFlagTrimAndSplit(t *testing.T) {
	got := parseActivateFlag(" terse , skill-x ,,ultra ")
	want := []string{"terse", "skill-x", "ultra"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNewChatActivatorLoadsTOML(t *testing.T) {
	ws := t.TempDir()
	toml := `[rule]
name = "terse-mode"
body = "be terse"
trigger = "/compact"
[default]
auto_on = true
`
	if err := os.MkdirAll(filepath.Join(ws, ".sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".sin-code", "autoactivate.toml"), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newChatActivator(ws, &chatOptions{})
	if c.Def.AutoOn == false {
		t.Errorf("AutoOn should have loaded from TOML")
	}
	if got := c.Defaults["terse-mode"]; got.Body != "be terse" || got.Trigger != "/compact" {
		t.Errorf("terse rule not loaded: %+v", got)
	}
}

func TestNewChatActivatorMissingTOML(t *testing.T) {
	ws := t.TempDir()
	c := newChatActivator(ws, &chatOptions{})
	if c.Def.AutoOn || len(c.Defaults) != 0 || len(c.Rules) != 0 {
		t.Errorf("missing TOML must produce zero state, got %+v", c)
	}
}

func TestNewChatActivatorFromCLIFlag(t *testing.T) {
	ws := t.TempDir()
	c := newChatActivator(ws, &chatOptions{activate: "a,b,c"})
	if len(c.Rules) != 3 || c.Rules[0] != "a" || c.Rules[2] != "c" {
		t.Errorf("CLI rules not parsed: %v", c.Rules)
	}
}

func TestChatActivatorEndToEndDispatch(t *testing.T) {
	ws := t.TempDir()
	toml := `[rule]
name = "terse-mode"
body = "be terse"
[default]
auto_on = true
`
	if err := os.MkdirAll(filepath.Join(ws, ".sin-code"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".sin-code", "autoactivate.toml"), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newChatActivator(ws, &chatOptions{})

	reg := hooklife.NewRegistry()
	autoOn := c.Def.AutoOn || len(c.Rules) > 0 || len(c.Defaults) > 0
	reg.Register(autoactivate.SessionStartHook{
		Act: c.Act, Defaults: c.Defaults, AutoOn: autoOn,
	})
	reg.Register(autoactivate.UserPromptHook{Act: c.Act})
	r := hooklife.NewRunner(reg).WithTimeout(2 * time.Second)

	sid := "test-session"
	ctx := context.Background()

	// SessionStart.
	sd := r.Dispatch(ctx, hooklife.Event{
		Phase: hooklife.SessionStart,
		Meta:  map[string]string{"session_id": sid},
	})
	if !strings.Contains(sd.Message, "terse-mode") {
		t.Errorf("SessionStart decision should include terse-mode, got %q", sd.Message)
	}

	// UserPrompt: per-turn anchor with the active rules.
	pd := r.Dispatch(ctx, hooklife.Event{
		Phase: hooklife.UserPrompt,
		Meta:  map[string]string{"session_id": sid, "prompt": "/compact please"},
	})
	if !strings.Contains(pd.Message, "terse-mode") {
		t.Errorf("UserPrompt decision should re-emit terse-mode, got %q", pd.Message)
	}

	// CLIRules application.
	for _, name := range c.Rules {
		c.Act.Activate(sid, autoactivate.Rule{Name: name})
	}
	st, ok := c.Act.Snapshot(sid)
	if !ok {
		t.Fatal("snapshot missing")
	}
	if len(st.ActiveRules.Names()) == 0 {
		t.Errorf("active rules should be present after Activate")
	}

	// Privacy: EndSession clears state.
	c.Act.EndSession(sid)
	if _, ok := c.Act.Snapshot(sid); ok {
		t.Errorf("EndSession should drop the state")
	}
}

func TestChatActivatorWithNoTrigger(t *testing.T) {
	ws := t.TempDir()
	c := newChatActivator(ws, &chatOptions{noTrigger: true})

	reg := hooklife.NewRegistry()
	reg.Register(autoactivate.SessionStartHook{Act: c.Act, Defaults: c.Defaults, AutoOn: true})
	reg.Register(autoactivate.UserPromptHook{Act: c.Act})
	r := hooklife.NewRunner(reg).WithTimeout(2 * time.Second)

	sid := "nt-session"
	ctx := context.Background()
	r.Dispatch(ctx, hooklife.Event{
		Phase: hooklife.SessionStart,
		Meta:  map[string]string{"session_id": sid, "no_trigger": "true"},
	})
	pd := r.Dispatch(ctx, hooklife.Event{
		Phase: hooklife.UserPrompt,
		Meta:  map[string]string{"session_id": sid, "prompt": "/compact"},
	})
	// With no defaults, the body is empty — but importantly no panic.
	if pd.Verdict != hooklife.Allow {
		t.Errorf("no_trigger override: expected Allow, got %v", pd.Verdict)
	}
	st, _ := c.Act.Snapshot(sid)
	if !st.NoTrigger {
		t.Errorf("no_trigger meta should propagate to session state")
	}
}

func TestBoolStr(t *testing.T) {
	if boolStr(true) != "true" || boolStr(false) != "false" {
		t.Errorf("boolStr wrong")
	}
}

// snapshotSandboxConfig copies the package-level sandboxConfig so the
// test can restore it without racing other tests. Returns the captured
// value so the test can also assert against it directly.
func snapshotSandboxConfig() (orig struct {
	enabled   bool
	workspace string
}) {
	return sandboxConfig
}

func restoreSandboxConfig(snap struct {
	enabled   bool
	workspace string
}) {
	sandboxConfig = snap
}

// swapChatStderr redirects the package-level chatStderr writer for
// the duration of a test so the policy helper's Warns / announcements
// can be asserted. Returns the captured buffer + a cleanup func.
func swapChatStderr(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	orig := chatStderr
	chatStderr = &buf
	return &buf, func() { chatStderr = orig }
}

// TestChat_HeadlessDefaultsSandboxOn (issue #420): when `sin-code chat`
// takes the headless path (-p or --json), the sandbox must default
// to ON with the workspace rooted at $PWD. The helper is the source
// of truth; this test pins the M3/M4 headless mandate so a future
// refactor cannot silently downgrade it.
func TestChat_HeadlessDefaultsSandboxOn(t *testing.T) {
	snap := snapshotSandboxConfig()
	defer restoreSandboxConfig(snap)
	sandboxConfig = struct {
		enabled   bool
		workspace string
	}{false, ""}

	stderrBuf, restoreStderr := swapChatStderr(t)
	defer restoreStderr()

	ws := "/tmp/issue-420-headless"
	opts := &chatOptions{prompt: "summarise this repo"} // -p implies headless
	applyChatSandboxPolicy(opts, true, ws)

	if !sandboxConfig.enabled {
		t.Errorf("headless chat (-p) must default sandbox=enabled; got enabled=false")
	}
	if sandboxConfig.workspace != ws {
		t.Errorf("sandbox workspace = %q, want %q", sandboxConfig.workspace, ws)
	}
	out := stderrBuf.String()
	if !strings.Contains(out, "sandbox enabled") {
		t.Errorf("headless mode must announce sandbox=enabled to stderr; got %q", out)
	}
	if !strings.Contains(out, "issue #420") {
		t.Errorf("headless announcement should cite issue #420 for traceability; got %q", out)
	}
	if strings.Contains(out, "WARN") {
		t.Errorf("default headless path must not emit a WARN; got %q", out)
	}
}

// TestChat_NoSandboxFlag_DisablesSandbox (issue #420): the
// --no-sandbox escape hatch must override the headless default and
// emit a WARN to stderr so the relaxed posture is visible in CI logs.
// Without the WARN, an operator who flipped the flag reflexively
// (e.g. copy-pasting a debug command) would lose all visibility.
func TestChat_NoSandboxFlag_DisablesSandbox(t *testing.T) {
	snap := snapshotSandboxConfig()
	defer restoreSandboxConfig(snap)
	sandboxConfig = struct {
		enabled   bool
		workspace string
	}{false, ""}

	stderrBuf, restoreStderr := swapChatStderr(t)
	defer restoreStderr()

	ws := "/tmp/issue-420-nosandbox"
	opts := &chatOptions{
		prompt:    "echo hello", // headless
		noSandbox: true,
	}
	applyChatSandboxPolicy(opts, true, ws)

	if sandboxConfig.enabled {
		t.Errorf("--no-sandbox must force sandbox=enabled-false; got enabled=true")
	}
	if sandboxConfig.workspace != ws {
		t.Errorf("sandbox workspace = %q, want %q", sandboxConfig.workspace, ws)
	}
	out := stderrBuf.String()
	if !strings.Contains(out, "WARN") || !strings.Contains(out, "--no-sandbox") {
		t.Errorf("--no-sandbox must emit a WARN citing the flag name; got %q", out)
	}
	if !strings.Contains(out, "issue #420") {
		t.Errorf("WARN should cite issue #420 for traceability; got %q", out)
	}
	if strings.Contains(out, "sandbox enabled") {
		t.Errorf("disabled path must NOT announce 'sandbox enabled'; got %q", out)
	}
}

// TestChat_NoSandboxNonHeadless_SilentOverride: when the operator
// passes --no-sandbox in the REPL, the WARN still fires (so the
// relaxed posture is visible) but the headless announcement is
// suppressed (we are not in headless mode).
func TestChat_NoSandboxNonHeadless_SilentOverride(t *testing.T) {
	snap := snapshotSandboxConfig()
	defer restoreSandboxConfig(snap)
	sandboxConfig = struct {
		enabled   bool
		workspace string
	}{false, ""}

	stderrBuf, restoreStderr := swapChatStderr(t)
	defer restoreStderr()

	ws := "/tmp/issue-420-repl"
	opts := &chatOptions{noSandbox: true} // REPL, no -p, no --json
	applyChatSandboxPolicy(opts, false, ws)

	if sandboxConfig.enabled {
		t.Errorf("--no-sandbox must disable sandbox outside headless mode too")
	}
	out := stderrBuf.String()
	if !strings.Contains(out, "WARN") {
		t.Errorf("--no-sandbox must WARN even in REPL mode; got %q", out)
	}
	if strings.Contains(out, "headless mode — sandbox enabled") {
		t.Errorf("REPL mode must not emit the headless announcement; got %q", out)
	}
}

// TestChat_ExplicitSandboxNoneHeadless_NoWarn: --sandbox none is a
// legacy escape hatch (already documented). It must disable the
// sandbox WITHOUT emitting the --no-sandbox WARN, because the
// operator used the explicit backend vocabulary.
func TestChat_ExplicitSandboxNoneHeadless_NoWarn(t *testing.T) {
	snap := snapshotSandboxConfig()
	defer restoreSandboxConfig(snap)
	sandboxConfig = struct {
		enabled   bool
		workspace string
	}{false, ""}

	stderrBuf, restoreStderr := swapChatStderr(t)
	defer restoreStderr()

	ws := "/tmp/issue-420-none"
	opts := &chatOptions{
		prompt:  "echo legacy",
		sandbox: "none",
	}
	applyChatSandboxPolicy(opts, true, ws)

	if sandboxConfig.enabled {
		t.Errorf("--sandbox none must disable sandbox")
	}
	out := stderrBuf.String()
	if strings.Contains(out, "WARN: --no-sandbox") {
		t.Errorf("--sandbox none must NOT emit the --no-sandbox WARN; got %q", out)
	}
}

// TestSandboxBackendDisplay pins the helper that maps ""/various
// backends to a stable stderr string. The four-arm matrix keeps
// downstream consumers (audit, OCR of CI logs) deterministic.
func TestSandboxBackendDisplay(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "platform-default"},
		{"none", "none"},
		{"landlock", "landlock"},
		{"seatbelt", "seatbelt"},
		{"bubblewrap", "bubblewrap"},
	}
	for _, c := range cases {
		if got := sandboxBackendDisplay(c.in); got != c.want {
			t.Errorf("sandboxBackendDisplay(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
