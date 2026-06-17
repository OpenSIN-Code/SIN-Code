// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for the remaining branches in mcp_cmd.go.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
)

// fakeMCPManager implements mcpManager for tests.
type fakeMCPManager struct {
	connectErr error
	tools      []mcpclient.Tool
	callResult string
	callErr    error
	connected  bool
	closed     bool
}

func (f *fakeMCPManager) ConnectAll(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	f.connected = true
	return f.connectErr
}

func (f *fakeMCPManager) Tools() []mcpclient.Tool { return f.tools }

func (f *fakeMCPManager) Call(ctx context.Context, qualified string, args map[string]any) (string, error) {
	return f.callResult + "-" + qualified, f.callErr
}

func (f *fakeMCPManager) Close() { f.closed = true }

var (
	mcpOrigLoadConfigs = mcpHookVars.loadConfigs
	mcpOrigNewManager  = mcpHookVars.newManager
	mcpOrigGetwd       = mcpHookVars.getwd
)

func resetMCPHooks(t *testing.T) {
	t.Cleanup(func() {
		mcpHookVars.loadConfigs = mcpOrigLoadConfigs
		mcpHookVars.newManager = mcpOrigNewManager
		mcpHookVars.getwd = mcpOrigGetwd
	})
}

func TestMCPCmd_ListTable(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.loadConfigs = func(ws string) []mcpclient.ServerConfig {
		return []mcpclient.ServerConfig{
			{Name: "a", Transport: "stdio", Command: "cmd", Args: []string{"1", "2"}},
			{Name: "b", Transport: "http", URL: "http://example.com"},
		}
	}

	cmd := NewMCPCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "a") || !strings.Contains(got, "cmd 1 2") || !strings.Contains(got, "http://example.com") {
		t.Fatalf("unexpected table output: %s", got)
	}
}

func TestMCPCmd_ListJSON(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.loadConfigs = func(ws string) []mcpclient.ServerConfig {
		return []mcpclient.ServerConfig{{Name: "x", Transport: "http", URL: "http://x"}}
	}

	cmd := NewMCPCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var cfgs []mcpclient.ServerConfig
	if err := json.Unmarshal(out.Bytes(), &cfgs); err != nil {
		t.Fatal(err)
	}
	if len(cfgs) != 1 || cfgs[0].Name != "x" {
		t.Fatalf("unexpected json output: %v", cfgs)
	}
}

func TestMCPCmd_StatusTable(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.loadConfigs = func(ws string) []mcpclient.ServerConfig {
		return []mcpclient.ServerConfig{{Name: "up", Transport: "http", URL: "http://up"}}
	}
	mgr := &fakeMCPManager{tools: []mcpclient.Tool{{Server: "up", Name: "tool"}}}
	mcpHookVars.newManager = func(cfgs []mcpclient.ServerConfig) mcpManager { return mgr }

	cmd := NewMCPCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--timeout", "1s"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !mgr.connected || !mgr.closed {
		t.Fatalf("manager lifecycle not observed: connected=%v closed=%v", mgr.connected, mgr.closed)
	}
	if !strings.Contains(out.String(), "up") || !strings.Contains(out.String(), "yes") {
		t.Fatalf("unexpected status output: %s", out.String())
	}
}

func TestMCPCmd_StatusJSON(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.loadConfigs = func(ws string) []mcpclient.ServerConfig {
		return []mcpclient.ServerConfig{
			{Name: "s1", Transport: "http", URL: "http://s1"},
			{Name: "s2", Transport: "http", URL: "http://s2"},
		}
	}
	mgr := &fakeMCPManager{tools: []mcpclient.Tool{{Server: "s1", Name: "t1"}}}
	mcpHookVars.newManager = func(cfgs []mcpclient.ServerConfig) mcpManager { return mgr }

	cmd := NewMCPCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--json", "--timeout", "1s"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestMCPCmd_StatusConnectError(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.loadConfigs = func(ws string) []mcpclient.ServerConfig {
		return []mcpclient.ServerConfig{{Name: "down", Transport: "http", URL: "http://down"}}
	}
	mgr := &fakeMCPManager{connectErr: errors.New("connect failed")}
	mcpHookVars.newManager = func(cfgs []mcpclient.ServerConfig) mcpManager { return mgr }

	cmd := NewMCPCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"status", "--timeout", "1s"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected connect error")
	}
}

func TestMCPCmd_Call(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.loadConfigs = func(ws string) []mcpclient.ServerConfig {
		return []mcpclient.ServerConfig{{Name: "srv", Transport: "http", URL: "http://srv"}}
	}
	mgr := &fakeMCPManager{callResult: "ok"}
	mcpHookVars.newManager = func(cfgs []mcpclient.ServerConfig) mcpManager { return mgr }

	cmd := NewMCPCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"call", "srv__tool", `{"x":1}`})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "ok-srv__tool\n"
	if out.String() != want {
		t.Fatalf("expected %q, got %q", want, out.String())
	}
}

func TestMCPCmd_CallInvalidArgs(t *testing.T) {
	resetMCPHooks(t)
	cmd := NewMCPCmd()
	cmd.SetArgs([]string{"call", "srv__tool", "not-json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected invalid args error")
	}
}

func TestMCPCmd_CallConnectError(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.loadConfigs = func(ws string) []mcpclient.ServerConfig { return nil }
	mgr := &fakeMCPManager{connectErr: errors.New("connect failed")}
	mcpHookVars.newManager = func(cfgs []mcpclient.ServerConfig) mcpManager { return mgr }

	cmd := NewMCPCmd()
	cmd.SetArgs([]string{"call", "srv__tool"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected connect error")
	}
}

func TestMCPCmd_CallError(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.loadConfigs = func(ws string) []mcpclient.ServerConfig { return nil }
	mgr := &fakeMCPManager{callErr: errors.New("call failed")}
	mcpHookVars.newManager = func(cfgs []mcpclient.ServerConfig) mcpManager { return mgr }

	cmd := NewMCPCmd()
	cmd.SetArgs([]string{"call", "srv__tool"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected call error")
	}
}

func TestMCPCmd_GetwdError_List(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.getwd = func() (string, error) { return "", errors.New("getwd failed") }
	cmd := NewMCPCmd()
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected getwd error")
	}
}

func TestMCPCmd_GetwdError_Status(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.getwd = func() (string, error) { return "", errors.New("getwd failed") }
	cmd := NewMCPCmd()
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected getwd error")
	}
}

func TestMCPCmd_GetwdError_Call(t *testing.T) {
	resetMCPHooks(t)
	mcpHookVars.getwd = func() (string, error) { return "", errors.New("getwd failed") }
	cmd := NewMCPCmd()
	cmd.SetArgs([]string{"call", "srv__tool"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected getwd error")
	}
}

func TestMCPDefaultHooks(t *testing.T) {
	resetMCPHooks(t)
	mgr := mcpHookVars.newManager(nil)
	if mgr == nil {
		t.Fatal("expected non-nil manager from default hook")
	}
	mgr.Close()
}
