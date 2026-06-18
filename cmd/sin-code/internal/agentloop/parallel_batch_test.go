// SPDX-License-Identifier: MIT
// Purpose: Tests for the parallel batch executor (issue #372).
package agentloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelBatch_BasicExecution(t *testing.T) {
	exec := NewBatchExecutor(4)
	reqs := []BatchRequest{
		{Tool: "sin_read", Args: map[string]any{"path": "a"}},
		{Tool: "sin_read", Args: map[string]any{"path": "b"}},
		{Tool: "sin_read", Args: map[string]any{"path": "c"}},
	}

	results := exec.Execute(context.Background(), reqs,
		func(ctx context.Context, r BatchRequest) (any, error) {
			return r.Args["path"], nil
		})

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("result %d error: %v", i, r.Error)
		}
		if r.Tool != reqs[i].Tool {
			t.Errorf("result %d tool = %q, want %q", i, r.Tool, reqs[i].Tool)
		}
	}
}

func TestParallelBatch_OrderPreserved(t *testing.T) {
	exec := NewBatchExecutor(4)
	reqs := make([]BatchRequest, 10)
	for i := range reqs {
		reqs[i] = BatchRequest{Tool: "sin_read", Args: map[string]any{"idx": i}}
	}

	results := exec.Execute(context.Background(), reqs,
		func(ctx context.Context, r BatchRequest) (any, error) {
			return r.Args["idx"], nil
		})

	for i, r := range results {
		got, ok := r.Result.(int)
		if !ok {
			t.Fatalf("result %d: unexpected type %T", i, r.Result)
		}
		if got != i {
			t.Errorf("result %d = %d, want %d", i, got, i)
		}
	}
}

func TestParallelBatch_ErrorPropagation(t *testing.T) {
	exec := NewBatchExecutor(2)
	reqs := []BatchRequest{
		{Tool: "ok", Args: nil},
		{Tool: "fail", Args: nil},
	}

	results := exec.Execute(context.Background(), reqs,
		func(ctx context.Context, r BatchRequest) (any, error) {
			if r.Tool == "fail" {
				return nil, errors.New("tool failed")
			}
			return "ok", nil
		})

	if results[0].Error != nil {
		t.Errorf("result 0 error: %v", results[0].Error)
	}
	if results[1].Error == nil {
		t.Error("result 1 should have error")
	}
}

func TestParallelBatch_EmptyRequests(t *testing.T) {
	exec := NewBatchExecutor(4)
	results := exec.Execute(context.Background(), nil,
		func(ctx context.Context, r BatchRequest) (any, error) {
			return nil, nil
		})
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestParallelBatch_MaxParallelRespected(t *testing.T) {
	const maxP = 2
	exec := NewBatchExecutor(maxP)

	var current atomic.Int32
	var peak atomic.Int32

	reqs := make([]BatchRequest, 10)
	for i := range reqs {
		reqs[i] = BatchRequest{Tool: "sin_read"}
	}

	exec.Execute(context.Background(), reqs,
		func(ctx context.Context, r BatchRequest) (any, error) {
			c := current.Add(1)
			for {
				p := peak.Load()
				if c <= p || peak.CompareAndSwap(p, c) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			current.Add(-1)
			return nil, nil
		})

	if peak.Load() > int32(maxP) {
		t.Errorf("peak concurrency %d exceeded max %d", peak.Load(), maxP)
	}
}

func TestParallelBatch_ContextCancellation(t *testing.T) {
	exec := NewBatchExecutor(2)
	ctx, cancel := context.WithCancel(context.Background())

	reqs := make([]BatchRequest, 5)
	for i := range reqs {
		reqs[i] = BatchRequest{Tool: "sin_read"}
	}

	cancel()
	results := exec.Execute(ctx, reqs,
		func(ctx context.Context, r BatchRequest) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})

	cancelled := 0
	for _, r := range results {
		if r.Error != nil {
			cancelled++
		}
	}
	if cancelled == 0 {
		t.Error("expected at least some results to have errors after cancellation")
	}
}

func TestParallelBatch_DefaultMaxParallel(t *testing.T) {
	exec := NewBatchExecutor(0)
	if exec.maxParallel != 4 {
		t.Errorf("default maxParallel = %d, want 4", exec.maxParallel)
	}
	exec2 := NewBatchExecutor(-1)
	if exec2.maxParallel != 4 {
		t.Errorf("default maxParallel = %d, want 4", exec2.maxParallel)
	}
}

func TestParallelBatch_ConcurrentSafe(t *testing.T) {
	exec := NewBatchExecutor(8)
	var wg sync.WaitGroup

	// Run multiple Execute calls concurrently to verify thread-safety.
	for run := 0; run < 5; run++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			reqs := make([]BatchRequest, 4)
			for i := range reqs {
				reqs[i] = BatchRequest{Tool: "sin_read"}
			}
			results := exec.Execute(context.Background(), reqs,
				func(ctx context.Context, r BatchRequest) (any, error) {
					return n, nil
				})
			if len(results) != 4 {
				t.Errorf("run %d: expected 4 results, got %d", n, len(results))
			}
		}(run)
	}
	wg.Wait()
}

func TestParallelBatch_DurationRecorded(t *testing.T) {
	exec := NewBatchExecutor(4)
	reqs := []BatchRequest{
		{Tool: "slow"},
	}

	results := exec.Execute(context.Background(), reqs,
		func(ctx context.Context, r BatchRequest) (any, error) {
			time.Sleep(15 * time.Millisecond)
			return nil, nil
		})

	if results[0].Duration < 10*time.Millisecond {
		t.Errorf("duration %v too short, expected >= 10ms", results[0].Duration)
	}
}
