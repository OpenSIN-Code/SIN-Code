// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for skillmgr/manager.go. Uses package-level
// test hooks so the tests stay hermetic and fast (no real git/python/go calls).
package skillmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestMain intercepts subprocess calls used by fakeCommand below. When the
// test binary is re-executed with SKILLMGR_FAKE=1 it prints the configured
// output and exits without running the real test suite.
func TestMain(m *testing.M) {
	if os.Getenv("SKILLMGR_FAKE") == "1" {
		out := os.Getenv("SKILLMGR_FAKE_OUTPUT")
		if ec := os.Getenv("SKILLMGR_FAKE_EXIT"); ec != "" && ec != "0" {
			fmt.Fprint(os.Stderr, out)
			code, _ := strconv.Atoi(ec)
			os.Exit(code)
		}
		fmt.Fprint(os.Stdout, out)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func saveHooks[T any](t *testing.T, ptr *T, val T) {
	orig := *ptr
	*ptr = val
	t.Cleanup(func() { *ptr = orig })
}

func fakeCommand(t *testing.T, output string, exitCode int) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// Use a shell helper so the child does not re-execute the coverage-
		// instrumented test binary (which would append a GOCOVERDIR warning to
		// stderr and corrupt JSON output captured by CombinedOutput).
		cmd := exec.CommandContext(ctx, "sh", "-c", `printf "%s" "$OUT"; exit "$EXIT"`)
		cmd.Env = []string{
			"OUT=" + output,
			"EXIT=" + strconv.Itoa(exitCode),
		}
		return cmd
	}
}

func fakeStat(exists ...string) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		for _, e := range exists {
			if path == e {
				return nil, nil
			}
		}
		return nil, os.ErrNotExist
	}
}

func fakeGlob(matches ...string) func(string) ([]string, error) {
	return func(pattern string) ([]string, error) {
		return matches, nil
	}
}

func TestSkillsDir_FromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)
	if got := SkillsDir(); got != dir {
		t.Fatalf("SkillsDir from env: got %q, want %q", got, dir)
	}
}

func TestSkillsDir_FromHome(t *testing.T) {
	t.Setenv("SIN_SKILLS_DIR", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".local", "share", "sin-code", "skills")
	if got := SkillsDir(); got != want {
		t.Fatalf("SkillsDir from home: got %q, want %q", got, want)
	}
}

func TestKnownSkills_NonEmpty(t *testing.T) {
	ks := KnownSkills()
	if len(ks) == 0 {
		t.Fatal("KnownSkills should not be empty")
	}
	for _, name := range []string{"websearch", "browser", "simone", "honcho"} {
		if _, ok := ks[name]; !ok {
			t.Errorf("expected %q in KnownSkills", name)
		}
	}
}

func TestInstall_UnknownSkill(t *testing.T) {
	_, err := Install(context.Background(), "not-a-known-skill")
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
}

func TestInstall_PullSuccess(t *testing.T) {
	skillsDir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", skillsDir)
	dir := filepath.Join(skillsDir, "web_search_bundle")

	saveHooks(t, &_osStat, fakeStat(filepath.Join(dir, ".git")))
	saveHooks(t, &_execCommandContext, fakeCommand(t, "", 0))

	st, err := Install(context.Background(), "websearch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.Installed {
		t.Fatal("expected installed")
	}
}

func TestInstall_PullError(t *testing.T) {
	skillsDir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", skillsDir)
	dir := filepath.Join(skillsDir, "web_search_bundle")

	saveHooks(t, &_osStat, fakeStat(filepath.Join(dir, ".git")))
	saveHooks(t, &_execCommandContext, fakeCommand(t, "pull failed", 1))

	st, err := Install(context.Background(), "websearch")
	if err == nil {
		t.Fatal("expected error")
	}
	if st == nil || st.Installed {
		t.Fatal("expected st installed false before verify")
	}
}

func TestInstall_CloneSuccess(t *testing.T) {
	skillsDir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", skillsDir)
	dir := filepath.Join(skillsDir, "web_search_bundle")
	_ = dir

	saveHooks(t, &_osStat, fakeStat())
	saveHooks(t, &_osMkdirAll, func(path string, perm os.FileMode) error { return nil })
	saveHooks(t, &_execCommandContext, fakeCommand(t, "", 0))

	st, err := Install(context.Background(), "websearch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.Installed {
		t.Fatal("expected installed")
	}
}

func TestInstall_CloneMkdirError(t *testing.T) {
	skillsDir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", skillsDir)
	dir := filepath.Join(skillsDir, "web_search_bundle")
	_ = dir

	saveHooks(t, &_osStat, fakeStat())
	saveHooks(t, &_osMkdirAll, func(path string, perm os.FileMode) error { return fmt.Errorf("denied") })

	_, err := Install(context.Background(), "websearch")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInstall_CloneError(t *testing.T) {
	skillsDir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", skillsDir)
	dir := filepath.Join(skillsDir, "web_search_bundle")

	_ = dir

	saveHooks(t, &_osStat, fakeStat())
	saveHooks(t, &_osMkdirAll, func(path string, perm os.FileMode) error { return nil })
	saveHooks(t, &_execCommandContext, fakeCommand(t, "clone failed", 1))

	_, err := Install(context.Background(), "websearch")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStatus_Mixed(t *testing.T) {
	skillsDir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", skillsDir)

	// Mark the websearch repo as installed and the rest as absent.
	websearchDir := filepath.Join(skillsDir, "web_search_bundle")
	entry := filepath.Join(websearchDir, "mcp_server.py")
	stat := func(path string) (os.FileInfo, error) {
		if path == websearchDir || path == entry {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	saveHooks(t, &_osStat, stat)
	saveHooks(t, &_execCommandContext, fakeCommand(t, `{"tools":[{"name":"x"}]}`, 0))

	sts := Status(context.Background())
	if len(sts) == 0 {
		t.Fatal("expected status entries")
	}
	foundInstalled := false
	for _, st := range sts {
		if st.Name == "websearch" {
			foundInstalled = true
			if !st.Installed {
				t.Fatal("websearch should be installed")
			}
			if !st.Runnable {
				t.Fatalf("websearch should be runnable: %s", st.Detail)
			}
		}
	}
	if !foundInstalled {
		t.Fatal("websearch entry missing")
	}
}

func TestVerifyEntrypoint_PythonWithTools(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)
	entry := filepath.Join(dir, "mcp_server.py")

	saveHooks(t, &_osStat, fakeStat(entry))
	saveHooks(t, &_execCommandContext, fakeCommand(t, `{"tools":[{"name":"a"}]}`, 0))

	runnable, detail := verifyEntrypoint(context.Background(), dir)
	if !runnable || detail != "1 tools" {
		t.Fatalf("expected runnable with 1 tools, got %v %q", runnable, detail)
	}
}

func TestVerifyEntrypoint_PythonNoTools(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "mcp_server.py")

	saveHooks(t, &_osStat, fakeStat(entry))
	saveHooks(t, &_execCommandContext, fakeCommand(t, `{"tools":[]}`, 0))

	runnable, detail := verifyEntrypoint(context.Background(), dir)
	if !runnable || detail != "entrypoint responds (tool list format unknown)" {
		t.Fatalf("unexpected result: %v %q", runnable, detail)
	}
}

func TestVerifyEntrypoint_PythonSmokeFail(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "mcp_server.py")

	saveHooks(t, &_osStat, fakeStat(entry))
	saveHooks(t, &_execCommandContext, fakeCommand(t, "boom", 1))

	runnable, detail := verifyEntrypoint(context.Background(), dir)
	if runnable || detail == "" {
		t.Fatalf("expected smoke-fail result, got %v %q", runnable, detail)
	}
}

func TestVerifyEntrypoint_PythonModule(t *testing.T) {
	dir := t.TempDir()

	saveHooks(t, &_osStat, fakeStat())
	saveHooks(t, &_filepathGlob, fakeGlob(filepath.Join(dir, "src", "foo", "__main__.py")))

	runnable, detail := verifyEntrypoint(context.Background(), dir)
	if !runnable || detail == "" {
		t.Fatalf("expected module result, got %v %q", runnable, detail)
	}
}

func TestVerifyEntrypoint_Node(t *testing.T) {
	dir := t.TempDir()

	stat := func(path string) (os.FileInfo, error) {
		if path == filepath.Join(dir, "package.json") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	saveHooks(t, &_osStat, stat)
	saveHooks(t, &_filepathGlob, fakeGlob())

	runnable, detail := verifyEntrypoint(context.Background(), dir)
	if !runnable || detail != "node entrypoint (package.json)" {
		t.Fatalf("unexpected node result: %v %q", runnable, detail)
	}
}

func TestVerifyEntrypoint_GoSuccess(t *testing.T) {
	dir := t.TempDir()

	stat := func(path string) (os.FileInfo, error) {
		if path == filepath.Join(dir, "go.mod") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	saveHooks(t, &_osStat, stat)
	saveHooks(t, &_filepathGlob, fakeGlob())
	saveHooks(t, &_execCommandContext, fakeCommand(t, "", 0))

	runnable, detail := verifyEntrypoint(context.Background(), dir)
	if !runnable || detail != "go entrypoint builds" {
		t.Fatalf("unexpected go success result: %v %q", runnable, detail)
	}
}

func TestVerifyEntrypoint_GoBuildFail(t *testing.T) {
	dir := t.TempDir()

	stat := func(path string) (os.FileInfo, error) {
		if path == filepath.Join(dir, "go.mod") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	saveHooks(t, &_osStat, stat)
	saveHooks(t, &_filepathGlob, fakeGlob())
	saveHooks(t, &_execCommandContext, fakeCommand(t, "build fail", 1))

	runnable, detail := verifyEntrypoint(context.Background(), dir)
	if runnable || detail == "" {
		t.Fatalf("expected go build failure result, got %v %q", runnable, detail)
	}
}

func TestVerifyEntrypoint_None(t *testing.T) {
	dir := t.TempDir()

	saveHooks(t, &_osStat, fakeStat())
	saveHooks(t, &_filepathGlob, fakeGlob())

	runnable, detail := verifyEntrypoint(context.Background(), dir)
	if runnable || detail != "no recognized MCP entrypoint" {
		t.Fatalf("expected none result, got %v %q", runnable, detail)
	}
}

func TestVerifyEntrypoint_ContextTimeout(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "mcp_server.py")

	saveHooks(t, &_osStat, fakeStat(entry))
	// Block longer than the supplied context timeout.
	saveHooks(t, &_execCommandContext, func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0])
		cmd.Env = []string{
			"SKILLMGR_FAKE=1",
			"SKILLMGR_FAKE_OUTPUT=",
			"SKILLMGR_FAKE_EXIT=0",
		}
		return cmd
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	<-ctx.Done() // ensure context is already expired

	verifyEntrypoint(ctx, dir)
}
