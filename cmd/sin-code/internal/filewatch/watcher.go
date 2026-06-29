// SPDX-License-Identifier: MIT
// Purpose: Workspace file-watch with pattern matching, debounce, and
// command execution (issue #486). Watches the current directory tree
// recursively (skipping VCS / build / vendor dirs), matches changed
// files against user-supplied glob patterns, debounces rapid-fire
// events, and runs an arbitrary shell command via os/exec on match.
//
// Mandates honored:
//   M2 — pure-Go (fsnotify has no CGO), CGO_ENABLED=0 compatible.
//   M4 — watch-triggered commands run via os/exec directly, NOT through
//        the agent-loop permission engine. This is a user-initiated CLI
//        tool, not an agent action; the operator opted in by running
//        `sin-code watch` interactively.
//   M5 — module path github.com/OpenSIN-Code/SIN-Code.
//   M7 — the watcher goroutine is race-free: all mutable state is
//        guarded by a sync.Mutex or communicated over channels.
package filewatch

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounce is the default delay (in milliseconds) between the last
// matched file event and the command invocation. Bulk saves (e.g. editor
// "save all") produce a flurry of WRITE/CREATE events; the debounce
// collapses them into a single run.
const DefaultDebounce = 2000

// skipDirs are directory basenames that are never watched. They match
// VCS metadata, dependency caches, and build outputs — recursing into
// them is noisy, expensive, and never useful.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".sin-code":    true,
	"dist":         true,
	"build":        true,
}

// Rule pairs a glob pattern (matched via filepath.Match against the
// changed file's basename or a path-relative segment) with the shell
// command to run on match. A Watcher holds zero or more Rules.
type Rule struct {
	Pattern string // e.g. "*.go", "*.ts", "src/*.py"
	Command string // shell command executed via `sh -c` (or cmd /c on Windows)
}

// Watcher is a recursive directory watcher that runs commands when
// files matching registered patterns change. The zero value is not
// usable — construct with New.
type Watcher struct {
	fw       *fsnotify.Watcher
	root     string
	rules    []Rule
	debounce time.Duration

	out io.Writer
	err io.Writer

	mu     sync.Mutex
	stopCh chan struct{}
	doneCh chan struct{}

	// timer is owned by the loop goroutine (only debouncedRun, called
	// from loop, touches it) — no lock needed.
	timer      *time.Timer
	pendingMu  sync.Mutex
	pendingRule Rule
	runMu      sync.Mutex
}

// New constructs a Watcher rooted at root that will run the supplied
// rules when files change. Output from triggered commands is written
// to out (stdout) and err (stderr); pass nil to use os.Stdout/os.Stderr.
// The watcher is not started until Watch is called.
func New(root string, rules []Rule, debounceMs int, out, errw io.Writer) (*Watcher, error) {
	if debounceMs < 0 {
		debounceMs = DefaultDebounce
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("filewatch: create fsnotify watcher: %w", err)
	}
	if out == nil {
		out = os.Stdout
	}
	if errw == nil {
		errw = os.Stderr
	}
	return &Watcher{
		fw:       fw,
		root:     root,
		rules:    append([]Rule(nil), rules...),
		debounce: time.Duration(debounceMs) * time.Millisecond,
		out:      out,
		err:      errw,
	}, nil
}

// Watch starts the watcher goroutine. It walks root recursively, adding
// a kernel watch to every non-skipped directory, then drains fsnotify
// events. Only WRITE and CREATE events are considered. The first event
// whose file matches a Rule pattern arms (or resets) the debounce timer;
// when the timer fires the matched rule's command runs. Watch blocks
// until ctx is cancelled or Stop is called.
//
// Watch is safe to call exactly once per Watcher. Calling it twice on
// the same Watcher is undefined.
func (w *Watcher) Watch(ctx context.Context) error {
	if err := w.addWatches(w.root); err != nil {
		return fmt.Errorf("filewatch: add watches: %w", err)
	}

	w.mu.Lock()
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	w.mu.Unlock()

	go w.loop(ctx)
	return nil
}

// Stop signals the watcher goroutine to exit and waits for it. It is
// idempotent and safe to call multiple times. After Stop returns the
// underlying fsnotify watcher is closed and no further events fire.
func (w *Watcher) Stop() {
	w.mu.Lock()
	stop := w.stopCh
	done := w.doneCh
	w.mu.Unlock()
	if stop == nil {
		return
	}
	select {
	case <-stop:
	default:
		close(stop)
	}
	if done != nil {
		<-done
	}
	_ = w.fw.Close()
}

// addWatches recursively adds kernel watches to every non-skipped
// directory under path. Symlinks are not followed.
func (w *Watcher) addWatches(path string) error {
	return filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		return w.fw.Add(p)
	})
}

// loop is the event-processing goroutine. It is race-free (M7): the
// debounce timer is accessed only on this goroutine; command execution
// is serialized by runMu; Stop/close coordination happens via stopCh
// and doneCh.
func (w *Watcher) loop(ctx context.Context) {
	defer func() {
		w.mu.Lock()
		if w.doneCh != nil {
			close(w.doneCh)
		}
		w.mu.Unlock()
	}()

	debounce := w.debouncedRun

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopSignal():
			return
		case ev, ok := <-w.fw.Events:
			if !ok {
				return
			}
			if !isWriteOrCreate(ev) {
				continue
			}
			rule, ok := w.matchRule(ev.Name)
			if !ok {
				continue
			}
			debounce(rule)
		case _, ok := <-w.fw.Errors:
			if !ok {
				return
			}
		}
	}
}

// stopSignal returns a receive-only view of stopCh, or a nil channel
// (which blocks forever) if Watch was never called. Reading from a nil
// channel in a select never fires, so the goroutine simply ignores it.
func (w *Watcher) stopSignal() <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stopCh
}

// debouncedRun arms (or resets) the single debounce timer. When the
// timer fires the last-matched rule's command runs. Subsequent events
// within the debounce window reset the timer and update the pending
// rule — this collapses bulk saves into one invocation that runs the
// most-recently-matched rule (time.Timer.Reset does not rebind closure
// variables, so the pending rule is stored in a mutex-protected field
// and read by the timer callback).
func (w *Watcher) debouncedRun(rule Rule) {
	w.pendingMu.Lock()
	w.pendingRule = rule
	w.pendingMu.Unlock()
	if w.timer == nil {
		w.timer = time.AfterFunc(w.debounce, func() {
			w.pendingMu.Lock()
			r := w.pendingRule
			w.pendingMu.Unlock()
			w.runMu.Lock()
			defer w.runMu.Unlock()
			w.runCommand(r)
		})
		return
	}
	w.timer.Reset(w.debounce)
}

// runCommand executes rule.Command via the system shell, streaming
// stdout/stderr live to the watcher's writers. On failure it prints
// the error and returns — the watcher keeps running (issue #486:
// "on command failure: print error but keep watching").
func (w *Watcher) runCommand(rule Rule) {
	cmd := shellCommand(rule.Command)
	cmd.Stdout = w.out
	cmd.Stderr = w.err
	fmt.Fprintf(w.out, "\n[filewatch] pattern %q → running: %s\n", rule.Pattern, rule.Command)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(w.err, "[filewatch] command failed (%.2fs): %v\n", time.Since(start).Seconds(), err)
		return
	}
	fmt.Fprintf(w.out, "[filewatch] command ok (%.2fs)\n", time.Since(start).Seconds())
}

// matchRule returns the first rule whose Pattern matches the changed
// file's name, plus true. Patterns are matched with filepath.Match.
// A pattern containing a slash is matched against the path relative to
// the watch root (using forward slashes for portability); otherwise it
// is matched against the basename only. This lets users write both
// "*.go" (any depth) and "src/*.py" (path-anchored).
func (w *Watcher) matchRule(name string) (Rule, bool) {
	rel, relErr := filepath.Rel(w.root, name)
	if relErr != nil {
		rel = name
	}
	rel = filepath.ToSlash(rel)
	base := filepath.Base(name)
	for _, r := range w.rules {
		if r.Pattern == "" {
			continue
		}
		if strings.Contains(r.Pattern, "/") {
			if ok, _ := filepath.Match(r.Pattern, rel); ok {
				return r, true
			}
		} else {
			if ok, _ := filepath.Match(r.Pattern, base); ok {
				return r, true
			}
		}
	}
	return Rule{}, false
}

// isWriteOrCreate reports whether ev corresponds to a file write or
// creation — the only events that should trigger a run (issue #486).
// REMOVE/RENAME/CHMOD are ignored.
func isWriteOrCreate(ev fsnotify.Event) bool {
	return ev.Op&(fsnotify.Write|fsnotify.Create) != 0
}

// shellCommand builds the os/exec.Cmd for a shell command string. On
// Unix it uses `sh -c`; on Windows `cmd /c`. The command inherits the
// parent environment.
func shellCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", command)
	}
	return exec.Command("sh", "-c", command)
}

// MatchRule is the package-level equivalent of (*Watcher).matchRule,
// exported for testing without constructing a full Watcher.
func MatchRule(root, name string, rules []Rule) (Rule, bool) {
	w := &Watcher{root: root, rules: rules}
	return w.matchRule(name)
}

// SkipDir reports whether a directory basename should be skipped
// during the recursive walk. Exported for testing.
func SkipDir(name string) bool {
	return skipDirs[name]
}

// IsWriteOrCreate is the package-level event filter, exported for
// testing.
func IsWriteOrCreate(ev fsnotify.Event) bool {
	return isWriteOrCreate(ev)
}
