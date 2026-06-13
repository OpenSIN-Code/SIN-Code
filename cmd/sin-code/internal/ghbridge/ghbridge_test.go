// SPDX-License-Identifier: MIT
// Purpose: tests for the ghbridge package. All tests are hermetic —
// no real `gh` binary is invoked, no real network, no real filesystem
// writes outside t.TempDir. The Runner interface is stubbed with a
// closure so each test asserts on classification + dispatch output
// without spawning a process.
// Docs: ghbridge.doc.md
package ghbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── Test helpers ──────────────────────────────────────────────────────

// fakeRunner returns a Runner that records every call and replies with
// the supplied stdout/stderr/err tuple. The callCount pointer (if
// non-nil) is atomically incremented on every invocation so tests can
// assert that forbidden calls NEVER reached the runner.
type fakeRunner struct {
	mu        sync.Mutex
	calls     [][]string
	responses []fakeResponse
}

type fakeResponse struct {
	stdout string
	stderr string
	err    error
}

func newFakeRunner(responses ...fakeResponse) *fakeRunner {
	return &fakeRunner{responses: responses}
}

// run is the Runner-compatible method.
func (f *fakeRunner) run(_ context.Context, args []string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Copy the slice so the test can compare against the captured
	// args without aliasing the caller's buffer.
	cp := make([]string, len(args))
	copy(cp, args)
	f.calls = append(f.calls, cp)
	// Pop the next response. If the test under-supplied responses,
	// return a generic "ok" so it does not block on missing data.
	idx := len(f.calls) - 1
	if idx < len(f.responses) {
		r := f.responses[idx]
		return r.stdout, r.stderr, r.err
	}
	return "", "", nil
}

func (f *fakeRunner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeRunner) lastCall() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

// runForBridge adapts a fakeRunner to the Runner signature with a
// stable reference (closures need a stable address for tests that
// swap Run later).
func runForBridge(f *fakeRunner) Runner {
	return f.run
}

// ── Tier classifier tests ─────────────────────────────────────────────

func TestClassifyReadOnly(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"issue_list", []string{"issue", "list"}},
		{"pr_view", []string{"pr", "view", "42"}},
		{"pr_checks", []string{"pr", "checks", "42"}},
		{"pr_diff", []string{"pr", "diff", "42"}},
		{"run_list", []string{"run", "list"}},
		{"release_list", []string{"release", "list"}},
		{"repo_view", []string{"repo", "view"}},
		{"search_issues", []string{"search", "issues", "bug"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, err := Classify(tc.args)
			if err != nil {
				t.Fatalf("expected nil err for read-only %v, got %v", tc.args, err)
			}
			if tier != TierReadOnly {
				t.Fatalf("expected TierReadOnly for %v, got %s", tc.args, tier)
			}
		})
	}
}

func TestClassifyMutating(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"issue_create", []string{"issue", "create", "--title", "x"}},
		{"issue_comment", []string{"issue", "comment", "42", "--body", "x"}},
		{"pr_create", []string{"pr", "create", "--title", "x", "--body", "x"}},
		{"pr_merge", []string{"pr", "merge", "42"}},
		{"issue_close", []string{"issue", "close", "42"}},
		{"run_rerun", []string{"run", "rerun", "12345"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, err := Classify(tc.args)
			if err != nil {
				t.Fatalf("expected nil err for mutating %v, got %v", tc.args, err)
			}
			if tier != TierMutating {
				t.Fatalf("expected TierMutating for %v, got %s", tc.args, tier)
			}
		})
	}
}

func TestClassifyForbidden(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"repo_delete", []string{"repo", "delete", "owner/repo"}},
		{"auth_logout", []string{"auth", "logout"}},
		{"secret_set", []string{"secret", "set", "FOO"}},
		{"api_call", []string{"api", "graphql"}},
		{"config_set", []string{"config", "set", "editor", "vim"}},
		{"alias_set", []string{"alias", "set", "co", "checkout"}},
		{"extension_install", []string{"extension", "install", "owner/ext"}},
		{"codespace_create", []string{"codespace", "create"}},
		{"repo_fork", []string{"repo", "fork", "owner/repo"}},
		// Defensive: forbidden verb inside an allowed group.
		{"issue_delete_inside_group", []string{"issue", "delete", "42"}},
		// Unknown verb fails closed.
		{"pr_frobnicate", []string{"pr", "frobnicate"}},
		// Group not exposed in the allowlist.
		{"gist_list", []string{"gist", "list"}},
		// Empty args.
		{"empty", []string{}},
		// Group with no verb.
		{"issue_alone", []string{"issue"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, err := Classify(tc.args)
			if err == nil {
				t.Fatalf("expected error for forbidden %v, got nil", tc.args)
			}
			if tier != TierForbidden {
				t.Fatalf("expected TierForbidden for %v, got %s (err=%v)", tc.args, tier, err)
			}
		})
	}
}

// TestClassifyForbiddenTokenInTail is the security-critical assertion
// that a forbidden verb hiding at any position (not just args[1]) is
// still hard-rejected. The classifier must scan the WHOLE args slice.
func TestClassifyForbiddenTokenInTail(t *testing.T) {
	args := []string{"issue", "list", "delete"}
	tier, err := Classify(args)
	if err == nil {
		t.Fatalf("expected error for tail forbidden token, got nil")
	}
	if tier != TierForbidden {
		t.Fatalf("expected TierForbidden, got %s", tier)
	}
	if !strings.Contains(err.Error(), "delete") {
		t.Fatalf("error should mention the forbidden token: %v", err)
	}
}

// ── Bridge.Execute tests ──────────────────────────────────────────────

func TestExecuteSuccess(t *testing.T) {
	want := "issue list output\n"
	fake := newFakeRunner(fakeResponse{stdout: want})
	b := NewWithRunner(runForBridge(fake), time.Second)
	out, tier, err := b.Execute(context.Background(), []string{"issue", "list"})
	if err != nil {
		t.Fatalf("Execute: unexpected err: %v", err)
	}
	if tier != TierReadOnly {
		t.Fatalf("tier: want read-only, got %s", tier)
	}
	if out != want {
		t.Fatalf("stdout: want %q, got %q", want, out)
	}
	if got := fake.lastCall(); len(got) != 2 || got[0] != "issue" || got[1] != "list" {
		t.Fatalf("runner called with %v, want [issue list]", got)
	}
}

// TestExecuteForbiddenNeverRuns is the CRITICAL security test: a
// forbidden command must NEVER reach the Runner. We assert on
// callCount to prove the runner was not invoked at all.
func TestExecuteForbiddenNeverRuns(t *testing.T) {
	var calls int32
	runner := Runner(func(_ context.Context, args []string) (string, string, error) {
		atomic.AddInt32(&calls, 1)
		return "should not see this", "", nil
	})
	b := NewWithRunner(runner, time.Second)
	forbidden := [][]string{
		{"repo", "delete", "x"},
		{"auth", "logout"},
		{"secret", "set", "X"},
		{"api"},
		{"config", "set", "x", "y"},
		{"issue", "delete", "1"},
		{"issue", "list", "delete"},
		{},
	}
	for _, args := range forbidden {
		out, tier, err := b.Execute(context.Background(), args)
		if err == nil {
			t.Fatalf("Execute(%v): expected error, got nil", args)
		}
		if tier != TierForbidden {
			t.Fatalf("Execute(%v): expected TierForbidden, got %s", args, tier)
		}
		if out != "" {
			t.Fatalf("Execute(%v): expected empty stdout, got %q", args, out)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("CRITICAL: runner was invoked %d times for forbidden commands", got)
	}
}

func TestExecuteFailureSurfacesStderr(t *testing.T) {
	fake := newFakeRunner(fakeResponse{stdout: "partial", stderr: "fatal: not a git repo", err: errors.New("exit status 1")})
	b := NewWithRunner(runForBridge(fake), time.Second)
	out, tier, err := b.Execute(context.Background(), []string{"issue", "list"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if out != "" {
		t.Fatalf("on failure, stdout should be empty in returned value, got %q", out)
	}
	if tier != TierReadOnly {
		t.Fatalf("tier: want read-only, got %s", tier)
	}
	if !strings.Contains(err.Error(), "fatal: not a git repo") {
		t.Fatalf("error should surface stderr, got: %v", err)
	}
}

// ── MCP dispatch tests ────────────────────────────────────────────────

// runMCPRequest feeds one JSON-RPC request through a fresh Server and
// returns the parsed response.
func runMCPRequest(t *testing.T, srv *Server, method string, params map[string]any) *jsonRPCResponse {
	t.Helper()
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errW := &bytes.Buffer{}
	local := NewServerWithIO(in, out, errW)
	// Share the same bridge so lazyBridge is hit in the local server.
	// We do this by writing one request and then reading one response.
	_ = srv // unused; we use local directly below

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      func() *json.RawMessage { b := json.RawMessage(`"test-1"`); return &b }(),
		Method:  method,
	}
	if params != nil {
		b, _ := json.Marshal(params)
		req.Params = b
	}
	rb, _ := json.Marshal(req)
	in.Write(rb)
	in.Write([]byte("\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := local.Serve(ctx); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Serve: %v (stderr=%q)", err, errW.String())
	}

	// First non-empty line of out is the response.
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("unmarshal response: %v (line=%q)", err, line)
		}
		return &resp
	}
	t.Fatal("no response received")
	return nil
}

func TestDispatchQueryRejectsMutation(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errW := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, errW)
	fake := newFakeRunner(fakeResponse{stdout: "ok"})
	srv.bridgeOnce.Do(func() {
		srv.bridge = NewWithRunner(runForBridge(fake), time.Second)
	})

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      func() *json.RawMessage { b := json.RawMessage(`"1"`); return &b }(),
		Method:  "tools/call",
		Params:  mustMarshalParams(t, "gh_query", map[string]any{"args": []string{"issue", "create", "--title", "x"}}),
	}
	rb, _ := json.Marshal(req)
	in.Write(rb)
	in.Write([]byte("\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Serve(ctx); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Serve: %v", err)
	}

	resp := firstResponse(t, out)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected isError=true for mutation through gh_query")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected error content")
	}
	if !strings.Contains(result.Content[0].Text, "gh_execute") {
		t.Fatalf("rejection should hint at gh_execute, got: %q", result.Content[0].Text)
	}
	if fake.callCount() != 0 {
		t.Fatalf("runner must NOT be called for rejected mutation, got %d calls", fake.callCount())
	}
}

func TestDispatchExecuteAllowsMutation(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errW := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, errW)
	fake := newFakeRunner(fakeResponse{stdout: "issue created"})
	srv.bridgeOnce.Do(func() {
		srv.bridge = NewWithRunner(runForBridge(fake), time.Second)
	})

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      func() *json.RawMessage { b := json.RawMessage(`"1"`); return &b }(),
		Method:  "tools/call",
		Params:  mustMarshalParams(t, "gh_execute", map[string]any{"args": []string{"issue", "create", "--title", "x"}}),
	}
	rb, _ := json.Marshal(req)
	in.Write(rb)
	in.Write([]byte("\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Serve(ctx); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Serve: %v", err)
	}

	resp := firstResponse(t, out)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected isError=false for gh_execute + mutation, got error: %v", result.Content)
	}
	if fake.callCount() != 1 {
		t.Fatalf("runner should be called once, got %d", fake.callCount())
	}
	if got := fake.lastCall(); len(got) < 2 || got[0] != "issue" || got[1] != "create" {
		t.Fatalf("runner called with %v, want [issue create ...]", got)
	}
}

// TestDispatchForbiddenBlockedOnBothTools ensures that a forbidden
// command is rejected regardless of which tool the agent called. This
// is the defense-in-depth invariant.
func TestDispatchForbiddenBlockedOnBothTools(t *testing.T) {
	for _, tool := range []string{"gh_query", "gh_execute"} {
		t.Run(tool, func(t *testing.T) {
			in := &bytes.Buffer{}
			out := &bytes.Buffer{}
			errW := &bytes.Buffer{}
			srv := NewServerWithIO(in, out, errW)
			fake := newFakeRunner(fakeResponse{stdout: "should not run"})
			srv.bridgeOnce.Do(func() {
				srv.bridge = NewWithRunner(runForBridge(fake), time.Second)
			})

			req := jsonRPCRequest{
				JSONRPC: "2.0",
				ID:      func() *json.RawMessage { b := json.RawMessage(`"1"`); return &b }(),
				Method:  "tools/call",
				Params:  mustMarshalParams(t, tool, map[string]any{"args": []string{"repo", "delete", "x"}}),
			}
			rb, _ := json.Marshal(req)
			in.Write(rb)
			in.Write([]byte("\n"))

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := srv.Serve(ctx); err != nil && !errors.Is(err, io.EOF) {
				t.Fatalf("Serve: %v", err)
			}

			resp := firstResponse(t, out)
			if resp.Error != nil {
				t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
			}
			var result toolResult
			if err := json.Unmarshal(resp.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected isError=true for forbidden command via %s", tool)
			}
			if fake.callCount() != 0 {
				t.Fatalf("runner must NOT be called for forbidden command via %s, got %d", tool, fake.callCount())
			}
		})
	}
}

// ── Health tests ──────────────────────────────────────────────────────

// TestHealthAuthMissing: a runner that returns an error must be
// surfaced as a wrapped error. We use a runner that always returns an
// error to simulate `gh auth status` reporting "not logged in".
func TestHealthAuthMissing(t *testing.T) {
	runner := Runner(func(_ context.Context, args []string) (string, string, error) {
		return "", "You are not logged into any GitHub hosts. Run gh auth login to authenticate.", errors.New("exit status 1")
	})
	b := NewWithRunner(runner, time.Second)
	err := b.Health(context.Background())
	if err == nil {
		t.Fatal("expected error from Health, got nil")
	}
	if !strings.Contains(err.Error(), "not logged") {
		t.Fatalf("error should surface stderr, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ghbridge:") {
		t.Fatalf("error should be wrapped with ghbridge: prefix, got: %v", err)
	}
}

// TestHealthOK: a runner that returns empty stderr and nil error is a
// successful auth probe.
func TestHealthOK(t *testing.T) {
	runner := Runner(func(_ context.Context, args []string) (string, string, error) {
		return "Logged in to github.com as user (oauth_token)\n", "", nil
	})
	b := NewWithRunner(runner, time.Second)
	if err := b.Health(context.Background()); err != nil {
		t.Fatalf("Health: unexpected err: %v", err)
	}
}

// ── AllowedSurface test ───────────────────────────────────────────────

func TestAllowedSurface(t *testing.T) {
	s := AllowedSurface()
	// Must include all expected groups.
	mustContain := []string{"issue", "pr", "release", "run", "workflow", "repo", "search", "label"}
	for _, g := range mustContain {
		if !strings.Contains(s, g) {
			t.Errorf("AllowedSurface() = %q, missing %q", s, g)
		}
	}
	// Must NOT include forbidden groups.
	mustNotContain := []string{"gist", "codespace", "auth", "secret", "api", "config"}
	for _, g := range mustNotContain {
		if strings.Contains(s, g) {
			t.Errorf("AllowedSurface() = %q, must not contain %q", s, g)
		}
	}
	// Sorted ascending: the first listed should be "issue" (alphabetical)
	// and the surface must be comma-separated.
	parts := strings.Split(s, ", ")
	if len(parts) < 5 {
		t.Errorf("AllowedSurface() = %q, expected at least 5 groups", s)
	}
	for i := 1; i < len(parts); i++ {
		if parts[i-1] >= parts[i] {
			t.Errorf("AllowedSurface() not sorted: %q", s)
			break
		}
	}
}

// ── RegisterMCP test ──────────────────────────────────────────────────

func TestRegisterMCP(t *testing.T) {
	// Pre-existing config with another server (e.g. "superpowers")
	// must survive the merge. Idempotency: calling twice produces the
	// same file.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mcp.json")
	pre := map[string]any{
		"mcpServers": map[string]any{
			"superpowers": map[string]any{
				"command": "/usr/local/bin/superpowers-mcp",
				"args":    []string{"serve"},
			},
		},
	}
	b, _ := json.MarshalIndent(pre, "", "  ")
	if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out, err := RegisterMCP(cfgPath)
	if err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	if out != cfgPath {
		t.Fatalf("RegisterMCP returned %q, want %q", out, cfgPath)
	}

	// Read back, verify both keys present.
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	servers, _ := got["mcpServers"].(map[string]any)
	if servers == nil {
		t.Fatal("mcpServers missing after RegisterMCP")
	}
	if _, ok := servers["superpowers"]; !ok {
		t.Fatal("superpowers entry was clobbered by RegisterMCP")
	}
	gh, ok := servers["gh"].(map[string]any)
	if !ok {
		t.Fatal("gh entry missing after RegisterMCP")
	}
	if _, ok := gh["command"]; !ok {
		t.Fatal("gh entry missing 'command' key")
	}
	args, _ := gh["args"].([]any)
	if len(args) < 2 {
		t.Fatalf("gh entry args = %v, want [gh serve]", args)
	}

	// Idempotency: second call should be a no-op (same command).
	if _, err := RegisterMCP(cfgPath); err != nil {
		t.Fatalf("RegisterMCP (2nd call): %v", err)
	}
	data2, _ := os.ReadFile(cfgPath)
	if !bytes.Equal(data, data2) {
		t.Fatal("RegisterMCP not idempotent: file changed on second call")
	}
}

// ── Truncate test ─────────────────────────────────────────────────────

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"", 10, ""},
		{"abc", 10, "abc"},
		{"abcdef", 3, "abc…"},
		{"abc", 0, "abc"},
		{"abcdef", -1, "abcdef"},
	}
	for _, tc := range cases {
		got := truncate(tc.in, tc.n)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

// ── Plumbing helpers (test-local) ─────────────────────────────────────

// mustMarshalParams builds the params RawMessage for a tools/call
// request.
func mustMarshalParams(t *testing.T, name string, args map[string]any) json.RawMessage {
	t.Helper()
	p := toolCallParams{Name: name}
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			t.Fatalf("marshal args: %v", err)
		}
		p.Arguments = b
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return b
}

// firstResponse returns the first non-empty JSON-RPC response line
// from the output buffer.
func firstResponse(t *testing.T, out *bytes.Buffer) *jsonRPCResponse {
	t.Helper()
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var resp jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("unmarshal response: %v (line=%q)", err, line)
		}
		return &resp
	}
	t.Fatal("no response received")
	return nil
}

// ── Additional coverage tests (target: >70%) ──────────────────────────

func TestTierString(t *testing.T) {
	cases := []struct {
		tier Tier
		want string
	}{
		{TierReadOnly, "read-only"},
		{TierMutating, "mutating"},
		{TierForbidden, "forbidden"},
		// Unknown tier (e.g. cast from a junk int) falls through to
		// the default branch — defense in depth for misbehaving
		// callers.
		{Tier(99), "forbidden"},
	}
	for _, tc := range cases {
		if got := tc.tier.String(); got != tc.want {
			t.Errorf("Tier(%d).String() = %q, want %q", tc.tier, got, tc.want)
		}
	}
}

func TestNewProduction(t *testing.T) {
	b := New()
	if b == nil {
		t.Fatal("New() returned nil")
	}
	if b.Run == nil {
		t.Fatal("New() did not set Run")
	}
	if b.Timeout != DefaultExecuteTimeout {
		t.Errorf("New() Timeout = %v, want %v", b.Timeout, DefaultExecuteTimeout)
	}
	// Run should be ExecRunner — verify by checking the function
	// pointer (best-effort: the function is package-private so we
	// can only assert it's not nil).
	if b.Run == nil {
		t.Fatal("New().Run is nil")
	}
}

// TestExecuteNilBridge checks the nil-guard on Execute. We don't
// expect production code to hit this, but the guard is documented
// behavior and we want it tested.
func TestExecuteNilBridge(t *testing.T) {
	var b *Bridge
	out, tier, err := b.Execute(context.Background(), []string{"issue", "list"})
	if err == nil {
		t.Fatal("expected error for nil bridge")
	}
	if tier != TierForbidden {
		t.Errorf("expected TierForbidden for nil bridge, got %s", tier)
	}
	if out != "" {
		t.Errorf("expected empty stdout for nil bridge, got %q", out)
	}
}

// TestHealthNilRunner: NewWithRunner(runner=nil) is a programming
// error. Health() must surface a clean error rather than panic.
func TestHealthNilRunner(t *testing.T) {
	b := &Bridge{Run: nil, Timeout: time.Second}
	err := b.Health(context.Background())
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
}

// TestExecuteAppliesTimeout ensures that a runner that blocks longer
// than the configured timeout gets canceled. We use a runner that
// honors ctx.Done() — production ExecRunner would do the same via
// exec.CommandContext.
func TestExecuteAppliesTimeout(t *testing.T) {
	started := make(chan struct{}, 1)
	runner := Runner(func(ctx context.Context, args []string) (string, string, error) {
		started <- struct{}{}
		<-ctx.Done()
		return "", "canceled", ctx.Err()
	})
	b := NewWithRunner(runner, 50*time.Millisecond)
	_, _, err := b.Execute(context.Background(), []string{"issue", "list"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "deadline") {
		// We accept either the stderr text or the wrapped ctx err.
		t.Logf("note: err = %v (acceptable as long as timeout fired)", err)
	}
	// The runner must have been entered.
	select {
	case <-started:
	default:
		t.Fatal("runner was never invoked")
	}
}

// TestServeLoop covers the happy path: initialize → tools/list →
// tools/call → ping in one session.
func TestServeLoop(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errW := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, errW)
	fake := newFakeRunner(fakeResponse{stdout: "ok"})
	srv.bridgeOnce.Do(func() {
		srv.bridge = NewWithRunner(runForBridge(fake), time.Second)
	})

	// initialize
	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("1"),
		Method:  "initialize",
	})
	// tools/list
	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("2"),
		Method:  "tools/list",
	})
	// tools/call gh_query issue list
	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("3"),
		Method:  "tools/call",
		Params:  mustMarshalParams(t, "gh_query", map[string]any{"args": []string{"issue", "list"}}),
	})
	// ping
	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("4"),
		Method:  "ping",
	})
	// unknown method
	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("5"),
		Method:  "totally/unknown",
	})
	// notification (no ID) — no response expected
	notif, _ := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	})
	in.Write(notif)
	in.Write([]byte("\n"))
	// EOF marker: close the writer side of nothing — we just stop
	// the serve loop by canceling ctx.

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Use io.Pipe-style signaling: the bufio.Scanner will hit EOF
	// once we close the buffer. bytes.Buffer does not implement
	// io.Closer, so we wait for ctx cancel to break the loop. The
	// scanner will block on Scan() — so we cancel from a
	// goroutine.
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- srv.Serve(ctx)
	}()
	// Give the goroutine a moment to process, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-doneCh

	// Parse all 5 responses (notification gets none).
	responses := parseAllResponses(t, out)
	if len(responses) != 5 {
		t.Fatalf("expected 5 responses, got %d (raw=%q)", len(responses), out.String())
	}
	// initialize: protocolVersion
	var init map[string]any
	if err := json.Unmarshal(responses[0].Result, &init); err != nil {
		t.Fatalf("init result: %v", err)
	}
	if init["protocolVersion"] != "2025-06-18" {
		t.Errorf("init protocolVersion = %v", init["protocolVersion"])
	}
	// tools/list: 3 tools
	var list struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(responses[1].Result, &list); err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(list.Tools) != 3 {
		t.Errorf("tools/list returned %d tools, want 3", len(list.Tools))
	}
	wantTools := map[string]bool{"gh_query": false, "gh_execute": false, "gh_health": false}
	for _, ts := range list.Tools {
		wantTools[ts.Name] = true
	}
	for name, seen := range wantTools {
		if !seen {
			t.Errorf("tool %q missing from tools/list", name)
		}
	}
	// tools/call: success
	var callResult toolResult
	if err := json.Unmarshal(responses[2].Result, &callResult); err != nil {
		t.Fatalf("tools/call result: %v", err)
	}
	if callResult.IsError {
		t.Errorf("tools/call gh_query issue list: unexpected isError: %v", callResult.Content)
	}
	// ping: pong
	var pong map[string]string
	if err := json.Unmarshal(responses[3].Result, &pong); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if pong["pong"] != "ok" {
		t.Errorf("ping pong = %q", pong["pong"])
	}
	// unknown method: -32601
	if responses[4].Error == nil || responses[4].Error.Code != -32601 {
		t.Errorf("unknown method: expected -32601, got %+v", responses[4].Error)
	}
}

// TestCallHealthSuccess / Failure cover the gh_health tool.
func TestCallHealthSuccess(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errW := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, errW)
	runner := Runner(func(_ context.Context, _ []string) (string, string, error) {
		return "Logged in to github.com as user", "", nil
	})
	srv.bridgeOnce.Do(func() {
		srv.bridge = NewWithRunner(runner, time.Second)
	})

	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("1"),
		Method:  "tools/call",
		Params:  mustMarshalParams(t, "gh_health", nil),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Serve(ctx); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result: %v", err)
	}
	if result.IsError {
		t.Errorf("expected isError=false, got content: %v", result.Content)
	}
}

func TestCallHealthFailure(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errW := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, errW)
	runner := Runner(func(_ context.Context, _ []string) (string, string, error) {
		return "", "not logged in", errors.New("exit status 1")
	})
	srv.bridgeOnce.Do(func() {
		srv.bridge = NewWithRunner(runner, time.Second)
	})

	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("1"),
		Method:  "tools/call",
		Params:  mustMarshalParams(t, "gh_health", nil),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Serve(ctx); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result: %v", err)
	}
	if !result.IsError {
		t.Error("expected isError=true for failed health check")
	}
}

// TestMCPConfigPathDefault covers the SIN_CODE_HOME / fallback
// branches. The function is hard to test in isolation because it
// reads the env, so we just smoke-test the empty case.
func TestMCPConfigPathDefault(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "")
	p := MCPConfigPath()
	if p == "" {
		t.Fatal("MCPConfigPath returned empty string")
	}
	if !strings.HasSuffix(p, "mcp.json") {
		t.Errorf("MCPConfigPath = %q, want suffix mcp.json", p)
	}
}

func TestMCPConfigPathOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	p := MCPConfigPath()
	want := filepath.Join(dir, "mcp.json")
	if p != want {
		t.Errorf("MCPConfigPath = %q, want %q", p, want)
	}
}

// TestRegisterMCPEmpty exercises the empty-config path (no mcp.json
// on disk).
func TestRegisterMCPEmpty(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mcp.json")
	out, err := RegisterMCP(cfgPath)
	if err != nil {
		t.Fatalf("RegisterMCP: %v", err)
	}
	if out != cfgPath {
		t.Fatalf("got %q, want %q", out, cfgPath)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	servers, _ := got["mcpServers"].(map[string]any)
	if servers == nil {
		t.Fatal("mcpServers missing")
	}
	if _, ok := servers["gh"]; !ok {
		t.Error("gh entry missing in fresh RegisterMCP")
	}
}

// TestRegisterMCPEmptyPath covers the "" path → MCPConfigPath()
// branch. We pre-set SIN_CODE_HOME so the test is hermetic.
func TestRegisterMCPEmptyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	out, err := RegisterMCP("")
	if err != nil {
		t.Fatalf("RegisterMCP(\"\"): %v", err)
	}
	if out != MCPConfigPath() {
		t.Errorf("RegisterMCP(\"\") = %q, want %q", out, MCPConfigPath())
	}
}

// TestEmptySchema + TestMarshalText are tiny smoke tests for the
// shape helpers.
func TestEmptySchema(t *testing.T) {
	s := emptySchema()
	if s["type"] != "object" {
		t.Errorf("emptySchema type = %v, want object", s["type"])
	}
}

// TestCallToolUnknown covers the unknown-tool branch of callTool.
func TestCallToolUnknown(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, &bytes.Buffer{})
	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("1"),
		Method:  "tools/call",
		Params:  mustMarshalParams(t, "gh_does_not_exist", nil),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Serve(ctx); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Serve: %v", err)
	}
	resp := firstResponse(t, out)
	if resp.Error == nil {
		t.Fatal("expected JSON-RPC error for unknown tool")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected code -32602, got %d", resp.Error.Code)
	}
}

// TestServeMalformedLine: a non-JSON line should be skipped (not
// fatal). The Serve loop continues and processes subsequent valid
// requests.
func TestServeMalformedLine(t *testing.T) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errW := &bytes.Buffer{}
	srv := NewServerWithIO(in, out, errW)
	// Garbage line
	in.WriteString("this is not json\n")
	// Valid ping
	writeReq(t, in, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      mustRawID("1"),
		Method:  "ping",
	})
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan error, 1)
	go func() { doneCh <- srv.Serve(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-doneCh
	responses := parseAllResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response (ping), got %d (stderr=%q)", len(responses), errW.String())
	}
}

// ── Plumbing helpers for the additional tests ────────────────────────

func writeReq(t *testing.T, w io.Writer, req jsonRPCRequest) {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}
	if _, err := w.Write(b); err != nil {
		t.Fatalf("write req: %v", err)
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		t.Fatalf("write newline: %v", err)
	}
}

func mustRawID(s string) *json.RawMessage {
	b := json.RawMessage(s)
	return &b
}

func parseAllResponses(t *testing.T, out *bytes.Buffer) []*jsonRPCResponse {
	t.Helper()
	var resp []*jsonRPCResponse
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var r jsonRPCResponse
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("unmarshal response: %v (line=%q)", err, line)
		}
		resp = append(resp, &r)
	}
	return resp
}
