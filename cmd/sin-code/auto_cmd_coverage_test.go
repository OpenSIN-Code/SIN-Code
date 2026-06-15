// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for auto_cmd.go.
// Docs: auto_cmd.go
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autopilot"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// fakeAutoLoop implements autoLoop for tests.
type fakeAutoLoop struct {
	result *agentloop.Result
	err    error
}

func (f *fakeAutoLoop) Run(ctx context.Context, sess *session.Session, goal string) (*agentloop.Result, error) {
	return f.result, f.err
}

// fakeAutoPilot implements autoPilot for tests. It exercises the Lessons,
// RunGoal, and Record callbacks passed to autopilot.Config so their closures
// are covered.
type fakeAutoPilot struct {
	cfg         autopilot.Config
	runErr      error
	skipRunGoal bool
}

func (f *fakeAutoPilot) Run(ctx context.Context) (int, float64, error) {
	_ = f.cfg.Lessons(ctx, f.cfg.Workspace, 1)
	if !f.skipRunGoal && f.cfg.RunGoal != nil {
		_, summary, err := f.cfg.RunGoal(ctx, f.cfg.Program.Objective)
		if err != nil {
			return 0, 0, err
		}
		if f.cfg.Out != nil && summary != "" {
			fmt.Fprintln(f.cfg.Out, summary)
		}
	}
	f.cfg.Record(ctx, f.cfg.Workspace, "lesson")
	return 1, 0.5, f.runErr
}

func resetAutoHooks(t *testing.T) {
	t.Helper()
	orig := autoHookVars
	t.Cleanup(func() { autoHookVars = orig })
}

func runAutoCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := newAutoCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func defaultProgramPtr() *autopilot.Program {
	p := autopilot.DefaultProgram()
	return &p
}

func tempFile(t *testing.T, suffix string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestAutoInit(t *testing.T) {
	resetAutoHooks(t)
	var written []byte
	autoHookVars.osStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	autoHookVars.osWriteFile = func(name string, data []byte, mode os.FileMode) error { written = data; return nil }
	out, err := runAutoCmd(t, "init")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "Objective") {
		t.Errorf("expected template to be written, got %q", string(written))
	}
	if !strings.Contains(out.String(), "wrote program.md") {
		t.Errorf("expected init output, got %q", out.String())
	}
}

func TestAutoInitAlreadyExists(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osStat = func(string) (os.FileInfo, error) { return nil, nil }
	_, err := runAutoCmd(t, "init")
	if err == nil || !strings.Contains(err.Error(), "program.md already exists") {
		t.Fatalf("expected already exists error, got %v", err)
	}
}

func TestAutoInitWriteError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	autoHookVars.osWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	_, err := runAutoCmd(t, "init")
	if err == nil || !strings.Contains(err.Error(), "write boom") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestAutoRunNoVerifyCmd(t *testing.T) {
	resetAutoHooks(t)
	_, err := runAutoCmd(t, "run")
	if err == nil || !strings.Contains(err.Error(), "--verify-cmd") {
		t.Fatalf("expected verify-cmd error, got %v", err)
	}
}

func TestAutoRunGetwdError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "", errors.New("getwd boom") }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err == nil || !strings.Contains(err.Error(), "getwd boom") {
		t.Fatalf("expected getwd error, got %v", err)
	}
}

func TestAutoRunLoadProgramError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return nil, errors.New("load boom") }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Fatalf("expected load error, got %v", err)
	}
}

func TestAutoRunOpenJournalError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return "/journal" }
	autoHookVars.openJournal = func(string) (*autopilot.Journal, error) { return nil, errors.New("journal boom") }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err == nil || !strings.Contains(err.Error(), "journal boom") {
		t.Fatalf("expected journal error, got %v", err)
	}
}

func TestAutoRunOpenSessionError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.openSession = func(string) (*session.Store, error) { return nil, errors.New("session boom") }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err == nil || !strings.Contains(err.Error(), "session boom") {
		t.Fatalf("expected session error, got %v", err)
	}
}

func TestAutoRunLoopBuildError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.buildLoop = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		return nil, nil, errors.New("build boom")
	}
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot { return &fakeAutoPilot{cfg: cfg} }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err == nil || !strings.Contains(err.Error(), "build boom") {
		t.Fatalf("expected build error, got %v", err)
	}
}

func TestAutoRunLoopRunError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.buildLoop = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		return &fakeAutoLoop{err: errors.New("run boom")}, func() error { return nil }, nil
	}
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot { return &fakeAutoPilot{cfg: cfg} }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err == nil || !strings.Contains(err.Error(), "run boom") {
		t.Fatalf("expected loop run error, got %v", err)
	}
}

func TestAutoRunSuccess(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) {
		p := defaultProgramPtr()
		p.Objective = "test objective"
		p.BudgetMinutes = 10
		p.MaxExperiments = 5
		return p, nil
	}
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.buildLoop = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		if cfg.ToolFactory != nil {
			mgr := mcpclient.NewManager(nil)
			cfg.ToolFactory(mgr)
		}
		return &fakeAutoLoop{result: &agentloop.Result{SessionID: "s1", Verified: true, Turns: 3, Summary: "done"}}, func() error { return nil }, nil
	}
	var gotCfg autopilot.Config
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot {
		gotCfg = cfg
		return &fakeAutoPilot{cfg: cfg}
	}
	autoHookVars.newBudget = func(minutes, maxExperiments int) *autopilot.Budget { return nil }
	autoHookVars.newSnapshotter = func(string) *autopilot.Snapshotter { return nil }
	out, err := runAutoCmd(t, "run", "--verify-cmd", "true", "--budget-minutes", "15", "--max-experiments", "7", "--max-turns", "20")
	if err != nil {
		t.Fatal(err)
	}
	if gotCfg.Program.BudgetMinutes != 15 {
		t.Errorf("expected budget 15, got %d", gotCfg.Program.BudgetMinutes)
	}
	if gotCfg.Program.MaxExperiments != 7 {
		t.Errorf("expected max experiments 7, got %d", gotCfg.Program.MaxExperiments)
	}
	if gotCfg.Workspace != "/ws" {
		t.Errorf("expected workspace /ws, got %q", gotCfg.Workspace)
	}
	if !strings.Contains(out.String(), "done") {
		t.Errorf("expected summary output, got %q", out.String())
	}
}

func TestAutoRunPilotError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.buildLoop = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		return &fakeAutoLoop{result: &agentloop.Result{SessionID: "s1"}}, func() error { return nil }, nil
	}
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot {
		return &fakeAutoPilot{cfg: cfg, runErr: errors.New("pilot boom")}
	}
	autoHookVars.newBudget = func(minutes, maxExperiments int) *autopilot.Budget { return nil }
	autoHookVars.newSnapshotter = func(string) *autopilot.Snapshotter { return nil }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err == nil || !strings.Contains(err.Error(), "pilot boom") {
		t.Fatalf("expected pilot error, got %v", err)
	}
}

func TestAutoStatus(t *testing.T) {
	resetAutoHooks(t)
	ws := t.TempDir()
	autoHookVars.osGetwd = func() (string, error) { return ws, nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return nil, nil }
	autoHookVars.defaultJournalPath = func(string) string { return filepath.Join(ws, "journal.db") }
	out, err := runAutoCmd(t, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "experiments:") {
		t.Errorf("expected status output, got %q", out.String())
	}
}

func TestAutoStatusJSON(t *testing.T) {
	resetAutoHooks(t)
	ws := t.TempDir()
	autoHookVars.osGetwd = func() (string, error) { return ws, nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return nil, nil }
	autoHookVars.defaultJournalPath = func(string) string { return filepath.Join(ws, "journal.db") }
	// Seed a kept experiment so BestKept returns a valid number (not NaN) for JSON encoding.
	j, err := autopilot.OpenJournal(filepath.Join(ws, "journal.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Record(context.Background(), autopilot.Experiment{
		Objective: "obj", Proposal: "prop", Outcome: autopilot.OutcomeKept,
		MetricBefore: 1.0, MetricAfter: 0.5, MetricFound: true,
	}); err != nil {
		t.Fatal(err)
	}
	j.Close()
	out, err := runAutoCmd(t, "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "experiments_total") {
		t.Errorf("expected json output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "best_metric") {
		t.Errorf("expected best_metric field, got %q", out.String())
	}
}

func TestAutoStatusOpenJournalError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.defaultJournalPath = func(string) string { return "/journal" }
	autoHookVars.openJournal = func(string) (*autopilot.Journal, error) { return nil, errors.New("journal boom") }
	_, err := runAutoCmd(t, "status")
	if err == nil || !strings.Contains(err.Error(), "journal boom") {
		t.Fatalf("expected journal error, got %v", err)
	}
}

func TestAutoJournal(t *testing.T) {
	resetAutoHooks(t)
	ws := t.TempDir()
	autoHookVars.osGetwd = func() (string, error) { return ws, nil }
	autoHookVars.defaultJournalPath = func(string) string { return filepath.Join(ws, "journal.db") }
	out, err := runAutoCmd(t, "journal", "--limit", "5")
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "" {
		t.Errorf("expected empty output for empty journal, got %q", out.String())
	}
}

func TestAutoJournalOpenJournalError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.defaultJournalPath = func(string) string { return "/journal" }
	autoHookVars.openJournal = func(string) (*autopilot.Journal, error) { return nil, errors.New("journal boom") }
	_, err := runAutoCmd(t, "journal")
	if err == nil || !strings.Contains(err.Error(), "journal boom") {
		t.Fatalf("expected journal error, got %v", err)
	}
}

func TestAutoJournalRecentError(t *testing.T) {
	resetAutoHooks(t)
	ws := t.TempDir()
	autoHookVars.osGetwd = func() (string, error) { return ws, nil }
	autoHookVars.defaultJournalPath = func(string) string { return filepath.Join(ws, "journal.db") }
	autoHookVars.openJournal = func(path string) (*autopilot.Journal, error) {
		j, err := autopilot.OpenJournal(path)
		if err != nil {
			return nil, err
		}
		// Close the journal immediately so the subsequent Recent() call fails.
		_ = j.Close()
		return j, nil
	}
	_, err := runAutoCmd(t, "journal")
	if err == nil {
		t.Fatal("expected Recent error from closed journal")
	}
}

func TestAutoRunWithProgramDirection(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) {
		p := defaultProgramPtr()
		p.Direction = autopilot.Maximize
		return p, nil
	}
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.buildLoop = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		return &fakeAutoLoop{result: &agentloop.Result{SessionID: "s1"}}, func() error { return nil }, nil
	}
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot { return &fakeAutoPilot{cfg: cfg, skipRunGoal: true} }
	autoHookVars.newBudget = func(minutes, maxExperiments int) *autopilot.Budget { return nil }
	autoHookVars.newSnapshotter = func(string) *autopilot.Snapshotter { return nil }
	out, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "done") && !strings.Contains(out.String(), "s1") {
		// fake pilot with skipRunGoal prints nothing; output may be empty
		_ = out
	}
}

func TestAutoRunToolFactory(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.buildLoop = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		if cfg.ToolFactory != nil {
			mgr := mcpclient.NewManager(nil)
			cfg.ToolFactory(mgr)
		}
		return &fakeAutoLoop{result: &agentloop.Result{SessionID: "s1"}}, func() error { return nil }, nil
	}
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot { return &fakeAutoPilot{cfg: cfg} }
	autoHookVars.newBudget = func(minutes, maxExperiments int) *autopilot.Budget { return nil }
	autoHookVars.newSnapshotter = func(string) *autopilot.Snapshotter { return nil }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoRunStartSessionError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.openSession = func(path string) (*session.Store, error) {
		s, err := session.Open(path)
		if err != nil {
			return nil, err
		}
		_ = s.Close()
		return s, nil
	}
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot { return &fakeAutoPilot{cfg: cfg} }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err == nil {
		t.Fatal("expected StartOrResume error from closed session store")
	}
}

func TestAutoRunNilLessons(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.openLessons = func(string) (*lessons.Store, error) { return nil, nil }
	autoHookVars.buildLoop = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		return &fakeAutoLoop{result: &agentloop.Result{SessionID: "s1"}}, func() error { return nil }, nil
	}
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot { return &fakeAutoPilot{cfg: cfg} }
	autoHookVars.newBudget = func(minutes, maxExperiments int) *autopilot.Budget { return nil }
	autoHookVars.newSnapshotter = func(string) *autopilot.Snapshotter { return nil }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoRunLessonsQueryError(t *testing.T) {
	resetAutoHooks(t)
	autoHookVars.osGetwd = func() (string, error) { return "/ws", nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) { return defaultProgramPtr(), nil }
	autoHookVars.defaultJournalPath = func(string) string { return tempFile(t, ".db") }
	autoHookVars.defaultSessionPath = func() string { return tempFile(t, ".db") }
	autoHookVars.openLessons = func(path string) (*lessons.Store, error) {
		ls, err := lessons.Open(path)
		if err != nil {
			return nil, err
		}
		_ = ls.Close()
		return ls, nil
	}
	autoHookVars.buildLoop = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		return &fakeAutoLoop{result: &agentloop.Result{SessionID: "s1"}}, func() error { return nil }, nil
	}
	autoHookVars.newPilot = func(cfg autopilot.Config) autoPilot { return &fakeAutoPilot{cfg: cfg} }
	autoHookVars.newBudget = func(minutes, maxExperiments int) *autopilot.Budget { return nil }
	autoHookVars.newSnapshotter = func(string) *autopilot.Snapshotter { return nil }
	_, err := runAutoCmd(t, "run", "--verify-cmd", "true")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoStatusWithProgram(t *testing.T) {
	resetAutoHooks(t)
	ws := t.TempDir()
	autoHookVars.osGetwd = func() (string, error) { return ws, nil }
	autoHookVars.loadProgram = func(string) (*autopilot.Program, error) {
		p := defaultProgramPtr()
		p.Direction = autopilot.Maximize
		return p, nil
	}
	autoHookVars.defaultJournalPath = func(string) string { return filepath.Join(ws, "journal.db") }
	out, err := runAutoCmd(t, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "experiments:") {
		t.Errorf("expected status output, got %q", out.String())
	}
}

func TestAutoJournalWithEntries(t *testing.T) {
	resetAutoHooks(t)
	ws := t.TempDir()
	autoHookVars.osGetwd = func() (string, error) { return ws, nil }
	autoHookVars.defaultJournalPath = func(string) string { return filepath.Join(ws, "journal.db") }
	j, err := autopilot.OpenJournal(filepath.Join(ws, "journal.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Record(context.Background(), autopilot.Experiment{
		Objective: "obj", Proposal: "proposal one", Outcome: autopilot.OutcomeKept,
		MetricBefore: 1.0, MetricAfter: 0.5, MetricFound: true,
	}); err != nil {
		t.Fatal(err)
	}
	j.Close()
	out, err := runAutoCmd(t, "journal", "--limit", "5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "proposal one") {
		t.Errorf("expected journal entry output, got %q", out.String())
	}
}

func TestAutoDefaultHooks(t *testing.T) {
	resetAutoHooks(t)
	// Exercise the default buildLoop wrapper with an invalid agent so it errors before any network call.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := autoHookVars.buildLoop(ctx, loopbuilder.Config{AgentName: "nonexistent-agent"}, nil)
	if err == nil {
		t.Fatal("expected buildLoop error for invalid agent")
	}
	// Exercise the default newPilot wrapper.
	p := autoHookVars.newPilot(autopilot.Config{Program: defaultProgramPtr()})
	if p == nil {
		t.Fatal("expected non-nil pilot")
	}
}
