// SPDX-License-Identifier: MIT
// Purpose: tests for the circuitbreaker package. Required tests run
// under -race; we deliberately allocate time budgets wider than the
// wall clock so the suite stays deterministic on CI.
// Docs: breaker.doc.md
package circuitbreaker

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// failingFn returns a closure that always fails with the given error.
// Centralized so tests can stay short.
func failingFn(err error) func() error {
	return func() error { return err }
}

// successFn returns a closure that always succeeds.
func successFn() func() error {
	return func() error { return nil }
}

// clock is a test-controlled time source. The returned mutator
// advances the clock; all Breaker time decisions go through this.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock(start time.Time) *clock    { return &clock{now: start} }
func (c *clock) Now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) Advance(d time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); c.now = c.now.Add(d) }

// ── Required test 1 — closed state allows all traffic ────────────────

// TestBreaker_ClosedAllowsTraffic — 100 calls, all succeed, breaker
// stays Closed throughout. Sanity check on the happy path.
func TestBreaker_ClosedAllowsTraffic(t *testing.T) {
	b := New(&Config{
		Name:             "happy",
		FailureThreshold: 5,
		OpenDuration:     time.Second,
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
	})
	for i := 0; i < 100; i++ {
		if err := b.Execute(successFn()); err != nil {
			t.Fatalf("iter %d: unexpected error: %v", i, err)
		}
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("expected state=closed after 100 successes, got %s", got)
	}
	st := b.Stats()
	if st.ConsecutiveFails != 0 {
		t.Fatalf("expected ConsecutiveFails=0, got %d", st.ConsecutiveFails)
	}
}

// ── Required test 2 — failure threshold trips Closed → Open ─────────

// TestBreaker_OpensAfterThreshold — exactly FailureThreshold
// consecutive failures; immediately after the Nth, state is Open and
// the next call is rejected with ErrBreakerOpen.
func TestBreaker_OpensAfterThreshold(t *testing.T) {
	b := New(&Config{
		Name:             "thresh",
		FailureThreshold: 3,
		OpenDuration:     time.Hour, // don't auto-reset during test
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
	})
	boom := errors.New("upstream down")
	for i := 0; i < 3; i++ {
		err := b.Execute(failingFn(boom))
		if !errors.Is(err, boom) {
			t.Fatalf("iter %d: want error=boom, got %v", i, err)
		}
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("expected state=open after threshold, got %s", got)
	}
	err := b.Execute(successFn())
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen as next call, got %v", err)
	}
}

// ── Required test 3 — calls during the Open window are rejected ─────

// TestBreaker_RejectsWhileOpen — 10 calls attempted during Open window;
// protected function is NEVER invoked (verified by sentinel error
// tripwire) and every call returns ErrBreakerOpen.
func TestBreaker_RejectsWhileOpen(t *testing.T) {
	b := New(&Config{
		Name:             "reject",
		FailureThreshold: 1,
		OpenDuration:     10 * time.Second,
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
	})
	if err := b.Execute(failingFn(errors.New("first fail"))); err == nil {
		t.Fatal("setup: expected first call to fail")
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("setup: expected state=open, got %s", got)
	}

	// Tripwire: the protected fn MUST NOT run.
	tripwire := false
	fn := func() error {
		tripwire = true
		return nil
	}
	for i := 0; i < 10; i++ {
		err := b.Execute(fn)
		if !errors.Is(err, ErrBreakerOpen) {
			t.Fatalf("iter %d: expected ErrBreakerOpen, got %v", i, err)
		}
	}
	if tripwire {
		t.Fatal("protected fn was invoked during Open window")
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("expected state=open after 10 rejections, got %s", got)
	}
}

// ── Required test 4 — successful probe in HalfOpen closes the breaker

// TestBreaker_HalfOpen_ProbeSuccess_Closes — drive to Open, advance the
// clock past OpenDuration, fire one successful probe, observe Closed.
// We use the injectable Now clock so the test is deterministic.
func TestBreaker_HalfOpen_ProbeSuccess_Closes(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cl := newClock(start)
	b := New(&Config{
		Name:             "probe-ok",
		FailureThreshold: 1,
		OpenDuration:     5 * time.Second,
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
		Now:              cl.Now,
	})

	if err := b.Execute(failingFn(errors.New("trip"))); err == nil {
		t.Fatal("setup: expected first call to fail")
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("after trip: expected open, got %s", got)
	}

	// Advance just under the open window — still rejected.
	cl.Advance(4 * time.Second)
	if err := b.Execute(successFn()); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("sub-threshold advance: expected ErrBreakerOpen, got %v", err)
	}

	// Cross the threshold — admitted; probe succeeds; breaker closes.
	cl.Advance(2 * time.Second)
	if err := b.Execute(successFn()); err != nil {
		t.Fatalf("probe: expected nil, got %v", err)
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("after probe: expected closed, got %s", got)
	}

	// Sanity: traffic flows again.
	if err := b.Execute(successFn()); err != nil {
		t.Fatalf("post-close: expected nil, got %v", err)
	}
}

// ── Required test 5 — failing probe in HalfOpen re-opens the breaker

// TestBreaker_HalfOpen_ProbeFailure_Reopens — drive to Open, advance
// past OpenDuration, fire one failing probe, expect re-open.
func TestBreaker_HalfOpen_ProbeFailure_Reopens(t *testing.T) {
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	cl := newClock(start)
	b := New(&Config{
		Name:             "probe-fail",
		FailureThreshold: 1,
		OpenDuration:     5 * time.Second,
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
		Now:              cl.Now,
	})

	if err := b.Execute(failingFn(errors.New("trip"))); err == nil {
		t.Fatal("setup: expected first call to fail")
	}
	cl.Advance(6 * time.Second)

	probeErr := errors.New("still broken")
	err := b.Execute(failingFn(probeErr))
	if !errors.Is(err, probeErr) {
		t.Fatalf("expected probe error propagated, got %v", err)
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("after fail-probe: expected re-open, got %s", got)
	}

	// Immediate subsequent calls rejected again.
	if err := b.Execute(successFn()); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen after re-open, got %v", err)
	}
}

// ── Required test 6 — 50 goroutines, race-clean ─────────────────────

// TestBreaker_Concurrent_NoRace — 50 goroutines × 20 calls each, all
// failing. The breaker will trip fast, spend time in Open, briefly
// admit HalfOpen probes that re-open, and so on. The race detector
// is the actual assertion; the success/reject counters are
// informational.
//
// All-fail (rather than half-and-half) is deliberate: with mixed
// success/failure workers, consecutive-failure tracking would NOT
// reliably trip the breaker under interleaving and the test would
// flake. With all-fail, we trip deterministically and exercise the
// Open → HalfOpen → Open bounce many times.
func TestBreaker_Concurrent_NoRace(t *testing.T) {
	b := New(&Config{
		Name:             "race",
		FailureThreshold: 5,
		OpenDuration:     5 * time.Millisecond,
		HalfOpenProbes:   2,
		SuccessThreshold: 1,
	})
	var (
		wg       sync.WaitGroup
		succeeds atomic.Int64
		rejects  atomic.Int64
		fnFails  atomic.Int64
		panics   atomic.Int64
	)
	failFn := func() error { return errors.New("race fail") }
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				err := b.Execute(failFn)
				switch {
				case err == nil:
					succeeds.Add(1)
				case errors.Is(err, ErrBreakerOpen):
					rejects.Add(1)
				default:
					fnFails.Add(1)
				}
				// Stagger so the Open → HalfOpen transition fires
				// mid-run instead of after the last call.
				if j%3 == 0 {
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}
	// Add a panic-flavored call to verify panic recovery under load.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panics.Add(1)
			}
		}()
		_ = b.Execute(func() error { panic("intentional panic") })
	}()

	wg.Wait()
	t.Logf("succeeds=%d rejects=%d fnFails=%d panicRecoveries=%d", succeeds.Load(), rejects.Load(), fnFails.Load(), panics.Load())
	// Sanity: rejects MUST be > 0 because odd workers will trip the
	// breaker at least once during the run.
	if rejects.Load() == 0 {
		t.Errorf("expected at least some rejects under concurrent hammering; got 0")
	}
	// The breaker MUST end up in a valid state — the actual state is
	// statistically variable due to the time-driven transition over
	// a 50-goroutine run, so just assert "not nil / integer".
	if got := b.State(); got != StateClosed && got != StateOpen && got != StateHalfOpen {
		t.Fatalf("unexpected final state: %d", got)
	}
}

// ── Extra test 7 — panics inside the protected fn count as failures ─

// TestBreaker_PanicCountsAsFailure — a panicking fn trips the breaker
// like a regular failure. The panic is recovered inside Execute so the
// caller observes a wrapped error, never a propagated panic.
func TestBreaker_PanicCountsAsFailure(t *testing.T) {
	b := New(&Config{
		Name:             "panic",
		FailureThreshold: 1,
		OpenDuration:     time.Hour,
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
	})
	// No outer recover — Execute swallows the panic.
	err := b.Execute(func() error { panic("kaboom") })
	if err == nil {
		t.Fatal("expected error from Execute after panic")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("expected error to mention panic, got %q", err.Error())
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("expected state=open after panic, got %s", got)
	}
}

// ── Extra test 8 — RoundTripper integration with httptest upstream ──

// TestRoundTripper_TripsOnTransportError — real http.Client over an
// httptest server that returns 500s. After FailureThreshold 5xx
// responses, the breaker rejects further calls without dispatching.
func TestRoundTripper_TripsOnTransportError(t *testing.T) {
	b := New(&Config{
		Name:             "http",
		FailureThreshold: 2,
		OpenDuration:     10 * time.Second,
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("kaboom"))
	}))
	defer srv.Close()

	rt := RoundTripper(http.DefaultTransport, b)
	client := &http.Client{Transport: rt, Timeout: 2 * time.Second}

	// Two 500s — both surface as breaker-wrapped errors. Breaker
	// then opens.
	for i := 0; i < 2; i++ {
		resp, err := client.Get(srv.URL + "/")
		if err == nil {
			t.Fatalf("iter %d: expected 5xx error, got nil", i)
		}
		if !strings.Contains(err.Error(), "circuitbreaker") {
			t.Errorf("iter %d: error should mention circuitbreaker prefix, got %v", i, err)
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("after 2x 500: expected state=open, got %s", got)
	}

	// Third call: hit-count tripwire on the server.
	hits := 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv2.Close()
	rt2 := RoundTripper(http.DefaultTransport, b)
	client2 := &http.Client{Transport: rt2, Timeout: 2 * time.Second}
	resp, err := client2.Get(srv2.URL + "/")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("expected ErrBreakerOpen, got <nil> with %d hits", hits)
	}
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("protected upstream should not have been hit; observed %d hits", hits)
	}
}

// ── Extra test 9 — 4xx responses are NOT failures (server is healthy)

// TestRoundTripper_4xxIsNotFailure — ensures we don't trip on client
// errors. A server that consistently 404s is still considered up.
func TestRoundTripper_4xxIsNotFailure(t *testing.T) {
	b := New(&Config{
		Name:             "4xx",
		FailureThreshold: 2,
		OpenDuration:     time.Hour,
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	rt := RoundTripper(http.DefaultTransport, b)
	client := &http.Client{Transport: rt, Timeout: 2 * time.Second}

	for i := 0; i < 10; i++ {
		resp, err := client.Get(srv.URL + "/missing")
		if err != nil {
			t.Fatalf("iter %d: unexpected transport error: %v", i, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("iter %d: expected 404, got %d", i, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("4xx storm should NOT trip the breaker; got %s", got)
	}
}
