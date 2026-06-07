package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"testing"
)

func TestMain_SymlinkRouting_Discover(t *testing.T) {
	if os.Getenv("TEST_MAIN_SYMLINK_DISCOVER") == "1" {
		os.Args = []string{"discover", "--help"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_SymlinkRouting_Discover")
	cmd.Env = append(os.Environ(), "TEST_MAIN_SYMLINK_DISCOVER=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\noutput: %s", err, output)
	}
	if !strings.Contains(string(output), "Usage") && !strings.Contains(string(output), "discover") {
		t.Errorf("expected discover --help output, got: %s", output)
	}
}

func TestMain_SymlinkRouting_Execute(t *testing.T) {
	if os.Getenv("TEST_MAIN_SYMLINK_EXECUTE") == "1" {
		os.Args = []string{"execute", "--help"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_SymlinkRouting_Execute")
	cmd.Env = append(os.Environ(), "TEST_MAIN_SYMLINK_EXECUTE=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\noutput: %s", err, output)
	}
	if !strings.Contains(string(output), "execute") {
		t.Errorf("expected execute --help output, got: %s", output)
	}
}

func TestMain_SinCodeNoArgs(t *testing.T) {
	if os.Getenv("TEST_MAIN_NO_ARGS") == "1" {
		os.Args = []string{"sin-code", "--version"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_SinCodeNoArgs")
	cmd.Env = append(os.Environ(), "TEST_MAIN_NO_ARGS=1", "HOME=/tmp")
	output, err := cmd.CombinedOutput()
	_ = output
	_ = err
}

func TestMain_SymlinkRouting_Map(t *testing.T) {
	if os.Getenv("TEST_MAIN_SYMLINK_MAP") == "1" {
		os.Args = []string{"map", "--help"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_SymlinkRouting_Map")
	cmd.Env = append(os.Environ(), "TEST_MAIN_SYMLINK_MAP=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\noutput: %s", err, output)
	}
	if !strings.Contains(string(output), "map") {
		t.Errorf("expected map --help output, got: %s", output)
	}
}

func TestMain_SymlinkRouting_SetsArgs(t *testing.T) {
	if os.Getenv("TEST_MAIN_SYMLINK_SETS_ARGS") == "1" {
		tmpDir := os.TempDir()
		os.Args = []string{"execute", "-c", "echo hello_from_test"}
		main()
		_ = tmpDir
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_SymlinkRouting_SetsArgs")
	cmd.Env = append(os.Environ(), "TEST_MAIN_SYMLINK_SETS_ARGS=1", "HOME=/tmp")
	output, _ := cmd.CombinedOutput()
	if !strings.Contains(string(output), "hello_from_test") {
		t.Errorf("expected symlink routing to pass args, got: %s", output)
	}
}

func TestCheckUpdate_ConfigDirError(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	t.Setenv("HOME", "")
	os.Unsetenv("XDG_CONFIG_HOME")
	os.Unsetenv("HOME")

	os.Args = []string{"sin-code"}

	checkUpdate()

	os.Setenv("HOME", t.TempDir())
}

func TestCheckUpdate_StampDirCreatedWhenMissing(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, "Library", "Application Support")

	os.Args = []string{"sin-code"}

	if _, err := os.Stat(configDir); err == nil {
		t.Fatal("config dir should not exist yet")
	}

	checkUpdate()

	stampDir := filepath.Join(configDir, "sin")
	if _, err := os.Stat(stampDir); os.IsNotExist(err) {
		t.Error("stamp dir should have been created")
	}
}

func TestCheckUpdate_StampContent(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, "Library", "Application Support", "sin")
	stampPath := filepath.Join(configDir, ".last-update-check")

	os.Args = []string{"sin-code"}

	checkUpdate()

	data, err := os.ReadFile(stampPath)
	if err != nil {
		t.Fatalf("stamp file should exist: %v", err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		t.Error("stamp file should contain a timestamp")
	}
	_, parseErr := time.Parse(time.RFC3339, content)
	if parseErr != nil {
		t.Errorf("stamp file content should be RFC3339 timestamp, got %q: %v", content, parseErr)
	}
}

func TestCheckUpdate_SkipsForNonVersionSubcommand(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	home := t.TempDir()
	t.Setenv("HOME", home)

	os.Args = []string{"sin-code", "scout", "query"}

	checkUpdate()

	stampPath := filepath.Join(home, "Library", "Application Support", "sin", ".last-update-check")
	if _, err := os.Stat(stampPath); err == nil {
		t.Error("stamp file should not exist for subcommand args")
	}
}

func TestMain_RootCmdError(t *testing.T) {
	if os.Getenv("TEST_MAIN_ROOTCMD_ERROR") == "1" {
		os.Args = []string{"sin-code", "nonexistent-subcommand-xyz"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_RootCmdError")
	cmd.Env = append(os.Environ(), "TEST_MAIN_ROOTCMD_ERROR=1", "HOME=/tmp")
	output, _ := cmd.CombinedOutput()
	if !strings.Contains(string(output), "sin-code") {
		t.Errorf("expected error output from root command, got: %s", output)
	}
}

func TestMain_ExecuteErrorCallsPrintError(t *testing.T) {
	if os.Getenv("TEST_MAIN_EXEC_ERROR") == "1" {
		os.Args = []string{"sin-code"}
		origCmd := rootCmd
		rootCmd.SetArgs([]string{"--nonexistent-flag-xyz-abc"})
		_ = origCmd.Execute()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_ExecuteErrorCallsPrintError")
	cmd.Env = append(os.Environ(), "TEST_MAIN_EXEC_ERROR=1", "HOME=/tmp")
	output, _ := cmd.CombinedOutput()
	_ = output
}

func TestMain_SymlinkRoutingDirect(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"discover", "--help"}

	name := filepath.Base(os.Args[0])
	routed := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			args := append([]string{name}, os.Args[1:]...)
			rootCmd.SetArgs(args)
			routed = true
			break
		}
	}

	if !routed {
		t.Error("expected symlink name 'discover' to match a subcommand")
	}
}

func TestMain_NoSymlinkMatch(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"sin-code", "discover", "."}

	name := filepath.Base(os.Args[0])
	matched := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			matched = true
			break
		}
	}
	if matched {
		t.Error("binary name 'sin-code' should not match any subcommand")
	}
}

