// SPDX-License-Identifier: MIT
// Purpose: coverage tests for eval_cmd.go (issue #75).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	sinctrace "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// resetEvalHooks restores all package-level hooks to their production defaults
// so tests can override them without leaking state.
func resetEvalHooks() {
	loadDatasetFn = dataset.LoadDataset
	listDatasetsFn = dataset.ListDatasets
	initTraceProviderFn = sinctrace.InitProvider
	shutdownTraceFn = sinctrace.Shutdown
	openSessionStoreFn = session.Open
	newRunnerFn = dataset.NewRunner
	runDatasetFn = func(r *dataset.Runner, ctx context.Context, ds *dataset.Dataset) ([]dataset.RunResult, error) {
		return r.RunDataset(ctx, ds)
	}
	newReportFn = eval.NewReport
	writeJSONFn = eval.WriteJSON
	formatHumanFn = eval.FormatHuman
	newJudgeFn = eval.NewJudge
	passRateFloorFn = eval.PassRateFloor
	parseExporterFn = sinctrace.ParseExporter
	newLLMClientFn = func(endpoint, apiKey string) *llm.Client { return llm.NewClient(endpoint, apiKey) }
	filepathAbsFn = filepath.Abs
	evalStdout = os.Stdout
	evalStderr = os.Stderr
}

// fakeDataset returns a minimal valid Dataset that passes dataset.Validate().
func fakeDataset() *dataset.Dataset {
	return &dataset.Dataset{
		Name:    "test",
		Version: "1.0",
		TestCases: []dataset.TestCase{
			{ID: "tc1", Prompt: "hello"},
		},
	}
}

// evalFakeSessionStore opens a real SQLite session store in a temporary directory
// so tests can exercise Open/Close without collisions.
func evalFakeSessionStore(t *testing.T) *session.Store {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("open session store: %v", err)
	}
	return store
}

// roundTripperFunc is a test helper that implements http.RoundTripper as a func.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// mockLLMClient returns an *llm.Client whose HTTP transport always returns the
// provided body/status, so the judge path can be tested without a real LLM.
func mockLLMClient(body string, status int) *llm.Client {
	return &llm.Client{
		BaseURL: "http://test",
		APIKey:  "key",
		HTTP: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: status,
					Body:       io.NopCloser(strings.NewReader(body)),
					Header:     http.Header{"Content-Type": []string{"application/json"}},
				}, nil
			}),
		},
	}
}

// ── NewEvalCmd ───────────────────────────────────────────────────────

func TestNewEvalCmd(t *testing.T) {
	cmd := NewEvalCmd()
	if cmd.Use != "eval" {
		t.Errorf("expected Use=eval, got %q", cmd.Use)
	}
	subs := cmd.Commands()
	if len(subs) != 2 {
		t.Errorf("expected 2 subcommands, got %d", len(subs))
	}
}

// ── newEvalRunCmd ────────────────────────────────────────────────────

func TestNewEvalRunCmd_EmptyDataset(t *testing.T) {
	defer resetEvalHooks()
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "--dataset is required") {
		t.Errorf("expected empty dataset error, got %v", err)
	}
}

func TestNewEvalRunCmd_LoadDatasetError(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return nil, errors.New("boom") }
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "eval run: load dataset") {
		t.Errorf("expected load dataset error, got %v", err)
	}
}

func TestNewEvalRunCmd_OpenSessionError(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return nil, errors.New("boom") }
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "eval run: open session store") {
		t.Errorf("expected open session store error, got %v", err)
	}
}

func TestNewEvalRunCmd_NewRunnerError(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return nil, errors.New("boom")
	}
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "eval run: new runner") {
		t.Errorf("expected new runner error, got %v", err)
	}
}

func TestNewEvalRunCmd_RunDatasetError(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return &dataset.Runner{}, nil
	}
	runDatasetFn = func(*dataset.Runner, context.Context, *dataset.Dataset) ([]dataset.RunResult, error) {
		return nil, errors.New("boom")
	}
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "eval run: run dataset") {
		t.Errorf("expected run dataset error, got %v", err)
	}
}

func TestNewEvalRunCmd_JSONWriteError(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return &dataset.Runner{}, nil
	}
	runDatasetFn = func(*dataset.Runner, context.Context, *dataset.Dataset) ([]dataset.RunResult, error) {
		return []dataset.RunResult{{TestCaseID: "tc1", Success: true}}, nil
	}
	writeJSONFn = func(io.Writer, *eval.Report) error { return errors.New("boom") }
	evalStdout = &bytes.Buffer{}
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	cmd.Flags().Set("json", "true")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "eval run: write json") {
		t.Errorf("expected write json error, got %v", err)
	}
}

func TestNewEvalRunCmd_HumanOutput(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return &dataset.Runner{}, nil
	}
	runDatasetFn = func(*dataset.Runner, context.Context, *dataset.Dataset) ([]dataset.RunResult, error) {
		return []dataset.RunResult{{TestCaseID: "tc1", Success: true}}, nil
	}
	var stdout bytes.Buffer
	evalStdout = &stdout
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Dataset: test") {
		t.Errorf("expected dataset header in output, got %q", out)
	}
	if !strings.Contains(out, "Total:") {
		t.Errorf("expected human summary in output, got %q", out)
	}
}

func TestNewEvalRunCmd_JSONOutput(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return &dataset.Runner{}, nil
	}
	runDatasetFn = func(*dataset.Runner, context.Context, *dataset.Dataset) ([]dataset.RunResult, error) {
		return []dataset.RunResult{{TestCaseID: "tc1", Success: true}}, nil
	}
	var stdout bytes.Buffer
	evalStdout = &stdout
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	cmd.Flags().Set("json", "true")
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	var report eval.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Errorf("invalid JSON output: %v", err)
	}
	if report.Dataset != "test" {
		t.Errorf("expected dataset test, got %q", report.Dataset)
	}
}

func TestNewEvalRunCmd_BelowMinPassRate(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return &dataset.Runner{}, nil
	}
	runDatasetFn = func(*dataset.Runner, context.Context, *dataset.Dataset) ([]dataset.RunResult, error) {
		return []dataset.RunResult{{TestCaseID: "tc1", Success: false}}, nil
	}
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	cmd.Flags().Set("min-pass-rate", "1.0")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "pass rate") {
		t.Errorf("expected pass rate floor error, got %v", err)
	}
}

func TestNewEvalRunCmd_TraceInitError(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return &dataset.Runner{}, nil
	}
	runDatasetFn = func(*dataset.Runner, context.Context, *dataset.Dataset) ([]dataset.RunResult, error) {
		return []dataset.RunResult{{TestCaseID: "tc1", Success: true}}, nil
	}
	initTraceProviderFn = func(context.Context, *sinctrace.ProviderConfig) (*sdktrace.TracerProvider, error) {
		return nil, errors.New("boom")
	}
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	cmd.Flags().Set("trace", "true")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "eval run: init trace") {
		t.Errorf("expected init trace error, got %v", err)
	}
}

func TestNewEvalRunCmd_TraceShutdownError(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return &dataset.Runner{}, nil
	}
	runDatasetFn = func(*dataset.Runner, context.Context, *dataset.Dataset) ([]dataset.RunResult, error) {
		return []dataset.RunResult{{TestCaseID: "tc1", Success: true}}, nil
	}
	initTraceProviderFn = func(context.Context, *sinctrace.ProviderConfig) (*sdktrace.TracerProvider, error) {
		return nil, nil
	}
	shutdownTraceFn = func(context.Context, *sdktrace.TracerProvider) error { return errors.New("boom") }
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	cmd.Flags().Set("trace", "true")
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Errorf("shutdown errors are swallowed, unexpected error: %v", err)
	}
}

func TestNewEvalRunCmd_JudgeWarning(t *testing.T) {
	defer resetEvalHooks()
	loadDatasetFn = func(string) (*dataset.Dataset, error) { return fakeDataset(), nil }
	openSessionStoreFn = func(string) (*session.Store, error) { return evalFakeSessionStore(t), nil }
	newRunnerFn = func(dataset.RunnerConfig, *agentloop.Loop, *session.Store) (*dataset.Runner, error) {
		return &dataset.Runner{}, nil
	}
	runDatasetFn = func(*dataset.Runner, context.Context, *dataset.Dataset) ([]dataset.RunResult, error) {
		return []dataset.RunResult{{TestCaseID: "tc1", Success: true}}, nil
	}
	var stderr bytes.Buffer
	evalStderr = &stderr
	t.Setenv("OPENAI_API_KEY", "")
	cmd := newEvalRunCmd()
	cmd.Flags().Set("dataset", "/tmp/dataset.json")
	cmd.Flags().Set("judge-model", "gpt-4")
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Errorf("judge errors are warnings, unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "warn: judge pass skipped") {
		t.Errorf("expected judge warning in stderr, got %q", stderr.String())
	}
}

// ── newEvalListCmd ───────────────────────────────────────────────────

func TestNewEvalListCmd_Error(t *testing.T) {
	defer resetEvalHooks()
	listDatasetsFn = func(string) ([]string, error) { return nil, errors.New("boom") }
	cmd := newEvalListCmd()
	cmd.Flags().Set("dir", "/tmp/evals")
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected list error, got %v", err)
	}
}

func TestNewEvalListCmd_Empty(t *testing.T) {
	defer resetEvalHooks()
	listDatasetsFn = func(string) ([]string, error) { return nil, nil }
	var stdout bytes.Buffer
	evalStdout = &stdout
	cmd := newEvalListCmd()
	cmd.Flags().Set("dir", "/tmp/evals")
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "no datasets found") {
		t.Errorf("expected no datasets message, got %q", stdout.String())
	}
}

func TestNewEvalListCmd_WithFiles(t *testing.T) {
	defer resetEvalHooks()
	listDatasetsFn = func(string) ([]string, error) { return []string{"a.json", "b.json"}, nil }
	var stdout bytes.Buffer
	evalStdout = &stdout
	cmd := newEvalListCmd()
	cmd.Flags().Set("dir", "/tmp/evals")
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	var out struct {
		Dir      string   `json:"dir"`
		Datasets []string `json:"datasets"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Errorf("invalid JSON output: %v", err)
	}
	if len(out.Datasets) != 2 {
		t.Errorf("expected 2 datasets, got %d", len(out.Datasets))
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func TestStubRunOverride(t *testing.T) {
	sess := &session.Session{ID: "sess-1"}
	res, err := stubRunOverride(context.Background(), sess, "hello")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if res.SessionID != "sess-1" {
		t.Errorf("expected SessionID sess-1, got %q", res.SessionID)
	}
	if res.Summary != "stub echo: hello" {
		t.Errorf("expected summary, got %q", res.Summary)
	}
	if !res.Verified || res.Turns != 1 {
		t.Errorf("expected Verified=true and Turns=1, got %v, %d", res.Verified, res.Turns)
	}
}

func TestNewLLMClientFor(t *testing.T) {
	defer resetEvalHooks()
	client := mockLLMClient(`{"pass": true, "score": 1.0}`, 200)
	newLLMClientFn = func(string, string) *llm.Client { return client }
	got := newLLMClientFor("http://test", "key")
	if got != client {
		t.Error("expected the mock client to be returned")
	}
}

func TestMustParseExporter(t *testing.T) {
	defer resetEvalHooks()
	var stderr bytes.Buffer
	evalStderr = &stderr

	if got := mustParseExporter("stdout"); got != sinctrace.ExporterStdout {
		t.Errorf("expected ExporterStdout, got %v", got)
	}

	if got := mustParseExporter("bad"); got != sinctrace.ExporterNoop {
		t.Errorf("expected ExporterNoop for unknown, got %v", got)
	}
	if !strings.Contains(stderr.String(), "warn: trace: unknown exporter") {
		t.Errorf("expected stderr warning for unknown exporter, got %q", stderr.String())
	}

	stderr.Reset()
	parseExporterFn = func(string) (sinctrace.ExporterKind, error) {
		return sinctrace.ExporterNoop, errors.New("boom")
	}
	if got := mustParseExporter("whatever"); got != sinctrace.ExporterNoop {
		t.Errorf("expected ExporterNoop for non-unknown error, got %v", got)
	}
	if stderr.String() != "" {
		t.Errorf("expected no warning for non-unknown error, got %q", stderr.String())
	}
}

func TestApplyJudge_EmptyAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	results := []dataset.RunResult{{TestCaseID: "tc1", Success: true}}
	err := applyJudge(context.Background(), results, "gpt-4", "https://api.openai.com/v1", "OPENAI_API_KEY")
	if err == nil || !strings.Contains(err.Error(), "judge: env OPENAI_API_KEY is empty") {
		t.Errorf("expected empty API key error, got %v", err)
	}
}

func TestApplyJudge_BuildError(t *testing.T) {
	defer resetEvalHooks()
	t.Setenv("OPENAI_API_KEY", "key")
	newLLMClientFn = func(string, string) *llm.Client { return nil }
	err := applyJudge(context.Background(), []dataset.RunResult{{TestCaseID: "tc1", Success: true}}, "gpt-4", "https://api.openai.com/v1", "OPENAI_API_KEY")
	if err == nil || !strings.Contains(err.Error(), "judge: build") {
		t.Errorf("expected build error, got %v", err)
	}
}

func TestApplyJudge_PerResultError(t *testing.T) {
	defer resetEvalHooks()
	t.Setenv("OPENAI_API_KEY", "key")
	newLLMClientFn = func(string, string) *llm.Client { return mockLLMClient("not json", 200) }
	results := []dataset.RunResult{
		{TestCaseID: "tc1", Success: false, FinalOutput: "out"},
		{TestCaseID: "tc2", Success: true, FinalOutput: "out"},
	}
	err := applyJudge(context.Background(), results, "gpt-4", "https://api.openai.com/v1", "OPENAI_API_KEY")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results[0].JudgeFeedback != "" {
		t.Errorf("failed results should be skipped, got feedback %q", results[0].JudgeFeedback)
	}
	if !strings.Contains(results[1].JudgeFeedback, "judge error") {
		t.Errorf("expected judge error feedback, got %q", results[1].JudgeFeedback)
	}
}

func judgeResponseJSON(pass bool, score float64, reason, feedback string) string {
	body := fmt.Sprintf(`{"pass": %v, "score": %g, "reason": %q, "feedback": %q}`, pass, score, reason, feedback)
	return fmt.Sprintf(`{"choices": [{"message": {"role": "assistant", "content": %q}}]}`, body)
}

func TestApplyJudge_Pass(t *testing.T) {
	defer resetEvalHooks()
	t.Setenv("OPENAI_API_KEY", "key")
	newLLMClientFn = func(string, string) *llm.Client {
		return mockLLMClient(judgeResponseJSON(true, 0.9, "ok", "good"), 200)
	}
	results := []dataset.RunResult{{TestCaseID: "tc1", Success: true, FinalOutput: "out"}}
	err := applyJudge(context.Background(), results, "gpt-4", "https://api.openai.com/v1", "OPENAI_API_KEY")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results[0].JudgeScore != 0.9 {
		t.Errorf("expected JudgeScore 0.9, got %f", results[0].JudgeScore)
	}
	if results[0].JudgeFeedback != "good" {
		t.Errorf("expected feedback good, got %q", results[0].JudgeFeedback)
	}
	if !results[0].Success {
		t.Error("expected Success to remain true")
	}
}

func TestApplyJudge_Fail(t *testing.T) {
	defer resetEvalHooks()
	t.Setenv("OPENAI_API_KEY", "key")
	newLLMClientFn = func(string, string) *llm.Client {
		return mockLLMClient(judgeResponseJSON(false, 0.5, "bad", "poor"), 200)
	}
	results := []dataset.RunResult{{TestCaseID: "tc1", Success: true, FinalOutput: "out", Error: "existing"}}
	err := applyJudge(context.Background(), results, "gpt-4", "https://api.openai.com/v1", "OPENAI_API_KEY")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results[0].Success {
		t.Error("expected Success to be flipped to false")
	}
	if results[0].Error != "existing" {
		t.Errorf("existing error should be preserved, got %q", results[0].Error)
	}
}

func TestApplyJudge_FailNoExistingError(t *testing.T) {
	defer resetEvalHooks()
	t.Setenv("OPENAI_API_KEY", "key")
	newLLMClientFn = func(string, string) *llm.Client {
		return mockLLMClient(judgeResponseJSON(false, 0.5, "bad", "poor"), 200)
	}
	results := []dataset.RunResult{{TestCaseID: "tc1", Success: true, FinalOutput: "out"}}
	err := applyJudge(context.Background(), results, "gpt-4", "https://api.openai.com/v1", "OPENAI_API_KEY")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results[0].Success {
		t.Error("expected Success to be flipped to false")
	}
	if results[0].Error != "judge failed" {
		t.Errorf("expected default error 'judge failed', got %q", results[0].Error)
	}
}

func TestWorkspaceRoot(t *testing.T) {
	defer resetEvalHooks()
	if got := workspaceRoot("/tmp/evals/critical.json"); got != "/tmp" {
		t.Errorf("workspaceRoot(/tmp/evals/critical.json) = %q, want /tmp", got)
	}
	if got := workspaceRoot("/tmp/data.json"); got != "/tmp" {
		t.Errorf("workspaceRoot(/tmp/data.json) = %q, want /tmp", got)
	}
	cwd, _ := os.Getwd()
	if got := workspaceRoot("../evals/critical.json"); got != filepath.Dir(cwd) {
		t.Errorf("workspaceRoot(../evals/critical.json) = %q, want %q", got, filepath.Dir(cwd))
	}
	filepathAbsFn = func(string) (string, error) { return "", errors.New("boom") }
	if got := workspaceRoot("whatever"); got != "." {
		t.Errorf("workspaceRoot on abs error should return '.', got %q", got)
	}
}
