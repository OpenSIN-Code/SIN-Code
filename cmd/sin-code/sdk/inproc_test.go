// SPDX-License-Identifier: MIT
// Purpose: race-clean tests for the in-process MCP SDK wrapper.
package sdk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewInProcessSession_ListTools(t *testing.T) {
	srv := NewServer("test-list", "v0.0.1")
	MustRegisterTool(srv, "echo", "echo a string back", func(ctx context.Context, args map[string]any) (string, error) {
		s, _ := args["s"].(string)
		return "echo:" + s, nil
	})
	MustRegisterTool(srv, "upper", "uppercase a string", func(ctx context.Context, args map[string]any) (string, error) {
		s, _ := args["s"].(string)
		return strings.ToUpper(s), nil
	})
	sess, err := NewInProcessSession(srv)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	tools, err := sess.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("want 2 tools, got %d", len(tools.Tools))
	}
	want := map[string]bool{"echo": false, "upper": false}
	for _, tool := range tools.Tools {
		want[tool.Name] = true
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("missing tool %s", k)
		}
	}
	_ = errors.New
}

func TestCallTool_ArgRoundTrip(t *testing.T) {
	srv := NewServer("test-rt", "v0")
	MustRegisterTool(srv, "double", "double a number", func(ctx context.Context, args map[string]any) (string, error) {
		if _, ok := args["x"].(float64); !ok {
			return "", errors.New("missing x")
		}
		return "doubled", nil
	})
	MustRegisterTool(srv, "echo", "echo args", func(ctx context.Context, args map[string]any) (string, error) {
		s, _ := args["s"].(string)
		return s, nil
	})
	sess, err := NewInProcessSession(srv)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "double",
		Arguments: map[string]any{"x": 3.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := FirstText(res)
	if !ok || got != "doubled" {
		t.Fatalf("FirstText = %q ok=%v", got, ok)
	}
}

func TestCallTool_HandlerError(t *testing.T) {
	srv := NewServer("test-err", "v0")
	MustRegisterTool(srv, "fail", "always errors", func(ctx context.Context, args map[string]any) (string, error) {
		return "", errors.New("intentional")
	})
	sess, err := NewInProcessSession(srv)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	_, err = sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "fail"})
	if err == nil {
		t.Fatal("want error from handler")
	}
	if !strings.Contains(err.Error(), "intentional") {
		t.Errorf("error must bubble the handler's text: %v", err)
	}
}

func TestNilServer(t *testing.T) {
	if _, err := NewInProcessSession(nil); err == nil {
		t.Fatal("nil server must error")
	}
}

func TestFirstText_Empty(t *testing.T) {
	if _, ok := FirstText(nil); ok {
		t.Fatal("nil result must return ok=false")
	}
	if _, ok := FirstText(&mcp.CallToolResult{}); ok {
		t.Fatal("empty result must return ok=false")
	}
}

func TestNewInProcessSession_Concurrent(t *testing.T) {
	srv := NewServer("test-conc", "v0")
	MustRegisterTool(srv, "echo", "echo args", func(ctx context.Context, args map[string]any) (string, error) {
		s, _ := args["s"].(string)
		if s == "" {
			return "", errors.New("empty")
		}
		return s, nil
	})
	sess, err := NewInProcessSession(srv)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	done := make(chan struct{}, 20)
	for i := 0; i < 20; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			res, err := sess.CallTool(ctx, &mcp.CallToolParams{
				Name:      "echo",
				Arguments: map[string]any{"s": time.Now().Format(time.RFC3339Nano)},
			})
			if err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			got, ok := FirstText(res)
			if !ok || got == "" {
				t.Errorf("call %d: ok=%v got=%q", i, ok, got)
			}
		}(i)
	}
	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("deadlock at i=%d", i)
		}
	}
}
