// SPDX-License-Identifier: MIT
// Purpose: tests for synchronous sub-goal delegation (issue #385). The
// -race flag exercises the mutex and Done-channel paths (mandate M7).
package autonomy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewSyncDelegator_DefaultTimeout(t *testing.T) {
	d := NewSyncDelegator(0)
	if d == nil {
		t.Fatal("expected non-nil delegator")
	}
	if d.timeout != 5*time.Minute {
		t.Errorf("expected default 5m timeout, got %s", d.timeout)
	}
	if d.ActiveCount() != 0 {
		t.Errorf("expected 0 active, got %d", d.ActiveCount())
	}
}

func TestNewSyncDelegator_CustomTimeout(t *testing.T) {
	d := NewSyncDelegator(30 * time.Second)
	if d.timeout != 30*time.Second {
		t.Errorf("expected 30s, got %s", d.timeout)
	}
}

func TestSyncDelegation_DelegateAndComplete(t *testing.T) {
	d := NewSyncDelegator(5 * time.Second)
	req := DelegationRequest{Goal: "write-tests", AgentName: "coder"}
	res, err := d.Delegate(context.Background(), req)
	if err != nil {
		t.Fatalf("Delegate: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Goal != "write-tests" {
		t.Errorf("Goal = %q", res.Goal)
	}
	if d.ActiveCount() != 1 {
		t.Errorf("expected 1 active, got %d", d.ActiveCount())
	}

	// Simulate sub-agent completion.
	d.Complete("write-tests", "done", nil)

	got, err := d.Wait("write-tests")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got.Output != "done" {
		t.Errorf("Output = %q, want %q", got.Output, "done")
	}
	if got.Error != nil {
		t.Errorf("Error = %v, want nil", got.Error)
	}
}

func TestSyncDelegation_WaitBlocksUntilComplete(t *testing.T) {
	d := NewSyncDelegator(5 * time.Second)
	req := DelegationRequest{Goal: "slow-task", AgentName: "coder"}
	if _, err := d.Delegate(context.Background(), req); err != nil {
		t.Fatalf("Delegate: %v", err)
	}

	completed := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		d.Complete("slow-task", "finished", nil)
		close(completed)
	}()

	start := time.Now()
	got, err := d.Wait("slow-task")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got.Output != "finished" {
		t.Errorf("Output = %q", got.Output)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("Wait returned too fast: %s", elapsed)
	}
	<-completed
}

func TestSyncDelegation_Timeout(t *testing.T) {
	d := NewSyncDelegator(50 * time.Millisecond)
	req := DelegationRequest{Goal: "never-completes", AgentName: "coder"}
	if _, err := d.Delegate(context.Background(), req); err != nil {
		t.Fatalf("Delegate: %v", err)
	}

	start := time.Now()
	got, err := d.Wait("never-completes")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if got == nil {
		t.Fatal("expected non-nil result on timeout")
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("Wait returned too fast on timeout: %s", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Wait took too long: %s", elapsed)
	}
}

func TestSyncDelegation_WaitNotFound(t *testing.T) {
	d := NewSyncDelegator(5 * time.Second)
	_, err := d.Wait("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent goal")
	}
}

func TestSyncDelegation_DelegateEmptyGoal(t *testing.T) {
	d := NewSyncDelegator(5 * time.Second)
	_, err := d.Delegate(context.Background(), DelegationRequest{Goal: ""})
	if err == nil {
		t.Fatal("expected error for empty goal")
	}
}

func TestSyncDelegation_DelegateDuplicate(t *testing.T) {
	d := NewSyncDelegator(5 * time.Second)
	req := DelegationRequest{Goal: "dup-task", AgentName: "coder"}
	if _, err := d.Delegate(context.Background(), req); err != nil {
		t.Fatalf("first Delegate: %v", err)
	}
	// Second delegate with the same uncompleted goal should fail.
	if _, err := d.Delegate(context.Background(), req); err == nil {
		t.Fatal("expected error for duplicate active delegation")
	}

	// After completion, re-delegating should succeed.
	d.Complete("dup-task", "ok", nil)
	if _, err := d.Delegate(context.Background(), req); err != nil {
		t.Fatalf("re-delegate after completion: %v", err)
	}
}

func TestSyncDelegation_CompleteNotFound(t *testing.T) {
	d := NewSyncDelegator(5 * time.Second)
	// Should not panic.
	d.Complete("nonexistent", "output", nil)
}

func TestSyncDelegation_Concurrent(t *testing.T) {
	d := NewSyncDelegator(10 * time.Second)
	var wg sync.WaitGroup
	const n = 50

	// Spawn N delegations concurrently.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			goal := fmt.Sprintf("task-%d", idx)
			req := DelegationRequest{Goal: goal, AgentName: "coder"}
			if _, err := d.Delegate(context.Background(), req); err != nil {
				t.Errorf("Delegate %d: %v", idx, err)
				return
			}
		}(i)
	}
	wg.Wait()

	if d.ActiveCount() != n {
		t.Errorf("expected %d active, got %d", n, d.ActiveCount())
	}

	// Complete all concurrently.
	var completed int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			goal := fmt.Sprintf("task-%d", idx)
			d.Complete(goal, "ok", nil)
			atomic.AddInt32(&completed, 1)
		}(i)
	}
	wg.Wait()

	if atomic.LoadInt32(&completed) != int32(n) {
		t.Errorf("completed = %d, want %d", completed, n)
	}

	// Wait for all and verify.
	for i := 0; i < n; i++ {
		goal := fmt.Sprintf("task-%d", i)
		res, err := d.Wait(goal)
		if err != nil {
			t.Errorf("Wait %s: %v", goal, err)
			continue
		}
		if res.Output != "ok" {
			t.Errorf("Output %s = %q", goal, res.Output)
		}
	}
}

func TestSyncDelegation_Cleanup(t *testing.T) {
	d := NewSyncDelegator(5 * time.Second)

	// Create and complete one.
	d.Delegate(context.Background(), DelegationRequest{Goal: "done-task"})
	d.Complete("done-task", "ok", nil)

	// Create one still in progress.
	d.Delegate(context.Background(), DelegationRequest{Goal: "pending-task"})

	if d.ActiveCount() != 2 {
		t.Fatalf("expected 2 active before cleanup, got %d", d.ActiveCount())
	}

	d.Cleanup()

	// Completed should be removed; pending should remain.
	if d.ActiveCount() != 1 {
		t.Errorf("expected 1 active after cleanup, got %d", d.ActiveCount())
	}

	// Verify the pending one is still waitable (will timeout quickly).
	d.Complete("pending-task", "late", nil)
	res, err := d.Wait("pending-task")
	if err != nil {
		t.Fatalf("Wait pending: %v", err)
	}
	if res.Output != "late" {
		t.Errorf("Output = %q", res.Output)
	}
}

func TestSyncDelegation_ErrorPropagation(t *testing.T) {
	d := NewSyncDelegator(5 * time.Second)
	d.Delegate(context.Background(), DelegationRequest{Goal: "fail-task"})

	expectedErr := errors.New("sub-agent crashed")
	d.Complete("fail-task", "", expectedErr)

	res, err := d.Wait("fail-task")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Error == nil {
		t.Fatal("expected non-nil error in result")
	}
	if res.Error.Error() != expectedErr.Error() {
		t.Errorf("Error = %v, want %v", res.Error, expectedErr)
	}
	if res.Output != "" {
		t.Errorf("Output = %q, want empty", res.Output)
	}
}
