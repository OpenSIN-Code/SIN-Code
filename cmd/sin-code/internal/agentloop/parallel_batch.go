// SPDX-License-Identifier: MIT
// Purpose: Parallel batching of independent read-only tool calls (issue
// #372). BatchExecutor runs a set of BatchRequests concurrently (bounded
// by maxParallel) and returns results in the same order as the input.
// Thread-safe (mandate M7) — results are written to pre-allocated slots
// protected by per-slot isolation (no shared mutable state beyond the
// results slice, which is written at unique indices).
package agentloop

import (
	"context"
	"sync"
	"time"
)

// BatchRequest represents a single tool call to be executed in a batch.
type BatchRequest struct {
	Tool string
	Args map[string]any
}

// BatchResult holds the outcome of a single batched tool call.
type BatchResult struct {
	Tool     string
	Result   any
	Error    error
	Duration time.Duration
}

// BatchExecutor runs tool calls in parallel with a bounded concurrency
// limit. It is safe for concurrent use (mandate M7).
type BatchExecutor struct {
	maxParallel int
}

// NewBatchExecutor creates a BatchExecutor with the given concurrency
// limit. If maxParallel <= 0, it defaults to 4.
func NewBatchExecutor(maxParallel int) *BatchExecutor {
	if maxParallel <= 0 {
		maxParallel = 4
	}
	return &BatchExecutor{maxParallel: maxParallel}
}

// Execute runs all requests in parallel, bounded by maxParallel, and
// returns results in the same order as the input slice. The exec function
// is called once per request with the context. If the context is
// cancelled, in-flight calls receive the cancellation and remaining
// calls return ctx.Err() as their result error.
func (e *BatchExecutor) Execute(
	ctx context.Context,
	reqs []BatchRequest,
	exec func(context.Context, BatchRequest) (any, error),
) []BatchResult {
	if len(reqs) == 0 {
		return nil
	}

	results := make([]BatchResult, len(reqs))
	sem := make(chan struct{}, e.maxParallel)
	var wg sync.WaitGroup

	for i, req := range reqs {
		wg.Add(1)
		go func(idx int, r BatchRequest) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = BatchResult{
					Tool:  r.Tool,
					Error: ctx.Err(),
				}
				return
			}

			// Check context before starting work.
			if err := ctx.Err(); err != nil {
				results[idx] = BatchResult{
					Tool:  r.Tool,
					Error: err,
				}
				return
			}

			start := time.Now()
			res, err := exec(ctx, r)
			results[idx] = BatchResult{
				Tool:     r.Tool,
				Result:   res,
				Error:    err,
				Duration: time.Since(start),
			}
		}(i, req)
	}

	wg.Wait()
	return results
}
