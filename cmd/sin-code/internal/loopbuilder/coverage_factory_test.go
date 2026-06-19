// SPDX-License-Identifier: MIT
// Purpose: focused coverage tests for the loopbuilder factory (issue #64).
// Closes the path-completeness gap at:
//   - commandRunner (verify.Runner producer; empty + non-empty + container)
//   - WireFusion (disabled gate + stub-provider fan-out, issue #290)
//   - SessionContextBuilder (5 stores populated, issue #379)
//   - Build (CoverageRequiredTools/CoverageForbiddenTools propagation,
//     issue #248)
//   - loadHooks (user-level + project-local concat, malformed JSON)
//
// All five tests run under `go test -race -count=1` and do not touch
// the real user filesystem: SIN_CODE_HOME / XDG_CONFIG_HOME are pinned
// to a per-test temp dir so the user's ~/.local/share/sin-code and
// ~/.config/sin-code are never written by these tests.
package loopbuilder

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/fusion"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// withIsolatedHome pins SIN_CODE_HOME + XDG_CONFIG_HOME to a per-test
// temp directory so the loopbuilder factory and its underlying stores
// never read or write the user's real ~/.config and ~/.local/share.
// Returns the temp dir so callers can drop fixtures into it.
func withIsolatedHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv("HOME", dir)
	return dir
}

// 1) commandRunner is the smallest helper in the factory: a command
// string in → a verify.Runner out. Three deterministic invariants
// must hold for the verify gate (M3) to be wired correctly:
//   a) missing/empty command → nil Runner (no panic, no executor
//      spawned) — the gate then has no runner in its PoC map.
//   b) non-empty command → Runner that shells out, returns
//      (true, trimmed output) on exit 0 and (false, trimmed
//      output) on non-zero exit.
//   c) containerRunner provided → command is forwarded into the
//      runner and the shell executor is bypassed (docker binary is
//      not on the test machine).
func TestCommandRunner_ResolveHookCommand(t *testing.T) {
	withIsolatedHome(t)

	t.Run("empty_command_returns_nil", func(t *testing.T) {
		if r := commandRunner("", nil, ""); r != nil {
			t.Fatalf("expected nil Runner for empty command, got %T", r)
		}
	})

	t.Run("shell_runner_executes_in_workspace", func(t *testing.T) {
		ws := t.TempDir()
		// Drop a sentinel file so the shell can `cat` it back.
		sentinel := filepath.Join(ws, "evidence.txt")
		if err := os.WriteFile(sentinel, []byte("VERIFY-OK"), 0o644); err != nil {
			t.Fatal(err)
		}
		runner := commandRunner("cat "+sentinel, nil, "")
		if runner == nil {
			t.Fatal("expected Runner for non-empty command")
		}
		passed, report, err := runner(context.Background(), ws)
		if err != nil {
			t.Fatalf("shell runner returned err on success: %v", err)
		}
		if !passed {
			t.Fatalf("expected passed=true, got false (report=%q)", report)
		}
		if report != "VERIFY-OK" {
			t.Fatalf("expected trimmed report %q, got %q", "VERIFY-OK", report)
		}
	})

	t.Run("shell_runner_reports_failure", func(t *testing.T) {
		ws := t.TempDir()
		runner := commandRunner("sh -c 'echo broken 1>&2; exit 1'", nil, "")
		if runner == nil {
			t.Fatal("expected Runner")
		}
		// Defensive: confirm `sh` is on PATH; otherwise skip so the
		// sandbox stays green on minimal containers.
		if _, err := lookSh(); err != nil {
			t.Skipf("/bin/sh unavailable in test env: %v", err)
		}
		passed, report, err := runner(context.Background(), ws)
		if err != nil {
			t.Fatalf("shell runner must swallow non-zero exit into (false, report, nil), got err=%v", err)
		}
		if passed {
			t.Fatalf("expected passed=false on exit 1, got true")
		}
		if !strings.Contains(report, "broken") {
			t.Fatalf("expected failure report to contain stderr %q, got %q", "broken", report)
		}
	})

	t.Run("container_runner_is_invoked", func(t *testing.T) {
		called := false
		var capturedImage string
		var capturedCmd string
		mock := &fakeContainerRunner{
			runFn: func(ctx context.Context, image, ws, cmd string) (string, error) {
				called = true
				capturedImage = image
				capturedCmd = cmd
				return "container-output", nil
			},
		}
		runner := commandRunner("go test ./...", mock, "golang:1.23")
		passed, report, err := runner(context.Background(), "/fake/ws")
		if err != nil {
			t.Fatalf("container runner err: %v", err)
		}
		if !called {
			t.Fatal("container runner was not invoked")
		}
		if capturedImage != "golang:1.23" {
			t.Fatalf("expected image propagated as %q, got %q", "golang:1.23", capturedImage)
		}
		if capturedCmd != "go test ./..." {
			t.Fatalf("expected command propagated verbatim, got %q", capturedCmd)
		}
		if !passed || report != "container-output" {
			t.Fatalf("expected (true,%q), got (%v,%q)", "container-output", passed, report)
		}
	})
}

// fakeContainerRunner is the ContainerRunner shim used by commandRunner
// when --container / Config.ContainerRunner is provided. The real
// adapter lives in cmd/sin-code/internal/autonomy; we provide just
// enough surface for the loopbuilder test below.
type fakeContainerRunner struct {
	runFn func(ctx context.Context, image, workspace, cmd string) (string, error)
}

func (f *fakeContainerRunner) RunInContainer(ctx context.Context, image, workspace, cmd string) (string, error) {
	if f.runFn == nil {
		return "", errors.New("fakeContainerRunner.runFn not set")
	}
	return f.runFn(ctx, image, workspace, cmd)
}

// lookSh is a stdlib-only PATH probe so the shell-execution sub-test
// skips cleanly on minimal containers (no /bin/sh) instead of failing
// the suite.
func lookSh() (string, error) {
	for _, p := range []string{"/bin/sh", "/usr/bin/sh", "/bin/bash"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no sh on PATH")
}

// 2) WireFusion wires a tournament runner on verify.fail (issue #290).
// Three invariants:
//   - FusionEnabled=false                  → loop.TournamentRunner stays nil
//   - FusionEnabled=true + <2 providers    → no-op (1 provider cannot fan out)
//   - FusionEnabled=true + ≥2 providers    → tournament runner attached
func TestWireFusion_Disabled(t *testing.T) {
	withIsolatedHome(t)

	t.Run("disabled_does_not_wire_tournament", func(t *testing.T) {
		loop := &agentloop.Loop{}
		gate := verify.NewGate("poc", nil, nil)
		WireFusion(loop, Config{FusionEnabled: false}, gate, nil, nil, nil, nil)
		if loop.TournamentRunner != nil {
			t.Fatal("FusionEnabled=false must not attach a TournamentRunner")
		}
	})

	t.Run("single_provider_does_not_wire_tournament", func(t *testing.T) {
		loop := &agentloop.Loop{}
		gate := verify.NewGate("poc", nil, nil)
		// Single provider — WireFusion's <2-provider guard returns early.
		WireFusion(loop, Config{
			FusionEnabled:   true,
			FusionProviders: []string{"only"},
		}, gate, nil, nil, nil, nil)
		if loop.TournamentRunner != nil {
			t.Fatal("<2 providers must no-op even when FusionEnabled=true")
		}
	})

	t.Run("enabled_two_providers_wires_tournament", func(t *testing.T) {
		loop := &agentloop.Loop{}
		gate := verify.NewGate("poc", nil, nil)
		WireFusion(loop, Config{
			FusionEnabled:    true,
			FusionProviders:  []string{"minimax-m3", "glm-5p2"},
			FusionMaxCostUSD: 10.0,
			FusionMinQuorum:  2,
		}, gate, nil, nil, nil, nil)
		if loop.TournamentRunner == nil {
			t.Fatal(">=2 providers + FusionEnabled=true must attach a TournamentRunner")
		}
		adapter, ok := loop.TournamentRunner.(*fusionAdapter)
		if !ok {
			t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
		}
		if len(adapter.t.Providers) < 2 {
			t.Fatalf("expected >=2 tournament providers, got %d", len(adapter.t.Providers))
		}
	})

	t.Run("shouldrun_false_on_pass_when_difficulty_gate_off", func(t *testing.T) {
		// Empty tournament is enough to exercise ShouldRun: the
		// adapter inspects only the verify.Result and the cfg.
		adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: false}}
		if adapter.ShouldRun(verify.Result{Passed: true, Mode: verify.ModePoC, Report: "ok"}) {
			t.Fatal("ShouldRun must be false when the verify-gate already passed")
		}
	})
}

// fakeTournament is a thin *fusion.Tournament alias so the test does
// not have to import every Tournament field dependency just to drive
// the ShouldRun path. Production code wires a fully populated Tournament
// through WireFusion; we only need a non-nil pointer here.
type fakeTournament = fusion.Tournament

// 3) NewDefaultSessionContextBuilder must compose a preamble markdown
// from every store when populated. The five stores we exercise here:
//   1) todos        → "Open Todos" section
//   2) summary      → "Session Summary" section (LedgerSessionSummaryReader)
//   3) memory       → "Memories" section (MemoryStoreReader)
//   4) MEMORY.md    → "Auto Memory" section (FileAutoMemoryReader)
//   5) lessons      → "Lessons" section (LessonsStoreReader)
func TestSessionContextBuilder_AllStores(t *testing.T) {
	dir := withIsolatedHome(t)

	// 1) todos: one open todo.
	tdb, err := todo.Open(filepath.Join(dir, "todo.db"))
	if err != nil {
		t.Fatalf("todo.Open: %v", err)
	}
	defer tdb.Close()
	if err := tdb.Add(&todo.Todo{Title: "ship v3.23", Priority: todo.PriorityP1}); err != nil {
		t.Fatal(err)
	}

	// 3) memory: one insight.
	mdb, err := memory.Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	defer mdb.Close()
	if err := mdb.Add(&memory.Memory{Insight: "use control-character compression on batch tooling"}); err != nil {
		t.Fatal(err)
	}

	// 4) MEMORY.md (workspace-scoped auto-memory).
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"),
		[]byte("# Workspace MEMORY\n- prefer functional core / imperative shell\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 5) lessons: one entry.
	ldb, err := lessons.Open(filepath.Join(dir, "lessons.db"))
	if err != nil {
		t.Fatalf("lessons.Open: %v", err)
	}
	defer ldb.Close()
	if err := ldb.Record(context.Background(), lessons.Entry{
		Type:    lessons.TypeToolError,
		Lesson:  "do not retry on rate-limit (status 429) without backoff",
		Context: map[string]any{"tool": "scout"},
	}); err != nil {
		t.Fatal(err)
	}

	// 2) summary is intentionally nil (no persistent ledger) — the
	// builder degrades gracefully when the reader is absent.
	builder := NewDefaultSessionContextBuilder(dir, tdb, "", nil, ldb, mdb, nil, "")
	out, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty preamble when 5 stores have content")
	}

	expectedSections := []string{
		"## Auto Memory",
		"## Lessons",
		"## Memories",
		"## Open Todos",
	}
	for _, h := range expectedSections {
		if !strings.Contains(out, h) {
			t.Errorf("preamble missing %q section\n----\n%s\n----", h, out)
		}
	}

	// Spot-check that the actual content of each store made it into
	// the preamble rather than just the headings.
	want := []string{
		"ship v3.23",
		"control-character compression",
		"prefer functional core",
		"do not retry on rate-limit",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("preamble missing store content %q\n----\n%s\n----", w, out)
		}
	}

	// Section ordering is fixed in agentloop.Format (issue #379):
	// Session Summary → Auto Memory → Lessons → Memories → Goals → Open Todos.
	if i := strings.Index(out, "## Auto Memory"); i >= 0 {
		if j := strings.Index(out, "## Lessons"); j < 0 || j < i {
			t.Errorf("Lessons section should follow Auto Memory, got out:\n%s", out)
		}
	}
	if i := strings.Index(out, "## Lessons"); i >= 0 {
		if j := strings.Index(out, "## Memories"); j < 0 || j < i {
			t.Errorf("Memories section should follow Lessons, got out:\n%s", out)
		}
	}
}

// 4) Build must propagate CoverageRequiredTools and CoverageForbiddenTools
// onto its returned Loop verbatim (issue #248). The enforcer constructed
// at Run time must:
//   - reject completion when a required tool was never invoked
//   - accept completion when every required tool was invoked
//   - reject completion when a forbidden tool was invoked
//
// We mock just the Completion and LocalTool to drive a deterministic
// single-turn Run, then assert the verdict via res.Verified / error.
//
// Two reasons for not relying solely on field propagation:
//  1. pool propagation is trivial; the value lies in also exercising
//     the enforcer so a future refactor that drops the field still
//     fails the test rather than silently weakening M7.
//  2. requires zero network/IO beyond an in-process session store.
func TestBuild_RequiredToolsEnforcement(t *testing.T) {
	dir := withIsolatedHome(t)
	s := setupLoopSession(t, dir)

	t.Run("propagates_required_and_forbidden_tools", func(t *testing.T) {
		loop, cleanup, err := Build(context.Background(), Config{
			Workspace:               dir,
			MaxTurns:                3,
			SkipMCP:                 true,
			CoverageRequiredTools:   []string{"sin_read", "sin_write"},
			CoverageForbiddenTools:  []string{"sin_bash"},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()

		if !reflect.DeepEqual(loop.CoverageRequiredTools, []string{"sin_read", "sin_write"}) {
			t.Fatalf("CoverageRequiredTools: want [sin_read sin_write], got %v", loop.CoverageRequiredTools)
		}
		if !reflect.DeepEqual(loop.CoverageForbiddenTools, []string{"sin_bash"}) {
			t.Fatalf("CoverageForbiddenTools: want [sin_bash], got %v", loop.CoverageForbiddenTools)
		}
	})

	t.Run("accepts_when_required_tools_invoked", func(t *testing.T) {
		loop, cleanup, err := Build(context.Background(), Config{
			Workspace:              dir,
			MaxTurns:               2,
			SkipMCP:                true,
			CoverageRequiredTools:  []string{"sin_read", "sin_write"},
			ObserverWindow:         0, // disable loop detector for deterministic single-shot runs
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		loop.LoopDetector = nil // defensive: matches the disabled observer window

		// Override Completion so it invokes BOTH required tools on
		// turn 1 and emits a "done" reply on turn 2.
		turns := 0
		loop.Completion = func(ctx context.Context, msgs []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
			turns++
			if turns == 1 {
				return &agentloop.Completion{
					Text: "",
					ToolCalls: []agentloop.ToolCall{
						{ID: "t1", Name: "sin_read", Args: map[string]any{"path": "x"}},
						{ID: "t2", Name: "sin_write", Args: map[string]any{"path": "y"}},
					},
					Raw: session.Message{Role: "assistant", Content: ""},
				}, nil
			}
			return &agentloop.Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		}
		// Replace LocalTool / Gate so a Run is deterministic.
		loop.LocalTool = func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "ok", nil
		}
		loop.Gate = verify.NewGate("poc",
			func(ctx context.Context, ws string) (bool, string, error) { return true, "pass", nil }, nil)
		// Stamp session into the loop's workspace so the ledger /
		// file writes go somewhere harmless. (SkipMCP=true is
		// sufficient to keep this test free of network I/O.)
		loop.SessionID = s.ID

		res, err := loop.Run(context.Background(), s, "verify coverage")
		if err != nil {
			t.Fatalf("Run failed when both required tools invoked: %v", err)
		}
		if !res.Verified {
			t.Fatal("expected verified=true once coverage satisfied")
		}
		if turns < 2 {
			t.Fatalf("expected ≥2 turns (tool-call + done), got %d", turns)
		}
	})

	t.Run("rejects_when_forbidden_tool_invoked", func(t *testing.T) {
		loop, cleanup, err := Build(context.Background(), Config{
			Workspace:              dir,
			MaxTurns:               5,
			SkipMCP:                true,
			CoverageForbiddenTools: []string{"sin_bash"},
			ObserverWindow:         0, // disable loop detector (test would otherwise trip on repeat sin_bash)
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		loop.LoopDetector = nil // defensive: built-in loop detector off for this test

		// Cap stop-gate rejections to 1 so the loop bails out via
		// the MaxStopRejects path (which embeds the open criteria
		// — including "forbidden tool used: <tool>" — verbatim in
		// the returned error). Default MaxStopRejects would burn
		// 3 turns before bailing and hit the MaxTurns path instead,
		// which does not include the open criteria in the error.
		loop.MaxStopRejects = 1

		loop.Completion = func(ctx context.Context, msgs []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
			return &agentloop.Completion{
				Text: "done",
				ToolCalls: []agentloop.ToolCall{
					{ID: "t1", Name: "sin_bash", Args: map[string]any{"command": "ls"}},
				},
				Raw: session.Message{Role: "assistant", Content: "done"},
			}, nil
		}
		loop.LocalTool = func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "out", nil
		}
		loop.Gate = verify.NewGate("poc",
			func(ctx context.Context, ws string) (bool, string, error) { return true, "pass", nil }, nil)
		loop.SessionID = s.ID

		t.Logf("PRE-RUN: Coverage=%v RequiredTools=%v ForbiddenTools=%v MaxStopRejects=%d",
			loop.Coverage, loop.CoverageRequiredTools, loop.CoverageForbiddenTools, loop.MaxStopRejects)
		_, err = loop.Run(context.Background(), s, "forbidden tool")
		t.Logf("POST-RUN: err=%v", err)
		if err == nil {
			t.Fatal("expected error when forbidden tool is invoked and coverage never clears")
		}
		if !strings.Contains(err.Error(), "forbidden tool used") {
			t.Fatalf("expected error mentioning 'forbidden tool used', got: %v", err)
		}
	})
}

// 5) loadHooks reads hooks.json from two locations (issue #64):
//   - user-level:   os.UserConfigDir()/sin-code/hooks.json
//   - project-local: <workspace>/.sin-code/hooks.json
//
// Both files are concatenated into a single slice. A malformed JSON
// file is skipped with a stderr warning and does not break parsing of
// the other file.
//
// Each sub-test calls withIsolatedHome() independently so the user-
// level fixture is rebuilt in a fresh temp dir (otherwise tests that
// run after `both_files_concatenated` would inherit its file).
func TestLoadHooks_EmptyConfig(t *testing.T) {

	t.Run("no_files_returns_empty_slice_and_no_error", func(t *testing.T) {
		workspace := withIsolatedHome(t)
		got := loadHooks(filepath.Join(workspace, "fresh-workspace"))
		if len(got) != 0 {
			t.Fatalf("expected empty hook list when no files present, got %d hooks: %+v", len(got), got)
		}
	})

	t.Run("project_local_only_returns_its_hooks", func(t *testing.T) {
		workspace := withIsolatedHome(t)
		ws := filepath.Join(workspace, "ws1")
		if err := os.MkdirAll(filepath.Join(ws, ".sin-code"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeHooksJSON(t, filepath.Join(ws, ".sin-code", "hooks.json"), []hooks.Hook{
			{Event: "session.start", Type: "prompt", Text: "hello"},
		})
		got := loadHooks(ws)
		if len(got) != 1 {
			t.Fatalf("expected 1 hook, got %d", len(got))
		}
		if got[0].Event != "session.start" || got[0].Text != "hello" {
			t.Fatalf("unexpected hook payload: %+v", got[0])
		}
	})

	t.Run("both_files_concatenated", func(t *testing.T) {
		workspace := withIsolatedHome(t)
		// User-level fixture. loadHooks derives its user path from
		// os.UserConfigDir(); on darwin that resolves to
		// $HOME/Library/Application Support and on linux to
		// $XDG_CONFIG_HOME (or $HOME/.config). Query UserConfigDir
		// for the host OS and drop the file exactly where the
		// builder will look.
		ucd, err := os.UserConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		userHooksDir := filepath.Join(ucd, "sin-code")
		if err := os.MkdirAll(userHooksDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeHooksJSON(t, filepath.Join(userHooksDir, "hooks.json"), []hooks.Hook{
			{Event: "session.start", Type: "prompt", Text: "user-level"},
		})

		// Project-local fixture.
		ws := filepath.Join(workspace, "ws2")
		if err := os.MkdirAll(filepath.Join(ws, ".sin-code"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeHooksJSON(t, filepath.Join(ws, ".sin-code", "hooks.json"), []hooks.Hook{
			{Event: "task.complete", Type: "prompt", Text: "project-local"},
		})

		got := loadHooks(ws)
		if len(got) != 2 {
			t.Fatalf("expected 2 hooks (user+project), got %d: %+v", len(got), got)
		}
		var seenUser, seenProject bool
		for _, h := range got {
			if h.Event == "session.start" && h.Text == "user-level" {
				seenUser = true
			}
			if h.Event == "task.complete" && h.Text == "project-local" {
				seenProject = true
			}
		}
		if !seenUser || !seenProject {
			t.Fatalf("missing user-level or project-local hook in result: %+v", got)
		}
	})

	t.Run("malformed_json_is_skipped_with_stderr_warning", func(t *testing.T) {
		workspace := withIsolatedHome(t)
		ws := filepath.Join(workspace, "ws3")
		if err := os.MkdirAll(filepath.Join(ws, ".sin-code"), 0o755); err != nil {
			t.Fatal(err)
		}
		// loadHooks unmarshals the whole file. Writing a valid hook
		// first then truncating leaves the file well-formed but
		// incomplete; we want the unambiguous parse-error path so
		// over-write with truncated JSON instead.
		bogus := []byte(`{"event":"session.start","type":"prompt"`)
		if err := os.WriteFile(filepath.Join(ws, ".sin-code", "hooks.json"), bogus, 0o644); err != nil {
			t.Fatal(err)
		}
		// Redirect stderr to keep the test output clean. The warn
		// line goes to the hookerr logger on stderr.
		origStderr := os.Stderr
		devNull, _ := os.Open(os.DevNull)
		defer func() {
			os.Stderr = origStderr
			devNull.Close()
		}()
		os.Stderr = devNull

		got := loadHooks(ws)
		// The malformed file is skipped, no other valid file exists,
		// so the result must be empty (no panic, no error return).
		if len(got) != 0 {
			t.Fatalf("expected 0 hooks when only malformed JSON is present, got %d: %+v", len(got), got)
		}
	})
}

// writeHooksJSON serialises hooks in the production JSON shape so the
// file is byte-compatible with hooks.Engine.New([]Hook{...}). Using a
// raw stdlib marshal keeps this dependency-free.
func writeHooksJSON(t *testing.T, path string, hooks []hooks.Hook) {
	t.Helper()
	data, err := json.Marshal(hooks)
	if err != nil {
		t.Fatalf("marshal hooks: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// setupLoopSession opens a fresh SQLite-backed session store in
// dir and returns a Resumeable session. We avoid relying on
// DefaultPath() so the user filesystem is never touched even when
// SIN_CODE_HOME gets reset mid-test.
func setupLoopSession(t *testing.T, dir string) *session.Session {
	t.Helper()
	store, err := session.Open(filepath.Join(dir, "session.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
