// SPDX-License-Identifier: MIT
// Purpose: Plan-DAG Executor — tasks as a dependency graph with maximal
// safe parallelism. LeaseTable is the admission gate: ready nodes run
// in parallel up to MaxParallel; conflicting nodes serialize via the
// lease conflict signal. Failures cancel only the dependent subtree;
// independent branches keep running.
package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type PlanNode struct {
	Task      *Task
	DependsOn []string
	PathGlobs []string
}

type NodeStatus string

const (
	NodePending NodeStatus = "pending"
	NodeRunning NodeStatus = "running"
	NodeGreen   NodeStatus = "green"
	NodeRed     NodeStatus = "red"
	NodeSkipped NodeStatus = "skipped"
)

type DagResult struct {
	Status map[string]NodeStatus
	Order  []string
}

type NodeRunner func(ctx context.Context, node *PlanNode) error

type DagExecutor struct {
	Leases      *LeaseTable
	MaxParallel int
	AgentID     string
}

func NewDagExecutor(leases *LeaseTable, agentID string) *DagExecutor {
	return &DagExecutor{Leases: leases, MaxParallel: 4, AgentID: agentID}
}

func (d *DagExecutor) Execute(ctx context.Context, nodes []*PlanNode, run NodeRunner) (*DagResult, error) {
	if err := validateDag(nodes); err != nil {
		return nil, err
	}

	res := &DagResult{Status: map[string]NodeStatus{}}
	for _, n := range nodes {
		res.Status[n.Task.ID] = NodePending
	}

	var mu sync.Mutex
	sem := make(chan struct{}, max(1, d.MaxParallel))
	wake := make(chan struct{}, len(nodes))
	var wg sync.WaitGroup
	var pendingActive int

	for {
		mu.Lock()
		launched := 0
		pendingActive = 0
		for _, n := range nodes {
			id := n.Task.ID
			switch res.Status[id] {
			case NodePending:
				if !depsGreen(n, res.Status) {
					if depsDoomed(n, res.Status) {
						res.Status[id] = NodeSkipped
						res.Order = append(res.Order, id)
					} else {
						// Still waiting on a green dep — must not let the loop
						// think work is done; a wake-up from a running node
						// may unblock us on the next iteration.
						pendingActive++
					}
					continue
				}
				lease, conflicts, err := d.Leases.Acquire(
					d.AgentID+":"+id, id, n.Task.Title, leaseGlobs(n))
				if err != nil || len(conflicts) > 0 {
					pendingActive++
					continue
				}
				res.Status[id] = NodeRunning
				launched++
				wg.Add(1)
				go func(n *PlanNode, leaseID int64) {
					defer wg.Done()
					sem <- struct{}{}
					err := run(ctx, n)
					<-sem
					_ = d.Leases.Release(leaseID, d.AgentID+":"+n.Task.ID)

					mu.Lock()
					if err != nil {
						res.Status[n.Task.ID] = NodeRed
					} else {
						res.Status[n.Task.ID] = NodeGreen
					}
					res.Order = append(res.Order, n.Task.ID)
					mu.Unlock()
					wake <- struct{}{}
				}(n, lease.ID)
			case NodeRunning:
				pendingActive++
			}
		}
		done := pendingActive == 0
		mu.Unlock()

		if done {
			break
		}
		if launched == 0 {
			select {
			case <-wake:
			case <-ctx.Done():
				wg.Wait()
				return res, ctx.Err()
			}
		}
	}
	wg.Wait()
	return res, nil
}

func (r *DagResult) Brief() string {
	var b strings.Builder
	counts := map[NodeStatus]int{}
	for _, s := range r.Status {
		counts[s]++
	}
	fmt.Fprintf(&b, "plan DAG: %d green, %d red, %d skipped\n",
		counts[NodeGreen], counts[NodeRed], counts[NodeSkipped])
	fmt.Fprintf(&b, "completion order: %s\n", strings.Join(r.Order, " -> "))
	return b.String()
}

func depsGreen(n *PlanNode, status map[string]NodeStatus) bool {
	for _, dep := range n.DependsOn {
		if status[dep] != NodeGreen {
			return false
		}
	}
	return true
}

func depsDoomed(n *PlanNode, status map[string]NodeStatus) bool {
	for _, dep := range n.DependsOn {
		if s := status[dep]; s == NodeRed || s == NodeSkipped {
			return true
		}
	}
	return false
}

func leaseGlobs(n *PlanNode) []string {
	if len(n.PathGlobs) > 0 {
		return n.PathGlobs
	}
	return []string{"**"}
}

func validateDag(nodes []*PlanNode) error {
	ids := map[string]bool{}
	for _, n := range nodes {
		ids[n.Task.ID] = true
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	adj := map[string][]string{}
	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("dag: task %q depends on unknown %q", n.Task.ID, dep)
			}
			adj[n.Task.ID] = append(adj[n.Task.ID], dep)
		}
	}
	var visit func(string) error
	visit = func(id string) error {
		switch color[id] {
		case gray:
			return fmt.Errorf("dag: cycle through %q", id)
		case black:
			return nil
		}
		color[id] = gray
		for _, next := range adj[id] {
			if err := visit(next); err != nil {
				return err
			}
		}
		color[id] = black
		return nil
	}
	for id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
