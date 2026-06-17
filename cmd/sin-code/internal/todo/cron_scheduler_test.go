// SPDX-License-Identifier: MIT
// Purpose: tests for issue #335 — Cron-based todo scheduling and dispatch.
package todo

import (
	"sync"
	"testing"
	"time"
)

func TestCronSchedulerScheduleAndList(t *testing.T) {
	s := NewCronScheduler()
	if err := s.Schedule("st-1", "0 9 * * *"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if err := s.Schedule("st-2", "*/5 * * * *"); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 scheduled todos, got %d", len(list))
	}
}

func TestCronSchedulerUnschedule(t *testing.T) {
	s := NewCronScheduler()
	_ = s.Schedule("st-1", "0 9 * * *")
	_ = s.Schedule("st-2", "0 10 * * *")

	s.Unschedule("st-1")

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 after unschedule, got %d", len(list))
	}
	if list[0].TodoID != "st-2" {
		t.Errorf("expected st-2, got %q", list[0].TodoID)
	}

	// Unschedule non-existent is a no-op.
	s.Unschedule("nonexistent")
	if len(s.List()) != 1 {
		t.Error("expected 1 after unscheduling non-existent")
	}
}

func TestCronSchedulerScheduleInvalid(t *testing.T) {
	s := NewCronScheduler()

	// Empty todo ID.
	if err := s.Schedule("", "0 9 * * *"); err == nil {
		t.Error("expected error for empty todoID")
	}

	// Wrong number of fields.
	if err := s.Schedule("st-1", "0 9 * *"); err == nil {
		t.Error("expected error for 4 fields")
	}

	// Invalid value.
	if err := s.Schedule("st-1", "60 9 * * *"); err == nil {
		t.Error("expected error for minute=60")
	}

	// Invalid syntax.
	if err := s.Schedule("st-1", "abc 9 * * *"); err == nil {
		t.Error("expected error for non-numeric minute")
	}
}

func TestCronSchedulerDue(t *testing.T) {
	s := NewCronScheduler()

	// Schedule two todos: one due in the past, one in the future.
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	s.mu.Lock()
	s.jobs["st-1"] = &ScheduledTodo{TodoID: "st-1", Cron: "0 9 * * *", NextRun: past}
	s.jobs["st-2"] = &ScheduledTodo{TodoID: "st-2", Cron: "0 10 * * *", NextRun: future}
	s.mu.Unlock()

	due := s.Due(time.Now())
	if len(due) != 1 {
		t.Fatalf("expected 1 due todo, got %d", len(due))
	}
	if due[0].TodoID != "st-1" {
		t.Errorf("expected st-1, got %q", due[0].TodoID)
	}
}

func TestCronSchedulerDueNone(t *testing.T) {
	s := NewCronScheduler()
	future := time.Now().Add(1 * time.Hour)

	s.mu.Lock()
	s.jobs["st-1"] = &ScheduledTodo{TodoID: "st-1", Cron: "0 9 * * *", NextRun: future}
	s.mu.Unlock()

	due := s.Due(time.Now())
	if len(due) != 0 {
		t.Errorf("expected 0 due todos, got %d", len(due))
	}
}

func TestCronNextRunEvery5Minutes(t *testing.T) {
	s := NewCronScheduler()
	from := time.Date(2026, 6, 18, 9, 2, 0, 0, time.UTC)
	next, err := s.NextRun("*/5 * * * *", from)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	expected := time.Date(2026, 6, 18, 9, 5, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestCronNextRunDailyAt9am(t *testing.T) {
	s := NewCronScheduler()
	from := time.Date(2026, 6, 18, 10, 30, 0, 0, time.UTC)
	next, err := s.NextRun("0 9 * * *", from)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	expected := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestCronNextRunHourly(t *testing.T) {
	s := NewCronScheduler()
	from := time.Date(2026, 6, 18, 9, 15, 0, 0, time.UTC)
	next, err := s.NextRun("0 * * * *", from)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	expected := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestCronNextRunWithRange(t *testing.T) {
	s := NewCronScheduler()
	// Every 15 minutes during 9-17 hours (business hours).
	from := time.Date(2026, 6, 18, 8, 50, 0, 0, time.UTC) // Thursday
	next, err := s.NextRun("*/15 9-17 * * *", from)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	expected := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestCronSchedulerAdvanceNextRun(t *testing.T) {
	s := NewCronScheduler()
	if err := s.Schedule("st-1", "*/5 * * * *"); err != nil {
		t.Fatal(err)
	}

	original := s.Get("st-1").NextRun
	// Use a time after the original NextRun to ensure the new NextRun advances.
	now := original.Add(1 * time.Minute)

	if err := s.AdvanceNextRun("st-1", now); err != nil {
		t.Fatalf("AdvanceNextRun: %v", err)
	}

	updated := s.Get("st-1")
	if !updated.LastRun.Equal(now) {
		t.Errorf("expected LastRun = %v, got %v", now, updated.LastRun)
	}
	if !updated.NextRun.After(now) {
		t.Errorf("expected NextRun after now (%v), got %v", now, updated.NextRun)
	}
	if !updated.NextRun.After(original) {
		t.Errorf("expected NextRun after original (%v), got %v", original, updated.NextRun)
	}
}

func TestCronSchedulerConcurrentAccess(t *testing.T) {
	s := NewCronScheduler()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "st-" + string(rune('A'+n%26)) + string(rune('0'+n/26))
			_ = s.Schedule(id, "*/5 * * * *")
			s.List()
			s.Due(time.Now())
			s.Unschedule(id)
		}(i)
	}
	wg.Wait()

	// All jobs should have been scheduled then unscheduled.
	if len(s.List()) != 0 {
		t.Errorf("expected 0 jobs after concurrent schedule+unschedule, got %d", len(s.List()))
	}
}
