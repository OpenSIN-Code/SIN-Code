// SPDX-License-Identifier: MIT
// Purpose: 100% statement coverage tests for the wiring package.
package wiring

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dispatch"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/learning"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// fakePromptSink satisfies dispatch.PromptSink.
type fakePromptSink struct{}

func (fakePromptSink) SubmitPrompt(context.Context, string, []string) error { return nil }

// fakeSubagentRunner satisfies dispatch.SubagentRunner.
type fakeSubagentRunner struct{}

func (fakeSubagentRunner) RunSubagent(context.Context, dispatch.AgentInvocation) (string, error) {
	return "", nil
}

// fakeVerifier satisfies hooklife.Verifier.
type fakeVerifier struct {
	passed bool
	report string
	err    error
}

func (f fakeVerifier) QualityGate(ctx context.Context, workdir string) (bool, string, error) {
	return f.passed, f.report, f.err
}

// fakeHTTPTransport stubs llm.Client HTTP calls without network access.
type fakeHTTPTransport struct {
	resp *http.Response
	err  error
}

func (f fakeHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func newFakeClient(resp *http.Response, err error) *llm.Client {
	return &llm.Client{
		BaseURL: "http://test",
		HTTP:    &http.Client{Transport: fakeHTTPTransport{resp: resp, err: err}},
	}
}

// --- wiring.go ---

func TestBuildSuccess(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	bundle, err := Build(Deps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle.Learner == nil || bundle.Dispatch == nil || bundle.Eval == nil || bundle.PRP.Verifier == nil {
		t.Fatalf("Build returned nil components: %+v", bundle)
	}
}

func TestBuildWithVerify(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	gate := verify.NewGate("off", nil, nil)
	bundle, err := Build(Deps{Verify: gate})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle.Learner == nil || bundle.Dispatch == nil || bundle.Eval == nil || bundle.PRP.Verifier == nil {
		t.Fatalf("Build returned nil components: %+v", bundle)
	}
}

func TestBuildLearningError(t *testing.T) {
	old := learningNewHook
	defer func() { learningNewHook = old }()
	learningNewHook = func(learning.Options) (*learning.Learner, error) {
		return nil, errors.New("learning failed")
	}
	_, err := Build(Deps{})
	if err == nil {
		t.Fatal("expected error from learning.New")
	}
}

func TestBuildDispatcherError(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", t.TempDir())
	old := buildDispatcherHook
	defer func() { buildDispatcherHook = old }()
	buildDispatcherHook = func(string, dispatch.PromptSink, dispatch.SubagentRunner) (*dispatch.Dispatcher, error) {
		return nil, errors.New("dispatch failed")
	}
	bundle, err := Build(Deps{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle.Dispatch == nil {
		t.Fatal("expected soft-fail dispatcher")
	}
}

// --- dispatch.go ---

func TestBuildDispatcherSuccess(t *testing.T) {
	disp, err := BuildDispatcher(t.TempDir(), fakePromptSink{}, fakeSubagentRunner{})
	if err != nil {
		t.Fatalf("BuildDispatcher: %v", err)
	}
	if disp == nil || disp.Reg == nil {
		t.Fatal("expected dispatcher with registry")
	}
}

func TestBuildDispatcherLoadError(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "bad.md"), []byte("no frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := BuildDispatcher(dir, fakePromptSink{}, fakeSubagentRunner{})
	if err == nil {
		t.Fatal("expected error from BuildDispatcher")
	}
}

// --- eval.go ---

func TestVerifySubjectNilVerifier(t *testing.T) {
	s := verifySubject{}
	out, err := s.Run(context.Background(), evalharness.EvalCase{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Success || out.Text != "no verifier wired" {
		t.Fatalf("got %+v", out)
	}
}

func TestVerifySubjectMetaWorkdir(t *testing.T) {
	s := verifySubject{verifier: fakeVerifier{passed: true, report: "ok"}}
	out, err := s.Run(context.Background(), evalharness.EvalCase{
		Meta: map[string]string{"workdir": "/meta"},
	})
	if err != nil || !out.Success || out.Text != "ok" {
		t.Fatalf("got %+v, err=%v", out, err)
	}
}

func TestVerifySubjectPromptFallback(t *testing.T) {
	s := verifySubject{verifier: fakeVerifier{passed: false, report: "bad"}}
	out, err := s.Run(context.Background(), evalharness.EvalCase{Prompt: "/prompt"})
	if err != nil || out.Success || out.Text != "bad" {
		t.Fatalf("got %+v, err=%v", out, err)
	}
}

func TestVerifySubjectError(t *testing.T) {
	s := verifySubject{verifier: fakeVerifier{err: errors.New("boom")}}
	_, err := s.Run(context.Background(), evalharness.EvalCase{Prompt: "/prompt"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEvalSubjectFactory(t *testing.T) {
	v := fakeVerifier{}
	factory := EvalSubjectFactory(v)
	for _, name := range []string{"verify", "gate", "other"} {
		subj, scorer, err := factory(name)
		if err != nil || subj == nil || scorer == nil {
			t.Fatalf("name=%s: got subj=%v scorer=%v err=%v", name, subj, scorer, err)
		}
	}
}

// --- prp.go ---

func TestPRPVerifierNil(t *testing.T) {
	v := prpVerifier{}
	passed, report, err := v.Verify(context.Background(), "/w")
	if err != nil || !passed || report != "" {
		t.Fatalf("got passed=%v report=%q err=%v", passed, report, err)
	}
}

func TestPRPVerifierNonNil(t *testing.T) {
	v := prpVerifier{v: fakeVerifier{passed: true, report: "rpt"}}
	passed, report, err := v.Verify(context.Background(), "/w")
	if err != nil || !passed || report != "rpt" {
		t.Fatalf("got passed=%v report=%q err=%v", passed, report, err)
	}
}

func TestPRPDeps(t *testing.T) {
	d := PRPDeps(fakeVerifier{}, nil, nil, nil)
	if d.Planner != nil || d.Implementer != nil || d.PR != nil {
		t.Fatal("expected nil collaborators")
	}
	if d.Verifier == nil {
		t.Fatal("expected verifier")
	}
}

// --- spec.go ---

func TestLLMCompleterNilClient(t *testing.T) {
	c := llmCompleter{}
	out, err := c.Complete(context.Background(), "sys", "user")
	if err != nil || out != "" {
		t.Fatalf("got %q %v", out, err)
	}
}

func TestLLMCompleterDefaultModel(t *testing.T) {
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	c := llmCompleter{client: newFakeClient(resp, nil), model: ""}
	out, err := c.Complete(context.Background(), "sys", "user")
	if err != nil || out != "ok" {
		t.Fatalf("got %q %v", out, err)
	}
}

func TestLLMCompleterCustomModel(t *testing.T) {
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"done"}}]}`)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	c := llmCompleter{client: newFakeClient(resp, nil), model: "custom-model"}
	out, err := c.Complete(context.Background(), "sys", "user")
	if err != nil || out != "done" {
		t.Fatalf("got %q %v", out, err)
	}
}

func TestLLMCompleterChatError(t *testing.T) {
	c := llmCompleter{client: newFakeClient(nil, errors.New("boom")), model: "m"}
	_, err := c.Complete(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewSpecCompleterNil(t *testing.T) {
	if NewSpecCompleter(nil, "") != nil {
		t.Fatal("expected nil completer")
	}
}

func TestNewSpecCompleterNonNil(t *testing.T) {
	client := &llm.Client{BaseURL: "http://test"}
	c := NewSpecCompleter(client, "m")
	if c == nil {
		t.Fatal("expected completer")
	}
}

func TestAuthorSpecDefaults(t *testing.T) {
	res, err := AuthorSpec(context.Background(), "test description", SpecAuthorOptions{})
	if err != nil {
		t.Fatalf("AuthorSpec: %v", err)
	}
	if res == nil || res.Spec == nil {
		t.Fatal("expected stub spec")
	}
}

func TestAuthorSpecCustomOptions(t *testing.T) {
	res, err := AuthorSpec(context.Background(), "test description", SpecAuthorOptions{
		Timeout:    30 * time.Second,
		MaxRetries: 5,
	})
	if err != nil {
		t.Fatalf("AuthorSpec: %v", err)
	}
	if res == nil || res.Spec == nil {
		t.Fatal("expected stub spec")
	}
}

func TestAuthorSpecError(t *testing.T) {
	_, err := AuthorSpec(context.Background(), "", SpecAuthorOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}
