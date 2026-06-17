// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for headroom_extra_cmd.go.
// Docs: headroom_extra_cmd.go
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
)

// fakeHeadroomProxy implements headroomProxy for tests.
type fakeHeadroomProxy struct {
	startErr    error
	shutdownErr error
	started     bool
	addr        string
	startDone   chan struct{}
}

func (f *fakeHeadroomProxy) Start(addr string) error {
	f.started = true
	f.addr = addr
	if f.startDone != nil {
		close(f.startDone)
	}
	return f.startErr
}

func (f *fakeHeadroomProxy) Shutdown(ctx context.Context) error { return f.shutdownErr }

// fakeHeadroomLessonStore implements headroomLessonStore for tests.
type fakeHeadroomLessonStore struct {
	lessons []*headroom.Lesson
	count   int
	topN    int
}

func (f *fakeHeadroomLessonStore) Top(n int) []*headroom.Lesson {
	f.topN = n
	return f.lessons
}

func (f *fakeHeadroomLessonStore) Count() int { return f.count }

func resetHeadroomExtraHooks(t *testing.T) {
	t.Helper()
	orig := headroomExtraHookVars
	t.Cleanup(func() { headroomExtraHookVars = orig })
}

func runHeadroomExtraCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewHeadroomCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestHeadroomProxy(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.loadConfig = func() headroom.Config { return headroom.Config{Mode: headroom.ModeProxy} }
	var gotCfg headroom.Config
	headroomExtraHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		gotCfg = cfg
		return &fakeHeadroomCompressor{}
	}
	proxy := &fakeHeadroomProxy{startDone: make(chan struct{})}
	headroomExtraHookVars.newProxy = func(cfg headroom.Config, comp headroomCompressor, upstream string) (headroomProxy, error) {
		return proxy, nil
	}
	headroomExtraHookVars.signalCh = func() <-chan os.Signal {
		ch := make(chan os.Signal, 1)
		go func() {
			<-proxy.startDone
			ch <- syscall.SIGINT
		}()
		return ch
	}
	out, err := runHeadroomExtraCmd(t, "proxy", "--addr", ":9999", "--upstream", "http://up")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "proxy listening") {
		t.Errorf("expected proxy listening output, got %q", out.String())
	}
	if proxy.addr != ":9999" {
		t.Errorf("expected addr :9999, got %q", proxy.addr)
	}
	if gotCfg.Mode != headroom.ModeCLI {
		t.Errorf("expected mode switched to CLI, got %q", gotCfg.Mode)
	}
}

func TestHeadroomProxyStartError(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor { return &fakeHeadroomCompressor{} }
	headroomExtraHookVars.newProxy = func(cfg headroom.Config, comp headroomCompressor, upstream string) (headroomProxy, error) {
		return &fakeHeadroomProxy{startErr: errors.New("start boom")}, nil
	}
	headroomExtraHookVars.signalCh = func() <-chan os.Signal { return make(chan os.Signal) }
	_, err := runHeadroomExtraCmd(t, "proxy")
	if err == nil || !strings.Contains(err.Error(), "start boom") {
		t.Fatalf("expected start error, got %v", err)
	}
}

func TestHeadroomProxyShutdownError(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor { return &fakeHeadroomCompressor{} }
	headroomExtraHookVars.newProxy = func(cfg headroom.Config, comp headroomCompressor, upstream string) (headroomProxy, error) {
		return &fakeHeadroomProxy{shutdownErr: errors.New("shutdown boom")}, nil
	}
	headroomExtraHookVars.signalCh = func() <-chan os.Signal {
		ch := make(chan os.Signal, 1)
		ch <- syscall.SIGINT
		return ch
	}
	_, err := runHeadroomExtraCmd(t, "proxy")
	if err == nil || !strings.Contains(err.Error(), "shutdown boom") {
		t.Fatalf("expected shutdown error, got %v", err)
	}
}

func TestHeadroomProxyNewProxyError(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor { return &fakeHeadroomCompressor{} }
	headroomExtraHookVars.newProxy = func(cfg headroom.Config, comp headroomCompressor, upstream string) (headroomProxy, error) {
		return nil, errors.New("proxy boom")
	}
	_, err := runHeadroomExtraCmd(t, "proxy")
	if err == nil || !strings.Contains(err.Error(), "proxy boom") {
		t.Fatalf("expected proxy error, got %v", err)
	}
}

func TestHeadroomProxyCompressorStartError(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		return &fakeHeadroomCompressor{startErr: errors.New("start boom")}
	}
	_, err := runHeadroomExtraCmd(t, "proxy")
	if err == nil || !strings.Contains(err.Error(), "headroom backend not available") {
		t.Fatalf("expected backend error, got %v", err)
	}
}

func TestHeadroomLessonsEmpty(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.newLessonStore = func(path string) (headroomLessonStore, error) {
		return &fakeHeadroomLessonStore{}, nil
	}
	out, err := runHeadroomExtraCmd(t, "lessons")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No lessons recorded") {
		t.Errorf("expected empty output, got %q", out.String())
	}
}

func TestHeadroomLessonsWithData(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.newLessonStore = func(path string) (headroomLessonStore, error) {
		return &fakeHeadroomLessonStore{
			count: 2,
			lessons: []*headroom.Lesson{{
				Category: "compression",
				Weight:   0.75,
				Hits:     3,
				Insight:  "keep imports",
			}},
		}, nil
	}
	out, err := runHeadroomExtraCmd(t, "lessons", "--path", "/tmp/x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Lessons (2):") {
		t.Errorf("expected lessons output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "keep imports") {
		t.Errorf("expected insight output, got %q", out.String())
	}
}

func TestHeadroomLessonsStoreError(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.newLessonStore = func(path string) (headroomLessonStore, error) {
		return nil, errors.New("store boom")
	}
	_, err := runHeadroomExtraCmd(t, "lessons")
	if err == nil || !strings.Contains(err.Error(), "opening lessons store") {
		t.Fatalf("expected store error, got %v", err)
	}
}

func TestHeadroomLessonsClear(t *testing.T) {
	resetHeadroomExtraHooks(t)
	var removed string
	headroomExtraHookVars.remove = func(path string) error { removed = path; return nil }
	headroomExtraHookVars.defaultLessonsPath = func() string { return "/default/path" }
	out, err := runHeadroomExtraCmd(t, "lessons", "clear", "--path", "/tmp/x")
	if err != nil {
		t.Fatal(err)
	}
	if removed != "/tmp/x" {
		t.Errorf("expected remove /tmp/x, got %q", removed)
	}
	if !strings.Contains(out.String(), "Lessons cleared") {
		t.Errorf("expected cleared output, got %q", out.String())
	}
}

func TestHeadroomLessonsClearDefaultPath(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.defaultLessonsPath = func() string { return "/default/path" }
	var removed string
	headroomExtraHookVars.remove = func(path string) error { removed = path; return nil }
	_, err := runHeadroomExtraCmd(t, "lessons", "clear")
	if err != nil {
		t.Fatal(err)
	}
	if removed != "/default/path" {
		t.Errorf("expected remove /default/path, got %q", removed)
	}
}

func TestHeadroomLessonsClearNotExist(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.remove = func(path string) error { return os.ErrNotExist }
	out, err := runHeadroomExtraCmd(t, "lessons", "clear")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Lessons cleared") {
		t.Errorf("expected cleared output, got %q", out.String())
	}
	_ = out
}

func TestHeadroomLessonsClearRemoveError(t *testing.T) {
	resetHeadroomExtraHooks(t)
	headroomExtraHookVars.remove = func(path string) error { return errors.New("remove boom") }
	_, err := runHeadroomExtraCmd(t, "lessons", "clear")
	if err == nil || !strings.Contains(err.Error(), "clearing lessons") {
		t.Fatalf("expected remove error, got %v", err)
	}
}

func TestHeadroomExtraDefaultHooks(t *testing.T) {
	resetHeadroomExtraHooks(t)
	_ = headroomExtraHookVars.loadConfig()
	_ = headroomExtraHookVars.defaultLessonsPath()
	_ = headroomExtraHookVars.newCompressor(headroom.Config{})
	ch := headroomExtraHookVars.signalCh()
	_ = ch

	comp := headroom.NewCompressor(headroom.Config{})
	proxy, err := headroomExtraHookVars.newProxy(headroom.Config{}, comp, "http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if proxy == nil {
		t.Fatal("expected non-nil proxy")
	}

	// newProxy error path when compressor is not *headroom.Compressor.
	_, err = headroomExtraHookVars.newProxy(headroom.Config{}, &fakeHeadroomCompressor{}, "http://example.com")
	if err == nil {
		t.Fatal("expected proxy error for non-*Compressor")
	}

	path := filepath.Join(t.TempDir(), "lessons.db")
	ls, err := headroomExtraHookVars.newLessonStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if ls == nil {
		t.Fatal("expected non-nil lesson store")
	}

	// newLessonStore error path when the path is a directory.
	_, err = headroomExtraHookVars.newLessonStore(t.TempDir())
	if err == nil {
		t.Fatal("expected lesson store error for directory path")
	}
}
