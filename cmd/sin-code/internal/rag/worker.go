// SPDX-License-Identifier: MIT
// Purpose: bounded-concurrency worker pool for embedding generation
// (issue #160, M7: "the embedding goroutine is a worker pool with
// bounded concurrency — the agent loop never blocks").
//
// The pool accepts Embed jobs via Embed(), returns results via
// a future-style channel per call, and rejects new work when
// closed. The pool size defaults to 4 (the same as the orchestrator
// max-parallel default) and is configurable.
package rag

import (
	"context"
	"errors"
	"sync"
)

// ErrPoolClosed is returned by Embed after Close.
var ErrPoolClosed = errors.New("rag: worker pool closed")

// job is one embedding request. The result channel is buffered
// (size 1) so the worker can send without blocking, and the
// caller can read without coordinating with the worker.
type job struct {
	ctx  context.Context
	text string
	done chan result
}

type result struct {
	vec []float32
	err error
}

// WorkerPool is the bounded-concurrency embedder.
type WorkerPool struct {
	embedder  Embedder
	queue     chan job
	wg        sync.WaitGroup
	closeOnce sync.Once
	closed    chan struct{}
}

// NewWorkerPool starts `size` workers, each calling
// embedder.Embed for incoming jobs. size <= 0 falls back to 1
// (single worker, still useful for tests).
func NewWorkerPool(embedder Embedder, size int) *WorkerPool {
	if size <= 0 {
		size = 1
	}
	p := &WorkerPool{
		embedder: embedder,
		queue:    make(chan job, size*4), // 4x buffer for burst
		closed:   make(chan struct{}),
	}
	for i := 0; i < size; i++ {
		p.wg.Add(1)
		go p.run()
	}
	return p
}

// run is one worker. It reads jobs until the queue is closed and
// the channel is drained.
func (p *WorkerPool) run() {
	defer p.wg.Done()
	for j := range p.queue {
		vec, err := p.embedder.Embed(j.ctx, j.text)
		// Send on a size-1 buffered channel; the caller is
		// guaranteed to be reading (or have given up). If the
		// caller gave up, the goroutine just blocks on send
		// until the test process exits — acceptable because
		// each Embed call uses at most 384 floats (~1.5 KB).
		select {
		case j.done <- result{vec: vec, err: err}:
		case <-j.ctx.Done():
			// Caller's context expired; result is dropped.
		}
		close(j.done)
	}
}

// Embed submits text to the pool and returns the embedding. The
// returned context is honored: if ctx is canceled before the
// worker picks the job up, Embed returns ctx.Err().
//
// Blocks until the worker finishes, the context is canceled, or
// the pool is closed.
func (p *WorkerPool) Embed(ctx context.Context, text string) ([]float32, error) {
	select {
	case <-p.closed:
		return nil, ErrPoolClosed
	default:
	}
	j := job{ctx: ctx, text: text, done: make(chan result, 1)}
	select {
	case p.queue <- j:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.closed:
		return nil, ErrPoolClosed
	}
	select {
	case r := <-j.done:
		return r.vec, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close stops accepting new jobs, drains the queue, and waits
// for all workers to exit. Idempotent.
func (p *WorkerPool) Close() error {
	p.closeOnce.Do(func() {
		close(p.closed)
		close(p.queue)
	})
	p.wg.Wait()
	return nil
}

// QueueDepth returns the current queue size. Useful for tests
// and operator-facing diagnostics (`sin instinct stats`).
func (p *WorkerPool) QueueDepth() int {
	return len(p.queue)
}
