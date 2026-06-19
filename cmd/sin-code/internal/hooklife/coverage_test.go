// SPDX-License-Identifier: MIT
// Purpose: 100% statement coverage tests for the hooklife package.
package hooklife

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockHook is a programmable hook for exercising registry and runner logic.
type mockHook struct {
	id     string
	phases []Phase
	run    func(context.Context, Event) Decision
}

func (m mockHook) ID() string      { return m.id }
func (m mockHook) Phases() []Phase { return m.phases }
func (m mockHook) Run(ctx context.Context, ev Event) Decision {
	if m.run != nil {
		return m.run(ctx, ev)
	}
	return Decision{Verdict: Allow}
}

// captureStdout runs fn and returns everything written to os.Stdout.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

// --- event.go ---

func TestVerdictString(t *testing.T) {
	if got := Allow.String(); got != "allow" {
		t.Errorf("Allow.String() = %q, want allow", got)
	}
	if got := Warn.String(); got != "warn" {
		t.Errorf("Warn.String() = %q, want warn", got)
	}
	if got := Block.String(); got != "block" {
		t.Errorf("Block.String() = %q, want block", got)
	}
}

// --- registry.go ---

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	h1 := mockHook{id: "a", phases: []Phase{PreToolUse}}
	h2 := mockHook{id: "b", phases: []Phase{PreToolUse, PostToolUse}}
	reg.Register(h1)
	reg.Register(h2)

	pre := reg.Hooks(PreToolUse)
	if len(pre) != 2 || pre[0].ID() != "a" || pre[1].ID() != "b" {
		t.Errorf("PreToolUse hooks = %v", ids(pre))
	}
	post := reg.Hooks(PostToolUse)
	if len(post) != 1 || post[0].ID() != "b" {
		t.Errorf("PostToolUse hooks = %v", ids(post))
	}
	if len(reg.Hooks(Stop)) != 0 {
		t.Errorf("expected no Stop hooks")
	}

	all := reg.All()
	if len(all) != 2 || all[0].ID() != "a" || all[1].ID() != "b" {
		t.Errorf("All() = %v", ids(all))
	}
}

func ids(hs []Hook) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.ID()
	}
	return out
}

// --- runner.go ---

func TestRunnerDefaults(t *testing.T) {
	reg := NewRegistry()
	r := NewRunner(reg)
	r.WithTimeout(5 * time.Second)
	r.WithLogger(nil) // nil branch

	var logs []string
	r.WithLogger(func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})

	// No hooks -> aggregate returns Allow with empty warnings slice.
	d := r.Dispatch(context.Background(), Event{Phase: PreToolUse})
	if d.Verdict != Allow || d.Message != "" {
		t.Errorf("no hooks: got %+v", d)
	}
	if len(logs) != 0 {
		t.Errorf("expected no logs, got %v", logs)
	}
}

func TestRunnerPreToolUseBlock(t *testing.T) {
	reg := NewRegistry()
	reg.Register(mockHook{id: "blocker", phases: []Phase{PreToolUse}, run: func(context.Context, Event) Decision {
		return Decision{Verdict: Block, Message: "blocked"}
	}})
	reg.Register(mockHook{id: "after", phases: []Phase{PreToolUse}, run: func(context.Context, Event) Decision {
		return Decision{Verdict: Warn, Message: "should not run"}
	}})
	r := NewRunner(reg)
	d := r.Dispatch(context.Background(), Event{Phase: PreToolUse})
	if d.Verdict != Block || d.Message != "blocked" || d.HookID != "blocker" {
		t.Errorf("expected block from blocker, got %+v", d)
	}
}

func TestRunnerWarnAggregation(t *testing.T) {
	reg := NewRegistry()
	reg.Register(mockHook{id: "w1", phases: []Phase{PostToolUse}, run: func(context.Context, Event) Decision {
		return Decision{Verdict: Warn, Message: "one"}
	}})
	reg.Register(mockHook{id: "w2", phases: []Phase{PostToolUse}, run: func(context.Context, Event) Decision {
		return Decision{Verdict: Warn, Message: "two"}
	}})
	r := NewRunner(reg)
	d := r.Dispatch(context.Background(), Event{Phase: PostToolUse})
	if d.Verdict != Warn {
		t.Errorf("expected Warn, got %s", d.Verdict)
	}
	if !strings.Contains(d.Message, "[w1] one") || !strings.Contains(d.Message, "[w2] two") {
		t.Errorf("expected aggregated warnings, got %q", d.Message)
	}
}

func TestRunnerWarnEmptyMessage(t *testing.T) {
	reg := NewRegistry()
	reg.Register(mockHook{id: "empty", phases: []Phase{PostToolUse}, run: func(context.Context, Event) Decision {
		return Decision{Verdict: Warn, Message: ""}
	}})
	r := NewRunner(reg)
	d := r.Dispatch(context.Background(), Event{Phase: PostToolUse})
	if d.Verdict != Allow || d.Message != "" {
		t.Errorf("expected Allow, got %+v", d)
	}
}

func TestRunnerBlockOnNonPreToolUse(t *testing.T) {
	reg := NewRegistry()
	reg.Register(mockHook{id: "b", phases: []Phase{PostToolUse}, run: func(context.Context, Event) Decision {
		return Decision{Verdict: Block, Message: "block"}
	}})
	r := NewRunner(reg)
	d := r.Dispatch(context.Background(), Event{Phase: PostToolUse})
	if d.Verdict != Warn {
		t.Errorf("expected Warn, got %s", d.Verdict)
	}
	if !strings.Contains(d.Message, "[b] block") {
		t.Errorf("expected warning message, got %q", d.Message)
	}
}

func TestRunnerTimeout(t *testing.T) {
	reg := NewRegistry()
	reg.Register(mockHook{id: "slow", phases: []Phase{PreToolUse}, run: func(context.Context, Event) Decision {
		time.Sleep(10 * time.Millisecond)
		return Decision{Verdict: Allow}
	}})
	var logs []string
	r := NewRunner(reg).WithTimeout(1 * time.Nanosecond).WithLogger(func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	d := r.Dispatch(context.Background(), Event{Phase: PreToolUse})
	// runOne returns an Allow decision for timeouts, but Dispatch aggregates it
	// into a plain Allow verdict with no HookID/Message.
	if d.Verdict != Allow {
		t.Errorf("timeout: expected Allow, got %s", d.Verdict)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "timed out") {
		t.Errorf("expected timeout log, got %v", logs)
	}
}

func TestRunnerPanic(t *testing.T) {
	reg := NewRegistry()
	reg.Register(mockHook{id: "panic", phases: []Phase{PreToolUse}, run: func(context.Context, Event) Decision {
		panic("boom")
	}})
	var logs []string
	r := NewRunner(reg).WithLogger(func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})
	d := r.Dispatch(context.Background(), Event{Phase: PreToolUse})
	if d.Verdict != Allow {
		t.Errorf("panic: expected Allow, got %s", d.Verdict)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "panicked") {
		t.Errorf("expected panic log, got %v", logs)
	}
}

func TestJoinLines(t *testing.T) {
	if got := joinLines(nil); got != "" {
		t.Errorf("nil: got %q", got)
	}
	if got := joinLines([]string{"a"}); got != "a" {
		t.Errorf("single: got %q", got)
	}
	if got := joinLines([]string{"a", "b"}); got != "a\nb" {
		t.Errorf("multi: got %q", got)
	}
}

// --- builtin.go ---

func TestBlockNoVerify(t *testing.T) {
	h := BlockNoVerify{}
	if h.ID() != "block-no-verify" {
		t.Errorf("ID = %q", h.ID())
	}
	if got := h.Run(context.Background(), Event{Tool: "Edit"}); got.Verdict != Allow {
		t.Errorf("Edit should be allowed, got %s", got.Verdict)
	}
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "git commit"}}); got.Verdict != Allow {
		t.Errorf("plain commit should be allowed, got %s", got.Verdict)
	}
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "git commit -n"}}); got.Verdict != Block {
		t.Errorf("-n commit should be blocked, got %s", got.Verdict)
	}
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "git commit --no-verify"}}); got.Verdict != Block {
		t.Errorf("--no-verify should be blocked, got %s", got.Verdict)
	}
	// -n without "git commit" must NOT block (covers the && precedence).
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "echo -n foo"}}); got.Verdict != Allow {
		t.Errorf("echo -n should be allowed, got %s", got.Verdict)
	}
}

func TestConfigProtection(t *testing.T) {
	h := ConfigProtection{Protected: []string{".git/", "go.sum"}}
	if h.ID() != "config-protection" {
		t.Errorf("ID = %q", h.ID())
	}
	if len(h.Phases()) != 1 || h.Phases()[0] != PreToolUse {
		t.Errorf("Phases = %v", h.Phases())
	}
	if got := h.Run(context.Background(), Event{Tool: "Bash"}); got.Verdict != Allow {
		t.Errorf("Bash should be allowed, got %s", got.Verdict)
	}
	if got := h.Run(context.Background(), Event{Tool: "Edit", Args: map[string]string{"path": "main.go"}}); got.Verdict != Allow {
		t.Errorf("Edit main.go should be allowed, got %s", got.Verdict)
	}
	if got := h.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": ".git/config"}}); got.Verdict != Block {
		t.Errorf("Write .git/config should be blocked, got %s", got.Verdict)
	}
	if got := h.Run(context.Background(), Event{Tool: "Edit", Args: map[string]string{"path": "go.sum"}}); got.Verdict != Block {
		t.Errorf("Edit go.sum should be blocked, got %s", got.Verdict)
	}
}

func TestDefaultFormatters(t *testing.T) {
	m := DefaultFormatters()
	if len(m) == 0 {
		t.Error("expected non-empty formatters map")
	}
	if _, ok := m[".go"]; !ok {
		t.Error("expected .go formatter")
	}
}

func TestPostEditFormat(t *testing.T) {
	f := PostEditFormat{Formatter: DefaultFormatters()}
	if f.ID() != "post-edit-format" {
		t.Errorf("ID = %q", f.ID())
	}
	if len(f.Phases()) != 1 || f.Phases()[0] != PostToolUse {
		t.Errorf("Phases = %v", f.Phases())
	}

	// Not Edit/Write.
	if got := f.Run(context.Background(), Event{Tool: "Bash"}); got.Verdict != Allow {
		t.Errorf("Bash should be allowed, got %s", got.Verdict)
	}

	// No formatter for extension.
	if got := f.Run(context.Background(), Event{Tool: "Edit", Args: map[string]string{"path": "foo.txt"}}); got.Verdict != Allow {
		t.Errorf("no formatter should be allowed, got %s", got.Verdict)
	}

	oldExec := execCommandContext
	defer func() { execCommandContext = oldExec }()

	// Formatter template expands to empty command -> len(parts)==0 branch.
	g := PostEditFormat{Formatter: map[string]string{".empty": ""}}
	if got := g.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "file.empty"}}); got.Verdict != Allow {
		t.Errorf("empty formatter should be allowed, got %s", got.Verdict)
	}

	// Error branch: stub returns a non-existent command.
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "this_command_definitely_does_not_exist_for_hooklife_test")
	}
	if got := f.Run(context.Background(), Event{Tool: "Edit", Args: map[string]string{"path": "foo.go"}}); got.Verdict != Warn || !strings.Contains(got.Message, "format failed") {
		t.Errorf("expected Warn format failed, got %+v", got)
	}

	// Success branch: stub returns a harmless command.
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "go", "version")
	}
	if got := f.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "foo.go"}}); got.Verdict != Allow {
		t.Errorf("expected Allow on success, got %s", got.Verdict)
	}
}

// fakeChecker satisfies TypeChecker.
type fakeChecker struct {
	errs []string
	err  error
}

func (f fakeChecker) Diagnostics(ctx context.Context, path string) ([]string, error) {
	return f.errs, f.err
}

func TestPostEditTypecheck(t *testing.T) {
	// ID/Phases on zero value.
	h := PostEditTypecheck{}
	if h.ID() != "post-edit-typecheck" {
		t.Errorf("ID = %q", h.ID())
	}
	if len(h.Phases()) != 1 || h.Phases()[0] != PostToolUse {
		t.Errorf("Phases = %v", h.Phases())
	}

	// Nil checker.
	if got := h.Run(context.Background(), Event{Tool: "Edit", Args: map[string]string{"path": "foo.go"}}); got.Verdict != Allow {
		t.Errorf("nil checker should be allowed, got %s", got.Verdict)
	}

	// Not Edit/Write.
	h = PostEditTypecheck{Checker: fakeChecker{}}
	if got := h.Run(context.Background(), Event{Tool: "Bash"}); got.Verdict != Allow {
		t.Errorf("Bash should be allowed, got %s", got.Verdict)
	}

	// Checker error.
	h = PostEditTypecheck{Checker: fakeChecker{err: fmt.Errorf("fail")}}
	if got := h.Run(context.Background(), Event{Tool: "Edit", Args: map[string]string{"path": "foo.go"}}); got.Verdict != Allow {
		t.Errorf("checker error should be allowed, got %s", got.Verdict)
	}

	// Checker returns diagnostics.
	h = PostEditTypecheck{Checker: fakeChecker{errs: []string{"e1", "e2"}}}
	if got := h.Run(context.Background(), Event{Tool: "Edit", Args: map[string]string{"path": "foo.go"}}); got.Verdict != Warn {
		t.Errorf("expected Warn, got %s", got.Verdict)
	}

	// Checker returns no diagnostics.
	h = PostEditTypecheck{Checker: fakeChecker{}}
	if got := h.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "foo.go"}}); got.Verdict != Allow {
		t.Errorf("expected Allow, got %s", got.Verdict)
	}
}

// fakeVerifier satisfies Verifier.
type fakeVerifier struct {
	passed bool
	report string
	err    error
}

func (f fakeVerifier) QualityGate(ctx context.Context, workdir string) (bool, string, error) {
	return f.passed, f.report, f.err
}

func TestQualityGate(t *testing.T) {
	// ID/Phases on zero value.
	h := QualityGate{}
	if h.ID() != "quality-gate" {
		t.Errorf("ID = %q", h.ID())
	}
	if len(h.Phases()) != 1 || h.Phases()[0] != PreToolUse {
		t.Errorf("Phases = %v", h.Phases())
	}

	// Nil verifier.
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "git commit"}}); got.Verdict != Allow {
		t.Errorf("nil verifier should be allowed, got %s", got.Verdict)
	}

	// Not Bash.
	h = QualityGate{Verifier: fakeVerifier{}}
	if got := h.Run(context.Background(), Event{Tool: "Edit"}); got.Verdict != Allow {
		t.Errorf("Edit should be allowed, got %s", got.Verdict)
	}

	// Bash but not a commit.
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "ls"}}); got.Verdict != Allow {
		t.Errorf("non-commit Bash should be allowed, got %s", got.Verdict)
	}

	// Quality gate error.
	h = QualityGate{Verifier: fakeVerifier{err: fmt.Errorf("fail")}}
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "git commit"}}); got.Verdict != Warn {
		t.Errorf("expected Warn on error, got %s", got.Verdict)
	}

	// Quality gate failed.
	h = QualityGate{Verifier: fakeVerifier{passed: false, report: "bad"}}
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "git commit"}}); got.Verdict != Block {
		t.Errorf("expected Block on failure, got %s", got.Verdict)
	}

	// Quality gate passed.
	h = QualityGate{Verifier: fakeVerifier{passed: true}}
	if got := h.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"command": "git commit"}}); got.Verdict != Allow {
		t.Errorf("expected Allow on pass, got %s", got.Verdict)
	}
}

// fakeLedger satisfies Ledger.
type fakeLedger struct {
	tracked []struct {
		tool string
		meta map[string]string
	}
}

func (f *fakeLedger) Track(tool string, meta map[string]string) {
	f.tracked = append(f.tracked, struct {
		tool string
		meta map[string]string
	}{tool, meta})
}

func TestCostTracker(t *testing.T) {
	// ID/Phases on zero value.
	h := CostTracker{}
	if h.ID() != "cost-tracker" {
		t.Errorf("ID = %q", h.ID())
	}
	if len(h.Phases()) != 1 || h.Phases()[0] != PostToolUse {
		t.Errorf("Phases = %v", h.Phases())
	}

	// Nil ledger.
	if got := h.Run(context.Background(), Event{Tool: "Bash"}); got.Verdict != Allow {
		t.Errorf("nil ledger should be allowed, got %s", got.Verdict)
	}

	// Ledger present.
	ledger := &fakeLedger{}
	h = CostTracker{Ledger: ledger}
	if got := h.Run(context.Background(), Event{Tool: "Edit", Meta: map[string]string{"cost": "1"}}); got.Verdict != Allow {
		t.Errorf("expected Allow, got %s", got.Verdict)
	}
	if len(ledger.tracked) != 1 || ledger.tracked[0].tool != "Edit" || ledger.tracked[0].meta["cost"] != "1" {
		t.Errorf("expected ledger track, got %+v", ledger.tracked)
	}
}

func TestSuggestCompact(t *testing.T) {
	// ID/Phases on zero value.
	h := SuggestCompact{Threshold: 100}
	if h.ID() != "suggest-compact" {
		t.Errorf("ID = %q", h.ID())
	}
	if len(h.Phases()) != 2 || h.Phases()[0] != Stop || h.Phases()[1] != PreCompact {
		t.Errorf("Phases = %v", h.Phases())
	}

	// Nil TokensUsed.
	if got := h.Run(context.Background(), Event{}); got.Verdict != Allow {
		t.Errorf("nil TokensUsed should be allowed, got %s", got.Verdict)
	}

	// Threshold <= 0.
	h = SuggestCompact{TokensUsed: func() int { return 1000 }, Threshold: 0}
	if got := h.Run(context.Background(), Event{}); got.Verdict != Allow {
		t.Errorf("threshold <= 0 should be allowed, got %s", got.Verdict)
	}

	// Below threshold.
	h = SuggestCompact{TokensUsed: func() int { return 99 }, Threshold: 100}
	if got := h.Run(context.Background(), Event{}); got.Verdict != Allow {
		t.Errorf("below threshold should be allowed, got %s", got.Verdict)
	}

	// At threshold.
	h = SuggestCompact{TokensUsed: func() int { return 100 }, Threshold: 100}
	if got := h.Run(context.Background(), Event{}); got.Verdict != Warn {
		t.Errorf("at threshold should warn, got %s", got.Verdict)
	}

	// Above threshold.
	h = SuggestCompact{TokensUsed: func() int { return 101 }, Threshold: 100}
	if got := h.Run(context.Background(), Event{}); got.Verdict != Warn {
		t.Errorf("above threshold should warn, got %s", got.Verdict)
	}
}

// --- cli.go ---

func TestCLIList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(BlockNoVerify{})
	cmd := NewCommand(reg)
	cmd.SetArgs([]string{"list"})
	out := captureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute list: %v", err)
		}
	})
	if !strings.Contains(out, "block-no-verify") {
		t.Errorf("expected list output, got %q", out)
	}
}

func TestCLITest(t *testing.T) {
	reg := NewRegistry()
	reg.Register(BlockNoVerify{})
	cmd := NewCommand(reg)
	cmd.SetArgs([]string{"test", "PreToolUse", "--command", "git commit --no-verify"})
	out := captureStdout(func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute test: %v", err)
		}
	})
	if !strings.Contains(out, "verdict=block") {
		t.Errorf("expected block verdict, got %q", out)
	}
}

// --- auto-coverage ---

func TestAutoCoverageIDAndPhases(t *testing.T) {
	a := AutoCoverage{}
	if a.ID() != "auto-coverage" {
		t.Errorf("ID = %q, want auto-coverage", a.ID())
	}
	phases := a.Phases()
	if len(phases) != 1 || phases[0] != PostToolUse {
		t.Errorf("Phases = %v, want [PostToolUse]", phases)
	}
}

func TestAutoCoverageDisabled(t *testing.T) {
	a := AutoCoverage{Enabled: false}
	d := a.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "x.go"}})
	if d.Verdict != Allow {
		t.Errorf("verdict = %v, want Allow", d.Verdict)
	}
}

func TestAutoCoverageIgnoresNonGo(t *testing.T) {
	a := AutoCoverage{Enabled: true}
	d := a.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "x.md"}})
	if d.Verdict != Allow {
		t.Errorf("verdict = %v, want Allow", d.Verdict)
	}
}

func TestAutoCoverageIgnoresNonEditTools(t *testing.T) {
	a := AutoCoverage{Enabled: true}
	d := a.Run(context.Background(), Event{Tool: "Bash", Args: map[string]string{"path": "x.go"}})
	if d.Verdict != Allow {
		t.Errorf("verdict = %v, want Allow", d.Verdict)
	}
}

func TestAutoCoverageQueuesRequest(t *testing.T) {
	tmp := t.TempDir()
	a := AutoCoverage{
		Enabled: true,
		PackagePath: func(root, file string) string {
			if root != tmp {
				t.Errorf("root = %q, want %q", root, tmp)
			}
			if file != "x.go" {
				t.Errorf("file = %q, want x.go", file)
			}
			return "example.com/demo/cmd/foo"
		},
	}
	d := a.Run(context.Background(), Event{Tool: "Write", Workdir: tmp, Args: map[string]string{"path": "x.go"}})
	if d.Verdict != Warn {
		t.Errorf("verdict = %v, want Warn", d.Verdict)
	}
	if !strings.Contains(d.Message, "example.com/demo/cmd/foo") {
		t.Errorf("message = %q, want import path", d.Message)
	}
	dest := filepath.Join(tmp, RequestDir, "example.com--demo--cmd--foo.json")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "example.com/demo/cmd/foo") {
		t.Errorf("request = %q, want import path", data)
	}
}

func TestAutoCoverageDefaultHooks(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := AutoCoverage{Enabled: true}
	path := filepath.Join(tmp, "x.go")
	d := a.Run(context.Background(), Event{Tool: "Write", Workdir: tmp, Args: map[string]string{"path": path}})
	if d.Verdict != Warn {
		t.Errorf("verdict = %v, want Warn", d.Verdict)
	}
	dest := filepath.Join(tmp, RequestDir, "example.com--demo.json")
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
}

func TestAutoCoverageEmptyWorkdir(t *testing.T) {
	a := AutoCoverage{
		Enabled:     true,
		PackagePath: func(_, _ string) string { return "pkg" },
		mkdirAll: func(dir string, _ os.FileMode) error {
			if dir != RequestDir {
				t.Errorf("dir = %q, want %q", dir, RequestDir)
			}
			return nil
		},
		writeFile: func(name string, _ []byte, _ os.FileMode) error {
			want := filepath.Join(RequestDir, "pkg.json")
			if name != want {
				t.Errorf("name = %q, want %q", name, want)
			}
			return nil
		},
	}
	d := a.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "x.go"}})
	if d.Verdict != Warn {
		t.Errorf("verdict = %v, want Warn", d.Verdict)
	}
}

func TestAutoCoverageRequestDirOverride(t *testing.T) {
	tmp := t.TempDir()
	a := AutoCoverage{
		Enabled:     true,
		RequestDir:  "custom-queue",
		PackagePath: func(_, _ string) string { return "pkg" },
		mkdirAll: func(dir string, _ os.FileMode) error {
			want := filepath.Join(tmp, "custom-queue")
			if dir != want {
				t.Errorf("dir = %q, want %q", dir, want)
			}
			return nil
		},
		writeFile: func(name string, _ []byte, _ os.FileMode) error { return nil },
	}
	d := a.Run(context.Background(), Event{Tool: "Write", Workdir: tmp, Args: map[string]string{"path": "x.go"}})
	if d.Verdict != Warn {
		t.Errorf("verdict = %v, want Warn", d.Verdict)
	}
}

func TestAutoCoverageMkdirError(t *testing.T) {
	a := AutoCoverage{
		Enabled:     true,
		PackagePath: func(_, _ string) string { return "pkg" },
		mkdirAll:    func(_ string, _ os.FileMode) error { return fmt.Errorf("mkdir err") },
	}
	d := a.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "x.go"}})
	if d.Verdict != Warn || !strings.Contains(d.Message, "mkdir err") {
		t.Errorf("decision = %+v, want mkdir error", d)
	}
}

func TestAutoCoverageWriteError(t *testing.T) {
	a := AutoCoverage{
		Enabled:     true,
		PackagePath: func(_, _ string) string { return "pkg" },
		mkdirAll:    func(_ string, _ os.FileMode) error { return nil },
		writeFile:   func(_ string, _ []byte, _ os.FileMode) error { return fmt.Errorf("write err") },
	}
	d := a.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "x.go"}})
	if d.Verdict != Warn || !strings.Contains(d.Message, "write err") {
		t.Errorf("decision = %+v, want write error", d)
	}
}

func TestAutoCoverageMarshalError(t *testing.T) {
	jsonMarshalIndentHook = func(_ any, _, _ string) ([]byte, error) {
		return nil, fmt.Errorf("marshal err")
	}
	defer func() { jsonMarshalIndentHook = json.MarshalIndent }()
	a := AutoCoverage{
		Enabled:     true,
		PackagePath: func(_, _ string) string { return "pkg" },
		mkdirAll:    func(_ string, _ os.FileMode) error { return nil },
	}
	d := a.Run(context.Background(), Event{Tool: "Write", Args: map[string]string{"path": "x.go"}})
	if d.Verdict != Warn || !strings.Contains(d.Message, "marshal err") {
		t.Errorf("decision = %+v, want marshal error", d)
	}
}

func TestCoverageRequestDirWithOverride(t *testing.T) {
	if got := coverageRequestDirWithOverride("", "override"); got != "override" {
		t.Errorf("empty workdir = %q, want override", got)
	}
	want := filepath.Join("work", "override")
	if got := coverageRequestDirWithOverride("work", "override"); got != want {
		t.Errorf("workdir = %q, want %q", got, want)
	}
	if got := coverageRequestDirWithOverride("work", ""); got != filepath.Join("work", RequestDir) {
		t.Errorf("default = %q, want %q", got, filepath.Join("work", RequestDir))
	}
}
