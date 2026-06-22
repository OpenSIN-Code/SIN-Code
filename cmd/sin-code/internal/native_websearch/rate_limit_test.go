// SPDX-License-Identifier: MIT
package native_websearch

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiterDefaults(t *testing.T) {
	rl := NewRateLimiter(0, 0)
	if rl.maxTokens != 1 {
		t.Errorf("maxTokens default = %d, want 1", rl.maxTokens)
	}
	if rl.refillRate != 1.0 {
		t.Errorf("refillRate default = %f, want 1.0", rl.refillRate)
	}
}

func TestRateLimiterBurstThenDeny(t *testing.T) {
	rl := NewRateLimiter(2, 100.0)
	if !rl.Allow() || !rl.Allow() {
		t.Fatal("first two Allow calls should both succeed")
	}
	if rl.Allow() {
		t.Fatal("third Allow should return false")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(1, 50.0)
	if !rl.Allow() {
		t.Fatal("first Allow should succeed")
	}
	if rl.Allow() {
		t.Fatal("second Allow should fail (cap=1)")
	}
	time.Sleep(40 * time.Millisecond)
	if !rl.Allow() {
		t.Fatal("after refill window, Allow should succeed")
	}
}

func TestRateLimiterNilSafe(t *testing.T) {
	var rl *RateLimiter
	if !rl.Allow() {
		t.Error("nil.Allow should return true")
	}
	if err := rl.Wait(context.Background()); err != nil {
		t.Errorf("nil.Wait should return nil; got %v", err)
	}
}

func TestRateLimiterWaitWithCtx(t *testing.T) {
	rl := NewRateLimiter(1, 0.5)
	if !rl.Allow() {
		t.Fatal("first Allow should succeed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("Wait returned %v; want nil", err)
	}
	elapsed := time.Since(start)
	if elapsed < 1500*time.Millisecond {
		t.Errorf("Wait returned in %v; refill 0.5/s => ~2s expected", elapsed)
	}
	if elapsed > 3500*time.Millisecond {
		t.Errorf("Wait returned in %v; refill 0.5/s => <3s expected", elapsed)
	}
}

func TestRateLimiterWaitCtxCancel(t *testing.T) {
	rl := NewRateLimiter(1, 0.1)
	if !rl.Allow() {
		t.Fatal("first Allow should succeed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := rl.Wait(ctx)
	if err == nil {
		t.Fatal("Wait returned nil after ctx cancel; want error")
	}
	if err != context.Canceled {
		t.Errorf("Wait returned %v; want context.Canceled", err)
	}
}
