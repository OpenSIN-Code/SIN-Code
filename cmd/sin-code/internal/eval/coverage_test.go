// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for eval package to reach 100% statement coverage.
package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

func TestEvaluate_NilContext(t *testing.T) {
	j, _ := NewJudge(JudgeConfig{Model: "gpt-test"}, llm.NewClient("http://x", "k"))
	if _, err := j.Evaluate(context.Background(), Trajectory{}); err == nil {
		// Need a server to hit nil ctx, but with a real client we hit Chat error before nil ctx.
		// Force nil ctx by calling directly.
	}
	_, err := j.Evaluate(nil, Trajectory{})
	if err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("expected nil context error, got %v", err)
	}
}

func TestEvaluate_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatWire{ID: "x"})
	}))
	defer srv.Close()
	j, _ := NewJudge(JudgeConfig{Model: "gpt-test"}, llm.NewClient(srv.URL, "k"))
	_, err := j.Evaluate(context.Background(), Trajectory{})
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("expected no choices error, got %v", err)
	}
}

func TestEvaluateBatch_Success(t *testing.T) {
	content := `{"pass":true,"score":0.8,"reason":"ok"}`
	srv := serveChat(t, content)
	j, _ := NewJudge(JudgeConfig{Model: "gpt-test"}, llm.NewClient(srv.URL, "k"))
	res, err := j.EvaluateBatch(context.Background(), []Trajectory{{Prompt: "a"}, {Prompt: "b"}})
	if err != nil {
		t.Fatalf("EvaluateBatch: %v", err)
	}
	if len(res) != 2 || !res[0].Pass || !res[1].Pass {
		t.Fatalf("expected 2 passing results, got %+v", res)
	}
}

func TestBuildSystemPrompt_Strict(t *testing.T) {
	j, _ := NewJudge(JudgeConfig{Model: "gpt-test", Strict: true}, llm.NewClient("http://x", "k"))
	prompt := j.buildSystemPrompt(Trajectory{})
	if !strings.Contains(prompt, "STRICT MODE") {
		t.Fatal("strict prompt missing STRICT MODE")
	}
}

func TestBuildUserPrompt_WithToolsAndCriteria(t *testing.T) {
	j, _ := NewJudge(JudgeConfig{Model: "gpt-test"}, llm.NewClient("http://x", "k"))
	traj := Trajectory{
		Prompt:         "fix it",
		ToolsUsed:      []string{"sin_read", "sin_write"},
		Turns:          3,
		Duration:       "1s",
		VerifyPassed:   true,
		FinalOutput:    "done",
		CustomCriteria: "must not break API",
	}
	prompt := j.buildUserPrompt(traj)
	for _, want := range []string{"sin_read, sin_write", "Turns: 3", "Verify passed: true", "must not break API"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestTruncate_Truncates(t *testing.T) {
	got := truncate("hello world", 5)
	if got != "hello…(truncated)" {
		t.Fatalf("got %q", got)
	}
}

func TestSummarise_Empty(t *testing.T) {
	s := Summarise(nil, 0.5)
	if s.Total != 0 || s.PassRate != 1.0 {
		t.Fatalf("got %+v", s)
	}
}

func TestSummarise_FailureWithoutID(t *testing.T) {
	rs := []dataset.RunResult{{Success: false, Error: "boom"}}
	s := Summarise(rs, 0.5)
	if s.Failed != 1 || len(s.Failures) != 0 {
		t.Fatalf("expected failure without ID not in list, got %+v", s)
	}
}

func TestSummarise_FailureWithIDNoError(t *testing.T) {
	rs := []dataset.RunResult{{TestCaseID: "case-x", Success: false}}
	s := Summarise(rs, 0.5)
	if len(s.Failures) != 1 || s.Failures[0] != "case-x" {
		t.Fatalf("expected failure label only, got %+v", s.Failures)
	}
}

func TestNewReport(t *testing.T) {
	ds := &dataset.Dataset{Name: "ds1", Version: "1.0"}
	start := time.Now()
	end := start.Add(time.Second)
	r := NewReport(ds, "p", 0.8, []dataset.RunResult{}, start, end)
	if r.Dataset != "ds1" || r.Version != "1.0" || r.Profile != "p" || r.MinRate != 0.8 {
		t.Fatalf("got %+v", r)
	}
	if !r.Started.Equal(start) || !r.Finished.Equal(end) {
		t.Fatal("timestamps wrong")
	}
}

func TestWriteJSON_Nil(t *testing.T) {
	if err := WriteJSON(&bytes.Buffer{}, nil); err == nil || !strings.Contains(err.Error(), "nil report") {
		t.Fatalf("expected nil report error, got %v", err)
	}
}

func TestWriteJSON_Valid(t *testing.T) {
	r := NewReport(&dataset.Dataset{Name: "x", Version: "1"}, "p", 1, []dataset.RunResult{}, time.Now(), time.Now())
	var buf bytes.Buffer
	if err := WriteJSON(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\"dataset\": \"x\"") {
		t.Fatalf("unexpected JSON: %s", buf.String())
	}
}

func TestBelowMinRate_Error(t *testing.T) {
	err := &BelowMinRate{PassRate: 0.5, Minimum: 0.9}
	if !strings.Contains(err.Error(), "50.00%") || !strings.Contains(err.Error(), "90.00%") {
		t.Fatalf("got %q", err.Error())
	}
}

func TestFormatHuman_AllBranches(t *testing.T) {
	s := Summary{
		Total: 10, Passed: 7, Failed: 3,
		PassRate: 0.7, MinRequired: 0.9,
		Timeouts: 1, MeanJudge: 0.85,
		Failures: []string{"a: boom", "b"},
	}
	out := FormatHuman(s)
	for _, want := range []string{"Total: 10", "Pass Rate: 70.00%", "Timeouts: 1", "Mean judge score: 0.85", "Failed cases:", "a: boom"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRoundPassRate(t *testing.T) {
	if got := RoundPassRate(2.0 / 3.0); got != 0.6667 {
		t.Fatalf("got %v", got)
	}
}

func TestPassRateFloor_ExactlyAtThreshold(t *testing.T) {
	if err := PassRateFloor(Summary{Total: 10, PassRate: 0.9, MinRequired: 0.9}); err != nil {
		t.Fatalf("exact threshold should pass: %v", err)
	}
}

func TestSummarise_NoDuration(t *testing.T) {
	rs := []dataset.RunResult{{Success: true, Duration: 0}}
	s := Summarise(rs, 0.5)
	if s.MeanDurMS != 0 {
		t.Fatalf("MeanDurMS should be 0 when duration is 0, got %v", s.MeanDurMS)
	}
}
