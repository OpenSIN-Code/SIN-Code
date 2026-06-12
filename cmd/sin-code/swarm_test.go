// SPDX-License-Identifier: MIT
// Purpose: tests for the swarm meta-command (issue #51).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// TestSwarm_RequiresAtLeastTwoAgents pins the CLI's hard contract: a
// single agent or empty list is a user error.
func TestSwarm_RequiresAtLeastTwoAgents(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		csv  string
	}{
		{"empty", ""},
		{"single", "coder"},
		{"single_with_spaces", " coder "},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := executeSwarm(context.Background(), &swarmOptions{
				prompt:   "x",
				agentCSV: c.csv,
			})
			if err == nil {
				t.Fatalf("expected error for csv=%q, got nil", c.csv)
			}
			if !strings.Contains(err.Error(), "at least 2 profiles") {
				t.Errorf("unexpected error message: %v", err)
			}
		})
	}
}

func TestSwarm_RequiresPrompt(t *testing.T) {
	t.Parallel()
	_, err := executeSwarm(context.Background(), &swarmOptions{
		agentCSV: "a,b",
	})
	if err == nil {
		t.Fatal("expected error when --prompt missing")
	}
	if !strings.Contains(err.Error(), "--prompt") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSplitNonEmpty(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, sep string
		want    []string
	}{
		{"", ",", nil},
		{"a", ",", []string{"a"}},
		{"a,b,c", ",", []string{"a", "b", "c"}},
		{"  a , b ,, c ", ",", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := splitNonEmpty(c.in, c.sep)
		if !equalStrings(got, c.want) {
			t.Errorf("splitNonEmpty(%q,%q) = %v, want %v", c.in, c.sep, got, c.want)
		}
	}
}

func TestClassifyErr(t *testing.T) {
	t.Parallel()
	if got := classifyErr(nil); got != "FAILED" {
		t.Errorf("nil -> %q, want FAILED", got)
	}
	if got := classifyErr(context.Canceled); got != "CANCELLED" {
		t.Errorf("Canceled -> %q, want CANCELLED", got)
	}
	if got := classifyErr(context.DeadlineExceeded); got != "TIMEOUT" {
		t.Errorf("DeadlineExceeded -> %q, want TIMEOUT", got)
	}
	if got := classifyErr(errors.New("boom")); got != "FAILED" {
		t.Errorf("generic -> %q, want FAILED", got)
	}
}

func TestCancelledMarkers(t *testing.T) {
	t.Parallel()
	extra := cancelledMarkers([]string{"a", "b", "c"}, []swarmResult{
		{Agent: "a", Status: "CANCELLED"},
	})
	if len(extra) != 2 || extra[0].Agent != "b" || extra[1].Agent != "c" {
		t.Errorf("unexpected markers: %+v", extra)
	}
	if extra[0].Status != "CANCELLED" {
		t.Errorf("expected CANCELLED status, got %q", extra[0].Status)
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short: %q", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate long: %q", got)
	}
}

func TestSanitizeFile(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"coder", "coder"},
		{"a-b_c", "a-b_c"},
		{"../escape/..", "___escape___"},
		{"   ", "agent"},
		{"", "agent"},
	}
	for _, c := range cases {
		if got := sanitizeFile(c.in); got != c.want {
			t.Errorf("sanitizeFile(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRandHex(t *testing.T) {
	t.Parallel()
	a := randHex(4)
	b := randHex(4)
	if a == "" || a == b {
		t.Errorf("expected distinct non-empty hex strings, got %q %q", a, b)
	}
	if len(a) != 8 {
		t.Errorf("expected 8-char hex, got %q", a)
	}
}

func TestPerAgentDBPath(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	p, err := perAgentDBPath(ws, "coder")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p, ws) {
		t.Errorf("path %q not under workspace %q", p, ws)
	}
	if !strings.HasSuffix(p, ".db") {
		t.Errorf("expected .db suffix, got %q", p)
	}
	if !strings.Contains(p, "swarm") {
		t.Errorf("expected swarm dir, got %q", p)
	}
	if _, err := os.Stat(filepath.Dir(p)); err != nil {
		t.Errorf("swarm dir should be created: %v", err)
	}
}

// TestSwarm_HeadlessAndNoYolo ensures the swarm never exposes Yolo or
// Ask (mandate M4 + swarm hard mandate). The runner installs "wrong"
// values; the swarm must override them post-Build.
func TestSwarm_HeadlessAndNoYolo(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	var captured atomic.Pointer[agentloop.Loop]
	runner := func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
		gate := verify.NewGate("off", nil, nil)
		perm := permission.New([]permission.Rule{{Tool: "*", Policy: "allow"}})
		perm.Headless = false // intentionally wrong; swarm must override
		perm.Yolo = true      // intentionally wrong; swarm must override
		ask := func(tc agentloop.ToolCall) bool { return true }
		// Use a Completion that captures the post-swarm-overrides.
		// The override is applied right before Loop.Run, which is
		// called inside the goroutine. We capture the loop pointer
		// and inspect it after executeSwarm returns.
		var loop *agentloop.Loop
		loop = &agentloop.Loop{
			Gate:      gate,
			Perm:      perm,
			Ask:       ask,
			LocalSpec: nil,
			Completion: func(_ context.Context, _ []session.Message, _ []agentloop.ToolSpec) (*agentloop.Completion, error) {
				captured.Store(loop)
				return &agentloop.Completion{
					Text: "done",
					Raw:  session.Message{Role: "assistant", Content: "done"},
				}, nil
			},
			Hooks:    hooks.New(nil),
			MaxTurns: 5,
		}
		store, err := session.Open(workspace + "/" + agentName + ".db")
		if err != nil {
			return nil, nil, nil, err
		}
		sess, err := store.StartOrResume("")
		if err != nil {
			_ = store.Close()
			return nil, nil, nil, err
		}
		return loop, sess, func() error { return store.Close() }, nil
	}

	_, err := executeSwarm(context.Background(), &swarmOptions{
		prompt:    "x",
		agentCSV:  "alpha,beta",
		workspace: ws,
		timeout:   3 * time.Second,
		maxTurns:  5,
		runner:    runner,
	})
	if err != nil {
		t.Fatalf("executeSwarm: %v", err)
	}
	got := captured.Load()
	if got == nil {
		t.Fatal("Completion never invoked; cannot verify overrides")
	}
	if !got.Perm.Headless {
		t.Error("Headless override not applied to loop.Perm")
	}
	if got.Perm.Yolo {
		t.Error("Yolo override not applied to loop.Perm (must be false in swarm)")
	}
	if got.Ask != nil {
		t.Error("Ask override not applied (must be nil in swarm)")
	}
}

// TestSwarm_CancellationOnFirstVerified exercises the core "first
// verified result wins" semantics with a mock Completion.
func TestSwarm_CancellationOnFirstVerified(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()

	makeRunner := func(verified bool, summary string, blocks bool) agentRunner {
		return func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
			loop := &agentloop.Loop{
				Gate: verify.NewGate("off", nil, nil),
				Perm: permission.New([]permission.Rule{{Tool: "*", Policy: "deny"}}),
				Completion: func(ctx context.Context, _ []session.Message, _ []agentloop.ToolSpec) (*agentloop.Completion, error) {
					if blocks {
						<-ctx.Done()
						return nil, ctx.Err()
					}
					return &agentloop.Completion{
						Text: summary,
						Raw:  session.Message{Role: "assistant", Content: summary},
					}, nil
				},
				MaxTurns: 200,
			}
			store, err := session.Open(workspace + "/" + agentName + ".db")
			if err != nil {
				return nil, nil, nil, err
			}
			sess, err := store.StartOrResume("")
			if err != nil {
				_ = store.Close()
				return nil, nil, nil, err
			}
			return loop, sess, func() error { return store.Close() }, nil
		}
	}

	report, err := executeSwarm(context.Background(), &swarmOptions{
		prompt:    "test prompt",
		agentCSV:  "fast,slow",
		workspace: ws,
		timeout:   5 * time.Second,
		maxTurns:  50,
		runner: func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
			if agentName == "fast" {
				return makeRunner(true, "fast done", false)(ctx, agentName, workspace)
			}
			return makeRunner(false, "slow done", true)(ctx, agentName, workspace)
		},
	})
	if err != nil {
		t.Fatalf("executeSwarm: %v", err)
	}
	if report.Winner != "fast" {
		t.Errorf("expected winner=fast, got %q (results=%+v)", report.Winner, report.Results)
	}
	if report.Error != "" {
		t.Errorf("unexpected error: %s", report.Error)
	}

	byAgent := map[string]swarmResult{}
	for _, r := range report.Results {
		byAgent[r.Agent] = r
	}
	if r := byAgent["fast"]; r.Status != "VERIFIED" {
		t.Errorf("fast status = %s, want VERIFIED", r.Status)
	}
	r, ok := byAgent["slow"]
	if !ok {
		t.Fatal("missing slow agent result")
	}
	// Slow blocks on ctx.Done; once fast wins, ctx is cancelled, so
	// slow's Completion returns ctx.Err() -> CANCELLED.
	if r.Status != "CANCELLED" {
		t.Errorf("slow status = %s, want CANCELLED", r.Status)
	}
}

// TestSwarm_NoWinnerWithinTimeout: when no agent verifies, the report
// has an empty Winner and a descriptive Error.
func TestSwarm_NoWinnerWithinTimeout(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	runner := func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
		loop := &agentloop.Loop{
			Gate: verify.NewGate("off", nil, nil),
			Perm: permission.New([]permission.Rule{{Tool: "*", Policy: "deny"}}),
			Completion: func(_ context.Context, _ []session.Message, _ []agentloop.ToolSpec) (*agentloop.Completion, error) {
				// Return unverified by setting up a gate that
				// "fails" is not possible here; gate "off" always
				// passes. To simulate no-verifier, we use a
				// Completion that returns a tool call so the loop
				// keeps spinning until ctx timeout.
				return &agentloop.Completion{
					Text:      "x",
					ToolCalls: []agentloop.ToolCall{{ID: "t", Name: "noop", Args: map[string]any{}}},
					Raw:       session.Message{Role: "assistant", Content: "x"},
				}, nil
			},
			MaxTurns: 5,
		}
		store, err := session.Open(workspace + "/" + agentName + ".db")
		if err != nil {
			return nil, nil, nil, err
		}
		sess, err := store.StartOrResume("")
		if err != nil {
			_ = store.Close()
			return nil, nil, nil, err
		}
		return loop, sess, func() error { return store.Close() }, nil
	}
	report, err := executeSwarm(context.Background(), &swarmOptions{
		prompt:    "x",
		agentCSV:  "a,b",
		workspace: ws,
		timeout:   300 * time.Millisecond,
		maxTurns:  5,
		runner:    runner,
	})
	if err != nil {
		t.Fatalf("executeSwarm: %v", err)
	}
	if report.Winner != "" {
		t.Errorf("expected no winner, got %q", report.Winner)
	}
	if report.Error == "" {
		t.Error("expected non-empty error in report")
	}
}

// TestSwarm_CobraHelp ensures the cobra command builds and exposes
// its flags.
func TestSwarm_CobraHelp(t *testing.T) {
	t.Parallel()
	cmd := NewSwarmCmd()
	if cmd.Use != "swarm" {
		t.Errorf("Use = %q, want swarm", cmd.Use)
	}
	for _, name := range []string{"agents", "timeout", "max-turns"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("expected --%s flag", name)
		}
	}
}

// TestSwarm_JSONContract sanity-checks the JSON output shape so we
// never break the stable contract from AGENTS.md §7.
func TestSwarm_JSONContract(t *testing.T) {
	t.Parallel()
	rep := swarmReport{
		Prompt: "p",
		Winner: "w",
		Results: []swarmResult{
			{Agent: "w", Status: "VERIFIED", Turns: 3, Summary: "ok"},
		},
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"prompt"`, `"winner"`, `"results"`, `"agent"`, `"status"`, `"turns"`, `"summary"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("json missing key %s in %s", key, string(data))
		}
	}
}

// TestSwarm_CobraExecuteRequiresTwoAgents runs the cobra command in
// a subprocess and checks it errors out for a single-agent invocation.
// Mandate from the issue: `sin-code swarm -p "test" --agents a,b` (with
// no real LLM backend) errors with "--agents requires at least 2".
func TestSwarm_CobraExecuteRequiresTwoAgents(t *testing.T) {
	t.Parallel()
	// We can't easily execute rootCmd with a real LLM, but the
	// argument-validation path runs first and errors out before any
	// backend call. Use the cobra command directly.
	cmd := NewSwarmCmd()
	cmd.SetArgs([]string{"-p", "test", "--agents", "single"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for single agent, got nil")
	}
	if !strings.Contains(err.Error(), "at least 2 profiles") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestClassifyCtxErr(t *testing.T) {
	t.Parallel()
	if got := classifyCtxErr(nil); got != "" {
		t.Errorf("nil -> %q, want empty", got)
	}
	if got := classifyCtxErr(context.DeadlineExceeded); got != "swarm timeout exceeded" {
		t.Errorf("DeadlineExceeded -> %q, want swarm timeout exceeded", got)
	}
	if got := classifyCtxErr(context.Canceled); got != "swarm cancelled" {
		t.Errorf("Canceled -> %q, want swarm cancelled", got)
	}
	if got := classifyCtxErr(errors.New("other")); got != "swarm cancelled" {
		t.Errorf("other -> %q, want swarm cancelled", got)
	}
}

// TestEmitSwarm_TextAndJSON covers emitSwarm in both modes. It does
// not assert on absolute formatting (which is fragile) but does
// verify the winner/error messages and the JSON contract.
func TestEmitSwarm_TextAndJSON(t *testing.T) {
	// No t.Parallel(): subtests swap os.Stdout, which is package-global.
	t.Run("text", func(t *testing.T) {
		// Capture stdout via redirect of os.Stdout.
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		orig := os.Stdout
		os.Stdout = w
		t.Cleanup(func() { os.Stdout = orig })
		opts := &swarmOptions{prompt: "p", timeout: time.Second, maxTurns: 1}
		if err := emitSwarm(opts, &swarmReport{
			Prompt: "p",
			Winner: "w",
			Results: []swarmResult{
				{Agent: "w", Status: "VERIFIED", Turns: 1, Summary: "ok"},
			},
		}); err != nil {
			t.Fatal(err)
		}
		_ = w.Close()
		data, _ := io.ReadAll(r)
		s := string(data)
		if !strings.Contains(s, "winner: w") {
			t.Errorf("missing winner line in %q", s)
		}
		if !strings.Contains(s, "VERIFIED") {
			t.Errorf("missing VERIFIED status in %q", s)
		}
	})
	t.Run("json", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		orig := os.Stdout
		os.Stdout = w
		t.Cleanup(func() { os.Stdout = orig })
		opts := &swarmOptions{prompt: "p", jsonOut: true, timeout: time.Second, maxTurns: 1}
		if err := emitSwarm(opts, &swarmReport{
			Prompt:  "p",
			Winner:  "w",
			Results: []swarmResult{{Agent: "w", Status: "VERIFIED", Turns: 1}},
		}); err != nil {
			t.Fatal(err)
		}
		_ = w.Close()
		data, _ := io.ReadAll(r)
		var rep swarmReport
		if err := json.Unmarshal(data, &rep); err != nil {
			t.Fatalf("json output invalid: %v: %s", err, string(data))
		}
		if rep.Winner != "w" {
			t.Errorf("winner = %q", rep.Winner)
		}
	})
	t.Run("error_path", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		origErr := os.Stderr
		os.Stderr = w
		t.Cleanup(func() { os.Stderr = origErr })
		opts := &swarmOptions{prompt: "p", timeout: time.Second, maxTurns: 1}
		if err := emitSwarm(opts, &swarmReport{
			Prompt: "p",
			Error:  "no agent verified",
		}); err != nil {
			t.Fatal(err)
		}
		_ = w.Close()
		data, _ := io.ReadAll(r)
		if !strings.Contains(string(data), "no agent verified") {
			t.Errorf("missing error line in %q", string(data))
		}
	})
}

// TestSinBootstrapSkill_InBuiltinSpecs pins that sin_bootstrap_skill
// is registered as a builtin tool in chat. The chat tool router
// iterates over builtinSpecs() when building the model surface, so
// any future refactor that drops the entry will fail this test.
func TestSinBootstrapSkill_InBuiltinSpecs(t *testing.T) {
	t.Parallel()
	found := false
	for _, s := range builtinSpecs() {
		if s.Name == "sin_bootstrap_skill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("sin_bootstrap_skill missing from builtinSpecs() — chat tool router will not advertise it")
	}
}
