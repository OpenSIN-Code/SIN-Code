// SPDX-License-Identifier: MIT
// Purpose: tests for the RepairLoop generate→compile→execute→repair
// cycle. Uses httptest to stub the LLM and swappable CompileFunc /
// RunTestFunc seams to control compile and test outcomes per round.
package testgen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

func TestRepairLoop_RunSuccessOneRound(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage calc\nfunc TestAdd(t *testing.T) {}\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) { return "PASS\nok\tcalc\n", true }),
	)

	res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src, MaxRounds: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.FinalPass {
		t.Errorf("expected FinalPass=true, got false. Results: %s", res.TestResults)
	}
	if res.RoundsUsed != 1 {
		t.Errorf("expected 1 round, got %d", res.RoundsUsed)
	}
	if res.CompileErrors != "" {
		t.Errorf("expected no compile errors, got %q", res.CompileErrors)
	}
}

func TestRepairLoop_RunWithRepairTwoRounds(t *testing.T) {
	src := llmFillerFixture(t)
	var llmCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&llmCalls, 1)
		var code string
		if n == 1 {
			code = "```go\npackage calc\nfunc TestAdd(t *testing.T) { t.Fail() }\n```"
		} else {
			code = "```go\npackage calc\nfunc TestAdd(t *testing.T) {}\n```"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse(code)))
	}))
	defer srv.Close()

	var testCalls int32
	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) {
			n := atomic.AddInt32(&testCalls, 1)
			if n == 1 {
				return "FAIL: TestAdd\n", false
			}
			return "PASS\nok\tcalc\n", true
		}),
	)

	res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src, MaxRounds: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.FinalPass {
		t.Error("expected FinalPass=true")
	}
	if res.RoundsUsed != 2 {
		t.Errorf("expected 2 rounds, got %d", res.RoundsUsed)
	}
	if atomic.LoadInt32(&llmCalls) != 2 {
		t.Errorf("expected 2 LLM calls, got %d", llmCalls)
	}
	if atomic.LoadInt32(&testCalls) != 2 {
		t.Errorf("expected 2 test calls, got %d", testCalls)
	}
}

func TestRepairLoop_MaxRoundsExceeded(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage calc\nfunc TestAdd(t *testing.T) { t.Fail() }\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) {
			return "FAIL\n", false
		}),
	)

	res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src, MaxRounds: 2})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinalPass {
		t.Error("expected FinalPass=false")
	}
	if res.RoundsUsed != 2 {
		t.Errorf("expected 2 rounds, got %d", res.RoundsUsed)
	}
	if !strings.Contains(res.TestResults, "FAIL") {
		t.Errorf("expected FAIL in test results: %s", res.TestResults)
	}
}

func TestRepairLoop_CompileErrorHandling(t *testing.T) {
	src := llmFillerFixture(t)
	var compileCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage calc\nfunc TestAdd(t *testing.T) {}\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) {
			n := atomic.AddInt32(&compileCalls, 1)
			if n == 1 {
				return "./calc_test.go:5: syntax error", fmt.Errorf("exit status 1")
			}
			return "", nil
		}),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) {
			return "PASS\nok\tcalc\n", true
		}),
	)

	res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src, MaxRounds: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.FinalPass {
		t.Error("expected FinalPass=true after repair")
	}
	if res.RoundsUsed != 2 {
		t.Errorf("expected 2 rounds, got %d", res.RoundsUsed)
	}
	if atomic.LoadInt32(&compileCalls) != 2 {
		t.Errorf("expected 2 compile calls, got %d", compileCalls)
	}
}

func TestRepairLoop_TestFailureHandling(t *testing.T) {
	src := llmFillerFixture(t)
	var llmCalls int32
	var capturedPrompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&llmCalls, 1)
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if msgs, ok := body["messages"].([]any); ok && len(msgs) > 1 {
			if um, ok := msgs[1].(map[string]any); ok {
				if c, ok := um["content"].(string); ok {
					capturedPrompts = append(capturedPrompts, c)
				}
			}
		}
		var code string
		if n == 1 {
			code = "```go\npackage calc\nfunc TestAdd(t *testing.T) { t.Fail() }\n```"
		} else {
			code = "```go\npackage calc\nfunc TestAdd(t *testing.T) {}\n```"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse(code)))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) {
			return "FAIL: TestAdd failed\n", false
		}),
	)

	res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src, MaxRounds: 1})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinalPass {
		t.Error("expected FinalPass=false with 1 round")
	}
	if res.RoundsUsed != 1 {
		t.Errorf("expected 1 round, got %d", res.RoundsUsed)
	}
	if len(capturedPrompts) >= 2 {
		if !strings.Contains(capturedPrompts[1], "FAIL") {
			t.Errorf("expected failure output in repair prompt: %s", capturedPrompts[1])
		}
	}
}

func TestRepairLoop_ContextCancel(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage calc\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) { return "PASS", true }),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := loop.Run(ctx, RepairRequest{SourceFile: src, MaxRounds: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinalPass {
		t.Error("expected FinalPass=false on cancelled context")
	}
	if res.RoundsUsed != 0 {
		t.Errorf("expected 0 rounds on cancelled context, got %d", res.RoundsUsed)
	}
}

func TestRepairLoop_ConcurrentRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage calc\nfunc TestAdd(t *testing.T) {}\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")

	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			dir, _ := os.MkdirTemp("", "repairloop-concurrent-")
			defer os.RemoveAll(dir)
			src := filepath.Join(dir, "calc.go")
			_ = os.WriteFile(src, []byte("package calc\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644)

			filler := NewLLMFiller(client, "test-model")
			loop := NewRepairLoop(filler,
				WithCompileFunc(func(ctx context.Context, d string) (string, error) { return "", nil }),
				WithRunTestFunc(func(ctx context.Context, d string) (string, bool) { return "PASS", true }),
			)
			res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src, MaxRounds: 2})
			if err != nil {
				t.Errorf("goroutine Run: %v", err)
				return
			}
			if !res.FinalPass {
				t.Errorf("goroutine expected FinalPass=true, got false (rounds=%d)", res.RoundsUsed)
			}
		}()
	}
	wg.Wait()
}

func TestRepairLoop_NilFiller(t *testing.T) {
	loop := NewRepairLoop(nil)
	_, err := loop.Run(context.Background(), RepairRequest{SourceFile: "foo.go"})
	if err == nil {
		t.Fatal("expected error for nil filler")
	}
}

func TestRepairLoop_MissingSourceFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) { return "PASS", true }),
	)

	res, err := loop.Run(context.Background(), RepairRequest{
		SourceFile: "/nonexistent/path/calc.go",
		MaxRounds:  2,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinalPass {
		t.Error("expected FinalPass=false for missing source file")
	}
	if res.CompileErrors == "" {
		t.Error("expected compile error message for missing source file")
	}
}

func TestRepairLoop_SafetyCapExceeded(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage calc\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) { return "FAIL", false }),
	)

	res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src, MaxRounds: 99})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.RoundsUsed > maxRepairRoundsCap {
		t.Errorf("expected rounds <= %d (safety cap), got %d", maxRepairRoundsCap, res.RoundsUsed)
	}
}

func TestRepairLoop_DefaultMaxRounds(t *testing.T) {
	src := llmFillerFixture(t)
	var llmCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&llmCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage calc\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) { return "FAIL", false }),
	)

	res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.RoundsUsed != DefaultRepairRounds {
		t.Errorf("expected default %d rounds, got %d", DefaultRepairRounds, res.RoundsUsed)
	}
}

func TestNewRepairLoop_Defaults(t *testing.T) {
	loop := NewRepairLoop(nil)
	if loop.maxRounds != DefaultRepairRounds {
		t.Errorf("default maxRounds: %d", loop.maxRounds)
	}
	if loop.timeout != DefaultTimeout {
		t.Errorf("default timeout: %v", loop.timeout)
	}
	if loop.compileFunc == nil || loop.runTestFunc == nil || loop.writeFile == nil {
		t.Error("expected non-nil default funcs")
	}
}

func TestRepairLoop_WithRepairTimeout(t *testing.T) {
	src := llmFillerFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatResponse("```go\npackage calc\n```")))
	}))
	defer srv.Close()

	client := llm.NewClient(srv.URL, "test-key")
	filler := NewLLMFiller(client, "test-model")
	loop := NewRepairLoop(filler,
		WithRepairTimeout(50*time.Millisecond),
		WithCompileFunc(func(ctx context.Context, dir string) (string, error) { return "", nil }),
		WithRunTestFunc(func(ctx context.Context, dir string) (string, bool) { return "PASS", true }),
	)

	res, err := loop.Run(context.Background(), RepairRequest{SourceFile: src, MaxRounds: 3})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinalPass {
		t.Error("expected FinalPass=false due to timeout")
	}
}
