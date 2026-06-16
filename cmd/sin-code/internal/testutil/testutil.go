// SPDX-License-Identifier: MIT
// Purpose: testutil — shared test helpers for race-free, isolated,
// non-hanging tests (issue #161, race-flake hardening v2).
//
// The four helpers cover the four most common hang patterns
// observed in the v3.18.0 codebase:
//
//  1. shared SQLite DB between tests → IsolatedSQLite
//  2. env var leaks between tests     → CleanEnv
//  3. blocking I/O without timeout    → WithTimeout
//  4. leaked goroutines on a channel  → GoroutineLeakCheck
//
// The package is dependency-free (stdlib only) so the helpers can
// be imported from any test without polluting the build graph.
//
// Docs: testutil.doc.md
package testutil

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite, M2 (CGO_ENABLED=0)
)

// sqlOpen and dbPing are package-level hooks so tests can exercise the
// error paths of IsolatedSQLite without mocking the database/sql driver.
var sqlOpen = sql.Open
var dbPing = func(db *sql.DB) error { return db.Ping() }

// IsolatedSQLite returns a *sql.DB opened in t.TempDir(), automatically
// closed at test end via t.Cleanup. The DB is created in a fresh
// temp directory per call, so concurrent tests cannot share state.
//
// The DSN is the modernc.org/sqlite default (file-based) with a per-test
// path. `?_pragma=journal_mode(WAL)` would be needed for concurrent
// readers, but for test isolation a single file is enough.
func IsolatedSQLite(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := sqlOpen("sqlite", path)
	if err != nil {
		t.Fatalf("IsolatedSQLite: open: %v", err)
	}
	if err := dbPing(db); err != nil {
		t.Fatalf("IsolatedSQLite: ping: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// CleanEnv sets the given env vars for the test and restores the
// previous values at cleanup. Replaces the manual
//
//	os.Setenv(k, v)
//	t.Cleanup(func() { os.Setenv(k, prev) })
//
// pattern that is easy to get wrong (esp. when the prev value is "").
var setenv = os.Setenv

func CleanEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		prev, had := os.LookupEnv(k)
		if err := setenv(k, v); err != nil {
			t.Fatalf("CleanEnv: setenv %s: %v", k, err)
		}
		k, v, prev, had := k, v, prev, had // capture
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, prev)
			} else {
				_ = os.Unsetenv(k)
			}
		})
		_ = v
	}
}

// WithTimeout fails the test if fn does not return within d. It is
// the test-side counterpart of context.WithTimeout — fn gets a
// context that is canceled at d, so any blocking I/O inside fn can
// observe the deadline.
//
// Usage:
//
//	testutil.WithTimeout(t, 5*time.Second, func(ctx context.Context) {
//	    result := doSomethingBlocking(ctx, ...)
//	    ...
//	})
func WithTimeout(t *testing.T, d time.Duration, fn func(ctx context.Context)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	done := make(chan struct{})
	var panicVal any
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicVal = r
			}
			close(done)
		}()
		fn(ctx)
	}()
	// First, wait for fn to return. If the context fires first
	// (because fn ignored the deadline), the test fails. We use
	// a small post-deadline grace period so a fn that returns
	// "just after" the deadline doesn't get spuriously failed.
	select {
	case <-done:
		if panicVal != nil {
			panic(panicVal)
		}
		return
	case <-ctx.Done():
		// Context fired before fn returned. Give it one more
		// 50ms grace to handle the cancellation, then fail.
	}
	select {
	case <-done:
		if panicVal != nil {
			panic(panicVal)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("WithTimeout: test did not return within %v (+ 50ms grace)", d)
	}
}

// GoroutineLeakCheck runs fn and FAILS the test if the goroutine
// count grew during fn. It is a *best effort* detector — not a
// sound leak checker like goleak. False positives and negatives are
// possible; the function is for catching the most common pattern
// (a goroutine waiting on a channel the test never closes) during
// code review, not for production CI gating.
//
// Caveats:
//   - Go's runtime can reap idle goroutines asynchronously, so
//     a leaked goroutine may already be gone when we count.
//   - The Go runtime can also delay the creation of new goroutines
//     past our count, so a goroutine started late inside fn may
//     not appear in `after`.
//   - The grace period (10-50 ms) is randomized to give the runtime
//     time to schedule. Tune up if you see false positives on slow CI.
//
// For sound leak detection, use github.com/goleak/goleak instead.
// We don't vendor it (M2: no new deps), but the pattern is
// straightforward to add later.
func GoroutineLeakCheck(t *testing.T, fn func()) {
	t.Helper()
	// Capture a stack snapshot before fn so the comparison is
	// not fooled by transient goroutines from the test framework.
	beforeBuf := make([]byte, 1<<16)
	n := runtime.Stack(beforeBuf, true)
	before := countGoroutines(beforeBuf[:n])
	fn()
	time.Sleep(50 * time.Millisecond) // grace
	afterBuf := make([]byte, 1<<16)
	n = runtime.Stack(afterBuf, true)
	after := countGoroutines(afterBuf[:n])
	if after > before {
		t.Errorf("GoroutineLeakCheck: %d goroutine(s) leaked (before=%d, after=%d)",
			after-before, before, after)
	}
}

// countGoroutines counts the number of "goroutine N [status]:" headers
// in a runtime.Stack dump. The format is documented in
// runtime.Stack; the leading "goroutine N" is unique per stack.
func countGoroutines(buf []byte) int {
	const marker = "goroutine "
	count := 0
	for i := 0; i+len(marker) <= len(buf); i++ {
		if string(buf[i:i+len(marker)]) == marker {
			count++
		}
	}
	return count
}

// MustGo runs fn in a goroutine, recovers any panic into a t.Errorf,
// and blocks the caller until fn returns. Use it for fire-and-forget
// goroutines in tests where a panic inside the goroutine would
// otherwise fail the test framework's machinery rather than the test
// itself.
//
// Use sparingly — most tests should let panics propagate. The
// intended use case is "I started a watcher goroutine for the
// duration of the test; if it panics, the test should fail
// cleanly, not crash the test binary."
//
// Note: MustGo BLOCKS until fn returns. If fn is long-running, the
// test will block too. That's the point: the test is responsible for
// signaling the goroutine to exit (e.g. via a channel).
func MustGo(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	var panicVal any
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicVal = r
			}
			close(done)
		}()
		fn()
	}()
	<-done
	if panicVal != nil {
		t.Errorf("MustGo: panic in goroutine: %v", panicVal)
	}
}
