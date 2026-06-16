// SPDX-License-Identifier: MIT
// Purpose: race-safe tests for the chat command's autoactivate
// wiring (issue #176). Covers the helpers, the session-start
// invocation through the hooklife runner, and the
// `.sin-code/autoactivate.toml` path under a temp workspace.
package main

import (
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
