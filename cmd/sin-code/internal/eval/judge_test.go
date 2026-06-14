// SPDX-License-Identifier: MIT
// Purpose: LLM-as-a-Judge tests + metrics tests (issue #75).
package eval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

func TestNewJudge_Validates(t *testing.T) {
	cases := []struct {
		name   string
		cfg    JudgeConfig
		client *llm.Client
	}{
		{"nil client", JudgeConfig{Model: "gpt-4o"}, nil},
		{"missing model", JudgeConfig{}, llm.NewClient("http://x", "k")},
		{"min out of range", JudgeConfig{Model: "gpt-4o", MinPassScore: 1.5}, llm.NewClient("http://x", "k")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j, err := NewJudge(c.cfg, c.client)
			if err == nil {
				t.Fatalf("%s: expected error, got %+v", c.name, j)
			}
		})
	}
}

func TestNewJudge_DefaultsApplied(t *testing.T) {
	j, err := NewJudge(JudgeConfig{Model: "gpt-4o"}, llm.NewClient("http://x", "k"))
	if err != nil {
		t.Fatal(err)
	}
	if j.cfg.MinPassScore != MinPassScore {
		t.Fatalf("MinPassScore default: got %v, want %v", j.cfg.MinPassScore, MinPassScore)
	}
	if j.cfg.Temperature == 0 {
		t.Fatal("Temperature default not applied")
	}
	if j.cfg.MaxTokens == 0 {
		t.Fatal("MaxTokens default not applied")
	}
}

// chatWire mirrors llm.ChatResponse — local copy keeps the test
// type-stable without importing the upstream struct directly so a
// hidden upstream rename breaks a SINGLE test.
type chatWire struct {
	ID      string `json:"id"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// serveChat wires an httptest server that responds with the given
// inner Chat Result content as the assistant message. Used by all
// Evaluate tests below so they share one envelope builder.
func serveChat(t *testing.T, content string) *httptest.Server {
	t.Helper()
	w := chatWire{ID: "x"}
	w.Choices = append(w.Choices, struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}{})
	w.Choices[0].Index = 0
	w.Choices[0].Message.Role = "assistant"
	w.Choices[0].Message.Content = content
	w.Choices[0].FinishReason = "stop"
	body, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestEvaluate_HappyPath: judge parses a canonical JSON result and
// sets Pass via local threshold so a model that ignores the prompt
// gets overridden.
func TestEvaluate_HappyPath(t *testing.T) {
	srv := serveChat(t, `{"pass":false,"score":0.4,"reason":"meh","feedback":"weak","criteria":{"goal":0.4}}`)
	j, err := NewJudge(JudgeConfig{Model: "gpt-test", MinPassScore: 0.7}, llm.NewClient(srv.URL, "k"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := j.Evaluate(context.Background(), Trajectory{
		Prompt: "fix bug", FinalOutput: "fixed", VerifyPassed: true, Turns: 1,
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected pass=false (score=0.4 < 0.7)")
	}
	if res.Score != 0.4 {
		t.Fatalf("Score: got %v, want 0.4", res.Score)
	}
	if res.Criteria["goal"] != 0.4 {
		t.Fatalf("Criteria[goal]: got %v, want 0.4", res.Criteria["goal"])
	}
}

// TestEvaluate_StripsJSONFence: an LLM that wraps its JSON in
// markdown code fences (` ```json ... ``` `) still parses cleanly.
func TestEvaluate_StripsJSONFence(t *testing.T) {
	content := "```json\n" + `{"pass":true,"score":0.95,"reason":"ok"}` + "\n```"
	srv := serveChat(t, content)
	j, err := NewJudge(JudgeConfig{Model: "gpt-test"}, llm.NewClient(srv.URL, "k"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := j.Evaluate(context.Background(), Trajectory{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pass || res.Score != 0.95 {
		t.Fatalf("want pass=true score=0.95, got %+v", res)
	}
}

// TestEvaluate_RejectsMalformedServer: a properly-shaped OpenAI
// envelope with garbage inner content surfaces a parse error and
// does not panic.
func TestEvaluate_RejectsMalformedServer(t *testing.T) {
	srv := serveChat(t, "not json at all")
	j, err := NewJudge(JudgeConfig{Model: "gpt-test"}, llm.NewClient(srv.URL, "k"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = j.Evaluate(context.Background(), Trajectory{})
	if err == nil {
		t.Fatal("expected error parsing bad response")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse-error mention, got %v", err)
	}
}

// TestEvaluate_ServerError: HTTP 500 surfaces an upstream error.
func TestEvaluate_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"kaboom"}`))
	}))
	defer srv.Close()
	j, _ := NewJudge(JudgeConfig{Model: "gpt-test"}, llm.NewClient(srv.URL, "k"))
	_, err := j.Evaluate(context.Background(), Trajectory{})
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 in error, got %v", err)
	}
}

// TestEvaluateBatch_PropagatesError: batch short-circuits so partial
// results are never returned to CI.
func TestEvaluateBatch_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	j, _ := NewJudge(JudgeConfig{Model: "gpt-test"}, llm.NewClient(srv.URL, "k"))
	_, err := j.EvaluateBatch(context.Background(), []Trajectory{{}, {}})
	if err == nil {
		t.Fatal("expected batch error")
	}
}

// TestSummarise_Aggregates: pass/fail/mean/timed-out counted correctly.
func TestSummarise_Aggregates(t *testing.T) {
	rs := []dataset.RunResult{
		{TestCaseID: "a", Success: true, Turns: 1, Duration: 10 * time.Millisecond, JudgeScore: 0.9},
		{TestCaseID: "b", Success: false, Turns: 3, Duration: 30 * time.Millisecond, Error: "boom"},
		{TestCaseID: "c", Success: true, Turns: 2, Duration: 20 * time.Millisecond, JudgeScore: 0.8, TimedOut: true},
	}
	s := Summarise(rs, 0.5)
	if s.Total != 3 || s.Passed != 2 || s.Failed != 1 {
		t.Fatalf("counts: %+v", s)
	}
	if s.PassRate != 2.0/3.0 {
		t.Errorf("pass rate: got %v", s.PassRate)
	}
	if s.Timeouts != 1 {
		t.Errorf("timeouts: got %d, want 1", s.Timeouts)
	}
	if !floatNear(s.MeanJudge, 0.85, 1e-9) {
		t.Errorf("mean judge: got %v, want 0.85", s.MeanJudge)
	}
	if s.MeanTurns != 2.0 {
		t.Errorf("mean turns: got %v", s.MeanTurns)
	}
	if len(s.Failures) != 1 || !strings.Contains(s.Failures[0], "boom") {
		t.Errorf("failures: %+v", s.Failures)
	}
}

// TestPassRateFloor: CI threshold gate. Empty summary is always
// accepted (avoids failing on a misconfigured CI step).
func TestPassRateFloor(t *testing.T) {
	if err := PassRateFloor(Summary{Total: 10, PassRate: 0.95, MinRequired: 0.9}); err != nil {
		t.Fatalf("expected nil error above threshold, got %v", err)
	}
	if err := PassRateFloor(Summary{Total: 10, PassRate: 0.8, MinRequired: 0.9}); err == nil {
		t.Fatal("expected error below threshold")
	} else {
		var bmr *BelowMinRate
		if !errors.As(err, &bmr) {
			t.Fatalf("expected *BelowMinRate, got %T", err)
		}
	}
	if err := PassRateFloor(Summary{}); err != nil {
		t.Fatalf("empty summary should never fail: %v", err)
	}
}

// floatNear reports whether two values are within tol of each other.
func floatNear(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}
