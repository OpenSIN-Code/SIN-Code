// SPDX-License-Identifier: MIT
// Purpose: CronScheduler — schedules todos based on cron expressions.
// Supports a subset of standard 5-field cron syntax:
//
//	minute hour day-of-month month day-of-week
//
// Supported patterns per field: * (any), */N (every N), specific number,
// comma-list (1,3,5), range (0-5). Thread-safe (M7).
package todo

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ScheduledTodo pairs a todo ID with a cron expression and computed
// next/last run times.
type ScheduledTodo struct {
	TodoID  string    `json:"todo_id"`
	Cron    string    `json:"cron"`
	NextRun time.Time `json:"next_run"`
	LastRun time.Time `json:"last_run"`
}

// CronScheduler schedules todos based on cron expressions. It is
// thread-safe (M7) — all mutations go through a mutex.
type CronScheduler struct {
	mu   sync.RWMutex
	jobs map[string]*ScheduledTodo
}

// NewCronScheduler creates an empty scheduler.
func NewCronScheduler() *CronScheduler {
	return &CronScheduler{
		jobs: make(map[string]*ScheduledTodo),
	}
}

// Schedule attaches a cron expression to a todo. If the todo already
// has a schedule, it is replaced. The NextRun is computed immediately.
func (s *CronScheduler) Schedule(todoID, cronExpr string) error {
	if todoID == "" {
		return fmt.Errorf("cron_scheduler: todoID required")
	}
	fields := strings.Fields(cronExpr)
	if len(fields) != 5 {
		return fmt.Errorf("cron_scheduler: expected 5 fields, got %d in %q", len(fields), cronExpr)
	}
	parsed, err := parseCronFields(fields)
	if err != nil {
		return fmt.Errorf("cron_scheduler: %w", err)
	}
	next, err := cronNext(parsed, time.Now())
	if err != nil {
		return fmt.Errorf("cron_scheduler: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[todoID] = &ScheduledTodo{
		TodoID:  todoID,
		Cron:    cronExpr,
		NextRun: next,
	}
	return nil
}

// Unschedule removes the cron schedule for a todo. No-op if the todo
// was not scheduled.
func (s *CronScheduler) Unschedule(todoID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, todoID)
}

// Due returns all scheduled todos whose NextRun is at or before `now`.
// The returned slice is sorted by NextRun ascending. This method does
// NOT update LastRun — call it in a loop and update NextRun separately.
func (s *CronScheduler) Due(now time.Time) []ScheduledTodo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []ScheduledTodo
	for _, job := range s.jobs {
		if !job.NextRun.After(now) {
			out = append(out, *job)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NextRun.Before(out[j].NextRun)
	})
	return out
}

// NextRun calculates the next time the given cron expression fires
// after `from`. Returns an error for invalid expressions.
func (s *CronScheduler) NextRun(cronExpr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(cronExpr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("expected 5 fields, got %d in %q", len(fields), cronExpr)
	}
	parsed, err := parseCronFields(fields)
	if err != nil {
		return time.Time{}, err
	}
	return cronNext(parsed, from)
}

// List returns all scheduled todos sorted by NextRun ascending.
func (s *CronScheduler) List() []ScheduledTodo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]ScheduledTodo, 0, len(s.jobs))
	for _, job := range s.jobs {
		out = append(out, *job)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NextRun.Before(out[j].NextRun)
	})
	return out
}

// Get returns the ScheduledTodo for a todoID, or nil if not scheduled.
func (s *CronScheduler) Get(todoID string) *ScheduledTodo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if job, ok := s.jobs[todoID]; ok {
		st := *job
		return &st
	}
	return nil
}

// AdvanceNextRun recomputes and sets the NextRun for a scheduled todo
// after it has been dispatched. Returns an error if the todo is not
// scheduled or the cron expression is invalid.
func (s *CronScheduler) AdvanceNextRun(todoID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[todoID]
	if !ok {
		return fmt.Errorf("cron_scheduler: todo %q not scheduled", todoID)
	}
	fields := strings.Fields(job.Cron)
	parsed, err := parseCronFields(fields)
	if err != nil {
		return err
	}
	next, err := cronNext(parsed, now)
	if err != nil {
		return err
	}
	job.LastRun = now
	job.NextRun = next
	return nil
}

// ── cron field parsing ──────────────────────────────────────────────

// cronField represents a parsed cron field: either a wildcard, a
// step value, or a set of specific values.
type cronField struct {
	wildcard bool
	step     int
	values   map[int]bool
}

type cronSchedule struct {
	minute, hour, dom, month, dow cronField
}

// cronRanges defines the min/max for each cron field.
var cronRanges = [5][2]int{
	{0, 59}, // minute
	{0, 23}, // hour
	{1, 31}, // day of month
	{1, 12}, // month
	{0, 6},  // day of week (0=Sun)
}

func parseCronFields(fields []string) (*cronSchedule, error) {
	cs := &cronSchedule{}
	var err error
	cs.minute, err = parseCronField(fields[0], 0)
	if err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	cs.hour, err = parseCronField(fields[1], 1)
	if err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	cs.dom, err = parseCronField(fields[2], 2)
	if err != nil {
		return nil, fmt.Errorf("day-of-month: %w", err)
	}
	cs.month, err = parseCronField(fields[3], 3)
	if err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	cs.dow, err = parseCronField(fields[4], 4)
	if err != nil {
		return nil, fmt.Errorf("day-of-week: %w", err)
	}
	return cs, nil
}

func parseCronField(s string, fieldIdx int) (cronField, error) {
	min, max := cronRanges[fieldIdx][0], cronRanges[fieldIdx][1]
	f := cronField{values: make(map[int]bool)}

	// Handle */N
	if strings.HasPrefix(s, "*/") {
		stepStr := strings.TrimPrefix(s, "*/")
		step, err := strconv.Atoi(stepStr)
		if err != nil || step <= 0 {
			return f, fmt.Errorf("invalid step %q", s)
		}
		f.wildcard = true
		f.step = step
		for v := min; v <= max; v += step {
			f.values[v] = true
		}
		return f, nil
	}

	// Handle * (wildcard)
	if s == "*" {
		f.wildcard = true
		f.step = 1
		for v := min; v <= max; v++ {
			f.values[v] = true
		}
		return f, nil
	}

	// Handle comma-separated list and ranges
	parts := strings.Split(s, ",")
	for _, part := range parts {
		if strings.Contains(part, "-") {
			rng := strings.SplitN(part, "-", 2)
			lo, err := strconv.Atoi(rng[0])
			if err != nil {
				return f, fmt.Errorf("invalid range %q", part)
			}
			hi, err := strconv.Atoi(rng[1])
			if err != nil {
				return f, fmt.Errorf("invalid range %q", part)
			}
			if lo < min || hi > max || lo > hi {
				return f, fmt.Errorf("range %q out of bounds [%d,%d]", part, min, max)
			}
			for v := lo; v <= hi; v++ {
				f.values[v] = true
			}
		} else {
			v, err := strconv.Atoi(part)
			if err != nil {
				return f, fmt.Errorf("invalid value %q", part)
			}
			if v < min || v > max {
				return f, fmt.Errorf("value %d out of bounds [%d,%d]", v, min, max)
			}
			f.values[v] = true
		}
	}

	return f, nil
}

// ── next-run computation ────────────────────────────────────────────

// cronNext returns the next time after `from` that matches the cron
// schedule. It iterates minute by minute up to a reasonable limit
// (2 years) to avoid infinite loops on impossible schedules.
func cronNext(cs *cronSchedule, from time.Time) (time.Time, error) {
	// Start from the next minute, truncating seconds.
	t := from.Truncate(time.Minute).Add(time.Minute)

	for i := 0; i < 365*24*60*2; i++ { // 2 years of minutes
		if cs.month.values[int(t.Month())] &&
			cs.dom.values[t.Day()] &&
			cs.dow.values[int(t.Weekday())] &&
			cs.hour.values[t.Hour()] &&
			cs.minute.values[t.Minute()] {
			return t, nil
		}
		t = t.Add(time.Minute)
	}

	return time.Time{}, fmt.Errorf("no matching time within 2 years")
}
