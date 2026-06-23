// SPDX-License-Identifier: MIT
// Purpose: coverage tests for serve_api_loop.go — exercise newAPILoop,
// newAPIServerForServe, and the init-time registration helper.
package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/apiweb"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
)

func openTestLessonsForAPI(t *testing.T) *lessons.Store {
	t.Helper()
	s, err := lessons.Open(filepath.Join(t.TempDir(), "lessons.db"))
	if err != nil {
		t.Fatalf("open lessons: %v", err)
	}
	return s
}

func TestNewAPILoop_LessonsOpenError(t *testing.T) {
	orig := lessonsOpenFn
	defer func() { lessonsOpenFn = orig }()

	lessonsOpenFn = func(string) (*lessons.Store, error) {
		return nil, errors.New("lessons boom")
	}

	_, _, err := newAPILoop(context.Background(), "sid", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "open lessons") {
		t.Errorf("expected open lessons error, got %v", err)
	}
}

func TestNewAPILoop_BuildError(t *testing.T) {
	origOpen := lessonsOpenFn
	origBuild := loopbuilderBuildFn
	defer func() {
		lessonsOpenFn = origOpen
		loopbuilderBuildFn = origBuild
	}()

	lessonsOpenFn = func(string) (*lessons.Store, error) {
		return openTestLessonsForAPI(t), nil
	}
	loopbuilderBuildFn = func(context.Context, loopbuilder.Config, *lessons.Store) (*agentloop.Loop, func() error, error) {
		return nil, nil, errors.New("build boom")
	}

	_, _, err := newAPILoop(context.Background(), "sid", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "build boom") {
		t.Errorf("expected build error, got %v", err)
	}
}

func TestNewAPILoop_Success(t *testing.T) {
	origOpen := lessonsOpenFn
	origBuild := loopbuilderBuildFn
	defer func() {
		lessonsOpenFn = origOpen
		loopbuilderBuildFn = origBuild
	}()

	lessonsOpenFn = func(string) (*lessons.Store, error) {
		return openTestLessonsForAPI(t), nil
	}
	loop := &agentloop.Loop{}
	cleanup := func() error { return nil }
	loopbuilderBuildFn = func(_ context.Context, cfg loopbuilder.Config, _ *lessons.Store) (*agentloop.Loop, func() error, error) {
		if cfg.Workspace == "" {
			return nil, nil, errors.New("workspace empty")
		}
		if cfg.SessionID != "sid" {
			return nil, nil, errors.New("session id mismatch")
		}
		if !cfg.Headless {
			return nil, nil, errors.New("expected headless")
		}
		return loop, cleanup, nil
	}

	gotLoop, gotCleanup, err := newAPILoop(context.Background(), "sid", t.TempDir())
	if err != nil {
		t.Fatalf("newAPILoop: %v", err)
	}
	if gotLoop != loop {
		t.Error("returned loop mismatch")
	}
	if err := gotCleanup(); err != nil {
		t.Errorf("cleanup: %v", err)
	}
}

func TestNewAPIServerForServe(t *testing.T) {
	ws := t.TempDir()
	srv := newAPIServerForServe(ws)
	if srv == nil {
		t.Fatal("expected non-nil APIServer")
	}
	if srv.NewLoop == nil {
		t.Error("expected NewLoop wired")
	}
	if srv.Workspace != ws {
		t.Errorf("workspace = %q, want %q", srv.Workspace, ws)
	}
}

func TestRegisterAPILoopFactory_Panic(t *testing.T) {
	orig := registerHTTPLoopFactoryFn
	defer func() { registerHTTPLoopFactoryFn = orig }()

	registerHTTPLoopFactoryFn = func(apiweb.NewLoopFunc) error {
		return errors.New("bad factory")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		} else if msg := fmt.Sprint(r); !strings.Contains(msg, "serve_api_loop: bad factory") {
			t.Errorf("panic message = %q", msg)
		}
	}()
	registerAPILoopFactory()
}

func TestRegisterAPILoopFactory_Success(t *testing.T) {
	orig := registerHTTPLoopFactoryFn
	defer func() { registerHTTPLoopFactoryFn = orig }()

	called := false
	registerHTTPLoopFactoryFn = func(f apiweb.NewLoopFunc) error {
		called = true
		if f == nil {
			return errors.New("nil factory")
		}
		return nil
	}
	registerAPILoopFactory()
	if !called {
		t.Error("expected factory registration")
	}
}

func TestNewAPILoop_ToolFactory(t *testing.T) {
	origOpen := lessonsOpenFn
	origBuild := loopbuilderBuildFn
	origTools := mcpManagerToolsFn
	defer func() {
		lessonsOpenFn = origOpen
		loopbuilderBuildFn = origBuild
		mcpManagerToolsFn = origTools
	}()

	lessonsOpenFn = func(string) (*lessons.Store, error) {
		return openTestLessonsForAPI(t), nil
	}
	loopbuilderBuildFn = func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (*agentloop.Loop, func() error, error) {
		if cfg.ToolFactory == nil {
			t.Fatal("expected ToolFactory")
		}
		_, _ = cfg.ToolFactory(nil)
		return &agentloop.Loop{}, func() error { return nil }, nil
	}
	mcpManagerToolsFn = func(*mcpclient.Manager) []mcpclient.Tool { return nil }

	_, cleanup, err := newAPILoop(context.Background(), "tf-sid", t.TempDir())
	if err != nil {
		t.Fatalf("newAPILoop: %v", err)
	}
	if cleanup != nil {
		cleanup()
	}
}
