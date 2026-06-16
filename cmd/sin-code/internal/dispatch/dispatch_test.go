// SPDX-License-Identifier: MIT
// Purpose: tests for the dispatch package — argument parsing,
// placeholder substitution, slash-command resolution, dispatcher
// routing, agent selection.
// Docs: dispatch_test.doc.md
package dispatch

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/assets"
)

func testReg() *assets.Registry {
	r := assets.NewRegistry()
	r.AddAll([]*assets.Asset{
		{Kind: assets.KindCommand, Name: "tdd", Body: "Run TDD for: $ARGUMENTS. Focus file $1."},
		{Kind: assets.KindAgent, Name: "go-reviewer", Domain: "go", Body: "You review Go.", Model: "m", Tools: []string{"Read"}},
	})
	return r
}

func TestParseArgs(t *testing.T) {
	a := ParseArgs(`add login --strict --file "auth/login.go"`)
	if len(a.Positional) != 2 || a.Positional[0] != "add" {
		t.Fatalf("positional wrong: %v", a.Positional)
	}
	if a.Flags["strict"] != "true" || a.Flags["file"] != "auth/login.go" {
		t.Fatalf("flags wrong: %v", a.Flags)
	}
}

func TestSubstitute(t *testing.T) {
	a := ParseArgs("foo bar")
	got := a.Substitute("X=$1 Y=$2 ALL=$ARGUMENTS")
	if got != "X=foo Y=bar ALL=foo bar" {
		t.Fatalf("substitute wrong: %q", got)
	}
}

func TestParseSlash(t *testing.T) {
	name, args, ok := ParseSlash("/tdd add login")
	if !ok || name != "tdd" || args != "add login" {
		t.Fatalf("slash parse wrong: %q %q %v", name, args, ok)
	}
	if _, _, ok := ParseSlash("not a command"); ok {
		t.Fatal("expected non-slash")
	}
}

func TestResolveCommand(t *testing.T) {
	rc, err := ResolveCommand(testReg(), "/tdd", "feature auth.go")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.Contains(rc.Prompt, "Run TDD for: feature auth.go") || !strings.Contains(rc.Prompt, "Focus file feature") {
		t.Fatalf("prompt wrong: %q", rc.Prompt)
	}
}

type capturingSink struct{ got string }

func (c *capturingSink) SubmitPrompt(_ context.Context, prompt string, _ []string) error {
	c.got = prompt
	return nil
}

func TestDispatcherSubmitsCommand(t *testing.T) {
	sink := &capturingSink{}
	d := &Dispatcher{Reg: testReg(), Prompts: sink}
	handled, err := d.Dispatch(context.Background(), "/tdd hello")
	if !handled || err != nil {
		t.Fatalf("dispatch failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(sink.got, "hello") {
		t.Fatalf("sink did not receive expanded prompt: %q", sink.got)
	}
}

type stubRunner struct{ inv AgentInvocation }

func (s *stubRunner) RunSubagent(_ context.Context, inv AgentInvocation) (string, error) {
	s.inv = inv
	return "done", nil
}

func TestDelegateToAgent(t *testing.T) {
	runner := &stubRunner{}
	d := &Dispatcher{Reg: testReg(), Agents: runner}
	out, err := d.DelegateToAgent(context.Background(), assets.Context{Domain: "go"}, "review my diff")
	if err != nil || out != "done" {
		t.Fatalf("delegate failed: %q %v", out, err)
	}
	if runner.inv.Name != "go-reviewer" || runner.inv.Task != "review my diff" {
		t.Fatalf("wrong invocation: %+v", runner.inv)
	}
}

func TestResolveAgent_Unknown(t *testing.T) {
	_, err := ResolveAgent(testReg(), "no-one", "task")
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("expected unknown agent error, got %v", err)
	}
}

func TestSelectAndResolveAgent_NoMatch(t *testing.T) {
	_, err := SelectAndResolveAgent(testReg(), assets.Context{Domain: "unknown"}, "task")
	if err == nil || !strings.Contains(err.Error(), "no agent matched") {
		t.Fatalf("expected no-match error, got %v", err)
	}
}

func TestParseArgs_FlagVariants(t *testing.T) {
	// --key=value and a boolean flag at end (no value).
	a := ParseArgs(`pos --flag=val --bool`)
	if len(a.Positional) != 1 || a.Positional[0] != "pos" {
		t.Fatalf("positional wrong: %v", a.Positional)
	}
	if a.Flags["flag"] != "val" || a.Flags["bool"] != "true" {
		t.Fatalf("flags wrong: %v", a.Flags)
	}
}

func TestParseArgs_QuotedPositional(t *testing.T) {
	a := ParseArgs(`pos "quoted"`)
	if len(a.Positional) != 2 || a.Positional[1] != "quoted" {
		t.Fatalf("positional wrong: %v", a.Positional)
	}
}

func TestSubstitute_FlagsAndPositions(t *testing.T) {
	a := Args{Positional: []string{"a", "b"}, Flags: map[string]string{"k": "v"}, Raw: "a b"}
	got := a.Substitute("$1 $2 $3 $ARGUMENTS ${k} $@")
	want := "a b  a b v a b"
	if got != want {
		t.Fatalf("substitute: got %q, want %q", got, want)
	}
}

func TestItoa_Zero(t *testing.T) {
	if itoa(0) != "0" {
		t.Fatalf("itoa(0) = %q", itoa(0))
	}
}

func TestResolveCommand_Unknown(t *testing.T) {
	_, err := ResolveCommand(testReg(), "/unknown", "")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestParseSlash_NoArgs(t *testing.T) {
	name, args, ok := ParseSlash("/tdd")
	if !ok || name != "tdd" || args != "" {
		t.Fatalf("got %q %q %v", name, args, ok)
	}
}

func TestDispatcher_NonSlash(t *testing.T) {
	d := &Dispatcher{Reg: testReg(), Prompts: &capturingSink{}}
	handled, err := d.Dispatch(context.Background(), "hello world")
	if handled || err != nil {
		t.Fatalf("expected non-slash to be unhandled: handled=%v err=%v", handled, err)
	}
}

func TestDispatcher_ResolveError(t *testing.T) {
	d := &Dispatcher{Reg: testReg(), Prompts: &capturingSink{}}
	handled, err := d.Dispatch(context.Background(), "/unknown")
	if !handled || err == nil {
		t.Fatalf("expected command error: handled=%v err=%v", handled, err)
	}
}

func TestDispatcher_NoPromptSink(t *testing.T) {
	d := &Dispatcher{Reg: testReg()}
	handled, err := d.Dispatch(context.Background(), "/tdd x")
	if !handled || err == nil || !strings.Contains(err.Error(), "no prompt sink") {
		t.Fatalf("expected prompt sink error: handled=%v err=%v", handled, err)
	}
}

func TestDelegateToAgent_NoRunner(t *testing.T) {
	d := &Dispatcher{Reg: testReg()}
	_, err := d.DelegateToAgent(context.Background(), assets.Context{Domain: "go"}, "task")
	if err == nil || !strings.Contains(err.Error(), "no subagent runner") {
		t.Fatalf("expected runner error, got %v", err)
	}
}

func TestDelegateToAgent_NoMatch(t *testing.T) {
	d := &Dispatcher{Reg: testReg(), Agents: &stubRunner{}}
	_, err := d.DelegateToAgent(context.Background(), assets.Context{Domain: "unknown"}, "task")
	if err == nil || !strings.Contains(err.Error(), "no agent matched") {
		t.Fatalf("expected no-match error, got %v", err)
	}
}

func TestRunNamedAgent_NoRunner(t *testing.T) {
	d := &Dispatcher{Reg: testReg()}
	_, err := d.RunNamedAgent(context.Background(), "go-reviewer", "task")
	if err == nil || !strings.Contains(err.Error(), "no subagent runner") {
		t.Fatalf("expected runner error, got %v", err)
	}
}

func TestRunNamedAgent_Unknown(t *testing.T) {
	d := &Dispatcher{Reg: testReg(), Agents: &stubRunner{}}
	_, err := d.RunNamedAgent(context.Background(), "unknown", "task")
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("expected unknown agent error, got %v", err)
	}
}

func TestRunNamedAgent_Success(t *testing.T) {
	runner := &stubRunner{}
	d := &Dispatcher{Reg: testReg(), Agents: runner}
	out, err := d.RunNamedAgent(context.Background(), "go-reviewer", "task")
	if err != nil || out != "done" {
		t.Fatalf("unexpected: %q %v", out, err)
	}
	if runner.inv.Name != "go-reviewer" || runner.inv.Task != "task" {
		t.Fatalf("wrong invocation: %+v", runner.inv)
	}
}
