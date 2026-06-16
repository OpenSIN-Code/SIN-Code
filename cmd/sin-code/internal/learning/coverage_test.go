package learning

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestNew_allOptions(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	opts := Options{
		Workdir:    t.TempDir(),
		LLM:        &llm.Client{},
		Memory:     &memory.Store{},
		VerifyGate: verify.NewGate("off", nil, nil),
	}
	l, err := New(opts)
	if err != nil {
		t.Fatalf("New with all options: %v", err)
	}
	if l == nil {
		t.Fatal("New returned nil Learner")
	}
	if l.Manager() == nil || l.Observer() == nil || l.Hooks() == nil {
		t.Fatal("New returned nil subsystems")
	}
}

func TestNew_zeroValue(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	l, err := New(Options{})
	if err != nil {
		t.Fatalf("New zero value: %v", err)
	}
	if l == nil {
		t.Fatal("New returned nil Learner")
	}
	_ = l.Manager()
	_ = l.Observer()
	_ = l.Hooks()
}

func TestBeforeTurn(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	l, err := New(Options{Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	block := l.BeforeTurn(context.Background(), &session.Session{})
	if block != "" {
		t.Errorf("expected empty block, got %q", block)
	}
}

func TestBeforeTool_allowAndBlock(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	l, err := New(Options{Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	allowed, msg := l.BeforeTool(context.Background(), "Read", map[string]any{"path": "foo.go"})
	if !allowed {
		t.Errorf("expected Read to be allowed, got blocked: %s", msg)
	}

	blocked, msg := l.BeforeTool(context.Background(), "Edit", map[string]any{"path": ".git/config"})
	if blocked {
		t.Errorf("expected Edit of .git/config to be blocked, got allowed")
	}
	if msg == "" {
		t.Errorf("expected block message, got empty")
	}
}

type warnHook struct{}

func (warnHook) ID() string               { return "test-warn" }
func (warnHook) Phases() []hooklife.Phase { return []hooklife.Phase{hooklife.PostToolUse} }
func (warnHook) Run(context.Context, hooklife.Event) hooklife.Decision {
	return hooklife.Decision{Verdict: hooklife.Warn, Message: "test warning"}
}

func TestAfterTool(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	l, err := New(Options{Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	// Capture stderr so the warning does not pollute test output.
	tmp, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = tmp
	t.Cleanup(func() {
		os.Stderr = oldStderr
		_ = tmp.Close()
	})

	// Warning path.
	l.Hooks().Register(warnHook{})
	l.AfterTool(context.Background(), "Read", map[string]any{"path": "foo.go"}, true, "")

	// Allow path with a failing success flag.
	l.AfterTool(context.Background(), "Bash", map[string]any{"command": "go test ./..."}, false, "boom")
}

func TestEndTurnAndPreCompact(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	l, err := New(Options{Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	// Empty flush.
	created, reinforced, err := l.EndTurn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 || reinforced != 0 {
		t.Errorf("empty EndTurn: expected 0,0, got %d,%d", created, reinforced)
	}

	// Record two identical successful observations so the heuristic extractor emits a candidate.
	l.AfterTool(context.Background(), "Bash", map[string]any{"command": "go test ./..."}, true, "")
	l.AfterTool(context.Background(), "Bash", map[string]any{"command": "go test ./..."}, true, "")

	created, reinforced, err = l.EndTurn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 || reinforced != 0 {
		t.Errorf("EndTurn after observations: expected 1,0, got %d,%d", created, reinforced)
	}

	// PreCompact also flushes and dispatches the PreCompact hook.
	// Use a different command so the heuristic extractor emits a new candidate.
	l.AfterTool(context.Background(), "Bash", map[string]any{"command": "go build ./..."}, true, "")
	l.AfterTool(context.Background(), "Bash", map[string]any{"command": "go build ./..."}, true, "")
	created, reinforced, err = l.PreCompact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 || reinforced != 0 {
		t.Errorf("PreCompact after observations: expected 1,0, got %d,%d", created, reinforced)
	}
}

func TestSetStyle(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	l, err := New(Options{Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	// Default style returns empty when no instincts are active.
	if got := l.BeforeTurn(context.Background(), &session.Session{}); got != "" {
		t.Fatalf("expected empty block with default style, got %q", got)
	}

	// Switch to terse mode; the style block should be emitted even with no active instincts.
	l.SetStyle("terse")
	got := l.BeforeTurn(context.Background(), &session.Session{})
	if !strings.Contains(got, "# Output style") {
		t.Fatalf("expected style block after SetStyle(terse), got %q", got)
	}

	// Revert to default.
	l.SetStyle("")
	if got := l.BeforeTurn(context.Background(), &session.Session{}); got != "" {
		t.Fatalf("expected empty block after reverting to default, got %q", got)
	}
}

func TestBeforeTurnActiveErrorFallsBackToStyle(t *testing.T) {
	instinctDir := t.TempDir()
	t.Setenv("SIN_INSTINCT_DIR", instinctDir)
	workdir := t.TempDir()
	l, err := New(Options{Workdir: workdir})
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the project's instinct directory so Manager.Active() returns an error.
	projID := l.Manager().Project().ID
	dir := filepath.Join(instinctDir, "projects", projID, "instincts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corrupt.md"), []byte("not valid instinct"), 0o644); err != nil {
		t.Fatal(err)
	}

	l.SetStyle("terse")
	got := l.BeforeTurn(context.Background(), &session.Session{})
	if !strings.Contains(got, "# Output style") {
		t.Fatalf("expected style block even when Active() errors, got %q", got)
	}
}

func TestWrap(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	l, err := New(Options{Workdir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	loop := &agentloop.Loop{}
	if got := l.Wrap(loop); got != loop {
		t.Error("Wrap did not return the same loop")
	}
}

func TestFlattenArgs(t *testing.T) {
	in := map[string]any{
		"s":     "hello",
		"bt":    true,
		"bf":    false,
		"f":     3.14,
		"n":     nil,
		"other": 42,
	}
	out := flattenArgs(in)
	if out["s"] != "hello" {
		t.Errorf("string: got %q", out["s"])
	}
	if out["bt"] != "true" {
		t.Errorf("true: got %q", out["bt"])
	}
	if out["bf"] != "false" {
		t.Errorf("false: got %q", out["bf"])
	}
	if out["f"] != "3.14" {
		t.Errorf("float: got %q", out["f"])
	}
	if out["n"] != "" {
		t.Errorf("nil: got %q", out["n"])
	}
	if out["other"] != "42" {
		t.Errorf("default: got %q", out["other"])
	}
}

func TestDescribeTool(t *testing.T) {
	if got := describeTool("Bash", map[string]string{"command": "go test"}); got != "Bash: go test" {
		t.Errorf("command: got %q", got)
	}
	if got := describeTool("Read", map[string]string{"path": "foo.go"}); got != "Read foo.go" {
		t.Errorf("path: got %q", got)
	}
	if got := describeTool("Grep", map[string]string{}); got != "Grep" {
		t.Errorf("none: got %q", got)
	}
}

func TestBoolStr(t *testing.T) {
	if boolStr(true) != "true" {
		t.Error("true")
	}
	if boolStr(false) != "false" {
		t.Error("false")
	}
}
