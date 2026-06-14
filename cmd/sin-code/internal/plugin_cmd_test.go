// SPDX-License-Identifier: MIT
// Purpose: Unit tests for plugin CLI commands. (st-cov1)
package internal

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/plugins"
)

func capturePluginCmd(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.RunE(cmd, args)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

func makePluginDir(t *testing.T, name string, enabled bool) string {
	t.Helper()
	dir := t.TempDir()
	pluginPath = dir
	subDir := filepath.Join(dir, name)
	os.MkdirAll(subDir, 0o755)
	manifest := `name = "` + name + `"`
	version := `version = "1.0.0"` + "\n"
	description := `description = "test plugin"` + "\n"
	os.WriteFile(filepath.Join(subDir, plugins.ManifestFile), []byte(manifest+"\n"+version+description), 0o644)
	if !enabled {
		os.WriteFile(filepath.Join(subDir, ".disabled"), []byte{}, 0o644)
	}
	return dir
}

func TestPluginList(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	// Nonexistent dir
	pluginPath = filepath.Join(t.TempDir(), "missing")
	out, err := capturePluginCmd(t, pluginListCmd, []string{})
	if err != nil {
		t.Fatalf("pluginListCmd failed: %v", err)
	}
	if !strings.Contains(out, "no plugins directory") {
		t.Errorf("expected no plugins dir message, got %q", out)
	}

	// Empty existing dir
	pluginPath = t.TempDir()
	out, err = capturePluginCmd(t, pluginListCmd, []string{})
	if err != nil {
		t.Fatalf("pluginListCmd failed: %v", err)
	}
	if !strings.Contains(out, "NAME") {
		t.Errorf("expected header, got %q", out)
	}

	// With one enabled plugin
	makePluginDir(t, "myplugin", true)
	out, err = capturePluginCmd(t, pluginListCmd, []string{})
	if err != nil {
		t.Fatalf("pluginListCmd failed: %v", err)
	}
	if !strings.Contains(out, "myplugin") {
		t.Errorf("expected plugin name in list, got %q", out)
	}

	// With disabled plugin
	makePluginDir(t, "disabled", false)
	out, err = capturePluginCmd(t, pluginListCmd, []string{})
	if err != nil {
		t.Fatalf("pluginListCmd failed: %v", err)
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("expected disabled status, got %q", out)
	}
}

func TestPluginInfo(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	makePluginDir(t, "info", true)
	out, err := capturePluginCmd(t, pluginInfoCmd, []string{"info"})
	if err != nil {
		t.Fatalf("pluginInfoCmd failed: %v", err)
	}
	if !strings.Contains(out, "Name:") || !strings.Contains(out, "info") {
		t.Errorf("expected plugin info, got %q", out)
	}
}

func TestPluginInstallUninstall(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	pluginPath = t.TempDir()

	// Create source plugin
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "srcplugin"), 0o755)
	os.WriteFile(filepath.Join(src, "srcplugin", plugins.ManifestFile), []byte("name = \"srcplugin\"\nversion = \"1.0.0\"\n"), 0o644)

	out, err := capturePluginCmd(t, pluginInstallCmd, []string{filepath.Join(src, "srcplugin")})
	if err != nil {
		t.Fatalf("pluginInstallCmd failed: %v", err)
	}
	if !strings.Contains(out, "Installed") {
		t.Errorf("expected install output, got %q", out)
	}

	out, err = capturePluginCmd(t, pluginUninstallCmd, []string{"srcplugin"})
	if err != nil {
		t.Fatalf("pluginUninstallCmd failed: %v", err)
	}
	if !strings.Contains(out, "Uninstalled") {
		t.Errorf("expected uninstall output, got %q", out)
	}
}

func TestPluginEnableDisable(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	makePluginDir(t, "toggle", true)

	out, err := capturePluginCmd(t, pluginDisableCmd, []string{"toggle"})
	if err != nil {
		t.Fatalf("pluginDisableCmd failed: %v", err)
	}
	if !strings.Contains(out, "Disabled") {
		t.Errorf("expected disable output, got %q", out)
	}

	out, err = capturePluginCmd(t, pluginEnableCmd, []string{"toggle"})
	if err != nil {
		t.Fatalf("pluginEnableCmd failed: %v", err)
	}
	if !strings.Contains(out, "Enabled") {
		t.Errorf("expected enable output, got %q", out)
	}
}

func TestPluginInfo_FullManifest(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	dir := t.TempDir()
	pluginPath = dir
	subDir := filepath.Join(dir, "full")
	os.MkdirAll(subDir, 0o755)
	manifest := `name = "full"
version = "1.0.0"
description = "A full plugin"
author = "tester"
homepage = "https://example.com"
license = "MIT"
min_sin_code = "v2.0.0"
capabilities = ["todo", "mcp"]

[[subcommand]]
name = "hello"
description = "say hello"
binary = "bin/hello"

[[agents]]
name = "helper"
type = "code"
model = "gpt-4"

[[tools]]
name = "grep"
description = "grep helper"
binary = "bin/grep"

[[hooks]]
event = "todo.completed"
command = "echo done"
`
	os.WriteFile(filepath.Join(subDir, plugins.ManifestFile), []byte(manifest), 0o644)

	out, err := capturePluginCmd(t, pluginInfoCmd, []string{"full"})
	if err != nil {
		t.Fatalf("pluginInfoCmd failed: %v", err)
	}
	for _, want := range []string{"full", "tester", "https://example.com", "MIT", "v2.0.0", "hello", "helper", "grep", "todo.completed"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got %q", want, out)
		}
	}
}

func TestPluginList_Broken(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	dir := t.TempDir()
	pluginPath = dir
	os.MkdirAll(filepath.Join(dir, "broken"), 0o755)
	os.WriteFile(filepath.Join(dir, "broken", "plugin.toml"), []byte("invalid toml = {{"), 0o644)

	out, err := capturePluginCmd(t, pluginListCmd, []string{})
	if err != nil {
		t.Fatalf("pluginListCmd failed: %v", err)
	}
	if !strings.Contains(out, "broken") {
		t.Errorf("expected broken plugin in list, got %q", out)
	}
}

func TestPluginInstall_AlreadyInstalled(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	pluginPath = t.TempDir()

	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "dup"), 0o755)
	os.WriteFile(filepath.Join(src, "dup", plugins.ManifestFile), []byte("name = \"dup\"\nversion = \"1.0.0\"\n"), 0o644)

	// Pre-create the destination to simulate already installed
	os.MkdirAll(filepath.Join(pluginPath, "dup"), 0o755)

	if _, err := capturePluginCmd(t, pluginInstallCmd, []string{filepath.Join(src, "dup")}); err == nil {
		t.Fatal("expected error when plugin already installed")
	}
}

func TestPluginInstall_InvalidSource(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	pluginPath = t.TempDir()

	if _, err := capturePluginCmd(t, pluginInstallCmd, []string{"/nonexistent/path"}); err == nil {
		t.Fatal("expected error for invalid source")
	}
}

func TestPluginList_ReadDirError(t *testing.T) {
	oldPath := pluginPath
	defer func() { pluginPath = oldPath }()

	// Set pluginPath to a file, not a directory, to trigger a non-IsNotExist ReadDir error.
	dir := t.TempDir()
	f := filepath.Join(dir, "pluginfiles")
	os.WriteFile(f, []byte("not-a-dir"), 0644)
	pluginPath = f

	_, err := capturePluginCmd(t, pluginListCmd, []string{})
	if err == nil {
		t.Fatal("expected error when plugin path is a file")
	}
}

func TestCopyDir_WalkError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	sub := filepath.Join(src, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "f.txt"), []byte("hi"), 0644)
	os.Chmod(sub, 0000)
	defer os.Chmod(sub, 0755)

	err := copyDir(src, dst)
	if err == nil {
		t.Error("expected error when subdirectory is unreadable")
	}
}

func TestCopyDir_FullCopy(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "nested.txt"), []byte("world"), 0644)

	err := copyDir(src, dst)
	if err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
	data2, err := os.ReadFile(filepath.Join(dst, "sub", "nested.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != "world" {
		t.Errorf("expected 'world', got %q", string(data2))
	}
}

func TestCopyDir_ReadFileError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	bad := filepath.Join(src, "unreadable.txt")
	os.WriteFile(bad, []byte("x"), 0644)
	os.Chmod(bad, 0000)
	defer os.Chmod(bad, 0644)

	err := copyDir(src, dst)
	if err == nil {
		t.Error("expected error when source file is unreadable")
	}
}

func TestLoadPlugin_InvalidManifest(t *testing.T) {
	oldPluginPath := pluginPath
	defer func() { pluginPath = oldPluginPath }()

	dir := t.TempDir()
	pluginPath = dir
	subDir := filepath.Join(dir, "bad")
	os.MkdirAll(subDir, 0o755)
	os.WriteFile(filepath.Join(subDir, "plugin.toml"), []byte("invalid toml = {{"), 0o644)

	if _, err := loadPlugin("bad"); err == nil {
		t.Fatal("expected error for invalid manifest")
	}
}

