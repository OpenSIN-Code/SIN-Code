// SPDX-License-Identifier: MIT
// Purpose: dataset parser + validator tests (issue #75).
package dataset

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "ds.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const goodJSON = `{
  "name": "Critical",
  "version": "1.0.0",
  "test_cases": [
    {"id": "a", "prompt": "first"},
    {"id": "b", "prompt": "second"}
  ]
}`

func TestLoadDataset_Good(t *testing.T) {
	path := writeTemp(t, goodJSON)
	ds, err := LoadDataset(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ds.Name != "Critical" || ds.Version != "1.0.0" || len(ds.TestCases) != 2 {
		t.Fatalf("unexpected dataset: %+v", ds)
	}
}

func TestLoadDataset_MissingFile(t *testing.T) {
	_, err := LoadDataset("/no/such/file.json")
	if err == nil {
		t.Fatal("expected error on missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Fatalf("expected 'read' in error, got %v", err)
	}
}

func TestDatasetValidate_Errors(t *testing.T) {
	cases := []struct {
		name string
		ds   Dataset
		want string
	}{
		{
			name: "missing name",
			ds:   Dataset{Version: "1.0", TestCases: []TestCase{{ID: "x", Prompt: "y"}}},
			want: "name is required",
		},
		{
			name: "missing version",
			ds:   Dataset{Name: "x", TestCases: []TestCase{{ID: "x", Prompt: "y"}}},
			want: "version is required",
		},
		{
			name: "no test cases",
			ds:   Dataset{Name: "x", Version: "1.0"},
			want: "empty",
		},
		{
			name: "missing prompt",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a"}, // prompt required
			}},
			want: "prompt is required",
		},
		{
			name: "duplicate id",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a", Prompt: "1"},
				{ID: "a", Prompt: "2"},
			}},
			want: "duplicate id",
		},
		{
			name: "require_verify without cmd",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a", Prompt: "p", Constraints: Constraints{RequireVerify: true}},
			}},
			want: "verify_cmd",
		},
		{
			name: "min_quality out of range",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a", Prompt: "p", Expected: Expected{MinQuality: 1.5}},
			}},
			want: "min_quality",
		},
		{
			name: "bad timeout",
			ds: Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{
				{ID: "a", Prompt: "p", Constraints: Constraints{Timeout: "lol"}},
			}},
			want: "timeout",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.ds.Validate()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q did not contain %q", err.Error(), c.want)
			}
		})
	}
}

func TestFilterByTag(t *testing.T) {
	ds := &Dataset{
		Name: "x", Version: "1.0",
		TestCases: []TestCase{
			{ID: "a", Prompt: "1", Tags: []string{"smoke", "critical"}},
			{ID: "b", Prompt: "2", Tags: []string{"regress"}},
			{ID: "c", Prompt: "3", Tags: []string{"smoke"}},
		},
	}
	got := ds.FilterByTag("smoke")
	if len(got.TestCases) != 2 {
		t.Fatalf("smoke: got %d, want 2", len(got.TestCases))
	}
	got = ds.FilterByTag("UNKNOWN")
	if len(got.TestCases) != 0 {
		t.Fatalf("unknown tag should yield zero cases, got %d", len(got.TestCases))
	}
	got = ds.FilterByTag("")
	if len(got.TestCases) != 3 {
		t.Fatalf("empty filter should yield all 3, got %d", len(got.TestCases))
	}
}

func TestListDatasets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(goodJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.json"), []byte(goodJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := ListDatasets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 datasets, got %d (%v)", len(files), files)
	}
}

func TestListDatasets_MissingDir(t *testing.T) {
	_, err := ListDatasets("/no/such/dir/at/all")
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestLoadDataset_EmptyPath(t *testing.T) {
	_, err := LoadDataset("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestLoadDataset_ParseError(t *testing.T) {
	p := writeTemp(t, "not json")
	_, err := LoadDataset(p)
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestLoadDataset_ValidateError(t *testing.T) {
	p := writeTemp(t, `{"version":"1.0","test_cases":[{"id":"x","prompt":"y"}]}`)
	_, err := LoadDataset(p)
	if err == nil || !strings.Contains(err.Error(), "validate") {
		t.Fatalf("expected validate error, got %v", err)
	}
}

func TestLoadDataset_AbsError(t *testing.T) {
	orig := filepathAbs
	filepathAbs = func(path string) (string, error) { return "", errors.New("boom") }
	defer func() { filepathAbs = orig }()
	_, err := LoadDataset("x.json")
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestListDatasets_EmptyDirDefaultsToEvals(t *testing.T) {
	root := t.TempDir()
	evals := filepath.Join(root, "evals")
	if err := os.MkdirAll(evals, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evals, "a.json"), []byte(goodJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	files, err := ListDatasets("")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %v", files)
	}
}

func TestListDatasets_AbsError(t *testing.T) {
	orig := filepathAbs
	filepathAbs = func(path string) (string, error) { return "", errors.New("boom") }
	defer func() { filepathAbs = orig }()
	_, err := ListDatasets("evals")
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestListDatasets_NotDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.json")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ListDatasets(p)
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not-directory error, got %v", err)
	}
}

func TestListDatasets_WalkError(t *testing.T) {
	orig := filepathWalkDir
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return errors.New("walk failed")
	}
	defer func() { filepathWalkDir = orig }()
	_, err := ListDatasets(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "walk") {
		t.Fatalf("expected walk error, got %v", err)
	}
}

func TestListDatasets_WalkCallbackError(t *testing.T) {
	orig := filepathWalkDir
	filepathWalkDir = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("callback err"))
	}
	defer func() { filepathWalkDir = orig }()
	_, err := ListDatasets(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "callback err") {
		t.Fatalf("expected callback error, got %v", err)
	}
}

func TestDatasetValidate_MissingID(t *testing.T) {
	ds := Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{{Prompt: "p"}}}
	err := ds.Validate()
	if err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Fatalf("expected id error, got %v", err)
	}
}

func TestDatasetValidate_NegativeMaxTurns(t *testing.T) {
	ds := Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{{ID: "a", Prompt: "p", Constraints: Constraints{MaxTurns: -1}}}}
	err := ds.Validate()
	if err == nil || !strings.Contains(err.Error(), "max_turns") {
		t.Fatalf("expected max_turns error, got %v", err)
	}
}

func TestDatasetValidate_NegativeMaxTokens(t *testing.T) {
	ds := Dataset{Name: "x", Version: "1.0", TestCases: []TestCase{{ID: "a", Prompt: "p", Constraints: Constraints{MaxTokens: -1}}}}
	err := ds.Validate()
	if err == nil || !strings.Contains(err.Error(), "max_tokens") {
		t.Fatalf("expected max_tokens error, got %v", err)
	}
}

func inMemoryStore(t *testing.T) *session.Store {
	s, err := session.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func loopingRunner(t *testing.T, override func(context.Context, *session.Session, string) (*agentloop.Result, error)) *Runner {
	loop := &agentloop.Loop{RunOverride: override}
	r, err := NewRunner(RunnerConfig{}, loop, inMemoryStore(t))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewRunner_NilLoop(t *testing.T) {
	_, err := NewRunner(RunnerConfig{}, nil, inMemoryStore(t))
	if err == nil || !strings.Contains(err.Error(), "loop is nil") {
		t.Fatalf("expected nil loop error, got %v", err)
	}
}

func TestNewRunner_NilStore(t *testing.T) {
	loop := &agentloop.Loop{RunOverride: func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{}, nil
	}}
	_, err := NewRunner(RunnerConfig{}, loop, nil)
	if err == nil || !strings.Contains(err.Error(), "session store is nil") {
		t.Fatalf("expected nil store error, got %v", err)
	}
}

func TestNewRunner_Defaults(t *testing.T) {
	loop := &agentloop.Loop{RunOverride: func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{}, nil
	}}
	r, err := NewRunner(RunnerConfig{}, loop, inMemoryStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.cfg.MaxConcurrency != 1 || r.cfg.TimeoutPerCase != 5*time.Minute || r.cfg.HeadlessMode != true {
		t.Fatalf("unexpected defaults: %+v", r.cfg)
	}
	if r.cfg.VerifyMode != string(verify.ModePoC) {
		t.Fatalf("expected verify mode %q, got %q", verify.ModePoC, r.cfg.VerifyMode)
	}
}

func TestRunDataset_Nil(t *testing.T) {
	r := loopingRunner(t, func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{}, nil
	})
	_, err := r.RunDataset(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "nil dataset") {
		t.Fatalf("expected nil dataset error, got %v", err)
	}
}

func TestRunDataset_Happy(t *testing.T) {
	r := loopingRunner(t, func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{Summary: "done", Verified: true, Turns: 3}, nil
	})
	ds := &Dataset{Name: "x", Version: "1", TestCases: []TestCase{{ID: "a", Prompt: "p"}}}
	res, err := r.RunDataset(context.Background(), ds)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || !res[0].Success || !res[0].VerifyPassed {
		t.Fatalf("unexpected result: %+v", res[0])
	}
}

func TestRunCase_ErrorNoResult(t *testing.T) {
	r := loopingRunner(t, func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return nil, errors.New("boom")
	})
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p"})
	if res.Error == "" || res.Success {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunCase_ErrorWithResult(t *testing.T) {
	r := loopingRunner(t, func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{Summary: "partial", Verified: true, Turns: 2}, errors.New("boom")
	})
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p"})
	if res.Error != "boom" || res.Turns != 2 || !res.VerifyPassed || res.FinalOutput != "partial" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunCase_Timeout(t *testing.T) {
	r := loopingRunner(t, func(ctx context.Context, s *session.Session, p string) (*agentloop.Result, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	// cfg.TimeoutPerCase defaults to 5m; override to 1ns.
	r.cfg.TimeoutPerCase = 1 * time.Nanosecond
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p"})
	if !res.TimedOut {
		t.Fatalf("expected timeout, got %+v", res)
	}
}

func TestRunCase_SessionError(t *testing.T) {
	loop := &agentloop.Loop{RunOverride: func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{}, nil
	}}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	r, err := NewRunner(RunnerConfig{}, loop, store)
	if err != nil {
		t.Fatal(err)
	}
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p"})
	if res.Error == "" || res.SessionID != "" {
		t.Fatalf("expected session error, got %+v", res)
	}
}

func TestRunCase_ValidConstraintTimeout(t *testing.T) {
	r := loopingRunner(t, func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{Summary: "ok", Verified: true}, nil
	})
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p", Constraints: Constraints{Timeout: "50ms"}})
	if !res.Success {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunCase_InvalidAndZeroTimeouts(t *testing.T) {
	loop := &agentloop.Loop{RunOverride: func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{Summary: "ok", Verified: true}, nil
	}}
	store := inMemoryStore(t)
	// Negative TimeoutPerCase is preserved by NewRunner, letting the last-resort 30s cap run.
	r, err := NewRunner(RunnerConfig{TimeoutPerCase: -1 * time.Second}, loop, store)
	if err != nil {
		t.Fatal(err)
	}
	res := r.RunCase(context.Background(), &TestCase{ID: "a", Prompt: "p", Constraints: Constraints{Timeout: "not-a-duration"}})
	if !res.Success {
		t.Fatalf("unexpected result: %+v", res)
	}
	// Zero/negative constraint duration also hits the fallback cap.
	res2 := r.RunCase(context.Background(), &TestCase{ID: "b", Prompt: "p", Constraints: Constraints{Timeout: "0s"}})
	if !res2.Success {
		t.Fatalf("unexpected result: %+v", res2)
	}
}

func TestApplyRules(t *testing.T) {
	r := loopingRunner(t, nil)
	tc := &TestCase{ID: "a", Prompt: "p"}
	res := RunResult{Success: true}
	out := r.applyRules(tc, &res)
	if !out.Success {
		t.Fatalf("empty rules should keep success: %+v", out)
	}

	tc2 := &TestCase{ID: "b", Prompt: "p", Constraints: Constraints{MaxTurns: 2}}
	out2 := r.applyRules(tc2, &RunResult{Success: true, Turns: 3})
	if out2.Success || !strings.Contains(out2.Error, "turns=3 > max_turns=2") {
		t.Fatalf("max_turns violation not recorded: %+v", out2)
	}

	tc3 := &TestCase{ID: "c", Prompt: "p", Constraints: Constraints{RequireVerify: true}}
	out3 := r.applyRules(tc3, &RunResult{Success: true, VerifyPassed: false})
	if out3.Success || !strings.Contains(out3.Error, "verify not passed") {
		t.Fatalf("require_verify violation not recorded: %+v", out3)
	}

	tc4 := &TestCase{ID: "d", Prompt: "p", Constraints: Constraints{MustUseTools: []string{"Read"}}}
	out4 := r.applyRules(tc4, &RunResult{Success: true, ToolsUsed: []string{"Write"}})
	if out4.Success || !strings.Contains(out4.Error, "missing required tool: Read") {
		t.Fatalf("must_use violation not recorded: %+v", out4)
	}

	tc5 := &TestCase{ID: "e", Prompt: "p", Constraints: Constraints{ForbiddenTools: []string{"Write"}}}
	out5 := r.applyRules(tc5, &RunResult{Success: true, ToolsUsed: []string{"Write"}})
	if out5.Success || !strings.Contains(out5.Error, "used forbidden tool: Write") {
		t.Fatalf("forbidden tool violation not recorded: %+v", out5)
	}

	tc6 := &TestCase{ID: "f", Prompt: "p", Expected: Expected{OutputContains: []string{"hello"}}}
	out6 := r.applyRules(tc6, &RunResult{Success: true, FinalOutput: "goodbye"})
	if out6.Success || !strings.Contains(out6.Error, "missing output keyword: hello") {
		t.Fatalf("output_contains violation not recorded: %+v", out6)
	}

	tc7 := &TestCase{ID: "g", Prompt: "p", Expected: Expected{OutputAvoids: []string{"bad"}}}
	out7 := r.applyRules(tc7, &RunResult{Success: true, FinalOutput: "bad idea"})
	if out7.Success || !strings.Contains(out7.Error, "contains forbidden output keyword: bad") {
		t.Fatalf("output_avoids violation not recorded: %+v", out7)
	}

	// Error already set: applyRules should not overwrite it.
	tc8 := &TestCase{ID: "h", Prompt: "p", Constraints: Constraints{MaxTurns: 1}}
	res8 := RunResult{Success: true, Turns: 2, Error: "existing"}
	out8 := r.applyRules(tc8, &res8)
	if out8.Success || out8.Error != "existing" {
		t.Fatalf("expected error not overwritten, got %+v", out8)
	}
}

func TestContains(t *testing.T) {
	if contains([]string{"a", "b"}, "a") != true {
		t.Fatal("expected contains a")
	}
	if contains([]string{"a", "b"}, "c") != false {
		t.Fatal("expected not contains c")
	}
}

// Runner wires dataset constraints into the loop's live coverage enforcer
// so the agent loop rejects completion instead of the runner doing it
// post-hoc (issue #248).
func TestRunCase_WiresCoverageConstraints(t *testing.T) {
	turns := 0
	loop := &agentloop.Loop{
		Gate: verify.NewGate("poc",
			func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
			nil),
		Workspace: "/tmp",
		LocalTool: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "out", nil
		},
		Completion: func(ctx context.Context, msgs []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
			turns++
			if turns == 1 {
				return &agentloop.Completion{
					Text: "",
					ToolCalls: []agentloop.ToolCall{
						{ID: "t1", Name: "sin_poc", Args: map[string]any{}},
					},
					Raw: session.Message{Role: "assistant", Content: ""},
				}, nil
			}
			return &agentloop.Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	r, err := NewRunner(RunnerConfig{}, loop, inMemoryStore(t))
	if err != nil {
		t.Fatal(err)
	}
	tc := &TestCase{
		ID: "a", Prompt: "p",
		Constraints: Constraints{MustUseTools: []string{"sin_poc"}, ForbiddenTools: []string{"sin_bash"}},
	}
	res := r.RunCase(context.Background(), tc)
	if !res.Success || !res.VerifyPassed {
		t.Fatalf("expected success with live coverage, got %+v", res)
	}
	if len(res.ToolsUsed) != 1 || res.ToolsUsed[0] != "sin_poc" {
		t.Fatalf("expected ToolsUsed populated from loop coverage, got %v", res.ToolsUsed)
	}
	if loop.Coverage == nil {
		t.Fatal("expected loop.Coverage to be created")
	}
}
