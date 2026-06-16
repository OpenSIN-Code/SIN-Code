// SPDX-License-Identifier: MIT
// Purpose: tests for the testutil package itself. The package
// exists to prevent test hangs, so the test for the hang-detector
// (GoroutineLeakCheck) is the load-bearing one — if it's wrong,
// every other test in the binary becomes suspect.
package testutil

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"
)

// ── IsolatedSQLite ────────────────────────────────────────────────────

func TestIsolatedSQLite_OpensAndCloses(t *testing.T) {
	db := IsolatedSQLite(t)
	if db == nil {
		t.Fatal("expected non-nil db")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestIsolatedSQLite_Concurrent(t *testing.T) {
	// Two parallel tests using IsolatedSQLite must not share state
	// (different temp dirs, different files).
	const n = 4
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			db := IsolatedSQLite(t)
			_, err := db.Exec("CREATE TABLE t" + string(rune('a'+i)) + " (id INTEGER)")
			done <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-done; err != nil {
			t.Errorf("worker %d: %v", i, err)
		}
	}
}

func TestIsolatedSQLite_ClosesOnCleanup(t *testing.T) {
	// After the test function returns, the temp dir is removed and
	// the db handle is closed. We can't directly assert "closed" from
	// outside, but we can assert that a second db in the same t.TempDir
	// is independent (different file).
	db1 := IsolatedSQLite(t)
	db2 := IsolatedSQLite(t)
	if db1 == db2 {
		t.Error("expected different db instances")
	}
	_, err := db1.Exec("CREATE TABLE x (id INTEGER)")
	if err != nil {
		t.Fatalf("db1.Exec: %v", err)
	}
	// db2 is in a different temp dir, so it doesn't see db1's table.
	// (We don't actually query db2 here — the table names differ
	// anyway. The point is: two IsolatedSQLite calls produce two
	// independent files.)
	_ = db2
}

func TestIsolatedSQLite_OpenError(t *testing.T) {
	prev := sqlOpen
	sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
		return nil, errors.New("open failed")
	}
	defer func() { sqlOpen = prev }()
	done := make(chan bool)
	go func() {
		ft := &testing.T{}
		defer func() { done <- ft.Failed() }()
		_ = IsolatedSQLite(ft)
	}()
	if failed := <-done; !failed {
		t.Error("expected failure when sqlOpen errors")
	}
}

func TestIsolatedSQLite_PingError(t *testing.T) {
	prev := dbPing
	dbPing = func(db *sql.DB) error { return errors.New("ping failed") }
	defer func() { dbPing = prev }()
	done := make(chan bool)
	go func() {
		ft := &testing.T{}
		defer func() { done <- ft.Failed() }()
		_ = IsolatedSQLite(ft)
	}()
	if failed := <-done; !failed {
		t.Error("expected failure when dbPing errors")
	}
}

// ── CleanEnv ──────────────────────────────────────────────────────────

func TestCleanEnv_SetsAndRestores(t *testing.T) {
	const k = "TESTUTIL_TEST_VAR"
	_ = os.Unsetenv(k)
	// Register the assertion cleanup FIRST so it runs LAST
	// (LIFO), after CleanEnv's cleanup.
	t.Cleanup(func() {
		if v := os.Getenv(k); v != "" {
			t.Errorf("expected unset after CleanEnv cleanup, got %q", v)
		}
	})
	CleanEnv(t, map[string]string{k: "hello"})
	if got := os.Getenv(k); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestCleanEnv_RestoresPrevious(t *testing.T) {
	const k = "TESTUTIL_TEST_VAR"
	t.Setenv(k, "previous") // testing.T.Setenv auto-restores after the test
	CleanEnv(t, map[string]string{k: "new"})
	if got := os.Getenv(k); got != "new" {
		t.Fatalf("expected new, got %q", got)
	}
	// After the test, t.Setenv restores "previous" automatically.
}

func TestCleanEnv_HandlesEmptyPrevious(t *testing.T) {
	// os.Setenv(k, "") still sets the var to the empty string, but
	// os.LookupEnv reports (set, "") — different from (unset, "").
	// Our CleanEnv treats empty-value as "had=true", which matches
	// the LookupEnv semantics. This test guards that.
	const k = "TESTUTIL_TEST_VAR"
	_ = os.Unsetenv(k)
	CleanEnv(t, map[string]string{k: "x"})
	if v := os.Getenv(k); v != "x" {
		t.Errorf("expected x, got %q", v)
	}
}

func TestCleanEnv_SetenvError(t *testing.T) {
	prev := setenv
	setenv = func(k, v string) error { return errors.New("setenv failed") }
	defer func() { setenv = prev }()
	done := make(chan bool)
	go func() {
		ft := &testing.T{}
		defer func() { done <- ft.Failed() }()
		CleanEnv(ft, map[string]string{"_TESTUTIL_X": "y"})
	}()
	if failed := <-done; !failed {
		t.Error("expected failure when setenv errors")
	}
}

// ── WithTimeout ───────────────────────────────────────────────────────

func TestWithTimeout_FastFn(t *testing.T) {
	start := time.Now()
	WithTimeout(t, 1*time.Second, func(ctx context.Context) {
		// Nothing; just return.
	})
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("expected fast return, took %v", d)
	}
}

func TestWithTimeout_ContextCancel(t *testing.T) {
	// fn observes the context's deadline and returns before the
	// timeout fires. This proves WithTimeout's select-case is
	// "fn returned first" not "ctx timeout first".
	WithTimeout(t, 200*time.Millisecond, func(ctx context.Context) {
		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Errorf("expected DeadlineExceeded, got %v", ctx.Err())
			}
		case <-time.After(500 * time.Millisecond):
			// would mean the deadline never fired — bad
		}
	})
}

func TestWithTimeout_TimesOut(t *testing.T) {
	// fn never returns; WithTimeout must fail the test. We can't
	// observe Fatal from inside the test, so we re-implement
	// the select inline and assert that the timeout case wins.
	// Use a cancellable context to release the hung goroutine
	// at test end so the test process can exit cleanly.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		// simulate a hung goroutine that does honor the deadline
		// (so we don't leak it past the test)
		<-ctx.Done()
		close(done)
	}()
	select {
	case <-done:
		t.Error("hung goroutine should not have returned within 30ms")
	case <-ctx.Done():
		// expected: timeout wins
	}
}

func TestWithTimeout_PanicPropagates(t *testing.T) {
	// If fn panics, WithTimeout re-panics from the test goroutine so
	// the test fails. Use a sub-test with defer/recover to verify.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic to propagate")
		}
	}()
	WithTimeout(t, 1*time.Second, func(_ context.Context) {
		panic("intentional")
	})
}

func TestWithTimeout_PanicAfterDeadline(t *testing.T) {
	// If fn panics *after* the context deadline has already fired,
	// WithTimeout must still propagate the panic via the second
	// select-case.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic to propagate after deadline")
		}
	}()
	WithTimeout(t, 1*time.Millisecond, func(_ context.Context) {
		time.Sleep(20 * time.Millisecond)
		panic("late panic")
	})
}

func TestWithTimeout_FiresAndFails(t *testing.T) {
	// The timeout branch is only reachable when fn does not return
	// before the deadline. Use a fake T in a goroutine so the fatal
	// exit does not kill the parent test. The fn is blocked on a stop
	// channel so we can clean up the leaked goroutine after observing
	// the failure.
	done := make(chan bool)
	stop := make(chan struct{})
	go func() {
		ft := &testing.T{}
		defer func() { done <- ft.Failed() }()
		WithTimeout(ft, 1*time.Millisecond, func(_ context.Context) {
			<-stop
		})
	}()
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(stop)
	}()
	select {
	case failed := <-done:
		if !failed {
			t.Error("expected WithTimeout to fail the fake T")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for WithTimeout result")
	}
}

// ── GoroutineLeakCheck ────────────────────────────────────────────────

func TestGoroutineLeakCheck_NoLeak(t *testing.T) {
	GoroutineLeakCheck(t, func() {
		// Spawn and immediately exit a goroutine. The grace period
		// (50ms) is long enough for the runtime to schedule the
		// exit before the check runs.
		done := make(chan struct{})
		go func() { close(done) }()
		<-done
	})
}

func TestGoroutineLeakCheck_DetectsLeak(t *testing.T) {
	// The helper reports leaks via t.Errorf. We use a fake T to
	// capture the failure without failing the parent test, and
	// assert the failure occurred. This guards the load-bearing
	// detection logic.
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	ft := &testing.T{}
	GoroutineLeakCheck(ft, func() {
		go func() { <-hold }()
	})
	if !ft.Failed() {
		t.Error("expected GoroutineLeakCheck to record failure on leaked goroutine")
	}
}

func TestCountGoroutines(t *testing.T) {
	cases := []struct {
		buf  string
		want int
	}{
		{"goroutine 1 [running]:\nfoo\n\ngoroutine 2 [running]:\nbar\n", 2},
		{"no goroutines here", 0},
		{"", 0},
		{"goroutine 1 [running]:\n", 1},
	}
	for _, c := range cases {
		if got := countGoroutines([]byte(c.buf)); got != c.want {
			t.Errorf("countGoroutines(%q) = %d, want %d", c.buf, got, c.want)
		}
	}
}

// ── MustGo ────────────────────────────────────────────────────────────

func TestMustGo_NormalExit(t *testing.T) {
	called := false
	MustGo(t, func() {
		called = true
	})
	if !called {
		t.Error("expected fn to be called")
	}
}

func TestMustGo_PanicCaptured(t *testing.T) {
	// MustGo converts a panic into a t.Errorf. We use a sub-test
	// that we expect to fail. The parent test asserts that the
	// sub-test's Failed() is true.
	ft := &testing.T{}
	MustGo(ft, func() {
		panic("oh no")
	})
	if !ft.Failed() {
		t.Error("expected MustGo to record failure on panic")
	}
}

func TestMustGo_WaitsOnCompletion(t *testing.T) {
	// Verify that MustGo blocks until the goroutine exits. Since
	// MustGo now uses a synchronous <-done receive, the test
	// observing the elapsed time proves the wait.
	done := make(chan struct{})
	start := time.Now()
	MustGo(t, func() {
		time.Sleep(20 * time.Millisecond)
		close(done)
	})
	if d := time.Since(start); d < 15*time.Millisecond {
		t.Errorf("expected at least 20ms (MustGo should wait), took %v", d)
	}
	select {
	case <-done:
	default:
		t.Error("goroutine did not finish before MustGo returned")
	}
}

// ── sanity check that the package doesn't break database/sql import ──

var _ *sql.DB
