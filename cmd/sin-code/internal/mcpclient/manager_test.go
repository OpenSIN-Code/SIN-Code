// SPDX-License-Identifier: MIT
// Purpose: smoke tests for internal/mcpclient (issue #50).
// Verifies the "additive, never fatal" guarantee: unreachable servers
// are logged to stderr and skipped, ConnectAll never returns an error.
package mcpclient

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureStderr redirects os.Stderr for the duration of fn.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	var buf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = buf.ReadFrom(r)
	}()
	fn()
	w.Close()
	wg.Wait()
	return buf.String()
}

func TestConnectAllUnreachableServerIsAdditiveNeverFatal(t *testing.T) {
	mgr := NewManager([]ServerConfig{
		{Name: "ghost-http", Transport: "http", URL: "http://127.0.0.1:1/mcp"},
		{Name: "ghost-stdio", Transport: "stdio", Command: "/nonexistent/sin-mcp-binary"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	logged := captureStderr(t, func() {
		err = mgr.ConnectAll(ctx)
	})

	if err != nil {
		t.Fatalf("ConnectAll must never be fatal, got: %v", err)
	}
	for _, name := range []string{"ghost-http", "ghost-stdio"} {
		if !strings.Contains(logged, name) {
			t.Errorf("expected stderr warning for %q, got: %s", name, logged)
		}
	}
	if tools := mgr.Tools(); len(tools) != 0 {
		t.Fatalf("expected 0 tools from unreachable servers, got %d", len(tools))
	}
}

func TestConnectUnknownTransportIsLogged(t *testing.T) {
	mgr := NewManager([]ServerConfig{
		{Name: "bad", Transport: "carrier-pigeon"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	logged := captureStderr(t, func() { err = mgr.ConnectAll(ctx) })
	if err != nil {
		t.Fatalf("ConnectAll must never be fatal, got: %v", err)
	}
	if !strings.Contains(logged, "unknown transport") {
		t.Errorf("expected 'unknown transport' warning, got: %s", logged)
	}
}

func TestCallRoutingErrors(t *testing.T) {
	mgr := NewManager(nil)
	ctx := context.Background()

	if _, err := mgr.Call(ctx, "sin_read", nil); err == nil ||
		!strings.Contains(err.Error(), "not an external tool") {
		t.Fatalf("expected 'not an external tool' error, got: %v", err)
	}
	if _, err := mgr.Call(ctx, "ghost__do_thing", nil); err == nil ||
		!strings.Contains(err.Error(), `no MCP session for server "ghost"`) {
		t.Fatalf("expected 'no MCP session' error, got: %v", err)
	}
}

func TestToolsReturnsCopy(t *testing.T) {
	mgr := NewManager(nil)
	mgr.tools = []Tool{{Server: "s", Name: "t", Qualified: "s__t"}}
	got := mgr.Tools()
	got[0].Name = "mutated"
	if mgr.tools[0].Name != "t" {
		t.Fatal("Tools() must return a defensive copy")
	}
}

func TestToolsConcurrentAccessRaceClean(t *testing.T) {
	mgr := NewManager(nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = mgr.Tools() }()
		go func() {
			defer wg.Done()
			mgr.mu.Lock()
			mgr.tools = append(mgr.tools, Tool{Server: "x", Name: "y", Qualified: "x__y"})
			mgr.mu.Unlock()
		}()
	}
	wg.Wait()
}
