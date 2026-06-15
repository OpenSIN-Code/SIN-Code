package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateCheckDisabled(t *testing.T) {
	keys := []string{"SIN_CODE_NO_UPDATE_CHECK", "NO_UPDATE_CHECK", "SIN_CODE_OFFLINE"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			for _, k := range keys {
				os.Unsetenv(k)
			}
			t.Setenv(key, "1")
			if !updateCheckDisabled() {
				t.Errorf("updateCheckDisabled() = false, want true with %s set", key)
			}
		})
	}
	t.Run("none", func(t *testing.T) {
		for _, k := range keys {
			os.Unsetenv(k)
		}
		if updateCheckDisabled() {
			t.Error("updateCheckDisabled() = true, want false")
		}
	})
}

func TestCheckUpdate_Timeout(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	_, stampPath := stampDirForTest(t)
	os.Args = []string{"sin-code"}
	checkUpdateFn = func() (string, bool, error) {
		time.Sleep(3 * time.Second)
		return "", false, nil
	}
	start := time.Now()
	checkUpdate()
	if time.Since(start) < 1500*time.Millisecond || time.Since(start) > 3*time.Second {
		t.Errorf("checkUpdate timeout took %v, expected ~2s", time.Since(start))
	}
	if _, err := os.Stat(stampPath); os.IsNotExist(err) {
		t.Error("stamp file should be created even on timeout")
	}
}

func TestMain_SandboxExecError(t *testing.T) {
	if os.Getenv("TEST_MAIN_SANDBOX_ERROR") == "1" {
		os.Args = []string{"sin-code", "__sandbox_exec", "sh", "-c", "echo hi"}
		t.Setenv("SIN_CODE_NO_UPDATE_CHECK", "1")
		sandboxApplyAndExecFn = func() error { return errors.New("sandbox fail") }
		var code int
		osExitFn = func(c int) { code = c }
		main()
		if code != 126 {
			t.Errorf("exit code = %d, want 126", code)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_SandboxExecError")
	cmd.Env = append(os.Environ(), "TEST_MAIN_SANDBOX_ERROR=1", "HOME=/tmp", "SIN_CODE_NO_UPDATE_CHECK=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\noutput: %s", err, out)
	}
	if !bytes.Contains(out, []byte("sandbox fail")) {
		t.Errorf("expected sandbox error output, got: %s", out)
	}
}

func TestMain_SandboxExecSuccess(t *testing.T) {
	if os.Getenv("TEST_MAIN_SANDBOX_SUCCESS") == "1" {
		os.Args = []string{"sin-code", "__sandbox_exec", "sh", "-c", "echo hi"}
		t.Setenv("SIN_CODE_NO_UPDATE_CHECK", "1")
		sandboxApplyAndExecFn = func() error { return nil }
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_SandboxExecSuccess")
	cmd.Env = append(os.Environ(), "TEST_MAIN_SANDBOX_SUCCESS=1", "HOME=/tmp", "SIN_CODE_NO_UPDATE_CHECK=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\noutput: %s", err, out)
	}
	if bytes.Contains(out, []byte("sandbox")) {
		t.Errorf("expected no sandbox output on success, got: %s", out)
	}
}

func TestMain_SymlinkRoutingInProcess(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	origCheckUpdate := checkUpdateFn
	defer func() { checkUpdateFn = origCheckUpdate }()
	t.Setenv("SIN_CODE_NO_UPDATE_CHECK", "1")
	checkUpdateFn = func() (string, bool, error) { return "", false, nil }

	os.Args = []string{"discover", "--version"}
	main()
	// If we reach here, routing + version flag executed without panicking.
}

func TestMain_SymlinkRoutingLoop_Break(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"sin-code", "discover", "."}
	name := filepath.Base(os.Args[0])
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			found = true
			break
		}
	}
	if found {
		t.Error("binary name 'sin-code' should not match any subcommand")
	}
}

func TestMain_SandboxExecErrorInProcess(t *testing.T) {
	origArgs := os.Args
	origSandbox := sandboxApplyAndExecFn
	origExit := osExitFn
	origStderr := osStderrFn
	origCheckUpdate := checkUpdateFn
	defer func() {
		os.Args = origArgs
		sandboxApplyAndExecFn = origSandbox
		osExitFn = origExit
		osStderrFn = origStderr
		checkUpdateFn = origCheckUpdate
		rootCmd.SetArgs(nil)
	}()

	t.Setenv("SIN_CODE_NO_UPDATE_CHECK", "1")
	checkUpdateFn = func() (string, bool, error) { return "", false, nil }

	errFile, err := os.CreateTemp("", "sin-stderr-*.log")
	if err != nil {
		t.Fatalf("create temp stderr: %v", err)
	}
	defer os.Remove(errFile.Name())

	os.Args = []string{"sin-code", "__sandbox_exec", "sh", "-c", "echo hi"}
	sandboxApplyAndExecFn = func() error { return errors.New("sandbox fail") }
	osStderrFn = errFile
	var code int
	var exited bool
	osExitFn = func(c int) { code = c; exited = true }

	main()

	if !exited || code != 126 {
		t.Errorf("exit code = %d (exited=%v), want 126", code, exited)
	}
	if _, err := errFile.Seek(0, 0); err != nil {
		t.Fatalf("seek stderr: %v", err)
	}
	out, _ := os.ReadFile(errFile.Name())
	if !bytes.Contains(out, []byte("sandbox fail")) {
		t.Errorf("expected stderr with sandbox error, got %q", out)
	}
}

func TestMain_SandboxExecSuccessInProcess(t *testing.T) {
	origArgs := os.Args
	origSandbox := sandboxApplyAndExecFn
	origExit := osExitFn
	origCheckUpdate := checkUpdateFn
	defer func() {
		os.Args = origArgs
		sandboxApplyAndExecFn = origSandbox
		osExitFn = origExit
		checkUpdateFn = origCheckUpdate
		rootCmd.SetArgs(nil)
	}()

	t.Setenv("SIN_CODE_NO_UPDATE_CHECK", "1")
	checkUpdateFn = func() (string, bool, error) { return "", false, nil }

	os.Args = []string{"sin-code", "__sandbox_exec", "sh", "-c", "echo hi"}
	sandboxApplyAndExecFn = func() error { return nil }
	var code int
	var exited bool
	osExitFn = func(c int) { code = c; exited = true }

	main()

	if exited {
		t.Errorf("expected main to return, but osExitFn called with %d", code)
	}
}

func TestMain_SymlinkRoutingNoMatchInProcess(t *testing.T) {
	origArgs := os.Args
	origCheckUpdate := checkUpdateFn
	defer func() {
		os.Args = origArgs
		checkUpdateFn = origCheckUpdate
		rootCmd.SetArgs(nil)
	}()

	t.Setenv("SIN_CODE_NO_UPDATE_CHECK", "1")
	checkUpdateFn = func() (string, bool, error) { return "", false, nil }

	os.Args = []string{"sin-code", "--version"}
	main()
	// If we reach here, the no-match symlink path executed without panic.
}
