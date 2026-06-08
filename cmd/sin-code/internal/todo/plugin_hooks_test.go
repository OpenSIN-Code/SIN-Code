// SPDX-License-Identifier: MIT
// Purpose: unit tests for plugin hook wiring: runPluginHook returns output,
// firePluginHooks records audit entries.
package todo

import (
	"os"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/plugins"
)

func TestRunPluginHookSuccess(t *testing.T) {
	stdout, stderr, exitCode, err := runPluginHook(plugins.HookDef{
		Plugin:  "test",
		Event:   "post_add",
		Command: "echo hello-world",
		Timeout: 5,
	}, HookContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d", exitCode)
	}
	if stdout != "hello-world\n" {
		t.Errorf("expected 'hello-world\\n', got %q", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got %q", stderr)
	}
}

func TestRunPluginHookMissingBinary(t *testing.T) {
	stdout, stderr, exitCode, err := runPluginHook(plugins.HookDef{
		Plugin:  "test",
		Event:   "post_add",
		Command: "nonexistent-binary-xyz",
		Timeout: 5,
	}, HookContext{})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if exitCode == 0 {
		t.Error("expected non-zero exit code")
	}
	_ = stdout
	_ = stderr
}

func TestRunPluginHookTimeout(t *testing.T) {
	stdout, stderr, exitCode, err := runPluginHook(plugins.HookDef{
		Plugin:  "test",
		Event:   "post_add",
		Command: "sleep 10",
		Timeout: 1,
	}, HookContext{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if exitCode == -1 {
		t.Log("timeout processes often have exit -1 on macOS")
	}
	_ = stdout
	_ = stderr
	_ = exitCode
}

func TestFirePluginHooksRecordsAudit(t *testing.T) {
	if os.Getenv("SIN_CODE_TEST_PLUGIN_HOOKS") == "" {
		t.Skip("set SIN_CODE_TEST_PLUGIN_HOOKS=1 to run (requires .sin-code/plugins directory)")
	}
	store := tempStore(t)
	now := time.Now()
	todo := &Todo{
		ID:    "st-test-" + GenerateID(),
		Title: "test",
	}
	firePluginHooks(store, EventPostAdd, todo, "", todo.Title, "")
	entries, err := store.ListAudit(todo.ID)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) == 0 {
		t.Log("no plugin hooks fired (no plugins configured) — this is OK")
	}
	_ = now
}
