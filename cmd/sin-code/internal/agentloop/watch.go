// SPDX-License-Identifier: MIT
// Purpose: Watch mode for the agent loop. Monitors file changes via polling
// (no fsnotify dependency — mandate M2) and triggers a callback when matching
// files are modified. Debounces bursts so multiple rapid changes fire only
// one callback. Thread-safe (mandate M7).
package agentloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	watchPollInterval = 500 * time.Millisecond
	watchDebounce     = 200 * time.Millisecond
)

var watchIgnoreDirs = []string{
	".git", "vendor", "node_modules", "__pycache__", ".sin-code",
	".sin", "dist", "build", "target", ".cache", "tmp",
}

type fileState struct {
	modTime time.Time
	size    int64
}

type WatchMode struct {
	patterns []string
	root     string

	mu       sync.Mutex
	started  bool
	cancel   context.CancelFunc
	snapshots map[string]fileState
	pending  bool
	lastFire time.Time
}

func NewWatchMode(patterns []string) *WatchMode {
	return &WatchMode{
		patterns:  patterns,
		snapshots: make(map[string]fileState),
	}
}

func (w *WatchMode) SetRoot(root string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.root = root
}

func (w *WatchMode) Start(ctx context.Context, callback func()) error {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.started = true
	w.mu.Unlock()

	go w.loop(ctx, callback)
	return nil
}

func (w *WatchMode) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	w.started = false
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.mu.Unlock()
}

func (w *WatchMode) Active() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.started
}

func (w *WatchMode) loop(ctx context.Context, callback func()) {
	w.initialScan()
	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()
	debounceTimer := time.NewTimer(0)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}
	var debounceActive bool
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if w.scanForChanges() {
				if debounceActive {
					debounceTimer.Reset(watchDebounce)
				} else {
					debounceTimer.Reset(watchDebounce)
					debounceActive = true
				}
			}
		case <-debounceTimer.C:
			debounceActive = false
			w.mu.Lock()
			w.lastFire = time.Now()
			w.mu.Unlock()
			callback()
		}
	}
}

func (w *WatchMode) initialScan() {
	root := w.root
	if root == "" {
		root, _ = os.Getwd()
	}
	w.mu.Lock()
	w.snapshots = make(map[string]fileState)
	w.mu.Unlock()
	w.scanDir(root, true)
}

func (w *WatchMode) scanForChanges() bool {
	root := w.root
	if root == "" {
		root, _ = os.Getwd()
	}
	return w.scanDir(root, false)
}

func (w *WatchMode) scanDir(dir string, initial bool) bool {
	changed := false
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		full := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if isWatchIgnored(entry.Name()) {
				continue
			}
			if w.scanDir(full, initial) {
				changed = true
			}
			continue
		}
		if !w.matchesPattern(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		state := fileState{modTime: info.ModTime(), size: info.Size()}
		w.mu.Lock()
		prev, exists := w.snapshots[full]
		w.snapshots[full] = state
		w.mu.Unlock()
		if !initial && (!exists || prev.modTime != state.modTime || prev.size != state.size) {
			changed = true
		}
	}
	return changed
}

func (w *WatchMode) matchesPattern(name string) bool {
	if len(w.patterns) == 0 {
		return true
	}
	for _, p := range w.patterns {
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		if strings.HasPrefix(p, "*.") {
			ext := p[1:]
			if strings.HasSuffix(name, ext) {
				return true
			}
		}
	}
	return false
}

func isWatchIgnored(name string) bool {
	for _, ignored := range watchIgnoreDirs {
		if name == ignored {
			return true
		}
	}
	return false
}
