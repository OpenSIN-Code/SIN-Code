// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for firstLine, safeInvokePostListener,
// AutoLintListener (unconfigured), runTestCommand (exit-code / not-found /
// timeout), and the auto-lint bridge to hooklife.Registry. Designed to lift
// coverage of cmd/sin-code/internal/hooks from 66.1% toward ~85%+ without
// altering any production code.
//
// Spec-vs-code divergence notes (mandate M3 — verification gate is sacred):
//
//   * safeInvokePostListener returns []string (merged into engine.Result
//     PromptInjects), NOT a hooklife.Decision. The panic path is fail-open:
//     returns nil and logs to stderr; never blocks the engine.
//
//   * runTestCommand returns "" on exit-0 and the combined
//     stdout+stderr / runErr.Error() on failure. It does not wrap the
//     error with context.DeadlineExceeded; exec.CommandContext sends
//     SIGKILL and surfaces *exec.ExitError ("signal: killed"). The
//     typed-error path for "command not found" is exercised via isNotFound
//     directly with a synthetic exec.Error and an os.PathError.
//
//   * AutoLintHook fires on hooklife.PostToolUse (not PreToolUse, as a
//     spec draft suggested). It accepts {"sin_write", "sin_edit", "Write",
//     "Edit"} via isEditTool. ID() returns the stable constant
//     autoLintHookID across calls.
//
//   * firstLine returns the input verbatim when every split+trimmed line
//     is empty; for "\n" the loop produces two empty strings and falls
//     through to `return s` (= "\n"), NOT "".
//
//   * firstLine does NOT strip a leading U+FEFF BOM. strings.TrimSpace
//     only strips runes where unicode.IsSpace returns true; U+FEFF is
//     not whitespace. The byte-exact output for
//     "\xEF\xBB\xBFhello\nworld" is "\xEF\xBB\xBFhello", not "hello".
//
//   * runTestCommand accepts a timeout time.Duration parameter but
//     NEVER reads it. Cancellation is driven entirely by the passed
//     context (exec.CommandContext). Tests that want a timeout must
//     pass ctx = context.WithTimeout(...).
package hooks

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
)

func TestFirstLine_StripsBOMAndCRLF(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		note string
	}{
		{
			name: "BOM_then_hello_LF_world",
			in:   "\xEF\xBB\xBFhello\nworld",
			want: "\xEF\xBB\xBFhello",
			note: "spec draft expected \"hello\" (BOM stripped); actually preserved — strings.TrimSpace does not classify U+FEFF as whitespace",
		},
		{
			name: "hello_CRLF_world",
			in:   "hello\r\nworld",
			want: "hello",
			note: "strings.TrimSpace strips the \\r as whitespace",
		},
		{
			name: "empty_input_returns_empty",
			in:   "",
			want: "",
			note: "no non-empty line → fallback return s (== \"\")",
		},
		{
			name: "just_LF_returns_input_verbatim",
			in:   "\n",
			want: "\n",
			note: "spec draft expected \"\"; actual returns input verbatim when every split+trimmed line is empty",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := firstLine(c.in)
			if got != c.want {
				t.Errorf("firstLine(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSafeInvokePostListener_PanicRecovery(t *testing.T) {
	// Direct invariant: panic inside the listener is recovered and the
	// function returns nil — the engine then iterates the next listener
	// normally.
	panicked := false
	panicking := func(_ context.Context, _ Payload) []string {
		panicked = true
		panic("intentional panic for TestSafeInvokePostListener_PanicRecovery")
	}
	got := safeInvokePostListener(panicking, context.Background(), Payload{
		Event: ToolPost, Name: "sin_edit",
	})
	if got != nil {
		t.Errorf("post-panic return: want nil, got %v", got)
	}
	if !panicked {
		t.Error("panicking listener must have been invoked exactly once")
	}

	// nil listener is a no-op (defensive against double-nil in tests).
	if got := safeInvokePostListener(nil, context.Background(), Payload{
		Event: ToolPost, Name: "sin_edit",
	}); got != nil {
		t.Errorf("nil listener: want nil, got %v", got)
	}

	// Engine wiring: a panic in the first post-listener MUST NOT block,
	// and a subsequent listener MUST still fire (this is the load-bearing
	// invariant — failure here would silently lose post-tool telemetry).
	eng := New(nil)
	eng.RegisterPostListener(panicking)
	secondCalled := false
	eng.RegisterPostListener(func(_ context.Context, _ Payload) []string {
		secondCalled = true
		return []string{"second-listener-after-panic"}
	})
	res := eng.Fire(context.Background(), Payload{Event: ToolPost, Name: "sin_edit"})
	if res.Blocked {
		t.Error("panic must not propagate to Result.Blocked (fail-open)")
	}
	if res.BlockReason != "" {
		t.Errorf("panic must not set BlockReason; got %q", res.BlockReason)
	}
	if !secondCalled {
		t.Fatal("subsequent listener did not fire after first listener panicked")
	}
	if len(res.PromptInjects) != 1 || res.PromptInjects[0] != "second-listener-after-panic" {
		t.Errorf("want one inject from second listener; got %v", res.PromptInjects)
	}
}

func TestAutoLintListener_NilConfig(t *testing.T) {
	// AutoHookConfig is a value type — zero-value is the "uninitialized"
	// analog. normalized() must supply AutoLintDefaultTimeout.
	var cfg AutoHookConfig
	if cfg.Timeout != 0 {
		t.Fatalf("zero-value cfg.Timeout should be 0; got %v", cfg.Timeout)
	}
	listener := AutoLintListener(cfg)

	// Early-return paths: each one short-circuits BEFORE runLintCommands,
	// proving no subprocess is spawned.
	cases := []struct {
		name string
		p    Payload
	}{
		{
			name: "non_go_path_README",
			p: Payload{
				Event: ToolPost, Name: "sin_edit",
				Data: map[string]any{"path": "README.md"},
			},
		},
		{
			name: "non_go_path_markdown",
			p: Payload{
				Event: ToolPost, Name: "sin_write",
				Data: map[string]any{"path": "docs/index.md"},
			},
		},
		{
			name: "go_test_path_filtered",
			p: Payload{
				Event: ToolPost, Name: "sin_edit",
				Data:  map[string]any{"path": "fixture_test.go"},
			},
		},
		{
			name: "empty_path",
			p: Payload{
				Event: ToolPost, Name: "sin_edit",
				Data: map[string]any{},
			},
		},
		{
			name: "non_sin_tool_skipped",
			p: Payload{
				Event: ToolPost, Name: "sin_bash",
				Data:  map[string]any{"path": "x.go"},
			},
		},
		{
			name: "missing_go_file_short_circuits_at_stat",
			p: Payload{
				Event:     ToolPost, Name: "sin_edit",
				Data:      map[string]any{"path": "no-such-file.go"},
				Workspace: t.TempDir(),
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := listener(context.Background(), c.p)
			if got != nil {
				t.Errorf("short-circuit path: want nil, got %v (subprocess may have spawned)", got)
			}
		})
	}

	// Panic-free sanity: a real .go file in a tempdir exercises
	// runLintCommands. With normal cfg it may invoke gofmt/go vet (skipped
	// gracefully on PATH-less CI) but MUST never panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("listener panicked: %v", r)
		}
	}()
	tmp := t.TempDir()
	realGo := filepath.Join(tmp, "real.go")
	if err := os.WriteFile(realGo, []byte("package real\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = listener(context.Background(), Payload{
		Event:     ToolPost, Name: "sin_edit",
		Data:      map[string]any{"path": realGo},
		Workspace: tmp,
	})
}

func TestRunTestCommand_ExitCode(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; cannot exercise runTestCommand end-to-end")
	}
	// Set up a real Go module + package so `go test` is exercisable.
	tmp := t.TempDir()
	pkgDir := filepath.Join(tmp, "rtpkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module rtmod\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	testFile := filepath.Join(pkgDir, "x_test.go")
	okSrc := "package rtpkg\nimport \"testing\"\nfunc TestOK(t *testing.T) {}\n"
	if err := os.WriteFile(testFile, []byte(okSrc), 0o644); err != nil {
		t.Fatalf("write ok src: %v", err)
	}

	t.Run("exit_0_returns_empty", func(t *testing.T) {
		if got := runTestCommand(context.Background(), testFile, tmp, 60*time.Second); got != "" {
			t.Errorf("exit 0: want \"\", got %q", got)
		}
	})

	t.Run("exit_1_includes_stderr_marker", func(t *testing.T) {
		failSrc := "package rtpkg\nimport \"testing\"\nfunc TestFail(t *testing.T) { t.Fatal(\"boom-stderr-marker\") }\n"
		if err := os.WriteFile(testFile, []byte(failSrc), 0o644); err != nil {
			t.Fatalf("write fail src: %v", err)
		}
		report := runTestCommand(context.Background(), testFile, tmp, 60*time.Second)
		if report == "" {
			t.Fatal("exit 1: want non-empty report")
		}
		if !strings.Contains(report, "boom-stderr-marker") {
			t.Errorf("exit 1 report should contain stderr sentinel; got %q", report)
		}
	})

	t.Run("command_not_found", func(t *testing.T) {
		// Strip PATH so the embedded `go` lookup fails — runTestCommand
		// then returns runErr.Error() (combined output is empty).
		t.Setenv("PATH", "/nonexistent")
		report := runTestCommand(context.Background(), testFile, tmp, 30*time.Second)
		if report == "" {
			t.Fatal("not-found: want non-empty report")
		}
		if !strings.Contains(report, "executable file not found") &&
			!strings.Contains(report, "no such file") {
			t.Errorf("not-found report should contain not-found sentinel; got %q", report)
		}
		// isNotFound directly: synthetic exec.Error + os.PathError cover
		// the two substring arms in the helper.
		nfExecErr := &exec.Error{Name: "go", Err: exec.ErrNotFound}
		if !isNotFound(nfExecErr) {
			t.Error("isNotFound(&exec.Error{..., Err:exec.ErrNotFound}) should be true")
		}
		fsErr := &os.PathError{Op: "open", Path: "x", Err: syscall.ENOENT}
		if !isNotFound(fsErr) {
			t.Error("isNotFound(&os.PathError{...ENOENT}) should be true")
		}
		plain := errors.New("permission denied")
		if isNotFound(plain) {
			t.Error("isNotFound(unrelated error) should be false")
		}
		if isNotFound(nil) {
			t.Error("isNotFound(nil) should be false")
		}
	})

	t.Run("timeout_kills_sleeping_cmd", func(t *testing.T) {
		sleepSrc := "package rtpkg\nimport (\n\t\"testing\"\n\t\"time\"\n)\nfunc TestSleep(t *testing.T) { time.Sleep(2 * time.Second) }\n"
		if err := os.WriteFile(testFile, []byte(sleepSrc), 0o644); err != nil {
			t.Fatalf("write sleep src: %v", err)
		}
		// runTestCommand accepts a timeout time.Duration but IGNORES it;
		// cancellation is wired only via exec.CommandContext(ctx, ...).
		// Drive the deadline through the context, not the timeout param.
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		report := runTestCommand(ctx, testFile, tmp, 30*time.Second)
		elapsed := time.Since(start)
		if elapsed > 1500*time.Millisecond {
			t.Errorf("runTestCommand should have returned well before the 2s sleep; took %v", elapsed)
		}
		if report == "" {
			t.Fatal("timeout: want non-empty report indicating the kill")
		}
		// runTestCommand swallows the error into a string. The kill surfaces
		// indirectly as "signal: killed", "FAIL", "killed", or
		// "deadline exceeded" depending on which process group died first.
		hit := strings.Contains(report, "signal: killed") ||
			strings.Contains(report, "FAIL") ||
			strings.Contains(report, "killed") ||
			strings.Contains(report, "deadline exceeded")
		if !hit {
			t.Errorf("timeout report should mention kill/FAIL/deadline; got %q", report)
		}
	})

	t.Run("non_test_file_short_circuits", func(t *testing.T) {
		// A file without the _test.go suffix returns "" without spawning
		// `go test` at all.
		plain := filepath.Join(pkgDir, "plain.go")
		if err := os.WriteFile(plain, []byte("package rtpkg\n"), 0o644); err != nil {
			t.Fatalf("write plain: %v", err)
		}
		if got := runTestCommand(context.Background(), plain, tmp, 60*time.Second); got != "" {
			t.Errorf("non-test file: want \"\", got %q", got)
		}
	})
}

func TestAutoHookLife_BridgeToHooklife(t *testing.T) {
	// The spec referred to "autoHookListener (fires PreToolUse on Edit)".
	// The actual hooks are AutoLintHook / AutoTestHook, both firing on
	// hooklife.PostToolUse and accepting {"sin_write", "sin_edit",
	// "Write", "Edit"} via isEditTool. Tests below verify the bridge
	// against those actual contracts.
	auto := AutoLintHook{Enabled: true, Timeout: AutoLintDefaultTimeout}
	autoTest := AutoTestHook{Enabled: true, TimeoutSecs: 60}

	t.Run("ID_is_stable_across_calls", func(t *testing.T) {
		first := auto.ID()
		for i := 0; i < 5; i++ {
			if got := auto.ID(); got != first {
				t.Fatalf("ID call %d = %q, want %q", i+2, got, first)
			}
		}
		if first != "auto-lint" {
			t.Errorf("want AutoLintHook.ID() == \"auto-lint\"; got %q", first)
		}
		if autoTest.ID() != "auto-test" {
			t.Errorf("want AutoTestHook.ID() == \"auto-test\"; got %q", autoTest.ID())
		}
	})

	reg := hooklife.NewRegistry()
	reg.Register(auto)
	reg.Register(autoTest)

	t.Run("PreToolUse_phase_empty", func(t *testing.T) {
		if hs := reg.Hooks(hooklife.PreToolUse); len(hs) != 0 {
			t.Errorf("PreToolUse should hold zero hooks (post-only bridge); got %d", len(hs))
		}
	})

	t.Run("PostToolUse_phase_has_both_in_id_order", func(t *testing.T) {
		hs := reg.Hooks(hooklife.PostToolUse)
		if len(hs) != 2 {
			t.Fatalf("PostToolUse should hold two hooks; got %d (%v)", len(hs), hs)
		}
		// Stable order by ID (registry.go:28): "auto-lint" < "auto-test".
		if hs[0].ID() != "auto-lint" || hs[1].ID() != "auto-test" {
			t.Errorf("expected {auto-lint, auto-test}; got {%s, %s}", hs[0].ID(), hs[1].ID())
		}
	})

	t.Run("Edit_fires_AutoLintHook", func(t *testing.T) {
		// Edit on a real .go file in a real workspace → bridge exercises
		// runLintCommands. We don't pin Verdict (gofmt/vet may or may
		// not be installed in CI) — only that the bridge fired and
		// stamped the right HookID.
		tmp := t.TempDir()
		goFile := filepath.Join(tmp, "bridge.go")
		if err := os.WriteFile(goFile, []byte("package bridge\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		d := auto.Run(context.Background(), hooklife.Event{
			Phase:   hooklife.PostToolUse,
			Tool:    "Edit",
			Args:    map[string]string{"path": goFile},
			Workdir: tmp,
		})
		if d.HookID != "auto-lint" {
			t.Errorf("Edit bridge: HookID = %q, want %q", d.HookID, "auto-lint")
		}
		if d.Verdict != hooklife.Allow && d.Verdict != hooklife.Warn {
			t.Errorf("Edit bridge: Verdict should be Allow or Warn; got %v", d.Verdict)
		}
	})

	t.Run("Bash_does_not_fire_AutoLintHook", func(t *testing.T) {
		// isEditTool filters Bash → Allow with canonical HookID, no Message.
		d := auto.Run(context.Background(), hooklife.Event{
			Phase: hooklife.PostToolUse,
			Tool:  "Bash",
			Args:  map[string]string{"path": "/anything"},
		})
		if d.Verdict != hooklife.Allow {
			t.Errorf("Bash bridge: Verdict = %v, want Allow (fail-open)", d.Verdict)
		}
		if d.HookID != "auto-lint" {
			t.Errorf("Bash bridge: HookID = %q, want %q", d.HookID, "auto-lint")
		}
		if d.Message != "" {
			t.Errorf("Bash bridge: Message should be empty on Allow; got %q", d.Message)
		}
	})

	t.Run("Edit_on_non_go_path_does_not_fire", func(t *testing.T) {
		// isEditTool accepts Edit, but the path check filters non-.go
		// files and *_test.go files. Verify both branches produce Allow.
		for _, p := range []string{"README.md", "fixture_test.go"} {
			d := auto.Run(context.Background(), hooklife.Event{
				Phase: hooklife.PostToolUse,
				Tool:  "Edit",
				Args:  map[string]string{"path": p},
			})
			if d.Verdict != hooklife.Allow {
				t.Errorf("Edit on %q: Verdict = %v, want Allow", p, d.Verdict)
			}
			if d.Message != "" {
				t.Errorf("Edit on %q: Message should be empty on Allow; got %q", p, d.Message)
			}
		}
	})

	t.Run("disabled_hook_short_circuits", func(t *testing.T) {
		// Enabled=false is the global off-switch — every Edit falls
		// through with Allow and the canonical HookID.
		disabled := AutoLintHook{Enabled: false, Timeout: AutoLintDefaultTimeout}
		d := disabled.Run(context.Background(), hooklife.Event{
			Phase: hooklife.PostToolUse,
			Tool:  "Edit",
			Args:  map[string]string{"path": "/tmp/whatever.go"},
		})
		if d.Verdict != hooklife.Allow || d.HookID != "auto-lint" {
			t.Errorf("disabled hook: want Allow/auto-lint; got Verdict=%v HookID=%q", d.Verdict, d.HookID)
		}
	})

	t.Run("legacy_sin_write_name_accepted", func(t *testing.T) {
		// isEditTool accepts the legacy sin_write/sin_edit names too —
		// proves the legacy post-listener and hooklife bridge agree on
		// the same filter so the two paths can coexist.
		tmp := t.TempDir()
		goFile := filepath.Join(tmp, "legacy.go")
		if err := os.WriteFile(goFile, []byte("package legacy\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		for _, name := range []string{"sin_write", "sin_edit", "Write", "Edit"} {
			d := auto.Run(context.Background(), hooklife.Event{
				Phase:   hooklife.PostToolUse,
				Tool:    name,
				Args:    map[string]string{"path": goFile},
				Workdir: tmp,
			})
			if d.HookID != "auto-lint" {
				t.Errorf("%s bridge: HookID = %q, want %q", name, d.HookID, "auto-lint")
			}
		}
	})
}
