// SPDX-License-Identifier: MIT
// Purpose: thread-safe token-bucket rate limiter for native websearch
// (issue #381). Pure stdlib — no golang.org/x/time/rate dependency, in
// keeping with mandate M2. Bucket state lives in atomic int64 fields
// so an Allow() call is a single CAS without touching a mutex; Wait()
// falls back to a sync.Cond wait so heavy callers block at the same
// per-second rate instead of busy-spinning.
package native_websearch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimiter is a deterministic token bucket. Allow() is the fast path:
// a single CAS on the tokens counter. Wait() is the slow path: it
// sleeps against a sync.Cond so bursts of callers coalesce on the same
// refill instant. The zero value is unusable; construct via NewRateLimiter.
type RateLimiter struct {
	tokens     atomic.Int64
	maxTokens  int64
	refillRate float64
	lastRefill atomic.Int64

	condMu sync.Mutex
	cond   *sync.Cond
}

// NewRateLimiter returns a RateLimiter that allows up to `burst` calls
// in a single instant and refills at `perSecond` tokens per second.
// burst <= 0 collapses to 1; perSecond <= 0 collapses to 1.0 (one
// call per second, the strictest sane default for the DuckDuckGo HTML
// endpoint where aggressive scraping earns a 429).
func NewRateLimiter(burst int, perSecond float64) *RateLimiter {
	if burst <= 0 {
		burst = 1
	}
	if perSecond <= 0 {
		perSecond = 1.0
	}
	rl := &RateLimiter{
		maxTokens:  int64(burst),
		refillRate: perSecond,
	}
	rl.tokens.Store(int64(burst))
	rl.lastRefill.Store(time.Now().UnixNano())
	rl.cond = sync.NewCond(&rl.condMu)
	return rl
}

// refill brings the bucket back to its current capacity given the elapsed
// wall-clock since lastRefill. Caller must hold condMu.
func (r *RateLimiter) refill(now time.Time) {
	last := time.Unix(0, r.lastRefill.Load())
	elapsed := now.Sub(last).Seconds()
	if elapsed <= 0 {
		return
	}
	gained := int64(elapsed * r.refillRate)
	if gained <= 0 {
		return
	}
	cur := r.tokens.Load()
	for cur < r.maxTokens {
		next := cur + gained
		if next > r.maxTokens {
			next = r.maxTokens
		}
		if r.tokens.CompareAndSwap(cur, next) {
			r.lastRefill.Store(now.UnixNano())
			return
		}
		cur = r.tokens.Load()
	}
}

// Allow returns true if a token can be consumed without waiting; false
// if the caller should fall back to Wait() or back off entirely.
func (r *RateLimiter) Allow() bool {
	if r == nil {
		return true
	}
	r.condMu.Lock()
	r.refill(time.Now())
	r.condMu.Unlock()
	for {
		cur := r.tokens.Load()
		if cur <= 0 {
			return false
		}
		if r.tokens.CompareAndSwap(cur, cur-1) {
			return true
		}
	}
}

// reserve atomically deducts one token if any are available. Caller
// must hold condMu.
func (r *RateLimiter) reserve(now time.Time) bool {
	r.refill(now)
	cur := r.tokens.Load()
	for cur > 0 {
		if r.tokens.CompareAndSwap(cur, cur-1) {
			return true
		}
		cur = r.tokens.Load()
	}
	return false
}

// Wait blocks until a token is available or ctx fires. Returns
// ctx.Err() when the context cancels before a token is freed.
// The returned error is non-nil only on context cancellation; a clean
// token hop returns nil.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for {
		r.condMu.Lock()
		now := time.Now()
		if r.reserve(now) {
			r.condMu.Unlock()
			return nil
		}
		wait := time.Duration(float64(time.Second) / r.refillRate)
		if wait < 10*time.Millisecond {
			wait = 10 * time.Millisecond
		}
		r.condMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			r.condMu.Lock()
			r.cond.Broadcast()
			r.condMu.Unlock()
		}
	}
}

// Tokens returns the current bucket level (snapshot). Useful for tests
// asserting that Allow() and Wait() actually deduct against the same
// counter and that refill restores it.
func (r *RateLimiter) Tokens() int64 {
	if r == nil {
		return 0
	}
	r.condMu.Lock()
	defer r.condMu.Unlock()
	r.refill(time.Now())
	return r.tokens.Load()
}

// ErrRateLimited is returned by Search when the rate limiter refuses
// the call AND the caller opted not to block. Callers should map this
// to a retry-with-backoff path or a graceful "no results" surface.
var ErrRateLimited = errors.New("native_websearch: rate limited")

// Cap is the canonical burst ceiling for the DuckDuckGo HTML endpoint.
// Public so command-line flags can read it directly without duplicating
// magic numbers in profile loaders.
const Cap = 5

// PerSecond is the canonical refill rate for the DuckDuckGo HTML endpoint.
// 1 request/second stays comfortably under the engine's anti-abuse ceiling.
const PerSecond = 1.0
