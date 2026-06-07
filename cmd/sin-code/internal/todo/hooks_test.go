// SPDX-License-Identifier: MIT
// Purpose: tests for the todo hooks system: TOML config, executor, env vars, timeout.
package todo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHookEventValid(t *testing.T) {
	for _, e := range AllEvents() {
		if !e.Valid() {
			t.Errorf("expected %q valid", e)
		}
	}
	if HookEvent("nope").Valid() {
		t.Error("expected 'nope' invalid")
	}
}

func TestHookValidate(t *testing.T) {
	cases := []struct {
		name    string
		h       Hook
		wantErr bool
	}{
		{"empty command", Hook{Command: ""}, true},
		{"valid", Hook{Command: "echo hi"}, false},
		{"valid timeout", Hook{Command: "echo hi", Timeout: 5 * time.Second}, false},
		{"negative timeout", Hook{Command: "echo hi", Timeout: -1 * time.Second}, true},
		{"invalid on_error", Hook{Command: "echo hi", OnError: "panic"}, true},
		{"ignore on_error", Hook{Command: "echo hi", OnError: "ignore"}, false},
		{"warn on_error", Hook{Command: "echo hi", OnError: "warn"}, false},
		{"fail on_error", Hook{Command: "echo hi", OnError: "fail"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.h.Validate()
			if c.wantErr && err == nil {
				t.Error("expected error")
			}
			if !c.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadHooksConfigMissing(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadHooksConfig(filepath.Join(dir, "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hooks == nil {
		t.Error("Hooks map should be initialized")
	}
}

func TestLoadHooksConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.toml")
	_ = os.WriteFile(path, []byte(""), 0644)
	cfg, err := LoadHooksConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hooks == nil {
		t.Error("Hooks map should be initialized")
	}
}

func TestLoadHooksConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	_ = os.WriteFile(path, []byte("hooks = { pre_add = [{ command = \"\" }] }"), 0644)
	_, err := LoadHooksConfig(path)
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestLoadHooksConfigValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.toml")
	body := `[[hooks.post_complete]]
command = "echo done"
timeout = "5s"
on_error = "warn"
[[hooks.pre_add]]
command = "echo creating $SIN_TODO_TITLE"
`
	_ = os.WriteFile(path, []byte(body), 0644)
	cfg, err := LoadHooksConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Hooks[EventPostComplete]) != 1 {
		t.Errorf("expected 1 post_complete, got %d", len(cfg.Hooks[EventPostComplete]))
	}
	if len(cfg.Hooks[EventPreAdd]) != 1 {
		t.Errorf("expected 1 pre_add, got %d", len(cfg.Hooks[EventPreAdd]))
	}
}

func TestHookConfigAddAndGet(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadHooksConfig(filepath.Join(dir, "hooks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	h := Hook{Command: "echo added", Timeout: time.Second}
	if err := cfg.Add(EventPreAdd, h); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Get(EventPreAdd); len(got) != 1 {
		t.Errorf("expected 1, got %d", len(got))
	}
}

func TestHookConfigAddInvalid(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadHooksConfig(filepath.Join(dir, "hooks.toml"))
	if err := cfg.Add(EventPreAdd, Hook{Command: ""}); err == nil {
		t.Error("expected validation error")
	}
}

func TestHookConfigRemove(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadHooksConfig(filepath.Join(dir, "hooks.toml"))
	_ = cfg.Add(EventPreAdd, Hook{Command: "a"})
	_ = cfg.Add(EventPreAdd, Hook{Command: "b"})
	if err := cfg.Remove(EventPreAdd, 0); err != nil {
		t.Fatal(err)
	}
	hooks := cfg.Get(EventPreAdd)
	if len(hooks) != 1 || hooks[0].Command != "b" {
		t.Errorf("expected only b, got %+v", hooks)
	}
}

func TestHookConfigRemoveLast(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadHooksConfig(filepath.Join(dir, "hooks.toml"))
	_ = cfg.Add(EventPreAdd, Hook{Command: "a"})
	_ = cfg.Remove(EventPreAdd, 0)
	if hooks := cfg.Get(EventPreAdd); len(hooks) != 0 {
		t.Errorf("expected empty, got %d", len(hooks))
	}
}

func TestHookConfigRemoveInvalidIndex(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadHooksConfig(filepath.Join(dir, "hooks.toml"))
	if err := cfg.Remove(EventPreAdd, 0); err == nil {
		t.Error("expected error for missing event")
	}
	_ = cfg.Add(EventPreAdd, Hook{Command: "a"})
	if err := cfg.Remove(EventPreAdd, 5); err == nil {
		t.Error("expected error for out-of-range index")
	}
}

func TestHookConfigPersistReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.toml")
	cfg, _ := LoadHooksConfig(path)
	_ = cfg.Add(EventPostComplete, Hook{Command: "echo persist", Timeout: time.Second})

	cfg2, err := LoadHooksConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	hooks := cfg2.Get(EventPostComplete)
	if len(hooks) != 1 || hooks[0].Command != "echo persist" {
		t.Errorf("persistence failed, got %+v", hooks)
	}
}

func TestBuildEnv(t *testing.T) {
	td := &Todo{
		ID:       "st-aaaa",
		Title:    "Test",
		Status:   StatusOpen,
		Priority: PriorityP0,
		Type:     TypeBug,
		Assignee: "alice",
		Tags:     []string{"urgent", "x"},
		Project:  "demo",
	}
	ctx := HookContext{
		Event: EventPostAdd,
		Todo:  td,
		Actor: "bob",
		From:  "open",
		To:    "open",
	}
	env := buildEnv(ctx)
	want := map[string]string{
		"SIN_EVENT":         "post_add",
		"SIN_ACTOR":         "bob",
		"SIN_TODO_ID":       "st-aaaa",
		"SIN_TODO_TITLE":    "Test",
		"SIN_TODO_STATUS":   "open",
		"SIN_TODO_PRIORITY": "P0",
		"SIN_TODO_TYPE":     "bug",
		"SIN_TODO_ASSIGNEE": "alice",
		"SIN_TODO_TAGS":     "urgent,x",
		"SIN_TODO_PROJECT":  "demo",
		"SIN_FROM":          "open",
		"SIN_TO":            "open",
	}
	envMap := map[string]string{}
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok {
			envMap[k] = v
		}
	}
	for k, v := range want {
		if envMap[k] != v {
			t.Errorf("env %s: got %q, want %q", k, envMap[k], v)
		}
	}
}

func TestRunHookSuccess(t *testing.T) {
	h := Hook{Command: "echo hello"}
	ctx := HookContext{Event: EventPostAdd}
	r := runHook(h, ctx)
	if r.Err != nil {
		t.Errorf("unexpected error: %v", r.Err)
	}
	if !strings.Contains(r.Stdout, "hello") {
		t.Errorf("expected stdout to contain 'hello', got %q", r.Stdout)
	}
}

func TestRunHookFailure(t *testing.T) {
	h := Hook{Command: "false"}
	r := runHook(h, HookContext{})
	if r.Err == nil {
		t.Error("expected error for false command")
	}
}

func TestRunHookTimeout(t *testing.T) {
	h := Hook{Command: "sleep 5", Timeout: 100 * time.Millisecond}
	start := time.Now()
	r := runHook(h, HookContext{})
	elapsed := time.Since(start)
	if r.Err == nil {
		t.Error("expected timeout error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("took too long: %s", elapsed)
	}
}

func TestRunHookEnvInjection(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "out.txt")
	cmd := "echo $SIN_TODO_TITLE > " + marker
	h := Hook{Command: cmd}
	td := &Todo{ID: "st-x", Title: "InjectedTitle", Status: StatusOpen, Priority: PriorityP0}
	r := runHook(h, HookContext{Todo: td, Actor: "tester"})
	if r.Err != nil {
		t.Fatalf("hook failed: %v", r.Err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "InjectedTitle") {
		t.Errorf("env not injected: %s", string(data))
	}
}

func TestFireHooksEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadHooksConfig(filepath.Join(dir, "hooks.toml"))
	results := cfg.Fire(HookContext{Event: EventPostAdd})
	if results != nil {
		t.Errorf("expected nil, got %v", results)
	}
}

func TestFireHooksMultiple(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadHooksConfig(filepath.Join(dir, "hooks.toml"))
	_ = cfg.Add(EventPostAdd, Hook{Command: "echo first"})
	_ = cfg.Add(EventPostAdd, Hook{Command: "echo second"})
	results := cfg.Fire(HookContext{Event: EventPostAdd})
	if len(results) != 2 {
		t.Errorf("expected 2, got %d", len(results))
	}
}

func TestFireHooksSequential(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := LoadHooksConfig(filepath.Join(dir, "hooks.toml"))
	_ = cfg.Add(EventPostAdd, Hook{Command: "echo a"})
	_ = cfg.Add(EventPostAdd, Hook{Command: "echo b"})
	results := cfg.Fire(HookContext{Event: EventPostAdd})
	if !strings.Contains(results[0].Stdout, "a") {
		t.Errorf("first hook should output a")
	}
	if !strings.Contains(results[1].Stdout, "b") {
		t.Errorf("second hook should output b")
	}
}

func TestHookResultOK(t *testing.T) {
	if !(HookResult{}).OK() {
		t.Error("empty result should be OK")
	}
	if (HookResult{Err: os.ErrNotExist}).OK() {
		t.Error("result with err should not be OK")
	}
}

func TestDefaultHooksPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := DefaultHooksPath()
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if !strings.HasSuffix(path, "hooks.toml") {
		t.Errorf("expected .toml suffix, got %q", path)
	}
}

func TestToStringSlice(t *testing.T) {
	es := AllEvents()
	ss := toStringSlice(es)
	if len(ss) != len(es) {
		t.Errorf("length mismatch")
	}
	for i, e := range es {
		if ss[i] != string(e) {
			t.Errorf("idx %d: got %q, want %q", i, ss[i], e)
		}
	}
}

func TestHookConfigPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.toml")
	cfg, _ := LoadHooksConfig(path)
	if cfg.Path() != path {
		t.Errorf("got %q, want %q", cfg.Path(), path)
	}
}

func TestSaveFailsOnReadonlyDir(t *testing.T) {
	cfg := &HookConfig{Hooks: map[HookEvent][]Hook{}, path: "/dev/null/impossible/hooks.toml"}
	err := cfg.save()
	if err == nil {
		t.Error("expected error saving to bad path")
	}
}
