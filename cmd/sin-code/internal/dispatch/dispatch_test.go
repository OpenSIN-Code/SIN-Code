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
