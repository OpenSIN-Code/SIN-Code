// SPDX-License-Identifier: MIT
// Purpose: coverage tests for tui.go — exercise getSubcommand, runNewTUI,
// and the tuiCmd RunE path without starting a real bubbletea program.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui"
)

type fakeTeaProgram struct {
	runErr error
}

func (f *fakeTeaProgram) Run() (tea.Model, error) { return nil, f.runErr }
func (f *fakeTeaProgram) Send(any)                {}

func setTUIHook[T any](t *testing.T, ptr *T, val T) {
	t.Helper()
	orig := *ptr
	*ptr = val
	t.Cleanup(func() { *ptr = orig })
}

func TestGetSubcommand_Found(t *testing.T) {
	if c := getSubcommand("discover"); c == nil {
		t.Error("expected discover subcommand")
	}
}

func TestGetSubcommand_NotFound(t *testing.T) {
	if c := getSubcommand("not-a-real-cmd"); c != nil {
		t.Errorf("expected nil, got %v", c)
	}
}

func TestModelOnRun_Unknown(t *testing.T) {
	pm := tui.NewModel()
	pm.OnRun = func(name string, args []string) error {
		c := getSubcommand(name)
		if c == nil {
			return fmt.Errorf("unknown subcommand: %s", name)
		}
		c.SetArgs(args)
		c.SetOut(io.Discard)
		c.SetErr(io.Discard)
		return c.Execute()
	}

	err := pm.OnRun("missing", []string{})
	if err == nil || !strings.Contains(err.Error(), "unknown subcommand: missing") {
		t.Errorf("expected unknown error, got %v", err)
	}
}

func TestModelOnRun_Known(t *testing.T) {
	pm := tui.NewModel()
	pm.OnRun = func(name string, args []string) error {
		c := getSubcommand(name)
		if c == nil {
			return fmt.Errorf("unknown subcommand: %s", name)
		}
		c.SetArgs(args)
		return c.Execute()
	}

	outFn := captureStdout(t)
	err := pm.OnRun("discover", []string{"--version"})
	out := outFn()
	if err != nil {
		t.Fatalf("OnRun discover: %v", err)
	}
	if !strings.Contains(out, "discover") {
		t.Errorf("output missing discover help: %q", out)
	}
}

func TestRunNewTUI_ErrorBranch(t *testing.T) {
	setTUIHook(t, &teaNewProgramFn, func(tea.Model, ...tea.ProgramOption) teaProgramRunner {
		return &fakeTeaProgram{runErr: errors.New("no tty available")}
	})

	var out bytes.Buffer
	runNewTUI(&out)

	str := out.String()
	if !strings.Contains(str, "sin-code subcommands (TUI not available") {
		t.Errorf("missing TUI unavailable header: %q", str)
	}
	if !strings.Contains(str, "discover") {
		t.Errorf("missing discover in catalog: %q", str)
	}
	if strings.Contains(str, "\n  tui ") || strings.Contains(str, "\n  help ") {
		t.Errorf("catalog should skip tui/help command lines: %q", str)
	}
}

func TestTuiCmd_RunE(t *testing.T) {
	setTUIHook(t, &teaNewProgramFn, func(tea.Model, ...tea.ProgramOption) teaProgramRunner {
		return &fakeTeaProgram{runErr: errors.New("no tty available")}
	})

	outFn := captureStdout(t)
	err := tuiCmd.RunE(tuiCmd, nil)
	out := outFn()
	if err != nil {
		t.Fatalf("RunE tui: %v", err)
	}
	if !strings.Contains(out, "sin-code subcommands (TUI not available") {
		t.Errorf("expected plain text catalog, got %q", out)
	}
}

func TestTuiHooks(t *testing.T) {
	called := map[string]bool{}
	setTUIHook(t, &tuiNewModelFn, func() *tui.Model {
		called["newModel"] = true
		return tui.NewModel()
	})
	setTUIHook(t, &teaNewProgramFn, func(tea.Model, ...tea.ProgramOption) teaProgramRunner {
		called["newProgram"] = true
		return &fakeTeaProgram{runErr: errors.New("boom")}
	})
	setTUIHook(t, &tuiProgramFromTeaProgramFn, func(p any) interface{ Send(any) } {
		called["programFromTeaProgram"] = true
		return p.(interface{ Send(any) })
	})

	var out bytes.Buffer
	runNewTUI(&out)

	if !called["newModel"] {
		t.Error("tuiNewModelFn not called")
	}
	if !called["newProgram"] {
		t.Error("teaNewProgramFn not called")
	}
	if !called["programFromTeaProgram"] {
		t.Error("tuiProgramFromTeaProgramFn not called")
	}
}

// quitModel exits a bubbletea program immediately so tests can exercise the
// real Program/adapter paths without needing an interactive terminal.
type quitModel struct{}

func (quitModel) Init() tea.Cmd                         { return tea.Quit }
func (m quitModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (quitModel) View() tea.View                        { return tea.NewView("") }

// fakeTeaProgramOnRun calls the model's OnRun callback during Run so the
// runNewTUI closure that wires subcommand execution is executed in tests.
type fakeTeaProgramOnRun struct {
	pm *tui.Model
}

func (f *fakeTeaProgramOnRun) Run() (tea.Model, error) {
	if f.pm.OnRun != nil {
		if err := f.pm.OnRun("discover", []string{}); err != nil {
			return nil, err
		}
	}
	return f.pm, nil
}
func (f *fakeTeaProgramOnRun) Send(any) {}

func TestTeaProgramAdapter_CallsRealProgram(t *testing.T) {
	p := tea.NewProgram(quitModel{}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	a := &teaProgramAdapter{p: p}
	if _, err := a.Run(); err != nil {
		t.Logf("Run returned (expected without TTY): %v", err)
	}
	a.Send(tea.Quit())
}

func TestTeaNewProgramFn_Default(t *testing.T) {
	orig := teaNewProgramFn
	defer func() { teaNewProgramFn = orig }()

	r := orig(quitModel{}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	if r == nil {
		t.Fatal("expected runner")
	}
	if _, err := r.Run(); err != nil {
		t.Logf("Run returned (expected without TTY): %v", err)
	}
}

func TestTuiProgramFromTeaProgramFn_Branches(t *testing.T) {
	if got := tuiProgramFromTeaProgramFn(nil); got != nil {
		t.Errorf("nil input: expected nil, got %v", got)
	}

	p := tea.NewProgram(quitModel{}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	a := &teaProgramAdapter{p: p}
	if got := tuiProgramFromTeaProgramFn(a); got == nil {
		t.Error("adapter input: expected non-nil wrapper")
	}

	fake := &fakeTeaProgram{}
	if got := tuiProgramFromTeaProgramFn(fake); got != fake {
		t.Errorf("Send-interface input: expected %v, got %v", fake, got)
	}
}

func TestRunNewTUI_OnRunInvoked(t *testing.T) {
	setTUIHook(t, &teaNewProgramFn, func(m tea.Model, opts ...tea.ProgramOption) teaProgramRunner {
		pm, ok := m.(*tui.Model)
		if !ok {
			t.Fatalf("model is not *tui.Model")
		}
		return &fakeTeaProgramOnRun{pm: pm}
	})

	outFn := captureStdout(t)
	var out bytes.Buffer
	runNewTUI(&out)
	stdout := outFn()
	if !strings.Contains(stdout, "discover") && !strings.Contains(out.String(), "discover") {
		t.Errorf("expected OnRun to execute discover command, got stdout=%q out=%q", stdout, out.String())
	}
}

// fakeRunnerNoSend implements teaProgramRunner but not the Send(any) interface
// shape used by the fallback branch of tuiProgramFromTeaProgramFn.
type fakeRunnerNoSend struct{}

func (fakeRunnerNoSend) Run() (tea.Model, error) { return nil, nil }

func TestTuiProgramFromTeaProgramFn_FallbackNil(t *testing.T) {
	if got := tuiProgramFromTeaProgramFn(fakeRunnerNoSend{}); got != nil {
		t.Errorf("expected nil for runner without Send, got %v", got)
	}
}

// fakeTeaProgramOnRunUnknown invokes the model OnRun with an unknown command
// so the `if c == nil` error branch inside runNewTUI is exercised.
type fakeTeaProgramOnRunUnknown struct {
	pm *tui.Model
}

func (f *fakeTeaProgramOnRunUnknown) Run() (tea.Model, error) {
	if f.pm.OnRun != nil {
		if err := f.pm.OnRun("unknown-cmd", []string{}); err != nil {
			return nil, err
		}
	}
	return f.pm, nil
}
func (f *fakeTeaProgramOnRunUnknown) Send(any) {}

func TestRunNewTUI_OnRunUnknownSubcommand(t *testing.T) {
	setTUIHook(t, &teaNewProgramFn, func(m tea.Model, opts ...tea.ProgramOption) teaProgramRunner {
		pm, ok := m.(*tui.Model)
		if !ok {
			t.Fatalf("model is not *tui.Model")
		}
		return &fakeTeaProgramOnRunUnknown{pm: pm}
	})

	var out bytes.Buffer
	runNewTUI(&out)
	if !strings.Contains(out.String(), "sin-code subcommands (TUI not available") {
		t.Errorf("expected plain text catalog after unknown subcommand error, got %q", out.String())
	}
}

// keep fmt import valid across edits.
var _ = fmt.Sprintf
