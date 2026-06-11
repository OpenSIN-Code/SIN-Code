package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stampDirForTest prepares an isolated, cross-platform config environment
// for update-check tests and stubs the network probe so no real GitHub
// request is made. It returns the expected stamp dir and stamp file path,
// derived from os.UserConfigDir() so the tests pass on Linux, macOS, and
// Windows alike.
func stampDirForTest(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SIN_CODE_NO_UPDATE_CHECK", "")
	t.Setenv("NO_UPDATE_CHECK", "")
	t.Setenv("SIN_CODE_OFFLINE", "")

	orig := checkUpdateFn
	checkUpdateFn = func() (string, bool, error) { return "", false, nil }
	t.Cleanup(func() { checkUpdateFn = orig })

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	stampDir := filepath.Join(configDir, "sin")
	stampPath := filepath.Join(stampDir, ".last-update-check")
	return stampDir, stampPath
}

func TestCheckUpdate_SkipsWithSubcommandArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	_, stampPath := stampDirForTest(t)

	os.Args = []string{"sin-code", "discover", "."}
	checkUpdate()

	if _, err := os.Stat(stampPath); err == nil {
		t.Error("stamp file should not exist when subcommand args are present")
	}
}

func TestCheckUpdate_SkipsWithRecentStamp(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	stampDir, stampPath := stampDirForTest(t)

	os.Args = []string{"sin-code"}

	os.MkdirAll(stampDir, 0755)
	os.WriteFile(stampPath, []byte(time.Now().Format(time.RFC3339)), 0644)
	originalModTime := time.Now()

	checkUpdate()

	info, err := os.Stat(stampPath)
	if err != nil {
		t.Fatalf("stamp file should still exist: %v", err)
	}
	if info.ModTime().Sub(originalModTime) > 5*time.Second {
		t.Error("stamp file should not have been updated when it was recent")
	}
}

func TestCheckUpdate_ProceedsWithOldStamp(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	stampDir, stampPath := stampDirForTest(t)

	os.Args = []string{"sin-code"}

	os.MkdirAll(stampDir, 0755)
	oldTime := time.Now().Add(-48 * time.Hour)
	os.WriteFile(stampPath, []byte(oldTime.Format(time.RFC3339)), 0644)
	os.Chtimes(stampPath, oldTime, oldTime)

	checkUpdate()

	info, err := os.Stat(stampPath)
	if err != nil {
		t.Fatalf("stamp file should exist after update check: %v", err)
	}
	if time.Since(info.ModTime()) > 5*time.Second {
		t.Error("stamp file should have been updated when it was old")
	}
}

func TestCheckUpdate_ProceedsWithNoStamp(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	_, stampPath := stampDirForTest(t)

	os.Args = []string{"sin-code"}

	checkUpdate()

	if _, err := os.Stat(stampPath); os.IsNotExist(err) {
		t.Error("stamp file should be created when it does not exist")
	}
}

func TestCheckUpdate_ProceedsWithVersionFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	_, stampPath := stampDirForTest(t)

	os.Args = []string{"sin-code", "--version"}

	checkUpdate()

	if _, err := os.Stat(stampPath); os.IsNotExist(err) {
		t.Error("stamp file should be created with --version flag")
	}
}

func TestCheckUpdate_ProceedsWithShortVFlag(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	_, stampPath := stampDirForTest(t)

	os.Args = []string{"sin-code", "-v"}

	checkUpdate()

	if _, err := os.Stat(stampPath); os.IsNotExist(err) {
		t.Error("stamp file should be created with -v flag")
	}
}

func TestSymlinkRouting_DiscoverNameMatches(t *testing.T) {
	name := "discover"
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find subcommand matching symlink name %q", name)
	}
}

func TestSymlinkRouting_ExecuteNameMatches(t *testing.T) {
	name := "execute"
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find subcommand matching symlink name %q", name)
	}
}

func TestSymlinkRouting_SinCodeNoSubcommandMatch(t *testing.T) {
	name := "sin-code"
	matched := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			matched = true
			break
		}
	}
	if matched {
		t.Error("expected no subcommand match when binary name is 'sin-code'")
	}
}

func TestSymlinkRouting_AllSubcommandsMatchable(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "help" {
			continue
		}
		name := cmd.Name()
		found := false
		for _, c := range rootCmd.Commands() {
			if c.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("subcommand %q not found in rootCmd.Commands()", name)
		}
	}
}

func TestSymlinkRouting_SetsArgsCorrectly(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"discover", "/tmp", "--format", "json"}

	name := filepath.Base(os.Args[0])
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			expected := append([]string{name}, os.Args[1:]...)
			if len(expected) != 4 {
				t.Errorf("expected 4 args after routing, got %d", len(expected))
			}
			if expected[0] != "discover" {
				t.Errorf("expected first arg 'discover', got %q", expected[0])
			}
			if expected[1] != "/tmp" {
				t.Errorf("expected second arg '/tmp', got %q", expected[1])
			}
			break
		}
	}
}
