// SPDX-License-Identifier: MIT
// Purpose: coverage tests for daemon_cmd.go — exercise every statement in
// runDaemon, runWorker, executeGoal, diskOK and dedupeRepos using hooks and
// lightweight fakes. No production logic is refactored; all package-level
// hooks are restored after each test.
package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/resource"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// daemonTempDBPath returns a temporary file path that is removed after the test.
func daemonTempDBPath(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "daemon_test_")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// daemonOpenTestQueueNoCleanup opens an autonomy queue on a temp DB; the caller is
// responsible for closing it (runDaemon closes it via defer).
func daemonOpenTestQueueNoCleanup(t *testing.T) *autonomy.Queue {
	t.Helper()
	q, err := autonomy.Open(daemonTempDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func daemonOpenTestMemoryNoCleanup(t *testing.T) *memory.Store {
	t.Helper()
	m, err := memory.Open(daemonTempDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func daemonOpenTestSessionNoCleanup(t *testing.T) *session.Store {
	t.Helper()
	s, err := session.Open(daemonTempDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func daemonOpenTestLessonsNoCleanup(t *testing.T) *lessons.Store {
	t.Helper()
	l, err := lessons.Open(daemonTempDBPath(t))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// daemonOpenTestQueue opens an autonomy queue and schedules cleanup for direct
// worker/executeGoal tests.
func daemonOpenTestQueue(t *testing.T) *autonomy.Queue {
	t.Helper()
	q := daemonOpenTestQueueNoCleanup(t)
	t.Cleanup(func() { q.Close() })
	return q
}

func daemonOpenTestMemory(t *testing.T) *memory.Store {
	t.Helper()
	m := daemonOpenTestMemoryNoCleanup(t)
	t.Cleanup(func() { m.Close() })
	return m
}

func daemonOpenTestSession(t *testing.T) *session.Store {
	t.Helper()
	s := daemonOpenTestSessionNoCleanup(t)
	t.Cleanup(func() { s.Close() })
	return s
}

func daemonOpenTestLessons(t *testing.T) *lessons.Store {
	t.Helper()
	l := daemonOpenTestLessonsNoCleanup(t)
	t.Cleanup(func() { l.Close() })
	return l
}

// daemonSetupDaemonStores wires all store-open hooks to the provided concrete
// stores. runDaemon closes the stores via its own defers, so the helpers
// above must be the no-cleanup variants.
func daemonSetupDaemonStores(t *testing.T, q *autonomy.Queue, m *memory.Store, s *session.Store, l *lessons.Store) {
	t.Helper()
	oldAutonomy := daemonAutonomyOpenHook
	daemonAutonomyOpenHook = func(string) (*autonomy.Queue, error) { return q, nil }
	t.Cleanup(func() { daemonAutonomyOpenHook = oldAutonomy })

	oldMem := memoryOpenHook
	memoryOpenHook = func(string) (*memory.Store, error) { return m, nil }
	t.Cleanup(func() { memoryOpenHook = oldMem })

	oldSess := sessionOpenHook
	sessionOpenHook = func(string) (*session.Store, error) { return s, nil }
	t.Cleanup(func() { sessionOpenHook = oldSess })

	oldLessons := lessonsOpenHook
	lessonsOpenHook = func(string) (*lessons.Store, error) { return l, nil }
	t.Cleanup(func() { lessonsOpenHook = oldLessons })
}

// ── NewDaemonCmd / RunE error paths ─────────────────────────────

func TestNewDaemonCmdResourceParseError(t *testing.T) {
	old := daemonResourceParseLimitsHook
	daemonResourceParseLimitsHook = func(string, int, string) (resource.Limits, error) {
		return resource.Limits{}, errors.New("bad limits")
	}
	t.Cleanup(func() { daemonResourceParseLimitsHook = old })

	cmd := NewDaemonCmd()
	cmd.SetArgs([]string{"--max-memory", "bad"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "bad limits") {
		t.Fatalf("expected resource parse error, got %v", err)
	}
}

func TestNewDaemonCmdConcurrencyNormalize(t *testing.T) {
	old := daemonResourceParseLimitsHook
	daemonResourceParseLimitsHook = resource.ParseLimits
	t.Cleanup(func() { daemonResourceParseLimitsHook = old })

	oldGetwd := daemonOSGetwdHook
	daemonOSGetwdHook = func() (string, error) { return "", errors.New("getwd error") }
	t.Cleanup(func() { daemonOSGetwdHook = oldGetwd })

	cmd := NewDaemonCmd()
	cmd.SetArgs([]string{"--verify-cmd", "true", "--concurrency", "0"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "getwd error") {
		t.Fatalf("expected getwd error after concurrency normalization, got %v", err)
	}
}

// ── runDaemon error paths ───────────────────────────────────────

func TestRunDaemonVerifyCmdMissing(t *testing.T) {
	err := runDaemon(context.Background(), daemonOptions{})
	if err == nil || !strings.Contains(err.Error(), "verify-cmd") {
		t.Fatalf("expected verify-cmd error, got %v", err)
	}
}

func TestRunDaemonGetwdError(t *testing.T) {
	old := daemonOSGetwdHook
	daemonOSGetwdHook = func() (string, error) { return "", errors.New("getwd error") }
	t.Cleanup(func() { daemonOSGetwdHook = old })

	err := runDaemon(context.Background(), daemonOptions{verifyCmd: "true"})
	if err == nil || !strings.Contains(err.Error(), "getwd error") {
		t.Fatalf("expected getwd error, got %v", err)
	}
}

func TestRunDaemonQueueOpenError(t *testing.T) {
	old := daemonAutonomyOpenHook
	daemonAutonomyOpenHook = func(string) (*autonomy.Queue, error) { return nil, errors.New("queue open error") }
	t.Cleanup(func() { daemonAutonomyOpenHook = old })

	err := runDaemon(context.Background(), daemonOptions{verifyCmd: "true"})
	if err == nil || !strings.Contains(err.Error(), "queue open error") {
		t.Fatalf("expected queue open error, got %v", err)
	}
}

func TestRunDaemonMemoryOpenError(t *testing.T) {
	q := daemonOpenTestQueueNoCleanup(t)
	old := daemonAutonomyOpenHook
	daemonAutonomyOpenHook = func(string) (*autonomy.Queue, error) { return q, nil }
	t.Cleanup(func() { daemonAutonomyOpenHook = old })

	oldMem := memoryOpenHook
	memoryOpenHook = func(string) (*memory.Store, error) { return nil, errors.New("memory open error") }
	t.Cleanup(func() { memoryOpenHook = oldMem })

	err := runDaemon(context.Background(), daemonOptions{verifyCmd: "true"})
	if err == nil || !strings.Contains(err.Error(), "memory open error") {
		t.Fatalf("expected memory open error, got %v", err)
	}
}

func TestRunDaemonSessionOpenError(t *testing.T) {
	q := daemonOpenTestQueueNoCleanup(t)
	m := daemonOpenTestMemoryNoCleanup(t)
	old := daemonAutonomyOpenHook
	daemonAutonomyOpenHook = func(string) (*autonomy.Queue, error) { return q, nil }
	t.Cleanup(func() { daemonAutonomyOpenHook = old })

	oldMem := memoryOpenHook
	memoryOpenHook = func(string) (*memory.Store, error) { return m, nil }
	t.Cleanup(func() { memoryOpenHook = oldMem })

	oldSess := sessionOpenHook
	sessionOpenHook = func(string) (*session.Store, error) { return nil, errors.New("session open error") }
	t.Cleanup(func() { sessionOpenHook = oldSess })

	err := runDaemon(context.Background(), daemonOptions{verifyCmd: "true"})
	if err == nil || !strings.Contains(err.Error(), "session open error") {
		t.Fatalf("expected session open error, got %v", err)
	}
}

func TestRunDaemonTriggerRegistration(t *testing.T) {
	q := daemonOpenTestQueueNoCleanup(t)
	m := daemonOpenTestMemoryNoCleanup(t)
	s := daemonOpenTestSessionNoCleanup(t)
	l := daemonOpenTestLessonsNoCleanup(t)
	daemonSetupDaemonStores(t, q, m, s, l)

	oldTriggers := autonomyLoadTriggersHook
	autonomyLoadTriggersHook = func(string) []autonomy.Trigger {
		return []autonomy.Trigger{{Type: "cron", Every: "1h", Prompt: "test"}}
	}
	t.Cleanup(func() { autonomyLoadTriggersHook = oldTriggers })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runDaemon(ctx, daemonOptions{verifyCmd: "true", pollEvery: 1 * time.Second, concurrency: 1, repos: []string{"/nonexistent"}})
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDaemonWorkerPool(t *testing.T) {
	q := daemonOpenTestQueueNoCleanup(t)
	m := daemonOpenTestMemoryNoCleanup(t)
	s := daemonOpenTestSessionNoCleanup(t)
	l := daemonOpenTestLessonsNoCleanup(t)
	daemonSetupDaemonStores(t, q, m, s, l)

	ctxAdd, cancelAdd := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAdd()
	_, err := q.Add(ctxAdd, "test prompt", "/tmp", 0, 1)
	if err != nil {
		t.Fatal(err)
	}

	oldTriggers := autonomyLoadTriggersHook
	autonomyLoadTriggersHook = func(string) []autonomy.Trigger { return nil }
	t.Cleanup(func() { autonomyLoadTriggersHook = oldTriggers })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var executed atomic.Bool
	oldLoop := loopbuilderBuildHook
	loopbuilderBuildHook = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{
			RunOverride: func(ctx context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
				executed.Store(true)
				defer cancel()
				return &agentloop.Result{SessionID: sess.ID, Verified: true, Turns: 3, Summary: "done"}, nil
			},
		}, func() error { return nil }, nil
	}
	t.Cleanup(func() { loopbuilderBuildHook = oldLoop })

	err = runDaemon(ctx, daemonOptions{verifyCmd: "true", pollEvery: 10 * time.Millisecond, concurrency: 1})
	if err != nil && err != context.Canceled {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executed.Load() {
		t.Fatal("expected worker to execute the leased goal")
	}
}

// ── runWorker paths ─────────────────────────────────────────────

func TestRunWorkerCtxDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	oldDisk := daemonDiskFreeHook
	daemonDiskFreeHook = func(string) (int64, bool) { return 1 << 30, true }
	defer func() { daemonDiskFreeHook = oldDisk }()

	runWorker(ctx, 1, daemonOpenTestQueue(t), daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), daemonOptions{pollEvery: time.Millisecond})
}

func TestRunWorkerDiskNotOK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	oldDisk := daemonDiskFreeHook
	daemonDiskFreeHook = func(string) (int64, bool) { return 0, true }
	defer func() { daemonDiskFreeHook = oldDisk }()

	runWorker(ctx, 1, daemonOpenTestQueue(t), daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), daemonOptions{
		pollEvery: 10 * time.Millisecond,
		limits:    resource.Limits{MinDiskBytes: 1},
	})
}

func TestRunWorkerLeaseError(t *testing.T) {
	q := daemonOpenTestQueueNoCleanup(t)
	q.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	oldDisk := daemonDiskFreeHook
	daemonDiskFreeHook = func(string) (int64, bool) { return 1 << 30, true }
	defer func() { daemonDiskFreeHook = oldDisk }()

	runWorker(ctx, 1, q, daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), daemonOptions{pollEvery: 10 * time.Millisecond})
}

func TestRunWorkerNilGoal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	oldDisk := daemonDiskFreeHook
	daemonDiskFreeHook = func(string) (int64, bool) { return 1 << 30, true }
	defer func() { daemonDiskFreeHook = oldDisk }()

	runWorker(ctx, 1, daemonOpenTestQueue(t), daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), daemonOptions{pollEvery: 10 * time.Millisecond})
}

func TestRunWorkerGoalExecuteSuccess(t *testing.T) {
	q := daemonOpenTestQueue(t)
	ctxAdd, cancelAdd := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAdd()
	goalID, err := q.Add(ctxAdd, "test prompt", "/tmp", 0, 1)
	if err != nil {
		t.Fatal(err)
	}

	oldDisk := daemonDiskFreeHook
	daemonDiskFreeHook = func(string) (int64, bool) { return 1 << 30, true }
	defer func() { daemonDiskFreeHook = oldDisk }()

	oldLoop := loopbuilderBuildHook
	loopbuilderBuildHook = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{
			RunOverride: func(ctx context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
				return &agentloop.Result{SessionID: sess.ID, Verified: true, Turns: 2, Summary: "ok"}, nil
			},
		}, func() error { return nil }, nil
	}
	defer func() { loopbuilderBuildHook = oldLoop }()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	runWorker(ctx, 1, q, daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), daemonOptions{
		pollEvery: 10 * time.Millisecond,
		verifyCmd: "true",
		maxTurns:  10,
	})

	completed, err := q.List(ctxAdd, autonomy.StatusVerified)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 1 || completed[0].ID != goalID {
		t.Fatalf("expected goal %d completed, got %v", goalID, completed)
	}
}

func TestRunWorkerGoalExecuteFail(t *testing.T) {
	q := daemonOpenTestQueue(t)
	ctxAdd, cancelAdd := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAdd()
	goalID, err := q.Add(ctxAdd, "test prompt", "/tmp", 0, 1)
	if err != nil {
		t.Fatal(err)
	}

	oldDisk := daemonDiskFreeHook
	daemonDiskFreeHook = func(string) (int64, bool) { return 1 << 30, true }
	defer func() { daemonDiskFreeHook = oldDisk }()

	oldLoop := loopbuilderBuildHook
	loopbuilderBuildHook = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{
			RunOverride: func(ctx context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
				return nil, errors.New("loop run failed")
			},
		}, func() error { return nil }, nil
	}
	defer func() { loopbuilderBuildHook = oldLoop }()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	runWorker(ctx, 1, q, daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), daemonOptions{
		pollEvery: 10 * time.Millisecond,
		verifyCmd: "true",
		maxTurns:  10,
	})

	failed, err := q.List(ctxAdd, autonomy.StatusExhausted)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].ID != goalID {
		t.Fatalf("expected goal %d exhausted, got %v", goalID, failed)
	}
}

// ── executeGoal paths ─────────────────────────────────────────────

func TestExecuteGoalSessionError(t *testing.T) {
	q := daemonOpenTestQueue(t)
	ctxAdd, cancelAdd := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAdd()
	goalID, err := q.Add(ctxAdd, "test prompt", "/tmp", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	s := daemonOpenTestSession(t)
	goal := &autonomy.Goal{ID: goalID, Prompt: "test", Workspace: "/tmp", SessionID: "nonexistent", Attempts: 0, MaxRetries: 0}
	executeGoal(ctxAdd, q, s, daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), goal, "true", 10)

	failed, err := q.List(ctxAdd, autonomy.StatusExhausted)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].ID != goalID {
		t.Fatalf("expected goal %d exhausted, got %v", goalID, failed)
	}
}

func TestExecuteGoalLoopBuildError(t *testing.T) {
	q := daemonOpenTestQueue(t)
	ctxAdd, cancelAdd := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAdd()
	goalID, err := q.Add(ctxAdd, "test prompt", "/tmp", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	oldLoop := loopbuilderBuildHook
	loopbuilderBuildHook = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (*agentloop.Loop, func() error, error) {
		return nil, nil, errors.New("loop build failed")
	}
	defer func() { loopbuilderBuildHook = oldLoop }()

	goal := &autonomy.Goal{ID: goalID, Prompt: "test", Workspace: "/tmp", Attempts: 0, MaxRetries: 0}
	executeGoal(ctxAdd, q, daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), goal, "true", 10)

	failed, err := q.List(ctxAdd, autonomy.StatusExhausted)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].ID != goalID {
		t.Fatalf("expected goal %d exhausted, got %v", goalID, failed)
	}
}

func TestExecuteGoalLoopRunError(t *testing.T) {
	q := daemonOpenTestQueue(t)
	ctxAdd, cancelAdd := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAdd()
	goalID, err := q.Add(ctxAdd, "test prompt", "/tmp", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	oldLoop := loopbuilderBuildHook
	loopbuilderBuildHook = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{
			RunOverride: func(ctx context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
				return nil, errors.New("loop run failed")
			},
		}, func() error { return nil }, nil
	}
	defer func() { loopbuilderBuildHook = oldLoop }()

	goal := &autonomy.Goal{ID: goalID, Prompt: "test", Workspace: "/tmp", Attempts: 0, MaxRetries: 0}
	executeGoal(ctxAdd, q, daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), goal, "true", 10)

	failed, err := q.List(ctxAdd, autonomy.StatusExhausted)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].ID != goalID {
		t.Fatalf("expected goal %d exhausted, got %v", goalID, failed)
	}
}

func TestExecuteGoalSuccess(t *testing.T) {
	q := daemonOpenTestQueue(t)
	ctxAdd, cancelAdd := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelAdd()
	goalID, err := q.Add(ctxAdd, "test prompt", "/tmp", 0, 1)
	if err != nil {
		t.Fatal(err)
	}

	oldTools := mcpManagerToolsFn
	mcpManagerToolsFn = func(mgr *mcpclient.Manager) []mcpclient.Tool { return nil }
	defer func() { mcpManagerToolsFn = oldTools }()

	oldLoop := loopbuilderBuildHook
	loopbuilderBuildHook = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (*agentloop.Loop, func() error, error) {
		_, _ = cfg.ToolFactory(nil)
		return &agentloop.Loop{
			RunOverride: func(ctx context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
				return &agentloop.Result{SessionID: sess.ID, Verified: true, Turns: 2, Summary: "ok"}, nil
			},
		}, func() error { return nil }, nil
	}
	defer func() { loopbuilderBuildHook = oldLoop }()

	goal := &autonomy.Goal{ID: goalID, Prompt: "test", Workspace: "/tmp", Attempts: 0, MaxRetries: 1}
	executeGoal(ctxAdd, q, daemonOpenTestSession(t), daemonOpenTestLessons(t), daemonOpenTestMemory(t), hooks.New(nil), goal, "true", 10)

	completed, err := q.List(ctxAdd, autonomy.StatusVerified)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 1 || completed[0].ID != goalID {
		t.Fatalf("expected goal %d completed, got %v", goalID, completed)
	}
}

// ── diskOK helper ───────────────────────────────────────────────

func TestDiskOKGetwdError(t *testing.T) {
	old := daemonOSGetwdHook
	daemonOSGetwdHook = func() (string, error) { return "", errors.New("getwd error") }
	defer func() { daemonOSGetwdHook = old }()

	if !diskOK(resource.Limits{MinDiskBytes: 1}) {
		t.Error("diskOK should be true when Getwd fails")
	}
}

func TestDiskOKDiskFreeUnavailable(t *testing.T) {
	old := daemonDiskFreeHook
	daemonDiskFreeHook = func(string) (int64, bool) { return 0, false }
	defer func() { daemonDiskFreeHook = old }()

	if !diskOK(resource.Limits{MinDiskBytes: 1}) {
		t.Error("diskOK should be true when DiskFree is unavailable")
	}
}

func TestDiskOKFloorComparison(t *testing.T) {
	old := daemonDiskFreeHook
	defer func() { daemonDiskFreeHook = old }()

	daemonDiskFreeHook = func(string) (int64, bool) { return 200, true }
	if !diskOK(resource.Limits{MinDiskBytes: 100}) {
		t.Error("expected true when free >= floor")
	}

	daemonDiskFreeHook = func(string) (int64, bool) { return 50, true }
	if diskOK(resource.Limits{MinDiskBytes: 100}) {
		t.Error("expected false when free < floor")
	}
}

// ── dedupeRepos helper (already partially covered) ───────────────

func TestDedupeReposCwdFirst(t *testing.T) {
	got := dedupeRepos("/cwd", []string{"/a", "/cwd", "/b", "/a"})
	want := []string{"/cwd", "/a", "/b"}
	if len(got) != len(want) {
		t.Fatalf("dedupeRepos = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dedupeRepos[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
