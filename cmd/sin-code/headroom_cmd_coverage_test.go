// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for headroom_cmd.go.
// Docs: headroom_cmd.go
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
)

// fakeHeadroomCompressor implements headroomCompressor for tests.
type fakeHeadroomCompressor struct {
	startErr    error
	closeErr    error
	stats       *headroom.Stats
	compressed  string
	result      *headroom.CompressionResult
	compressErr error
}

func (f *fakeHeadroomCompressor) Start(ctx context.Context) error { return f.startErr }
func (f *fakeHeadroomCompressor) Close() error                    { return f.closeErr }
func (f *fakeHeadroomCompressor) GetStats() *headroom.Stats       { return f.stats }
func (f *fakeHeadroomCompressor) CompressContent(ctx context.Context, content string) (string, *headroom.CompressionResult, error) {
	return f.compressed, f.result, f.compressErr
}

// fakeHeadroomCLIClient implements headroomCLIClient for tests.
type fakeHeadroomCLIClient struct {
	learnErr error
}

func (f *fakeHeadroomCLIClient) Learn(ctx context.Context, sessionLog string) error {
	return f.learnErr
}

func resetHeadroomHooks(t *testing.T) {
	t.Helper()
	orig := headroomHookVars
	t.Cleanup(func() { headroomHookVars = orig })
}

func runHeadroomCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewHeadroomCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestHeadroomCmdHelp(t *testing.T) {
	resetHeadroomHooks(t)
	out, err := runHeadroomCmd(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "headroom") {
		t.Errorf("expected help output, got %q", out.String())
	}
}

func TestHeadroomEnable(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.loadConfig = func() headroom.Config {
		return headroom.Config{Mode: headroom.ModeProxy, CompressionLevel: "normal"}
	}
	out, err := runHeadroomCmd(t, "enable")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "enabled") || !strings.Contains(out.String(), "proxy") {
		t.Errorf("expected enable output, got %q", out.String())
	}
}

func TestHeadroomDisable(t *testing.T) {
	resetHeadroomHooks(t)
	out, err := runHeadroomCmd(t, "disable")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "disabled") {
		t.Errorf("expected disable output, got %q", out.String())
	}
}

func TestHeadroomStats(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		return &fakeHeadroomCompressor{stats: &headroom.Stats{
			TotalRequests:         1,
			TotalCompressed:       2,
			TotalOriginalTokens:   100,
			TotalCompressedTokens: 50,
			AverageSavings:        50.0,
			LastLearnTime:         time.Now(),
		}}
	}
	out, err := runHeadroomCmd(t, "stats")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Total requests", "Total compressed", "Original tokens", "Compressed tokens", "Average savings", "Last learning"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %q in output, got %q", want, out.String())
		}
	}
}

func TestHeadroomStatsNoLastLearn(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		return &fakeHeadroomCompressor{stats: &headroom.Stats{}}
	}
	out, err := runHeadroomCmd(t, "stats")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Last learning") {
		t.Errorf("expected no Last learning line, got %q", out.String())
	}
	_ = out
}

func TestHeadroomStatsStartError(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		return &fakeHeadroomCompressor{startErr: errors.New("start boom")}
	}
	_, err := runHeadroomCmd(t, "stats")
	if err == nil || !strings.Contains(err.Error(), "headroom not available") {
		t.Fatalf("expected start error, got %v", err)
	}
}

func TestHeadroomTest(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		return &fakeHeadroomCompressor{
			compressed: "small",
			result:     &headroom.CompressionResult{SavingsPercent: 50.0, Algorithm: "x", DurationMs: 10},
		}
	}
	out, err := runHeadroomCmd(t, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Headroom Test Result", "Original length", "Compressed length", "Savings", "Algorithm", "Duration", "working correctly"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("expected %q in output, got %q", want, out.String())
		}
	}
}

func TestHeadroomTestNoResult(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		return &fakeHeadroomCompressor{compressed: "small"}
	}
	out, err := runHeadroomCmd(t, "test")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Savings") {
		t.Errorf("expected no savings line, got %q", out.String())
	}
	_ = out
}

func TestHeadroomTestStartError(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		return &fakeHeadroomCompressor{startErr: errors.New("start boom")}
	}
	_, err := runHeadroomCmd(t, "test")
	if err == nil || !strings.Contains(err.Error(), "headroom not available") {
		t.Fatalf("expected start error, got %v", err)
	}
}

func TestHeadroomTestCompressError(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.newCompressor = func(cfg headroom.Config) headroomCompressor {
		return &fakeHeadroomCompressor{compressErr: errors.New("compress boom")}
	}
	_, err := runHeadroomCmd(t, "test")
	if err == nil || !strings.Contains(err.Error(), "compression test failed") {
		t.Fatalf("expected compress error, got %v", err)
	}
}

func TestHeadroomLearnFile(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.readFile = func(string) ([]byte, error) { return []byte("log"), nil }
	headroomHookVars.newCLIClient = func(cfg headroom.Config) headroomCLIClient { return &fakeHeadroomCLIClient{} }
	out, err := runHeadroomCmd(t, "learn", "/tmp/log")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "learned") {
		t.Errorf("expected learn output, got %q", out.String())
	}
}

func TestHeadroomLearnFileError(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.readFile = func(string) ([]byte, error) { return nil, errors.New("read boom") }
	_, err := runHeadroomCmd(t, "learn", "/tmp/log")
	if err == nil || !strings.Contains(err.Error(), "failed to read log file") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestHeadroomLearnStdin(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.readStdin = func() ([]byte, error) { return []byte("stdin log"), nil }
	headroomHookVars.newCLIClient = func(cfg headroom.Config) headroomCLIClient { return &fakeHeadroomCLIClient{} }
	out, err := runHeadroomCmd(t, "learn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "learned") {
		t.Errorf("expected learn output, got %q", out.String())
	}
}

func TestHeadroomLearnStdinError(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.readStdin = func() ([]byte, error) { return nil, errors.New("no stdin") }
	_, err := runHeadroomCmd(t, "learn")
	if err == nil || !strings.Contains(err.Error(), "please provide a session log file") {
		t.Fatalf("expected stdin error, got %v", err)
	}
}

func TestHeadroomLearnError(t *testing.T) {
	resetHeadroomHooks(t)
	headroomHookVars.readStdin = func() ([]byte, error) { return []byte("log"), nil }
	headroomHookVars.newCLIClient = func(cfg headroom.Config) headroomCLIClient {
		return &fakeHeadroomCLIClient{learnErr: errors.New("learn boom")}
	}
	_, err := runHeadroomCmd(t, "learn")
	if err == nil || !strings.Contains(err.Error(), "learning failed") {
		t.Fatalf("expected learn error, got %v", err)
	}
}

func TestHeadroomDefaultHooks(t *testing.T) {
	resetHeadroomHooks(t)
	_ = headroomHookVars.newCompressor(headroom.Config{})
	_ = headroomHookVars.newCLIClient(headroom.Config{})

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	// readStdin error path when Stat fails.
	f, err := os.CreateTemp(t.TempDir(), "closed-stdin")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Stdin = f
	_, err = headroomHookVars.readStdin()
	if err == nil {
		t.Fatal("expected error when Stat fails")
	}

	// readStdin error path when stdin is a char device (not a pipe).
	f2, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	os.Stdin = f2
	_, err = headroomHookVars.readStdin()
	if err == nil {
		t.Fatal("expected error when stdin is not a pipe")
	}

	// readStdin success path with a real pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("hello stdin")
		_ = w.Close()
	}()
	os.Stdin = r
	data, err := headroomHookVars.readStdin()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello stdin" {
		t.Errorf("expected pipe data, got %q", string(data))
	}
}
