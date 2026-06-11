// SPDX-License-Identifier: MIT
package orchestrator

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type assertErr struct{ msg string }

func (e *assertErr) Error() string { return e.msg }

func assertAnError(msg string) error { return &assertErr{msg: msg} }

func node(id string, deps []string, globs ...string) *PlanNode {
	return &PlanNode{
		Task:      &Task{ID: id, Title: "task " + id},
		DependsOn: deps,
		PathGlobs: globs,
	}
}

func TestDagRunsIndependentNodesInParallel(t *testing.T) {
	lt := NewLeaseTable()
	exec := NewDagExecutor(lt, "agent-a")
	exec.MaxParallel = 8

	var inFlight, peak int64
	gate := make(chan struct{})
	started := make(chan struct{}, 8)

	run := func(ctx context.Context, n *PlanNode) error {
		started <- struct{}{}
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			old := atomic.LoadInt64(&peak)
			if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
				break
			}
		}
		<-gate
		atomic.AddInt64(&inFlight, -1)
		return nil
	}

	nodes := []*PlanNode{
		node("a", nil, "pkg/a/**"),
		node("b", nil, "pkg/b/**"),
		node("c", nil, "pkg/c/**"),
	}

	type execOut struct {
		res *DagResult
		err error
	}
	done := make(chan execOut, 1)
	go func() {
		r, err := exec.Execute(context.Background(), nodes, run)
		done <- execOut{r, err}
	}()

	// Wait for all 3 runners to be in flight and blocked on `gate`.
	for i := 0; i < len(nodes); i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for runner %d to start (peak so far=%d)", i, atomic.LoadInt64(&peak))
		}
	}
	close(gate)

	out := <-done
	if out.err != nil {
		t.Fatalf("Execute: %v", out.err)
	}
	for _, n := range nodes {
		if got := out.res.Status[n.Task.ID]; got != NodeGreen {
			t.Errorf("node %s: want %s, got %s", n.Task.ID, NodeGreen, got)
		}
	}
	if peak < 2 {
		t.Errorf("expected peak concurrency >= 2, got %d", peak)
	}
}

func TestDagRespectsDependencies(t *testing.T) {
	lt := NewLeaseTable()
	exec := NewDagExecutor(lt, "agent-a")

	var mu sync.Mutex
	var order []string
	run := func(ctx context.Context, n *PlanNode) error {
		mu.Lock()
		order = append(order, n.Task.ID)
		mu.Unlock()
		return nil
	}

	a := node("a", nil, "pkg/a/**")
	b := node("b", []string{"a"}, "pkg/b/**")
	c := node("c", []string{"b"}, "pkg/c/**")

	if _, err := exec.Execute(context.Background(), []*PlanNode{a, b, c}, run); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	want := []string{"a", "b", "c"}
	if len(order) != len(want) {
		t.Fatalf("execution order length: want %d, got %d (%v)", len(want), len(order), order)
	}
	for i, id := range want {
		if order[i] != id {
			t.Errorf("position %d: want %s, got %s (full=%v)", i, id, order[i], order)
		}
	}
}

func TestDagFailurePropagatesOnlyToSubtree(t *testing.T) {
	lt := NewLeaseTable()
	exec := NewDagExecutor(lt, "agent-a")

	a := node("a", nil, "pkg/a/**")
	b := node("b", []string{"a"}, "pkg/b/**")
	c := node("c", nil, "pkg/c/**")
	d := node("d", []string{"c"}, "pkg/d/**")

	run := func(ctx context.Context, n *PlanNode) error {
		if n.Task.ID == "a" {
			return assertAnError("a boom")
		}
		return nil
	}

	res, err := exec.Execute(context.Background(), []*PlanNode{a, b, c, d}, run)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := map[string]NodeStatus{
		"a": NodeRed,
		"b": NodeSkipped,
		"c": NodeGreen,
		"d": NodeGreen,
	}
	for id, st := range want {
		if got := res.Status[id]; got != st {
			t.Errorf("node %s: want %s, got %s", id, st, got)
		}
	}
}

func TestDagRejectsCycles(t *testing.T) {
	lt := NewLeaseTable()
	exec := NewDagExecutor(lt, "agent-a")

	a := node("a", []string{"b"}, "pkg/a/**")
	b := node("b", []string{"a"}, "pkg/b/**")

	_, err := exec.Execute(context.Background(), []*PlanNode{a, b}, func(context.Context, *PlanNode) error { return nil })
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected error to mention 'cycle', got %q", err.Error())
	}
}

func TestDagRejectsDanglingDep(t *testing.T) {
	lt := NewLeaseTable()
	exec := NewDagExecutor(lt, "agent-a")

	a := node("a", []string{"nonexistent"}, "pkg/a/**")

	_, err := exec.Execute(context.Background(), []*PlanNode{a}, func(context.Context, *PlanNode) error { return nil })
	if err == nil {
		t.Fatal("expected unknown-dependency error, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("expected error to mention missing dep name, got %q", err.Error())
	}
}

func TestDagSerializesLeaseConflicts(t *testing.T) {
	lt := NewLeaseTable()
	exec := NewDagExecutor(lt, "agent-a")
	exec.MaxParallel = 8

	var inFlight, peak, inCount int64
	gate := make(chan struct{})

	run := func(ctx context.Context, n *PlanNode) error {
		atomic.AddInt64(&inCount, 1)
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			old := atomic.LoadInt64(&peak)
			if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
				break
			}
		}
		<-gate
		atomic.AddInt64(&inFlight, -1)
		return nil
	}

	nodes := []*PlanNode{
		node("a", nil, "shared/**"),
		node("b", nil, "shared/**"),
	}

	type execOut struct {
		res *DagResult
		err error
	}
	done := make(chan execOut, 1)
	go func() {
		r, err := exec.Execute(context.Background(), nodes, run)
		done <- execOut{r, err}
	}()

	// Wait for the first runner to be in flight and blocked on `gate`.
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt64(&inCount) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for first runner to enter")
		}
		time.Sleep(2 * time.Millisecond)
	}
	// Give the executor time to (try to) launch the second node; it must
	// not, because of the lease conflict.
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt64(&inCount); got != 1 {
		t.Fatalf("expected exactly 1 runner in-flight due to lease conflict, got %d", got)
	}
	close(gate)

	out := <-done
	if out.err != nil {
		t.Fatalf("Execute: %v", out.err)
	}
	for _, n := range nodes {
		if got := out.res.Status[n.Task.ID]; got != NodeGreen {
			t.Errorf("node %s: want %s, got %s", n.Task.ID, NodeGreen, got)
		}
	}
	if peak != 1 {
		t.Errorf("expected serialized execution (peak == 1), got %d", peak)
	}
}

func TestDagSkippedNodeIsRecordedInOrder(t *testing.T) {
	lt := NewLeaseTable()
	exec := NewDagExecutor(lt, "agent-a")

	a := node("a", nil, "pkg/a/**")
	b := node("b", []string{"a"}, "pkg/b/**")
	c := node("c", []string{"b"}, "pkg/c/**")

	run := func(ctx context.Context, n *PlanNode) error {
		if n.Task.ID == "a" {
			return assertAnError("a boom")
		}
		return nil
	}

	res, err := exec.Execute(context.Background(), []*PlanNode{a, b, c}, run)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for _, id := range []string{"a", "b", "c"} {
		found := false
		for _, x := range res.Order {
			if x == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected id %q in res.Order, got %v", id, res.Order)
		}
	}

	redSeen := false
	for i, id := range res.Order {
		if id == "a" {
			redSeen = true
		}
		if id == "b" && !redSeen {
			t.Errorf("b (skipped) recorded at %d before a (red) at: %v", i, res.Order)
		}
	}
}

func TestDagBriefRendersCounts(t *testing.T) {
	lt := NewLeaseTable()
	exec := NewDagExecutor(lt, "agent-a")

	a := node("a", nil, "pkg/a/**")
	b := node("b", []string{"a"}, "pkg/b/**")
	c := node("c", []string{"b"}, "pkg/c/**")
	d := node("d", nil, "pkg/d/**")

	run := func(ctx context.Context, n *PlanNode) error {
		if n.Task.ID == "b" {
			return assertAnError("b boom")
		}
		return nil
	}

	res, err := exec.Execute(context.Background(), []*PlanNode{a, b, c, d}, run)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	brief := res.Brief()
	for _, want := range []string{"green", "red", "skipped"} {
		if !strings.Contains(brief, want) {
			t.Errorf("Brief() missing %q: %q", want, brief)
		}
	}

	statuses := make([]NodeStatus, 0, len(res.Status))
	for _, s := range res.Status {
		statuses = append(statuses, s)
	}
	sort.Slice(statuses, func(i, j int) bool { return string(statuses[i]) < string(statuses[j]) })
	counts := map[NodeStatus]int{}
	for _, s := range statuses {
		counts[s]++
	}
	if counts[NodeGreen] == 0 || counts[NodeRed] == 0 || counts[NodeSkipped] == 0 {
		t.Errorf("expected non-zero green/red/skipped, got %v", counts)
	}
}
