// SPDX-License-Identifier: MIT
// Purpose: Race-safety and correctness tests for the fusion tournament
// (mandate M7). All tests must pass under `go test -race -count=1`.
package fusion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// fakeProvider simulates a provider with configurable delay, output, and
// token count. It is the simplest possible RunFunc for testing.
type fakeProvider struct {
	name   string
	delay  time.Duration
	output string
	tokens int
	calls  atomic.Int32
}

// verifyState is a shared marker that fake providers write to and the
// fake verify gate reads from. Simulates workspace state changes.
type verifyState struct {
	mu      sync.Mutex
	outputs map[string]string // provider name → last output
}

func newVerifyState() *verifyState {
	return &verifyState{outputs: make(map[string]string)}
}

func (vs *verifyState) set(name, output string) {
	vs.mu.Lock()
	vs.outputs[name] = output
	vs.mu.Unlock()
}

// hasAny checks if any provider produced output containing the magic string.
// Simulates checking the workspace for the expected changes.
func (vs *verifyState) hasAny(magic string) bool {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	for _, out := range vs.outputs {
		if strings.Contains(out, magic) {
			return true
		}
	}
	return false
}

// hasProvider checks if a specific provider produced output containing magic.
func (vs *verifyState) hasProvider(name, magic string) bool {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	return strings.Contains(vs.outputs[name], magic)
}

func makeRunFunc(providers map[string]*fakeProvider, vs *verifyState) RunFunc {
	return func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		fp := providers[prov.Name]
		if fp == nil {
			return nil, errors.New("unknown provider: " + prov.Name)
		}
		fp.calls.Add(1)
		select {
		case <-time.After(fp.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		vs.set(prov.Name, fp.output)
		return &agentloop.Result{
			SessionID: sess.ID,
			Summary:   fp.output,
			Verified:  false,
			Turns:     1,
			Tokens:    fp.tokens,
		}, nil
	}
}

func makeForkFunc() ForkFunc {
	var counter atomic.Int32
	return func(srcSessionID string, turn int) (*session.Session, error) {
		c := counter.Add(1)
		return &session.Session{ID: fmt.Sprintf("fork-%d", c)}, nil
	}
}

// makeVerifyFn returns a VerifyFn that passes when the verify state
// contains the magic string from any provider. In the real implementation,
// each provider would have its own worktree and the gate would check that
// specific worktree — here we simulate workspace state via shared state.
func makeVerifyFn(vs *verifyState, magic string) func(ctx context.Context, workspace string) verify.Result {
	return func(ctx context.Context, workspace string) verify.Result {
		if vs.hasAny(magic) {
			return verify.Result{Passed: true, Mode: verify.ModePoC, Report: "passed: workspace contains correct output"}
		}
		return verify.Result{Passed: false, Mode: verify.ModePoC, Report: "failed: workspace does not contain correct output"}
	}
}

func TestTournament_FirstPassWins(t *testing.T) {
	vs := newVerifyState()
	providers := map[string]*fakeProvider{
		"slow-good":  {name: "slow-good", delay: 100 * time.Millisecond, output: "CORRECT answer", tokens: 500},
		"fast-bad":   {name: "fast-bad", delay: 20 * time.Millisecond, output: "wrong answer", tokens: 300},
		"medium-bad": {name: "medium-bad", delay: 50 * time.Millisecond, output: "also wrong", tokens: 400},
	}

	provCfgs := []ProviderConfig{
		{Name: "slow-good"},
		{Name: "fast-bad"},
		{Name: "medium-bad"},
	}

	tournament := &Tournament{
		Providers:       provCfgs,
		RunFunc:         makeRunFunc(providers, vs),
		ForkFunc:        makeForkFunc(),
		VerifyFn:        makeVerifyFn(vs, "CORRECT"),
		MinQuorum:       2,
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected winner, got error: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected winner, got nil")
	}
	if result.Winner.Provider != "slow-good" {
		t.Errorf("expected winner 'slow-good', got %q", result.Winner.Provider)
	}
	if result.AllFailed {
		t.Error("expected AllFailed=false")
	}
	if len(result.Losers) < 1 {
		t.Errorf("expected at least 1 loser, got %d", len(result.Losers))
	}
}

func TestTournament_AllProvidersFail(t *testing.T) {
	vs := newVerifyState()
	providers := map[string]*fakeProvider{
		"a": {name: "a", delay: 10 * time.Millisecond, output: "wrong1", tokens: 100},
		"b": {name: "b", delay: 10 * time.Millisecond, output: "wrong2", tokens: 200},
	}

	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "a"}, {Name: "b"}},
		RunFunc:         makeRunFunc(providers, vs),
		ForkFunc:        makeForkFunc(),
		VerifyFn:        makeVerifyFn(vs, "CORRECT"),
		MinQuorum:       2,
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if !errors.Is(err, ErrAllProvidersFailed) {
		t.Fatalf("expected ErrAllProvidersFailed, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result even on all-fail")
	}
	if !result.AllFailed {
		t.Error("expected AllFailed=true")
	}
	if result.Winner != nil {
		t.Error("expected nil winner on all-fail")
	}
	if len(result.Losers) != 2 {
		t.Errorf("expected 2 losers, got %d", len(result.Losers))
	}
}

func TestTournament_InsufficientQuorum(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "only-one"}},
		RunFunc:   makeRunFunc(map[string]*fakeProvider{}, newVerifyState()),
		ForkFunc:  makeForkFunc(),
		VerifyFn:  makeVerifyFn(newVerifyState(), "x"),
		MinQuorum: 2,
	}

	_, err := tournament.Run(context.Background())
	if !errors.Is(err, ErrInsufficientQuorum) {
		t.Fatalf("expected ErrInsufficientQuorum, got: %v", err)
	}
}

func TestTournament_NilRunFunc(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "a"}, {Name: "b"}},
		RunFunc:   nil,
		ForkFunc:  makeForkFunc(),
		VerifyFn:  makeVerifyFn(newVerifyState(), "x"),
		MinQuorum: 2,
	}

	_, err := tournament.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil RunFunc")
	}
}

func TestTournament_NilForkFunc(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "a"}, {Name: "b"}},
		RunFunc:   makeRunFunc(map[string]*fakeProvider{}, newVerifyState()),
		ForkFunc:  nil,
		VerifyFn:  makeVerifyFn(newVerifyState(), "x"),
		MinQuorum: 2,
	}

	_, err := tournament.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil ForkFunc")
	}
}

func TestTournament_CostCeilingExceeded(t *testing.T) {
	vs := newVerifyState()
	providers := map[string]*fakeProvider{
		"expensive": {name: "expensive", delay: 10 * time.Millisecond, output: "wrong", tokens: 10_000_000},
	}

	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "expensive", InputPer1M: 5.0, OutputPer1M: 5.0}},
		RunFunc:         makeRunFunc(providers, vs),
		ForkFunc:        makeForkFunc(),
		VerifyFn:        makeVerifyFn(vs, "CORRECT"),
		MinQuorum:       1,
		MaxCostUSD:      0.01, // 10M tokens * $10/1M = $100 >> $0.01
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if !errors.Is(err, ErrCostCeilingExceeded) {
		t.Fatalf("expected ErrCostCeilingExceeded, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result on cost exceed")
	}
	if result.TotalCostUSD <= 0 {
		t.Error("expected positive cost on cost exceed")
	}
}

func TestTournament_LoserCancellation(t *testing.T) {
	vs := newVerifyState()
	providers := map[string]*fakeProvider{
		"fast-good": {name: "fast-good", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
		"slow-bad":  {name: "slow-bad", delay: 500 * time.Millisecond, output: "wrong", tokens: 200},
	}

	tournament := &Tournament{
		Providers:          []ProviderConfig{{Name: "fast-good"}, {Name: "slow-bad"}},
		RunFunc:            makeRunFunc(providers, vs),
		ForkFunc:           makeForkFunc(),
		VerifyFn:           makeVerifyFn(vs, "CORRECT"),
		MinQuorum:          2,
		PerProviderTimeout: 5 * time.Second,
		Workspace:          "/test/ws",
		Prompt:             "do the thing",
		SourceSessionID:    "src-1",
	}

	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected winner, got error: %v", err)
	}
	if result.Winner == nil || result.Winner.Provider != "fast-good" {
		t.Fatalf("expected winner 'fast-good', got %v", result.Winner)
	}

	if vs.hasProvider("slow-bad", "wrong") {
		t.Error("expected slow-bad to be cancelled before setting output in verify state")
	}
}

func TestTournament_PerProviderTimeout(t *testing.T) {
	vs := newVerifyState()
	providers := map[string]*fakeProvider{
		"timeout": {name: "timeout", delay: 10 * time.Second, output: "CORRECT", tokens: 100},
		"fast":    {name: "fast", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
	}

	tournament := &Tournament{
		Providers:          []ProviderConfig{{Name: "timeout"}, {Name: "fast"}},
		RunFunc:            makeRunFunc(providers, vs),
		ForkFunc:           makeForkFunc(),
		VerifyFn:           makeVerifyFn(vs, "CORRECT"),
		MinQuorum:          2,
		PerProviderTimeout: 100 * time.Millisecond,
		Workspace:          "/test/ws",
		Prompt:             "do the thing",
		SourceSessionID:    "src-1",
	}

	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected winner, got error: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected winner")
	}
	if result.Winner.Provider != "fast" {
		t.Errorf("expected 'fast' winner (timeout should be cancelled), got %q", result.Winner.Provider)
	}
}

func TestTournament_ParallelProvidersAllRun(t *testing.T) {
	vs := newVerifyState()
	var startCount atomic.Int32

	runFunc := func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		startCount.Add(1)
		vs.set(prov.Name, "CORRECT")
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &agentloop.Result{
			SessionID: sess.ID,
			Summary:   "CORRECT",
			Turns:     1,
			Tokens:    100,
		}, nil
	}

	tournament := &Tournament{
		Providers: []ProviderConfig{
			{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"},
		},
		RunFunc:         runFunc,
		ForkFunc:        makeForkFunc(),
		VerifyFn:        makeVerifyFn(vs, "CORRECT"),
		MinQuorum:       3,
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected winner, got error: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected winner")
	}
	if startCount.Load() < 3 {
		t.Errorf("expected at least 3 providers to start, got %d", startCount.Load())
	}
}

func TestTournament_RaceSafety(t *testing.T) {
	for i := 0; i < 20; i++ {
		vs := newVerifyState()
		providers := map[string]*fakeProvider{
			"a": {name: "a", delay: time.Duration(i+1) * time.Millisecond, output: "CORRECT", tokens: 100 * (i + 1)},
			"b": {name: "b", delay: time.Duration(20-i) * time.Millisecond, output: "CORRECT", tokens: 200},
			"c": {name: "c", delay: time.Duration(i+5) * time.Millisecond, output: "wrong", tokens: 150},
		}

		tournament := &Tournament{
			Providers:       []ProviderConfig{{Name: "a"}, {Name: "b"}, {Name: "c"}},
			RunFunc:         makeRunFunc(providers, vs),
			ForkFunc:        makeForkFunc(),
			VerifyFn:        makeVerifyFn(vs, "CORRECT"),
			MinQuorum:       2,
			Workspace:       "/test/ws",
			Prompt:          "do the thing",
			SourceSessionID: "src-1",
		}

		result, err := tournament.Run(context.Background())
		if err != nil && !errors.Is(err, ErrAllProvidersFailed) {
			t.Fatalf("iter %d: unexpected error: %v", i, err)
		}
		if result == nil {
			t.Fatalf("iter %d: expected non-nil result", i)
		}
	}
}

func TestTournament_ContextCancellation(t *testing.T) {
	vs := newVerifyState()
	providers := map[string]*fakeProvider{
		"slow": {name: "slow", delay: 10 * time.Second, output: "CORRECT", tokens: 100},
	}

	tournament := &Tournament{
		Providers:          []ProviderConfig{{Name: "slow"}},
		RunFunc:            makeRunFunc(providers, vs),
		ForkFunc:           makeForkFunc(),
		VerifyFn:           makeVerifyFn(vs, "CORRECT"),
		MinQuorum:          1,
		PerProviderTimeout: 5 * time.Second,
		Workspace:          "/test/ws",
		Prompt:             "do the thing",
		SourceSessionID:    "src-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := tournament.Run(ctx)
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

func TestTournament_DeterministicTieBreak(t *testing.T) {
	vs := newVerifyState()
	providers := map[string]*fakeProvider{
		"zeta":  {name: "zeta", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
		"alpha": {name: "alpha", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
	}

	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "zeta"}, {Name: "alpha"}},
		RunFunc:         makeRunFunc(providers, vs),
		ForkFunc:        makeForkFunc(),
		VerifyFn:        makeVerifyFn(vs, "CORRECT"),
		MinQuorum:       2,
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}

	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected winner, got error: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected winner")
	}
}
