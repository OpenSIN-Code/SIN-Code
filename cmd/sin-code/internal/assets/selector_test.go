// SPDX-License-Identifier: MIT
// Purpose: tests for the asset selector — domain match, keyword bonus,
// no-match short-circuit.
// Docs: selector_test.doc.md
package assets

import "testing"

func newTestReg() *Registry {
	r := NewRegistry()
	r.AddAll([]*Asset{
		{Kind: KindAgent, Name: "go-reviewer", Domain: "go", Description: "Reviews Go for race conditions"},
		{Kind: KindAgent, Name: "security-reviewer", Domain: "security", Description: "Finds auth and secret leaks"},
		{Kind: KindAgent, Name: "python-reviewer", Domain: "python", Description: "Reviews Python"},
		{Kind: KindCommand, Name: "tdd", Description: "Test driven development loop"},
	})
	return r
}

func TestSelectAgentsByDomain(t *testing.T) {
	sel := NewSelector(newTestReg())
	got := sel.SelectAgents(Context{Domain: "go"}, 3)
	if len(got) == 0 || got[0].Asset.Name != "go-reviewer" {
		t.Fatalf("expected go-reviewer first, got %+v", got)
	}
}

func TestSelectAgentsByKeyword(t *testing.T) {
	sel := NewSelector(newTestReg())
	got := sel.SelectAgents(Context{Domain: "security", Keywords: []string{"auth", "secret"}}, 3)
	if len(got) == 0 || got[0].Asset.Name != "security-reviewer" {
		t.Fatalf("expected security-reviewer first, got %+v", got)
	}
	if got[0].Score <= 10 {
		t.Fatalf("expected keyword bonus on top of domain match, score=%d", got[0].Score)
	}
}

func TestSelectNoMatch(t *testing.T) {
	sel := NewSelector(newTestReg())
	if got := sel.SelectAgents(Context{Domain: "rust"}, 3); len(got) != 0 {
		t.Fatalf("expected no matches for rust, got %+v", got)
	}
}
