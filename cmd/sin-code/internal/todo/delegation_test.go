// SPDX-License-Identifier: MIT
// Purpose: tests for the todo delegation tracker (issue #334). All tests
// use the in-memory Delegation directly; the -race flag exercises the
// RWMutex paths (M7).
package todo

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// newTestDelegation returns a Delegation whose session ID generator is
// deterministic so assertions can check exact values.
func newTestDelegation() *Delegation {
	d := NewDelegation()
	var n uint64
	d.sessionIDFn = func() string {
		n++
		return fmt.Sprintf("sess-test-%d", n)
	}
	return d
}

func TestNewDelegationEmpty(t *testing.T) {
	d := NewDelegation()
	all, err := d.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 delegations, got %d", len(all))
	}
}

func TestDelegateCreatesDelegation(t *testing.T) {
	d := newTestDelegation()
	rec, err := d.Delegate("st-aaaa", "coder-sin-swarm")
	if err != nil {
		t.Fatalf("Delegate: %v", err)
	}
	if rec.TodoID != "st-aaaa" {
		t.Errorf("TodoID = %q", rec.TodoID)
	}
	if rec.AgentName != "coder-sin-swarm" {
		t.Errorf("AgentName = %q", rec.AgentName)
	}
	if rec.SessionID == "" {
		t.Error("expected non-empty SessionID")
	}
	if rec.Status != DelegationPending {
		t.Errorf("Status = %q, want pending", rec.Status)
	}
	if rec.DelegatedAt.IsZero() {
		t.Error("expected non-zero DelegatedAt")
	}
}

func TestDelegateEmptyTodoID(t *testing.T) {
	d := newTestDelegation()
	if _, err := d.Delegate("", "agent"); err == nil {
		t.Error("expected error for empty todoID")
	}
}

func TestDelegateEmptyAgentName(t *testing.T) {
	d := newTestDelegation()
	if _, err := d.Delegate("st-aaaa", ""); err == nil {
		t.Error("expected error for empty agentName")
	}
}

func TestDelegatePreventsDoubleDelegation(t *testing.T) {
	d := newTestDelegation()
	if _, err := d.Delegate("st-aaaa", "agent-a"); err != nil {
		t.Fatalf("first Delegate: %v", err)
	}
	_, err := d.Delegate("st-aaaa", "agent-b")
	if err == nil {
		t.Fatal("expected error for re-delegation of active todo")
	}
	if !strings.Contains(err.Error(), "already delegated") {
		t.Errorf("expected 'already delegated' in error, got %v", err)
	}
}

func TestDelegateAllowsRedelegationAfterRecall(t *testing.T) {
	d := newTestDelegation()
	if _, err := d.Delegate("st-aaaa", "agent-a"); err != nil {
		t.Fatalf("first Delegate: %v", err)
	}
	if err := d.Recall("st-aaaa"); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	rec, err := d.Delegate("st-aaaa", "agent-b")
	if err != nil {
		t.Fatalf("re-Delegate after recall: %v", err)
	}
	if rec.AgentName != "agent-b" {
		t.Errorf("AgentName = %q, want agent-b", rec.AgentName)
	}
	if rec.Status != DelegationPending {
		t.Errorf("Status = %q, want pending", rec.Status)
	}
}

func TestRecallSetsStatus(t *testing.T) {
	d := newTestDelegation()
	_, _ = d.Delegate("st-aaaa", "agent")
	if err := d.Recall("st-aaaa"); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	rec, err := d.Status("st-aaaa")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if rec.Status != DelegationRecalled {
		t.Errorf("Status = %q, want recalled", rec.Status)
	}
}

func TestRecallNotFound(t *testing.T) {
	d := newTestDelegation()
	err := d.Recall("st-missing")
	if err == nil {
		t.Fatal("expected error for missing todo")
	}
	if !strings.Contains(err.Error(), "no delegation") {
		t.Errorf("expected 'no delegation' in error, got %v", err)
	}
}

func TestRecallIdempotentOnComplete(t *testing.T) {
	d := newTestDelegation()
	_, _ = d.Delegate("st-aaaa", "agent")
	if err := d.Complete("st-aaaa", "done"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	// Recalling a completed delegation should be a no-op (no error).
	if err := d.Recall("st-aaaa"); err != nil {
		t.Errorf("Recall after complete: %v", err)
	}
	rec, _ := d.Status("st-aaaa")
	if rec.Status != DelegationComplete {
		t.Errorf("Status = %q, want complete", rec.Status)
	}
}

func TestStatusReturnsCopy(t *testing.T) {
	d := newTestDelegation()
	_, _ = d.Delegate("st-aaaa", "agent")
	rec, err := d.Status("st-aaaa")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	rec.Status = DelegationComplete // mutate the returned copy
	again, _ := d.Status("st-aaaa")
	if again.Status != DelegationPending {
		t.Errorf("in-memory record mutated via returned copy: %q", again.Status)
	}
}

func TestStatusNotFound(t *testing.T) {
	d := newTestDelegation()
	if _, err := d.Status("st-missing"); err == nil {
		t.Error("expected error for missing todo")
	}
}

func TestListByAgent(t *testing.T) {
	d := newTestDelegation()
	_, _ = d.Delegate("st-aaaa", "agent-a")
	_, _ = d.Delegate("st-bbbb", "agent-b")
	_, _ = d.Delegate("st-cccc", "agent-a")

	got, err := d.ListByAgent("agent-a")
	if err != nil {
		t.Fatalf("ListByAgent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 delegations for agent-a, got %d", len(got))
	}
	ids := map[string]bool{}
	for _, r := range got {
		if r.AgentName != "agent-a" {
			t.Errorf("returned wrong agent: %q", r.AgentName)
		}
		ids[r.TodoID] = true
	}
	if !ids["st-aaaa"] || !ids["st-cccc"] {
		t.Errorf("missing expected todo IDs, got %v", ids)
	}
}

func TestListByAgentEmpty(t *testing.T) {
	d := newTestDelegation()
	got, err := d.ListByAgent("nobody")
	if err != nil {
		t.Fatalf("ListByAgent: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestListByAgentEmptyName(t *testing.T) {
	d := newTestDelegation()
	if _, err := d.ListByAgent(""); err == nil {
		t.Error("expected error for empty agentName")
	}
}

func TestListAll(t *testing.T) {
	d := newTestDelegation()
	_, _ = d.Delegate("st-aaaa", "agent-a")
	_, _ = d.Delegate("st-bbbb", "agent-b")
	got, err := d.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
}

func TestCompleteSetsStatusAndResult(t *testing.T) {
	d := newTestDelegation()
	_, _ = d.Delegate("st-aaaa", "agent")
	if err := d.Complete("st-aaaa", "all green"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	rec, err := d.Status("st-aaaa")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if rec.Status != DelegationComplete {
		t.Errorf("Status = %q, want complete", rec.Status)
	}
	if rec.Result != "all green" {
		t.Errorf("Result = %q, want 'all green'", rec.Result)
	}
	if rec.CompletedAt == nil {
		t.Error("expected non-nil CompletedAt")
	}
}

func TestCompleteNotFound(t *testing.T) {
	d := newTestDelegation()
	err := d.Complete("st-missing", "x")
	if err == nil {
		t.Fatal("expected error for missing todo")
	}
	if !strings.Contains(err.Error(), "no delegation") {
		t.Errorf("expected 'no delegation' in error, got %v", err)
	}
}

func TestCompleteRejectsRecalled(t *testing.T) {
	d := newTestDelegation()
	_, _ = d.Delegate("st-aaaa", "agent")
	if err := d.Recall("st-aaaa"); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	err := d.Complete("st-aaaa", "done")
	if err == nil {
		t.Fatal("expected error for completing recalled todo")
	}
	if !strings.Contains(err.Error(), "recalled") {
		t.Errorf("expected 'recalled' in error, got %v", err)
	}
}

func TestDelegationConcurrent(t *testing.T) {
	d := newTestDelegation()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("st-%d", i)
			_, _ = d.Delegate(id, "agent")
			_ = d.Complete(id, "ok")
			_, _ = d.Status(id)
			_, _ = d.ListAll()
			_, _ = d.ListByAgent("agent")
		}(i)
	}
	wg.Wait()

	all, _ := d.ListAll()
	if len(all) != 30 {
		t.Errorf("expected 30 delegations after concurrent run, got %d", len(all))
	}
}
