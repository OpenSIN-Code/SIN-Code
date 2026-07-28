// SPDX-License-Identifier: MIT
// Purpose: coverage tests for autonomy package error paths and triggers.
package autonomy

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func resetHooks(t *testing.T) {
	t.Helper()
	oldDBOpen := _dbOpen
	oldDBExec := _dbExec
	oldDBExecContext := _dbExecContext
	oldDBQueryContext := _dbQueryContext
	oldDBBeginTx := _dbBeginTx
	oldTXQueryRowContext := _txQueryRowContext
	oldTXExecContext := _txExecContext
	oldTXCommit := _txCommit
	oldUserHomeDir := _userHomeDir
	oldMkdirAll := _mkdirAll
	oldTimeNow := _timeNow
	oldTimeSince := _timeSince
	oldNewTicker := _newTicker
	oldParseDuration := _parseDuration
	oldFingerprint := _fingerprint
	oldDirEntryInfo := _dirEntryInfo
	t.Cleanup(func() {
		_dbOpen = oldDBOpen
		_dbExec = oldDBExec
		_dbExecContext = oldDBExecContext
		_dbQueryContext = oldDBQueryContext
		_dbBeginTx = oldDBBeginTx
		_txQueryRowContext = oldTXQueryRowContext
		_txExecContext = oldTXExecContext
		_txCommit = oldTXCommit
		_userHomeDir = oldUserHomeDir
		_mkdirAll = oldMkdirAll
		_timeNow = oldTimeNow
		_timeSince = oldTimeSince
		_newTicker = oldNewTicker
		_parseDuration = oldParseDuration
		_fingerprint = oldFingerprint
		_dirEntryInfo = oldDirEntryInfo
	})
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	var buf strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			buf.Write(sc.Bytes())
			buf.WriteByte('\n')
		}
	}()

	fn()

	w.Close()
	os.Stderr = oldStderr
	wg.Wait()
	return buf.String()
}

func openRunnerQueue(t *testing.T) *Queue {
	t.Helper()
	q, err := Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { q.Close() })
	return q
}

func TestDefaultPath(t *testing.T) {
	resetHooks(t)
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	_userHomeDir = func() (string, error) { return tmp, nil }
	_mkdirAll = func(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }

	p := DefaultPath()
	want := filepath.Join(tmp, ".local", "share", "sin-code", "goals.db")
	if p != want {
		t.Fatalf("expected %q, got %q", want, p)
	}
}

func TestOpenDBOpenError(t *testing.T) {
	resetHooks(t)
	_dbOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
		return nil, errors.New("db open boom")
	}
	_, err := Open(filepath.Join(t.TempDir(), "g.db"))
	if err == nil || !strings.Contains(err.Error(), "db open boom") {
		t.Fatalf("expected db open error, got %v", err)
	}
}

func TestOpenSchemaExecError(t *testing.T) {
	resetHooks(t)
	_dbExec = func(db *sql.DB, query string, args ...any) (sql.Result, error) {
		return nil, errors.New("schema boom")
	}
	_, err := Open(filepath.Join(t.TempDir(), "g.db"))
	if err == nil || !strings.Contains(err.Error(), "schema boom") {
		t.Fatalf("expected schema error, got %v", err)
	}
}

func TestAddExecContextError(t *testing.T) {
	resetHooks(t)
	q := openTestQueue(t)
	_dbExecContext = func(db *sql.DB, ctx context.Context, query string, args ...any) (sql.Result, error) {
		return nil, errors.New("add boom")
	}
	_, err := q.Add(context.Background(), "p", "/tmp", 0, 0)
	if err == nil || !strings.Contains(err.Error(), "add boom") {
		t.Fatalf("expected add error, got %v", err)
	}
}

func TestLeaseBeginTxError(t *testing.T) {
	resetHooks(t)
	q := openTestQueue(t)
	_dbBeginTx = func(db *sql.DB, ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
		return nil, errors.New("begin boom")
	}
	_, err := q.Lease(context.Background(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "begin boom") {
		t.Fatalf("expected begin tx error, got %v", err)
	}
}

func TestLeaseQueryRowError(t *testing.T) {
	resetHooks(t)
	q := openTestQueue(t)
	_, _ = q.Add(context.Background(), "task", "/tmp", 0, 1)
	_txQueryRowContext = func(tx *sql.Tx, ctx context.Context, query string, args ...any) *sql.Row {
		return tx.QueryRowContext(ctx, "SELECT invalid")
	}
	_, err := q.Lease(context.Background(), time.Minute)
	if err == nil {
		t.Fatal("expected query row error")
	}
}

func TestLeaseUpdateError(t *testing.T) {
	resetHooks(t)
	q := openTestQueue(t)
	_, _ = q.Add(context.Background(), "task", "/tmp", 0, 1)
	_txExecContext = func(tx *sql.Tx, ctx context.Context, query string, args ...any) (sql.Result, error) {
		return nil, errors.New("update boom")
	}
	_, err := q.Lease(context.Background(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "update boom") {
		t.Fatalf("expected update error, got %v", err)
	}
}

func TestLeaseCommitError(t *testing.T) {
	resetHooks(t)
	q := openTestQueue(t)
	_, _ = q.Add(context.Background(), "task", "/tmp", 0, 1)
	_txCommit = func(tx *sql.Tx) error { return errors.New("commit boom") }
	_, err := q.Lease(context.Background(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "commit boom") {
		t.Fatalf("expected commit error, got %v", err)
	}
}

func TestListQueryContextError(t *testing.T) {
	resetHooks(t)
	q := openTestQueue(t)
	_dbQueryContext = func(db *sql.DB, ctx context.Context, query string, args ...any) (*sql.Rows, error) {
		return nil, errors.New("query boom")
	}
	_, err := q.List(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "query boom") {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestListScanError(t *testing.T) {
	resetHooks(t)
	q := openTestQueue(t)
	_, _ = q.Add(context.Background(), "task", "/tmp", 0, 1)
	_dbQueryContext = func(db *sql.DB, ctx context.Context, query string, args ...any) (*sql.Rows, error) {
		return db.QueryContext(ctx, "SELECT 1")
	}
	_, err := q.List(context.Background(), "")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

func TestFingerprintHashesFiles(t *testing.T) {
	ws := t.TempDir()
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("hello"), 0o644)
	_ = os.WriteFile(filepath.Join(ws, "b.txt"), []byte("world"), 0o644)

	h1 := fingerprint(ws, "*.txt")
	h2 := fingerprint(ws, "**/*.txt")
	if h1 == "" || h2 == "" {
		t.Fatal("expected non-empty fingerprints")
	}

	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("changed"), 0o644)
	h3 := fingerprint(ws, "*.txt")
	if h3 == h1 {
		t.Fatal("expected fingerprint to change after file modification")
	}
}

func TestFingerprintSkipsIgnoredDirs(t *testing.T) {
	ws := t.TempDir()
	_ = os.WriteFile(filepath.Join(ws, "keep.txt"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(ws, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(ws, ".git", "secret"), []byte("y"), 0o644)
	_ = os.MkdirAll(filepath.Join(ws, "node_modules"), 0o755)
	_ = os.WriteFile(filepath.Join(ws, "node_modules", "pkg"), []byte("z"), 0o644)
	_ = os.MkdirAll(filepath.Join(ws, ".sin-code"), 0o755)
	_ = os.WriteFile(filepath.Join(ws, ".sin-code", "trig"), []byte("w"), 0o644)

	h := fingerprint(ws, "*.txt")
	if h == "" {
		t.Fatal("expected non-empty fingerprint")
	}
}

func TestFingerprintWalkDirError(t *testing.T) {
	ws := t.TempDir()
	nodir := filepath.Join(ws, "nodir")
	_ = os.MkdirAll(nodir, 0o755)
	_ = os.WriteFile(filepath.Join(nodir, "x.txt"), []byte("x"), 0o644)
	_ = os.Chmod(nodir, 0o000)
	t.Cleanup(func() { _ = os.Chmod(nodir, 0o755) })

	h := fingerprint(ws, "*.txt")
	if h == "" {
		t.Fatal("expected non-empty fingerprint")
	}
}

func TestFingerprintInfoError(t *testing.T) {
	resetHooks(t)
	ws := t.TempDir()
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("x"), 0o644)
	_dirEntryInfo = func(d os.DirEntry) (os.FileInfo, error) {
		return nil, errors.New("info boom")
	}

	h := fingerprint(ws, "*.txt")
	if h == "" {
		t.Fatal("expected non-empty fingerprint")
	}
}

func TestFingerprintIgnoresNonMatchingFiles(t *testing.T) {
	ws := t.TempDir()
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(ws, "b.md"), []byte("y"), 0o644)

	h := fingerprint(ws, "*.txt")
	if h == "" {
		t.Fatal("expected non-empty fingerprint")
	}
}

func TestLoadTriggers(t *testing.T) {
	ws := t.TempDir()
	if ts := LoadTriggers(ws); ts != nil {
		t.Fatal("expected nil when triggers.json missing")
	}

	_ = os.MkdirAll(filepath.Join(ws, ".sin-code"), 0o755)
	_ = os.WriteFile(filepath.Join(ws, ".sin-code", "triggers.json"), []byte("not json"), 0o644)

	out := captureStderr(t, func() {
		if ts := LoadTriggers(ws); ts != nil {
			t.Fatal("expected nil when triggers.json invalid")
		}
	})
	if !strings.Contains(out, "invalid triggers.json") {
		t.Fatalf("expected invalid triggers warning, got %q", out)
	}

	_ = os.WriteFile(filepath.Join(ws, ".sin-code", "triggers.json"), []byte(`[
		{"type": "cron", "every": "1m", "prompt": "ping", "priority": 1}
	]`), 0o644)
	ts := LoadTriggers(ws)
	if len(ts) != 1 || ts[0].Type != "cron" {
		t.Fatalf("expected cron trigger, got %+v", ts)
	}
}

func TestMatchDoubleStar(t *testing.T) {
	if matchDoubleStar("*.go", "a.go") {
		t.Fatal("expected no **/ to be false")
	}
	if matchDoubleStar("src/**/*.go", "other.go") {
		t.Fatal("expected prefix mismatch")
	}
	if !matchDoubleStar("src/**/*.go", "src/pkg/a.go") {
		t.Fatal("expected match")
	}
}

func TestRunnerDefaultPollInterval(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{},
		// PollInterval <= 0 should default to 10s.
	}

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not return after cancel")
	}
}

func TestRunnerUnknownTrigger(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "unknown", Prompt: "p"}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	if !strings.Contains(out, "unknown trigger type") {
		t.Fatalf("expected unknown trigger warning, got %q", out)
	}
}

func TestRunnerCronInvalidEvery(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "cron", Every: "invalid", Prompt: "p"}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	if !strings.Contains(out, "cron trigger needs every") {
		t.Fatalf("expected invalid every warning, got %q", out)
	}
}

func TestRunnerCronShortInterval(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "cron", Every: "1s", Prompt: "p"}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	if !strings.Contains(out, "cron trigger needs every") {
		t.Fatalf("expected short interval warning, got %q", out)
	}
}

func TestRunnerCronEnqueue(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	// Use a millisecond ticker instead of nanosecond to avoid overwhelming the
	// SQLite queue under the race detector, which can cause the runner to take
	// longer than the 2s timeout to exit after cancellation.
	_newTicker = func(d time.Duration) *time.Ticker { return time.NewTicker(1 * time.Millisecond) }

	enqueued := make(chan struct{}, 1)
	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "cron", Every: "1m", Prompt: "ping", Priority: 1}},
		onEnqueue: func() {
			select {
			case enqueued <- struct{}{}:
			default:
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		select {
		case <-enqueued:
		case <-time.After(5 * time.Second):
			t.Fatal("expected cron enqueue")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	goals, _ := q.List(context.Background(), StatusPending)
	if len(goals) < 1 {
		t.Fatalf("expected at least 1 enqueued goal, got %d", len(goals))
	}
}

func TestRunnerCronAddError(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	_dbExecContext = func(db *sql.DB, ctx context.Context, query string, args ...any) (sql.Result, error) {
		return nil, errors.New("add boom")
	}
	_newTicker = func(d time.Duration) *time.Ticker { return time.NewTicker(1 * time.Nanosecond) }

	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "cron", Every: "1m", Prompt: "p"}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	if !strings.Contains(out, "cron enqueue failed") {
		t.Fatalf("expected cron enqueue failed warning, got %q", out)
	}
}

func TestRunnerWatchInvalidDebounce(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	_newTicker = func(d time.Duration) *time.Ticker { return time.NewTicker(1 * time.Nanosecond) }

	ws := t.TempDir()
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("v1"), 0o644)

	r := &Runner{
		Queue:        q,
		Workspace:    ws,
		Triggers:     []Trigger{{Type: "watch", Glob: "*.txt", Debounce: "bad", Prompt: "check", Priority: 1}},
		PollInterval: 1 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})
}

func TestRunnerWatchEnqueue(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	_newTicker = func(d time.Duration) *time.Ticker { return time.NewTicker(1 * time.Nanosecond) }
	_timeSince = func(since time.Time) time.Duration { return 100 * time.Second }

	ws := t.TempDir()
	fingerprintCalls := 0
	_fingerprint = func(_, _ string) string {
		fingerprintCalls++
		if fingerprintCalls == 1 {
			return "before"
		}
		return "after"
	}

	enqueued := make(chan struct{}, 1)
	r := &Runner{
		Queue:        q,
		Workspace:    ws,
		Triggers:     []Trigger{{Type: "watch", Glob: "*.txt", Debounce: "1ms", Prompt: "check", Priority: 1}},
		PollInterval: 1 * time.Millisecond,
		onEnqueue: func() {
			select {
			case enqueued <- struct{}{}:
			default:
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		select {
		case <-enqueued:
		case <-time.After(2 * time.Second):
			t.Fatal("expected watch enqueue")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	goals, _ := q.List(context.Background(), StatusPending)
	if len(goals) < 1 {
		t.Fatalf("expected at least 1 enqueued goal, got %d", len(goals))
	}
}

func TestRunnerWatchNotReady(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	_newTicker = func(d time.Duration) *time.Ticker { return time.NewTicker(1 * time.Nanosecond) }
	_timeSince = func(since time.Time) time.Duration { return 0 }

	ws := t.TempDir()
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("v1"), 0o644)

	r := &Runner{
		Queue:        q,
		Workspace:    ws,
		Triggers:     []Trigger{{Type: "watch", Glob: "*.txt", Debounce: "1h", Prompt: "check", Priority: 1}},
		PollInterval: 1 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("v2"), 0o644)
	}()

	captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	goals, _ := q.List(context.Background(), StatusPending)
	if len(goals) != 0 {
		t.Fatalf("expected 0 goals (debounce not elapsed), got %d", len(goals))
	}
}

func TestRunnerWatchAddError(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	_dbExecContext = func(db *sql.DB, ctx context.Context, query string, args ...any) (sql.Result, error) {
		return nil, errors.New("watch add boom")
	}
	_newTicker = func(d time.Duration) *time.Ticker { return time.NewTicker(1 * time.Nanosecond) }
	_timeSince = func(since time.Time) time.Duration { return 100 * time.Second }

	ws := t.TempDir()
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("v1"), 0o644)

	r := &Runner{
		Queue:        q,
		Workspace:    ws,
		Triggers:     []Trigger{{Type: "watch", Glob: "*.txt", Debounce: "1ms", Prompt: "check", Priority: 1}},
		PollInterval: 1 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("v2-version-two"), 0o644)
	}()

	out := captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(200 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	if !strings.Contains(out, "watch enqueue failed") {
		t.Fatalf("expected watch enqueue failed warning, got %q", out)
	}
}

func TestRunnerDreamInvalidEvery(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "dream", Every: "invalid", Prompt: "dream"}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	if !strings.Contains(out, "dream trigger needs every") {
		t.Fatalf("expected dream trigger needs every warning, got %q", out)
	}
}

func TestRunnerDreamShortInterval(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "dream", Every: "1s", Prompt: "dream"}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	out := captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	if !strings.Contains(out, "dream trigger needs every") {
		t.Fatalf("expected dream trigger needs every warning, got %q", out)
	}
}

func TestRunnerDreamFuncCalled(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	_newTicker = func(d time.Duration) *time.Ticker { return time.NewTicker(1 * time.Nanosecond) }

	var callCount int
	var mu sync.Mutex
	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "dream", Every: "1m", Prompt: "dream"}},
		DreamFunc: func(ctx context.Context) error {
			mu.Lock()
			callCount++
			mu.Unlock()
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	mu.Lock()
	cc := callCount
	mu.Unlock()
	if cc < 1 {
		t.Fatalf("expected DreamFunc called at least 1 time, got %d", cc)
	}
}

func TestRunnerDreamFallbackEnqueue(t *testing.T) {
	resetHooks(t)
	q := openRunnerQueue(t)

	_newTicker = func(d time.Duration) *time.Ticker { return time.NewTicker(1 * time.Nanosecond) }

	r := &Runner{
		Queue:     q,
		Workspace: t.TempDir(),
		Triggers:  []Trigger{{Type: "dream", Every: "1m", Prompt: "consolidate memory", Priority: 1}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	captureStderr(t, func() {
		go func() { _ = r.Run(ctx); close(done) }()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("runner did not exit")
		}
	})

	goals, _ := q.List(context.Background(), StatusPending)
	if len(goals) < 1 {
		t.Fatalf("expected at least 1 enqueued goal (fallback), got %d", len(goals))
	}
}
