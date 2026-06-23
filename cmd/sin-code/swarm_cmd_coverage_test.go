// SPDX-License-Identifier: MIT
// Purpose: coverage tests for swarm_cmd.go — every remaining branch of
// executeSwarm, defaultAgentRunner, perAgentDBPath, and randHex is
// exercised through package-level hooks so no real LLM backend is needed.
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
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// setSwarmHook replaces a package-level hook for the duration of a test.
func setSwarmHook[T any](t *testing.T, ptr *T, val T) {
	orig := *ptr
	*ptr = val
	t.Cleanup(func() { *ptr = orig })
}

func TestSwarm_RunSwarm(t *testing.T) {
	ws := t.TempDir()
	setSwarmHook(t, &defaultAgentRunnerHook, func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
		loop := &agentloop.Loop{
			Gate: verify.NewGate("off", nil, nil),
			Perm: permission.New([]permission.Rule{{Tool: "*", Policy: "deny"}}),
			Completion: func(_ context.Context, _ []session.Message, _ []agentloop.ToolSpec) (*agentloop.Completion, error) {
				return &agentloop.Completion{
					Text: "done",
					Raw:  session.Message{Role: "assistant", Content: "done"},
				}, nil
			},
			MaxTurns: 1,
		}
		store, err := session.Open(workspace + "/" + agentName + ".db")
		if err != nil {
			return nil, nil, nil, err
		}
		sess, err := store.StartOrResume("")
		if err != nil {
			_ = store.Close()
			return nil, nil, nil, err
		}
		return loop, sess, func() error { return store.Close() }, nil
	})

	opts := &swarmOptions{
		prompt:    "p",
		agentCSV:  "a,b",
		workspace: ws,
		timeout:   5 * time.Second,
		maxTurns:  1,
	}
	if err := runSwarm(context.Background(), opts); err != nil {
		t.Fatalf("runSwarm: %v", err)
	}
}

func TestSwarm_NormalizeDefaults(t *testing.T) {
	ws := t.TempDir()
	setSwarmHook(t, &defaultAgentRunnerHook, func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
		loop := &agentloop.Loop{
			Gate: verify.NewGate("off", nil, nil),
			Perm: permission.New([]permission.Rule{{Tool: "*", Policy: "deny"}}),
			Completion: func(_ context.Context, _ []session.Message, _ []agentloop.ToolSpec) (*agentloop.Completion, error) {
				return &agentloop.Completion{
					Text: "done",
					Raw:  session.Message{Role: "assistant", Content: "done"},
				}, nil
			},
			MaxTurns: 1,
		}
		store, err := session.Open(workspace + "/" + agentName + ".db")
		if err != nil {
			return nil, nil, nil, err
		}
		sess, err := store.StartOrResume("")
		if err != nil {
			_ = store.Close()
			return nil, nil, nil, err
		}
		return loop, sess, func() error { return store.Close() }, nil
	})

	opts := &swarmOptions{
		prompt:    "p",
		agentCSV:  "a,b",
		workspace: ws,
		timeout:   0,
		maxTurns:  0,
	}
	if _, err := executeSwarm(context.Background(), opts); err != nil {
		t.Fatalf("executeSwarm: %v", err)
	}
	if opts.timeout != swarmDefaultTimeout {
		t.Errorf("timeout = %v, want %v", opts.timeout, swarmDefaultTimeout)
	}
	if opts.maxTurns != swarmDefaultTurns {
		t.Errorf("maxTurns = %d, want %d", opts.maxTurns, swarmDefaultTurns)
	}
}

func TestSwarm_OSGetwdSuccess(t *testing.T) {
	ws := t.TempDir()
	setSwarmHook(t, &swarmOSGetwdHook, func() (string, error) {
		return ws, nil
	})
	setSwarmHook(t, &defaultAgentRunnerHook, func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
		loop := &agentloop.Loop{
			Gate: verify.NewGate("off", nil, nil),
			Perm: permission.New([]permission.Rule{{Tool: "*", Policy: "deny"}}),
			Completion: func(_ context.Context, _ []session.Message, _ []agentloop.ToolSpec) (*agentloop.Completion, error) {
				return &agentloop.Completion{
					Text: "done",
					Raw:  session.Message{Role: "assistant", Content: "done"},
				}, nil
			},
			MaxTurns: 1,
		}
		store, err := session.Open(workspace + "/" + agentName + ".db")
		if err != nil {
			return nil, nil, nil, err
		}
		sess, err := store.StartOrResume("")
		if err != nil {
			_ = store.Close()
			return nil, nil, nil, err
		}
		return loop, sess, func() error { return store.Close() }, nil
	})

	opts := &swarmOptions{
		prompt:   "p",
		agentCSV: "a,b",
		timeout:  5 * time.Second,
		maxTurns: 1,
	}
	if _, err := executeSwarm(context.Background(), opts); err != nil {
		t.Fatalf("executeSwarm: %v", err)
	}
}

func TestSwarm_OSGetwdError(t *testing.T) {
	setSwarmHook(t, &swarmOSGetwdHook, func() (string, error) {
		return "", errors.New("getwd failed")
	})
	opts := &swarmOptions{
		prompt:   "p",
		agentCSV: "a,b",
	}
	_, err := executeSwarm(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "getwd failed") {
		t.Fatalf("expected getwd error, got %v", err)
	}
}

func TestSwarm_DefaultRunnerAssignment(t *testing.T) {
	ws := t.TempDir()
	var called atomic.Bool
	setSwarmHook(t, &defaultAgentRunnerHook, func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
		called.Store(true)
		loop := &agentloop.Loop{
			Gate: verify.NewGate("off", nil, nil),
			Perm: permission.New([]permission.Rule{{Tool: "*", Policy: "deny"}}),
			Completion: func(_ context.Context, _ []session.Message, _ []agentloop.ToolSpec) (*agentloop.Completion, error) {
				return &agentloop.Completion{
					Text: "done",
					Raw:  session.Message{Role: "assistant", Content: "done"},
				}, nil
			},
			MaxTurns: 1,
		}
		store, err := session.Open(workspace + "/" + agentName + ".db")
		if err != nil {
			return nil, nil, nil, err
		}
		sess, err := store.StartOrResume("")
		if err != nil {
			_ = store.Close()
			return nil, nil, nil, err
		}
		return loop, sess, func() error { return store.Close() }, nil
	})

	opts := &swarmOptions{
		prompt:    "p",
		agentCSV:  "a,b",
		workspace: ws,
		timeout:   5 * time.Second,
		maxTurns:  1,
		runner:    nil,
	}
	if _, err := executeSwarm(context.Background(), opts); err != nil {
		t.Fatalf("executeSwarm: %v", err)
	}
	if !called.Load() {
		t.Error("defaultAgentRunnerHook was not used")
	}
}

func TestSwarm_RunnerSetupError(t *testing.T) {
	ws := t.TempDir()
	setSwarmHook(t, &defaultAgentRunnerHook, func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
		return nil, nil, nil, errors.New("setup failed")
	})
	opts := &swarmOptions{
		prompt:    "p",
		agentCSV:  "a,b",
		workspace: ws,
		timeout:   5 * time.Second,
		maxTurns:  1,
	}
	report, err := executeSwarm(context.Background(), opts)
	if err != nil {
		t.Fatalf("executeSwarm: %v", err)
	}
	for _, r := range report.Results {
		if r.Status != "FAILED" {
			t.Errorf("expected FAILED for setup error, got %s", r.Status)
		}
	}
}

func TestSwarm_ContextCancellation(t *testing.T) {
	ws := t.TempDir()
	setSwarmHook(t, &defaultAgentRunnerHook, func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
		loop := &agentloop.Loop{
			Gate: verify.NewGate("off", nil, nil),
			Perm: permission.New([]permission.Rule{{Tool: "*", Policy: "deny"}}),
			Completion: func(ctx context.Context, _ []session.Message, _ []agentloop.ToolSpec) (*agentloop.Completion, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			MaxTurns: 200,
		}
		store, err := session.Open(workspace + "/" + agentName + ".db")
		if err != nil {
			return nil, nil, nil, err
		}
		sess, err := store.StartOrResume("")
		if err != nil {
			_ = store.Close()
			return nil, nil, nil, err
		}
		return loop, sess, func() error { return store.Close() }, nil
	})
	opts := &swarmOptions{
		prompt:    "p",
		agentCSV:  "a,b",
		workspace: ws,
		timeout:   1 * time.Millisecond,
		maxTurns:  50,
	}
	report, err := executeSwarm(context.Background(), opts)
	if err != nil {
		t.Fatalf("executeSwarm: %v", err)
	}
	if report.Error == "" {
		t.Error("expected non-empty error in report after timeout")
	}
	for _, r := range report.Results {
		if r.Status != "CANCELLED" && r.Status != "TIMEOUT" {
			t.Errorf("unexpected status %s for cancelled agent", r.Status)
		}
	}
}

func TestDefaultAgentRunner_BuildError(t *testing.T) {
	setSwarmHook(t, &swarmLoopbuilderBuildHook, func(ctx context.Context, cfg loopbuilder.Config, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
		return nil, nil, errors.New("build failed")
	})
	_, _, _, err := defaultAgentRunner(context.Background(), "agent", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Fatalf("expected build error, got %v", err)
	}
}

func TestDefaultAgentRunner_DBPathError(t *testing.T) {
	setSwarmHook(t, &swarmLoopbuilderBuildHook, func(ctx context.Context, cfg loopbuilder.Config, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{}, func() error { return nil }, nil
	})
	setSwarmHook(t, &swarmMkdirAllHook, func(path string, perm os.FileMode) error {
		return errors.New("mkdir failed")
	})
	_, _, _, err := defaultAgentRunner(context.Background(), "agent", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "mkdir failed") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestDefaultAgentRunner_SessionOpenError(t *testing.T) {
	setSwarmHook(t, &swarmLoopbuilderBuildHook, func(ctx context.Context, cfg loopbuilder.Config, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{}, func() error { return nil }, nil
	})
	setSwarmHook(t, &swarmSessionOpenHook, func(dbPath string) (*session.Store, error) {
		return nil, errors.New("session open failed")
	})
	_, _, _, err := defaultAgentRunner(context.Background(), "agent", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "session open failed") {
		t.Fatalf("expected session open error, got %v", err)
	}
}

func TestDefaultAgentRunner_StartOrResumeError(t *testing.T) {
	setSwarmHook(t, &swarmLoopbuilderBuildHook, func(ctx context.Context, cfg loopbuilder.Config, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{}, func() error { return nil }, nil
	})
	setSwarmHook(t, &swarmSessionOpenHook, func(dbPath string) (*session.Store, error) {
		store, err := session.Open(dbPath)
		if err != nil {
			return nil, err
		}
		_ = store.Close()
		return store, nil
	})
	_, _, _, err := defaultAgentRunner(context.Background(), "agent", t.TempDir())
	if err == nil {
		t.Fatal("expected start/resume error")
	}
}

func TestDefaultAgentRunner_CleanupError(t *testing.T) {
	setSwarmHook(t, &swarmLoopbuilderBuildHook, func(ctx context.Context, cfg loopbuilder.Config, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{}, func() error { return errors.New("cleanup failed") }, nil
	})
	setSwarmHook(t, &swarmSessionOpenHook, func(dbPath string) (*session.Store, error) {
		return session.Open(dbPath)
	})
	_, _, cleanup, err := defaultAgentRunner(context.Background(), "agent", t.TempDir())
	if err != nil {
		t.Fatalf("defaultAgentRunner: %v", err)
	}
	if err := cleanup(); err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("expected cleanup error, got %v", err)
	}
}

func TestPerAgentDBPathMkdirError(t *testing.T) {
	setSwarmHook(t, &swarmMkdirAllHook, func(path string, perm os.FileMode) error {
		return errors.New("mkdir failed")
	})
	_, err := perAgentDBPath(t.TempDir(), "agent")
	if err == nil || !strings.Contains(err.Error(), "mkdir failed") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestRandHexError(t *testing.T) {
	setSwarmHook(t, &swarmRandReadHook, func(b []byte) (int, error) {
		return 0, errors.New("rand failed")
	})
	if got := randHex(4); got != "00000000" {
		t.Errorf("randHex error = %q, want 00000000", got)
	}
}
