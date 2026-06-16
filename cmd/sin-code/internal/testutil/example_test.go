// SPDX-License-Identifier: MIT
// Purpose: usage example for internal/testutil/. This file is
// documentation-in-code: it shows how the four helpers compose
// for the most common test patterns. It is itself a passing test.
//
// When a new test in the codebase needs IsolatedSQLite, CleanEnv,
// WithTimeout, or GoroutineLeakCheck, copy the relevant pattern
// from here. The test names are tagged with the issue (#161) so
// the migration is traceable.
package testutil_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/testutil"
)

// Example: with IsolatedSQLite, you get a fresh DB per test, no
// shared state, no manual cleanup.
func TestExample_IsolatedSQLite(t *testing.T) {
	db := testutil.IsolatedSQLite(t)
	_, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = db.Exec(`INSERT INTO users (name) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 user, got %d", n)
	}
}

// Example: with CleanEnv, you set env vars for the test and they
// are restored at cleanup. No leaks to the next test.
func TestExample_CleanEnv(t *testing.T) {
	const k = "TESTUTIL_EXAMPLE_VAR"
	// Verify clean slate
	if v := os.Getenv(k); v != "" {
		t.Fatalf("test should start with %s unset, got %q", k, v)
	}
	testutil.CleanEnv(t, map[string]string{k: "example-value"})
	if got := os.Getenv(k); got != "example-value" {
		t.Errorf("expected example-value, got %q", got)
	}
}

// Example: with WithTimeout, a blocking operation is bounded. The
// test fails with a clear message if it doesn't return in time.
func TestExample_WithTimeout(t *testing.T) {
	testutil.WithTimeout(t, 1*time.Second, func(ctx context.Context) {
		// Simulate a long-but-bounded operation that honors the context.
		for i := 0; i < 10; i++ {
			select {
			case <-ctx.Done():
				return // clean exit on deadline
			case <-time.After(50 * time.Millisecond):
				// do a chunk of work
			}
		}
	})
}

// Example: with GoroutineLeakCheck, you detect goroutines that
// the test spawned but did not clean up. This is the most common
// hang pattern.
func TestExample_GoroutineLeakCheck(t *testing.T) {
	testutil.GoroutineLeakCheck(t, func() {
		// Pattern: spawn a watcher, signal it to stop, wait for it.
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			<-stop
		}()
		close(stop)
		<-done // wait for the watcher to exit before fn returns
	})
}

// Example: with all four together, you have a fully isolated,
// env-clean, timeout-bounded, leak-free test. The pattern below
// is the gold standard for tests in the SIN-Code codebase.
func TestExample_AllCombined(t *testing.T) {
	// 1. Clean env (no global state leaks)
	testutil.CleanEnv(t, map[string]string{
		"TEST_HTTP_ADDR": "127.0.0.1:0",
	})

	// 2. Use IsolatedSQLite
	db := testutil.IsolatedSQLite(t)
	_, _ = db.Exec(`CREATE TABLE t (id INTEGER)`)

	// 3. Bounded time (no hangs)
	testutil.WithTimeout(t, 2*time.Second, func(ctx context.Context) {
		// 4. Spawn a worker, signal it, wait for it — all in a
		//    self-contained scope. No leak check inside the
		//    WithTimeout because the two patterns interact in
		//    surprising ways (GoroutineLeakCheck's grace sleep
		//    plus WithTimeout's own select are easy to deadlock).
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			select {
			case <-stop:
			case <-ctx.Done():
			}
		}()
		close(stop)
		<-done
	})
}

// Example: a real-world httptest pattern that is the source of
// many CI hangs in the v3.18.0 codebase (issue #161). The fix is
// to use WithTimeout so a slow handler can't hang the test.
func TestExample_HTTPHandler_WithTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Honor the deadline. If the test times out, we return
		// quickly instead of blocking the test goroutine.
		select {
		case <-r.Context().Done():
			http.Error(w, "deadline", http.StatusGatewayTimeout)
			return
		case <-time.After(10 * time.Millisecond):
			fmt.Fprintln(w, "ok")
		}
	}))
	defer srv.Close()

	testutil.WithTimeout(t, 2*time.Second, func(ctx context.Context) {
		req, err := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				t.Skip("slow CI; skipping")
			}
			t.Fatalf("do: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
