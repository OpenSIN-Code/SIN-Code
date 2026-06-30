// SPDX-License-Identifier: MIT
// Purpose: tests for the background agent manager (issue #479).
// Mandate M7: race-free under `go test -race`.
package background

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStartAndComplete(t *testing.T) {
	m := NewManager()
	job := m.Start(context.Background(), "job-1", "fix the tests", func(ctx context.Context) (string, error) {
		return "all tests passed", nil
	})

	// Wait for completion.
	waitForStatus(t, m, "job-1", StatusDone, 2*time.Second)

	got, ok := m.Get("job-1")
	if !ok {
		t.Fatal("job not found after completion")
	}
	if got.Status != StatusDone {
		t.Errorf("status = %s, want done", got.Status)
	}
	if got.Result != "all tests passed" {
		t.Errorf("result = %q, want %q", got.Result, "all tests passed")
	}
	if got.Error != nil {
		t.Errorf("error = %v, want nil", got.Error)
	}
	if got.EndedAt.IsZero() {
		t.Error("EndedAt should be set after completion")
	}
	if got.EndedAt.Before(got.StartedAt) {
		t.Error("EndedAt should be after StartedAt")
	}
	_ = job
}

func TestStartAndFail(t *testing.T) {
	m := NewManager()
	m.Start(context.Background(), "job-2", "fail me", func(ctx context.Context) (string, error) {
		return "", errors.New("boom")
	})

	waitForStatus(t, m, "job-2", StatusFailed, 2*time.Second)

	got, ok := m.Get("job-2")
	if !ok {
		t.Fatal("job not found after failure")
	}
	if got.Status != StatusFailed {
		t.Errorf("status = %s, want failed", got.Status)
	}
	if got.Error == nil || got.Error.Error() != "boom" {
		t.Errorf("error = %v, want boom", got.Error)
	}
}

func TestList(t *testing.T) {
	m := NewManager()
	m.Start(context.Background(), "a", "prompt a", func(ctx context.Context) (string, error) {
		return "ok", nil
	})
	m.Start(context.Background(), "b", "prompt b", func(ctx context.Context) (string, error) {
		return "ok", nil
	})
	m.Start(context.Background(), "c", "prompt c", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	waitForStatus(t, m, "a", StatusDone, 2*time.Second)
	waitForStatus(t, m, "b", StatusDone, 2*time.Second)
	waitForStatus(t, m, "c", StatusDone, 2*time.Second)

	jobs := m.List()
	if len(jobs) != 3 {
		t.Errorf("list count = %d, want 3", len(jobs))
	}
}

func TestActive(t *testing.T) {
	m := NewManager()

	// Job that blocks so it stays active.
	blockCh := make(chan struct{})
	m.Start(context.Background(), "blocked", "slow", func(ctx context.Context) (string, error) {
		<-blockCh
		return "done", nil
	})
	// Job that completes quickly.
	m.Start(context.Background(), "fast", "quick", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	// Wait for fast to finish.
	waitForStatus(t, m, "fast", StatusDone, 2*time.Second)

	// Give the blocked job time to be in Running state.
	time.Sleep(50 * time.Millisecond)

	active := m.Active()
	if len(active) != 1 {
		t.Fatalf("active count = %d, want 1", len(active))
	}
	if active[0].ID != "blocked" {
		t.Errorf("active job ID = %s, want blocked", active[0].ID)
	}

	// Unblock and verify no active remain.
	close(blockCh)
	waitForStatus(t, m, "blocked", StatusDone, 2*time.Second)

	active = m.Active()
	if len(active) != 0 {
		t.Errorf("active count after unblock = %d, want 0", len(active))
	}
}

func TestGet(t *testing.T) {
	m := NewManager()
	m.Start(context.Background(), "known", "test", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	got, ok := m.Get("known")
	if !ok {
		t.Fatal("known job not found")
	}
	if got.ID != "known" {
		t.Errorf("ID = %s, want known", got.ID)
	}
	if got.Prompt != "test" {
		t.Errorf("Prompt = %s, want test", got.Prompt)
	}

	_, ok = m.Get("nonexistent")
	if ok {
		t.Error("nonexistent job should return false")
	}
}

func TestConcurrentAccess(t *testing.T) {
	m := NewManager()
	ctx := context.Background()
	var wg sync.WaitGroup

	// Parallel starters.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := string(rune('A'+n))
			m.Start(ctx, id, "concurrent", func(ctx context.Context) (string, error) {
				time.Sleep(10 * time.Millisecond)
				return "ok", nil
			})
		}(i)
	}

	// Parallel readers.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.List()
			_ = m.Active()
			_, _ = m.Get("A")
		}()
	}

	wg.Wait()

	// All jobs should eventually complete.
	jobs := m.List()
	if len(jobs) != 20 {
		t.Errorf("list count = %d, want 20", len(jobs))
	}
}

func TestFormatJob(t *testing.T) {
	j := &Job{
		ID:        "test-42",
		Prompt:    "fix all the tests in the repo",
		Status:    StatusDone,
		StartedAt: time.Now().Add(-5 * time.Second),
		EndedAt:   time.Now(),
	}
	out := FormatJob(j)
	if !strings.Contains(out, "test-42") {
		t.Errorf("output missing job ID: %q", out)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("output missing status: %q", out)
	}
	if !strings.Contains(out, "fix all the tests in the repo") {
		t.Errorf("output missing prompt: %q", out)
	}
}

func TestFormatJobTruncation(t *testing.T) {
	longPrompt := strings.Repeat("a", 80)
	j := &Job{
		ID:        "trunc",
		Prompt:    longPrompt,
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}
	out := FormatJob(j)
	if !strings.Contains(out, "...") {
		t.Errorf("output should contain truncation marker: %q", out)
	}
	if strings.Contains(out, longPrompt) {
		t.Errorf("output should not contain full long prompt: %q", out)
	}
}

func TestJobStatusString(t *testing.T) {
	tests := []struct {
		status JobStatus
		want   string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusDone, "done"},
		{StatusFailed, "failed"},
		{JobStatus(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("JobStatus(%d).String() = %s, want %s", tt.status, got, tt.want)
		}
	}
}

// waitForStatus polls a job until it reaches the desired status or times out.
func waitForStatus(t *testing.T, m *Manager, id string, want JobStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		j, ok := m.Get(id)
		if ok {
			m.mu.RLock()
			s := j.Status
			m.mu.RUnlock()
			if s == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach status %s within %s", id, want, timeout)
}
