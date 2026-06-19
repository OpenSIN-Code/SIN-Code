// SPDX-License-Identifier: MIT
package mcpclient

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryNewDefaultPaths(t *testing.T) {
	dir := t.TempDir()
	oldHook := userConfigDirHook
	userConfigDirHook = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDirHook = oldHook })

	d := NewServerDiscovery(nil)
	paths := d.ConfigPaths()
	if len(paths) == 0 {
		t.Fatal("expected default paths, got none")
	}
	// first default path is <cfg>/mcp
	want := filepath.Join(dir, "mcp")
	if paths[0] != want {
		t.Errorf("first path = %q, want %q", paths[0], want)
	}
}

func TestDiscoveryParseConfigMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	writeJSON(t, path, `{"mcpServers":{"foo":{"command":"npx","args":["-y","x"],"env":{"K":"V"}}}}`)

	d := NewServerDiscovery([]string{path})
	got, err := d.ParseConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "foo" {
		t.Fatalf("got %+v, want foo", got)
	}
	if got[0].Transport != "stdio" {
		t.Errorf("transport = %q, want stdio", got[0].Transport)
	}
	if got[0].Command != "npx" {
		t.Errorf("command = %q, want npx", got[0].Command)
	}
	if got[0].Env["K"] != "V" {
		t.Errorf("env = %v, want K=V", got[0].Env)
	}
	if got[0].Source != path {
		t.Errorf("source = %q, want %q", got[0].Source, path)
	}
}

func TestDiscoveryParseConfigSingle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "single.json")
	writeJSON(t, path, `{"name":"bar","command":"echo","transport":"stdio"}`)

	d := NewServerDiscovery([]string{path})
	got, err := d.ParseConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "bar" {
		t.Fatalf("got %+v, want bar", got)
	}
}

func TestDiscoveryDiscoverDir(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "a.json"), `{"name":"alpha","command":"echo"}`)
	writeJSON(t, filepath.Join(dir, "b.json"), `{"name":"beta","command":"cat"}`)
	writeJSON(t, filepath.Join(dir, "ignore.txt"), `not json`)

	d := NewServerDiscovery([]string{dir})
	got, err := d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d servers, want 2", len(got))
	}
	// sorted by name
	if got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Errorf("order = %s,%s; want alpha,beta", got[0].Name, got[1].Name)
	}
}

func TestDiscoveryDedupe(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeJSON(t, filepath.Join(d1, "x.json"), `{"name":"dup","command":"first"}`)
	writeJSON(t, filepath.Join(d2, "x.json"), `{"name":"dup","command":"second"}`)

	d := NewServerDiscovery([]string{d1, d2})
	got, err := d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (deduped)", len(got))
	}
	if got[0].Command != "first" {
		t.Errorf("kept = %q, want first occurrence", got[0].Command)
	}
}

func TestDiscoveryAddRemoveServer(t *testing.T) {
	dir := t.TempDir()
	oldHook := userConfigDirHook
	userConfigDirHook = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDirHook = oldHook })

	d := NewServerDiscovery(nil)
	if err := d.AddServer(DiscoveredServer{Name: "added", Command: "echo", Transport: "stdio"}); err != nil {
		t.Fatal(err)
	}
	serversDir := filepath.Join(dir, "mcp", "servers")
	if _, err := os.Stat(filepath.Join(serversDir, "added.json")); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	// remove
	if err := d.RemoveServer("added"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(serversDir, "added.json")); !os.IsNotExist(err) {
		t.Fatalf("file still present after remove: %v", err)
	}
	// idempotent remove
	if err := d.RemoveServer("added"); err != nil {
		t.Errorf("idempotent remove failed: %v", err)
	}
}

func TestDiscoveryMissingPath(t *testing.T) {
	d := NewServerDiscovery([]string{"/nonexistent/path/that/does/not/exist"})
	got, err := d.Discover()
	if err != nil {
		t.Errorf("missing path should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d servers, want 0", len(got))
	}
}

func TestDiscoveryParseConfigSSE(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sse.json")
	writeJSON(t, path, `{"mcpServers":{"remote":{"url":"https://example.com/sse","command":""}}}`)
	d := NewServerDiscovery([]string{path})
	got, err := d.ParseConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Transport != "sse" {
		t.Fatalf("got %+v, want sse transport", got)
	}
}

func TestDiscoveryConcurrent(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "shared.json"), `{"name":"shared","command":"echo"}`)
	d := NewServerDiscovery([]string{dir})

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.Discover()
			_ = d.ConfigPaths()
		}()
	}
	wg.Wait()
}
