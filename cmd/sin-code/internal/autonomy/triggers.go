// SPDX-License-Identifier: MIT
// Purpose: autonomy triggers — cron schedules and file watchers that
// enqueue goals without human prompts. Reactive becomes autonomous.
package autonomy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Trigger struct {
	Type     string `json:"type"`
	Every    string `json:"every"`
	Glob     string `json:"glob"`
	Debounce string `json:"debounce"`
	Prompt   string `json:"prompt"`
	Priority int    `json:"priority"`
}

// LoadTriggers reads .sin-code/triggers.json from the workspace.
func LoadTriggers(workspace string) []Trigger {
	data, err := os.ReadFile(filepath.Join(workspace, ".sin-code", "triggers.json"))
	if err != nil {
		return nil
	}
	var ts []Trigger
	if err := jsonUnmarshal(data, &ts); err != nil {
		fmt.Fprintf(os.Stderr, "warn: invalid triggers.json: %v\n", err)
		return nil
	}
	return ts
}

// Runner drives all triggers for one workspace, enqueueing goals.
type Runner struct {
	Queue        *Queue
	Workspace    string
	Triggers     []Trigger
	PollInterval time.Duration
}

func (r *Runner) Run(ctx context.Context) error {
	if r.PollInterval <= 0 {
		r.PollInterval = 10 * time.Second
	}
	for i := range r.Triggers {
		t := r.Triggers[i]
		switch t.Type {
		case "cron":
			go r.runCron(ctx, t)
		case "watch":
			go r.runWatch(ctx, t)
		default:
			fmt.Fprintf(os.Stderr, "warn: unknown trigger type %q\n", t.Type)
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (r *Runner) runCron(ctx context.Context, t Trigger) {
	interval, err := time.ParseDuration(t.Every)
	if err != nil || interval < time.Minute {
		fmt.Fprintf(os.Stderr, "warn: cron trigger needs every >= 1m, got %q\n", t.Every)
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.Queue.Add(ctx, t.Prompt, r.Workspace, t.Priority, 1); err != nil {
				fmt.Fprintf(os.Stderr, "warn: cron enqueue failed: %v\n", err)
			}
		}
	}
}

func (r *Runner) runWatch(ctx context.Context, t Trigger) {
	debounce := 30 * time.Second
	if d, err := time.ParseDuration(t.Debounce); err == nil && d > 0 {
		debounce = d
	}
	last := fingerprint(r.Workspace, t.Glob)
	var dirtySince time.Time

	ticker := time.NewTicker(r.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := fingerprint(r.Workspace, t.Glob)
			if current != last {
				last = current
				dirtySince = time.Now()
				continue
			}
			if !dirtySince.IsZero() && time.Since(dirtySince) >= debounce {
				dirtySince = time.Time{}
				prompt := t.Prompt + "\n(triggered by changes matching " + t.Glob + ")"
				if _, err := r.Queue.Add(ctx, prompt, r.Workspace, t.Priority, 1); err != nil {
					fmt.Fprintf(os.Stderr, "warn: watch enqueue failed: %v\n", err)
				}
			}
		}
	}
}

// fingerprint hashes path+mtime+size of all glob matches (content-cheap,
// CGo-free; poll-based so no fsnotify dependency).
func fingerprint(workspace, glob string) string {
	h := sha256Sum()
	_ = filepath.WalkDir(workspace, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == ".sin-code" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(workspace, path)
		ok, _ := filepath.Match(glob, rel)
		if !ok && !matchDoubleStar(glob, rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s|%d|%d\n", rel, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return hexEncodeToString(h)
}

// matchDoubleStar supports the "dir/**/*.ext" pattern that
// filepath.Match does not.
func matchDoubleStar(pattern, rel string) bool {
	parts := strings.SplitN(pattern, "**/", 2)
	if len(parts) != 2 {
		return false
	}
	if parts[0] != "" && !strings.HasPrefix(rel, strings.TrimSuffix(parts[0], "/")) {
		return false
	}
	ok, _ := filepath.Match(parts[1], filepath.Base(rel))
	return ok
}
