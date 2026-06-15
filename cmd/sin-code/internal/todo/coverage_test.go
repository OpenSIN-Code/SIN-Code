// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for the todo package — exercise error paths
// and CLI commands that the main CRUD tests do not reach.
package todo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/plugins"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	bolt "go.etcd.io/bbolt"
)

func setTestGlobals(t *testing.T) (string, func()) {
	dir := t.TempDir()
	db := filepath.Join(dir, "todo.db")
	oldDB := todoDBPath
	oldAs := todoAs
	oldProject := todoProject
	t.Setenv("XDG_CONFIG_HOME", dir)
	todoDBPath = db
	todoAs = "tester"
	todoProject = "testproj"
	return db, func() {
		todoDBPath = oldDB
		todoAs = oldAs
		todoProject = oldProject
	}
}

func setupCmdTest(t *testing.T) func() {
	_, cleanup := setTestGlobals(t)
	oldFormat := todoFormat
	todoFormat = "text"
	origNotify := notifyFn
	origFireHooks := fireHooksFn
	origFirePluginHooks := firePluginHooksFn
	origGetHookConfig := getHookConfigFn
	origPrintJSON := printJSONFn
	origOpenStore := openStoreFn
	origActor := currentActorFn
	origProject := currentProjectFn
	origConfigDirHooks := osUserConfigDirHooks
	origConfigDirTodo := osUserConfigDirTodo
	dir := t.TempDir()
	notifyFn = func(nt notifications.Type, todoID, title, message, actor string) {}
	fireHooksFn = func(store *Store, event HookEvent, t *Todo, from, to, note string) {}
	firePluginHooksFn = func(store *Store, event HookEvent, t *Todo, from, to, note string) {}
	getHookConfigFn = func() *HookConfig { return &HookConfig{Hooks: map[HookEvent][]Hook{}} }
	osUserConfigDirHooks = func() (string, error) { return dir, nil }
	osUserConfigDirTodo = func() (string, error) { return dir, nil }
	return func() {
		todoFormat = oldFormat
		notifyFn = origNotify
		fireHooksFn = origFireHooks
		firePluginHooksFn = origFirePluginHooks
		getHookConfigFn = origGetHookConfig
		printJSONFn = origPrintJSON
		openStoreFn = origOpenStore
		currentActorFn = origActor
		currentProjectFn = origProject
		osUserConfigDirHooks = origConfigDirHooks
		osUserConfigDirTodo = origConfigDirTodo
		cleanup()
	}
}

func runCmd(cmd *cobra.Command, args []string, flags map[string]string, format string) error {
	oldVals := map[string]string{}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		oldVals[f.Name] = f.Value.String()
	})
	if flags != nil {
		for name, val := range flags {
			_ = cmd.Flags().Set(name, val)
		}
	}
	_ = cmd.Flags().Set("format", format)
	root := cmd.Root()
	if root == cmd {
		root.SetArgs(args)
	} else {
		root.SetArgs(append(strings.Split(cmd.CommandPath(), " ")[1:], args...))
	}
	err := root.Execute()
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if v, ok := oldVals[f.Name]; ok {
			_ = f.Value.Set(v)
		}
		f.Changed = false
	})
	return err
}

// ── remaining coverage tests — fill the statement gaps identified by
// `go tool cover -func` after the first pass. ───────────────────────────────

func TestListAuditSorted(t *testing.T) {
	s := tempStore(t)
	_ = s.AppendAudit(AuditEntry{TodoID: "x", Action: "a", Timestamp: time.Now().Add(-2 * time.Hour)})
	_ = s.AppendAudit(AuditEntry{TodoID: "x", Action: "b", Timestamp: time.Now().Add(-1 * time.Hour)})
	entries, err := s.ListAudit("x")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Action != "a" || entries[1].Action != "b" {
		t.Errorf("expected sorted order a,b, got %v", []string{entries[0].Action, entries[1].Action})
	}
}

func TestCompactSkipOpen(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "Open", Status: StatusOpen})
	old := time.Now().Add(-1000 * time.Hour)
	done := &Todo{Title: "Done", Status: StatusDone, UpdatedAt: old, ClosedAt: &old}
	_ = s.Add(done)
	res, err := s.Compact(CompactOptions{OlderThan: 720 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if res.Compacted != 1 {
		t.Errorf("expected 1 compacted, got %d", res.Compacted)
	}
}

func TestOpenMkdirError(t *testing.T) {
	orig := osMkdirAllStore
	osMkdirAllStore = func(path string, perm os.FileMode) error { return errors.New("boom") }
	defer func() { osMkdirAllStore = orig }()
	if _, err := Open(filepath.Join(t.TempDir(), "todo.db")); err == nil {
		t.Error("expected mkdir error")
	}
}

func TestOpenCreateBucketError(t *testing.T) {
	orig := createBucketIfNotExistsFn
	createBucketIfNotExistsFn = func(tx *bolt.Tx, name []byte) (*bolt.Bucket, error) { return nil, errors.New("boom") }
	defer func() { createBucketIfNotExistsFn = orig }()
	if _, err := Open(filepath.Join(t.TempDir(), "todo.db")); err == nil {
		t.Error("expected create bucket error")
	}
}

func TestAddInvalid(t *testing.T) {
	s := tempStore(t)
	if err := s.Add(nil); err == nil {
		t.Error("expected nil todo error")
	}
	if err := s.Add(&Todo{}); err == nil {
		t.Error("expected title required")
	}
	if err := s.Add(&Todo{Title: "A", Priority: "P9"}); err == nil {
		t.Error("expected invalid priority")
	}
	if err := s.Add(&Todo{Title: "A", Type: "nope"}); err == nil {
		t.Error("expected invalid type")
	}
	if err := s.Add(&Todo{Title: "A", Status: "nope"}); err == nil {
		t.Error("expected invalid status")
	}
}

func TestAddBucketPutError(t *testing.T) {
	s := tempStore(t)
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("boom") }
	defer func() { bucketPutFn = orig }()
	if err := s.Add(&Todo{Title: "A"}); err == nil {
		t.Error("expected put error")
	}
}

func TestUpdateInvalid(t *testing.T) {
	s := tempStore(t)
	if err := s.Update(nil); err == nil {
		t.Error("expected nil error")
	}
	if err := s.Update(&Todo{Title: "A"}); err == nil {
		t.Error("expected missing id")
	}
	if err := s.Update(&Todo{ID: "st-aaaa", Status: "nope"}); err == nil {
		t.Error("expected invalid status")
	}
	if err := s.Update(&Todo{ID: "st-aaaa", Title: "A"}); err == nil {
		t.Error("expected not found")
	}
}

func TestUpdateBucketPutError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	ts, _ := s.List()
	id := ts[0].ID
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("boom") }
	defer func() { bucketPutFn = orig }()
	if err := s.Update(&Todo{ID: id, Title: "B"}); err == nil {
		t.Error("expected put error")
	}
}

func TestUpdateProjectAndTags(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Project: "p1", Tags: []string{"a", "b"}})
	ts, _ := s.List()
	id := ts[0].ID
	td, _ := s.Get(id)
	td.Project = "p2"
	td.Tags = []string{"a"}
	if err := s.Update(td); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteSoftMarshalError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	ts, _ := s.List()
	id := ts[0].ID
	orig := jsonMarshalStore
	jsonMarshalStore = func(v interface{}) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { jsonMarshalStore = orig }()
	if err := s.Delete(id, false); err == nil {
		t.Error("expected marshal error")
	}
}

func TestDeleteHardWithTags(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Tags: []string{"x"}})
	ts, _ := s.List()
	id := ts[0].ID
	if err := s.Delete(id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(id); err == nil {
		t.Error("expected not found")
	}
}

func TestNormalizeTags(t *testing.T) {
	tags := normalizeTags([]string{"a", "  a  ", "", "b", "a"})
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("unexpected tags: %v", tags)
	}
}

func TestAddDepInvalid(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	if err := s.AddDep(Dependency{From: "", To: "st-aaaa", Type: DepBlocks}); err == nil {
		t.Error("expected empty from")
	}
	if err := s.AddDep(Dependency{From: "st-aaaa", To: "st-aaaa", Type: DepBlocks}); err == nil {
		t.Error("expected self-dependency")
	}
	if err := s.AddDep(Dependency{From: "st-aaaa", To: "st-bbbb", Type: DepType("nope")}); err == nil {
		t.Error("expected invalid type")
	}
	ts, _ := s.List()
	a, b := ts[0].ID, ts[1].ID
	if err := s.AddDep(Dependency{From: "st-missing", To: b, Type: DepBlocks}); err == nil {
		t.Error("expected from not found")
	}
	if err := s.AddDep(Dependency{From: a, To: "st-missing", Type: DepBlocks}); err == nil {
		t.Error("expected to not found")
	}
}

func TestAddDepCycle(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	ts, _ := s.List()
	a, b := ts[0].ID, ts[1].ID
	if err := s.AddDep(Dependency{From: a, To: b, Type: DepBlocks}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDep(Dependency{From: b, To: a, Type: DepBlocks}); err == nil {
		t.Error("expected cycle")
	}
}

func TestAddDepBucketPutError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	ts, _ := s.List()
	a, b := ts[0].ID, ts[1].ID
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("boom") }
	defer func() { bucketPutFn = orig }()
	if err := s.AddDep(Dependency{From: a, To: b, Type: DepBlocks}); err == nil {
		t.Error("expected put error")
	}
}

func TestRemoveDepBucketDeleteError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	ts, _ := s.List()
	a, b := ts[0].ID, ts[1].ID
	_ = s.AddDep(Dependency{From: a, To: b, Type: DepBlocks})
	orig := bucketDeleteFn
	bucketDeleteFn = func(b *bolt.Bucket, k []byte) error { return errors.New("boom") }
	defer func() { bucketDeleteFn = orig }()
	if err := s.RemoveDep(a, b); err == nil {
		t.Error("expected delete error")
	}
}

func TestGetDepsInvalidKey(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeps))
		return b.Put([]byte("b\x00x"), []byte("1"))
	})
	deps, err := s.GetDeps("b")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0, got %d", len(deps))
	}
}

func TestWouldCreateCycleVisited(t *testing.T) {
	s := tempStore(t)
	// Manually create a cycle A->B->C->A.
	_ = s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeps))
		for _, k := range [][]byte{
			[]byte("a\x00b\x00blocks"),
			[]byte("b\x00c\x00blocks"),
			[]byte("c\x00a\x00blocks"),
		} {
			if err := b.Put(k, []byte("1")); err != nil {
				return err
			}
		}
		return nil
	})
	cycle, err := s.wouldCreateCycle("z", "a")
	if err != nil {
		t.Fatal(err)
	}
	if cycle {
		t.Error("expected no cycle to z")
	}
}

func TestWouldCreateCycleNonBlocks(t *testing.T) {
	s := tempStore(t)
	_ = s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketDeps))
		return b.Put([]byte("a\x00b\x00related"), []byte("1"))
	})
	cycle, err := s.wouldCreateCycle("b", "a")
	if err != nil {
		t.Fatal(err)
	}
	if cycle {
		t.Error("expected no cycle through non-blocks")
	}
}

func TestDependencyTreeWalkError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	ts, _ := s.List()
	a, b := ts[0].ID, ts[1].ID
	_ = s.AddDep(Dependency{From: a, To: b, Type: DepBlocks})
	orig := getDepsFn
	getDepsFn = func(st *Store, id string) ([]Dependency, error) {
		if id == b {
			return nil, errors.New("boom")
		}
		return orig(st, id)
	}
	defer func() { getDepsFn = orig }()
	if _, err := s.DependencyTree(a, 5); err == nil {
		t.Error("expected walk error")
	}
}

func TestListFilteredTagNotFound(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Tags: []string{"x"}})
	out, err := s.ListFiltered(ListFilter{Tag: "y"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Errorf("expected 0, got %d", len(out))
	}
}

func TestReadyAndBlockedMissingBlocker(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Status: StatusOpen})
	_ = s.Add(&Todo{Title: "B", Status: StatusOpen})
	ts, _ := s.List()
	a, b := ts[0].ID, ts[1].ID
	_ = s.AddDep(Dependency{From: a, To: b, Type: DepBlocks})
	_ = s.Delete(b, true)
	ready, err := s.Ready()
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 {
		t.Errorf("expected 1 ready, got %d", len(ready))
	}
	blocked, err := s.Blocked()
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 0 {
		t.Errorf("expected 0 blocked, got %d", len(blocked))
	}
}

func TestComputeStatsReadyError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	orig := getDepsFn
	getDepsFn = func(st *Store, id string) ([]Dependency, error) { return nil, errors.New("boom") }
	defer func() { getDepsFn = orig }()
	if _, err := s.ComputeStats(); err == nil {
		t.Error("expected stats error")
	}
}

func TestHookCmdErrors(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(hookAddCmd, []string{"invalid"}, map[string]string{"command": "echo"}, "text"); err == nil {
		t.Error("expected invalid event")
	}
	if err := runCmd(hookAddCmd, []string{"post_complete"}, nil, "text"); err == nil {
		t.Error("expected empty command")
	}
	if err := runCmd(hookRemoveCmd, []string{"invalid"}, nil, "text"); err == nil {
		t.Error("expected invalid event remove")
	}
	if err := runCmd(hookRemoveCmd, []string{"post_complete"}, map[string]string{"index": "0"}, "text"); err == nil {
		t.Error("expected remove hook not found")
	}
	if err := runCmd(hookTestCmd, []string{"invalid"}, nil, "text"); err == nil {
		t.Error("expected invalid event test")
	}
}

func TestHookCmdAddRemoveList(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(hookAddCmd, []string{"post_complete"}, map[string]string{"command": "echo done", "timeout": "1s", "on-error": "warn"}, "text"); err != nil {
		t.Fatal(err)
	}
	if err := runCmd(hookListCmd, []string{}, nil, "text"); err != nil {
		t.Fatal(err)
	}
	if err := runCmd(hookRemoveCmd, []string{"post_complete"}, map[string]string{"index": "0"}, "text"); err != nil {
		t.Fatal(err)
	}
	if err := runCmd(hookTestCmd, []string{"post_complete"}, nil, "text"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadHooksConfigEmptyPath(t *testing.T) {
	orig := osUserConfigDirHooks
	osUserConfigDirHooks = func() (string, error) { return "", errors.New("boom") }
	defer func() { osUserConfigDirHooks = orig }()
	cfg, err := LoadHooksConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path() != "" {
		t.Errorf("expected empty path, got %q", cfg.Path())
	}
}

func TestLoadHooksConfigNilHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.toml")
	_ = os.WriteFile(path, []byte(""), 0644)
	orig := tomlDecodeFileHooks
	tomlDecodeFileHooks = func(p string, v interface{}) (toml.MetaData, error) {
		md, err := orig(p, v)
		if c, ok := v.(*HookConfig); ok {
			c.Hooks = nil
		}
		return md, err
	}
	defer func() { tomlDecodeFileHooks = orig }()
	cfg, err := LoadHooksConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hooks == nil {
		t.Fatal("expected non-nil hooks")
	}
	if len(cfg.Hooks) != 0 {
		t.Errorf("expected empty hooks, got %d", len(cfg.Hooks))
	}
}

func TestHookConfigSaveEmptyPath(t *testing.T) {
	cfg := &HookConfig{Hooks: map[HookEvent][]Hook{}}
	if err := cfg.Add(EventPostComplete, Hook{Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Remove(EventPostComplete, 0); err != nil {
		t.Fatal(err)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func TestCurrentProjectError(t *testing.T) {
	orig := osGetwdTodo
	osGetwdTodo = func() (string, error) { return "", errors.New("boom") }
	defer func() { osGetwdTodo = orig }()
	old := todoProject
	todoProject = ""
	defer func() { todoProject = old }()
	if got := currentProject(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestGetHookConfigWarning(t *testing.T) {
	orig := tomlDecodeFileHooks
	tomlDecodeFileHooks = func(path string, v interface{}) (toml.MetaData, error) { return toml.MetaData{}, errors.New("boom") }
	defer func() { tomlDecodeFileHooks = orig }()
	hookConfigOnce = sync.Once{}
	hookConfig = nil
	out := captureStderr(t, func() { _ = getHookConfig() })
	if !strings.Contains(out, "warning") {
		t.Errorf("expected warning, got %q", out)
	}
}

func TestFireHooks(t *testing.T) {
	s := tempStore(t)
	// nil config branch
	hookConfigOnce = sync.Once{}
	hookConfig = nil
	fireHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")

	// warning branch
	hookConfigOnce = sync.Once{}
	hookConfig = &HookConfig{Hooks: map[HookEvent][]Hook{EventPostAdd: {{Command: "false", OnError: "warn"}}}}
	out := captureStderr(t, func() {
		fireHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")
	})
	if !strings.Contains(out, "hook warning") {
		t.Errorf("expected warning, got %q", out)
	}

	// fail branch
	hookConfigOnce = sync.Once{}
	hookConfig = &HookConfig{Hooks: map[HookEvent][]Hook{EventPostAdd: {{Command: "false", OnError: "fail"}}}}
	out = captureStderr(t, func() {
		fireHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")
	})
	if !strings.Contains(out, "hook failed") {
		t.Errorf("expected fail message, got %q", out)
	}

	// ignore branch
	hookConfigOnce = sync.Once{}
	hookConfig = &HookConfig{Hooks: map[HookEvent][]Hook{EventPostAdd: {{Command: "false", OnError: "ignore"}}}}
	out = captureStderr(t, func() {
		fireHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")
	})
	if out != "" {
		t.Errorf("expected silence, got %q", out)
	}
}

func TestFirePluginHooksNilRegistry(t *testing.T) {
	s := tempStore(t)
	orig := pluginRegistryFn
	pluginRegistryFn = func() *plugins.Registry { return nil }
	defer func() { pluginRegistryFn = orig }()
	firePluginHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")
}

func pluginRegistryWithHook(t *testing.T, event, command string) *plugins.Registry {
	dir := t.TempDir()
	sub := filepath.Join(dir, "p1")
	_ = os.MkdirAll(sub, 0755)
	body := fmt.Sprintf(`
name = "p1"
version = "1.0.0"
[[hooks]]
event = %q
command = %q
`, event, command)
	_ = os.WriteFile(filepath.Join(sub, "plugin.toml"), []byte(body), 0644)
	reg := plugins.NewRegistry()
	_ = reg.LoadFromDir(dir)
	return reg
}

func TestFirePluginHooksWithHook(t *testing.T) {
	s := tempStore(t)
	orig := pluginRegistryFn
	reg := pluginRegistryWithHook(t, "post_add", "echo hello")
	pluginRegistryFn = func() *plugins.Registry { return reg }
	defer func() { pluginRegistryFn = orig }()
	firePluginHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")
	entries, _ := s.ListAudit("st-1")
	if len(entries) == 0 {
		t.Error("expected audit entry")
	}
}

func TestFirePluginHooksStderr(t *testing.T) {
	s := tempStore(t)
	origReg := pluginRegistryFn
	reg := pluginRegistryWithHook(t, "post_add", "echo err >&2")
	pluginRegistryFn = func() *plugins.Registry { return reg }
	defer func() { pluginRegistryFn = origReg }()
	firePluginHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")
}

func TestFirePluginHooksError(t *testing.T) {
	s := tempStore(t)
	origReg := pluginRegistryFn
	reg := pluginRegistryWithHook(t, "post_add", "exit 1")
	pluginRegistryFn = func() *plugins.Registry { return reg }
	defer func() { pluginRegistryFn = origReg }()
	firePluginHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")
}

func setOpenStoreError(t *testing.T) func() {
	t.Helper()
	orig := openStoreFn
	openStoreFn = func() (*Store, error) { return nil, errors.New("openStore error") }
	return func() { openStoreFn = orig }
}

func seedStore(t *testing.T, todos ...*Todo) {
	t.Helper()
	s, err := Open(todoDBPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	for _, td := range todos {
		if err := s.Add(td); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
}

func TestCommandAdd(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(addCmd, []string{}, map[string]string{"title": "A"}, "text"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := runCmd(addCmd, []string{}, map[string]string{"title": "B", "priority": "P0", "type": "feature", "tags": "x,y", "assignee": "alice", "project": "p1"}, "json"); err != nil {
		t.Fatalf("add json: %v", err)
	}
	if err := runCmd(addCmd, []string{}, map[string]string{"title": "C", "priority": "bad"}, "text"); err == nil {
		t.Error("expected invalid priority error")
	}
	if err := runCmd(addCmd, []string{}, map[string]string{"title": "D", "type": "bad"}, "text"); err == nil {
		t.Error("expected invalid type error")
	}
	if err := runCmd(addCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected missing title error")
	}
	stop := setOpenStoreError(t)
	if err := runCmd(addCmd, []string{}, map[string]string{"title": "E"}, "text"); err == nil {
		t.Error("expected openStore error")
	}
	stop()
}

func TestCommandList(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Priority: PriorityP0, Tags: []string{"x"}, Assignee: "alice", Project: "p1"})
	if err := runCmd(listCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := runCmd(listCmd, []string{}, map[string]string{"all": "true"}, "json"); err != nil {
		t.Fatalf("list all json: %v", err)
	}
	if err := runCmd(listCmd, []string{}, map[string]string{"status": "open", "priority": "P0", "tag": "x", "assignee": "alice", "project": "p1", "search": "A"}, "text"); err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if err := runCmd(listCmd, []string{}, map[string]string{"tag": "notag"}, "text"); err != nil {
		t.Fatalf("list empty: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(listCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected openStore error")
	}
	stop()
}

func TestCommandShow(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Description: "desc", Status: StatusInProgress, Assignee: "alice", Parent: "st-p", ExternalRef: "ref", Project: "p1", Tags: []string{"x"}, Notes: "notes"})
	if err := runCmd(showCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("show text: %v", err)
	}
	if err := runCmd(showCmd, []string{"st-a"}, nil, "json"); err != nil {
		t.Fatalf("show json: %v", err)
	}
	if err := runCmd(showCmd, []string{"st-missing"}, nil, "text"); err == nil {
		t.Error("expected not found")
	}
	stop := setOpenStoreError(t)
	if err := runCmd(showCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected openStore error")
	}
	stop()
}

func TestCommandUpdate(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Status: StatusOpen})
	if err := runCmd(updateCmd, []string{"st-a"}, map[string]string{"title": "B", "desc": "d", "priority": "P1", "type": "bug", "status": "in_progress", "tags": "t", "assignee": "bob", "external-ref": "ref", "parent": "st-p", "notes": "n"}, "text"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := runCmd(updateCmd, []string{"st-a"}, map[string]string{"status": "bad"}, "text"); err == nil {
		t.Error("expected invalid status error")
	}
	if err := runCmd(updateCmd, []string{"st-a"}, map[string]string{"priority": "bad"}, "text"); err == nil {
		t.Error("expected invalid priority error")
	}
	if err := runCmd(updateCmd, []string{"st-a"}, map[string]string{"type": "bad"}, "text"); err == nil {
		t.Error("expected invalid type error")
	}
	if err := runCmd(updateCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected no changes error")
	}
	if err := runCmd(updateCmd, []string{"st-missing"}, map[string]string{"title": "X"}, "text"); err == nil {
		t.Error("expected not found")
	}
	if err := runCmd(updateCmd, []string{"st-a"}, map[string]string{"title": "C"}, "json"); err != nil {
		t.Fatalf("update json: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(updateCmd, []string{"st-a"}, map[string]string{"title": "D"}, "text"); err == nil {
		t.Error("expected openStore error")
	}
	stop()
}

func TestCommandClaimUnclaim(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	if err := runCmd(claimCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := runCmd(unclaimCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("unclaim: %v", err)
	}
	seedStore(t, &Todo{ID: "st-b", Title: "B", Assignee: "other"})
	if err := runCmd(claimCmd, []string{"st-b"}, nil, "text"); err == nil {
		t.Error("expected already claimed error")
	}
	if err := runCmd(claimCmd, []string{"st-missing"}, nil, "text"); err == nil {
		t.Error("expected claim not found error")
	}
	if err := runCmd(unclaimCmd, []string{"st-missing"}, nil, "text"); err == nil {
		t.Error("expected unclaim not found error")
	}
	stop := setOpenStoreError(t)
	if err := runCmd(claimCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected openStore error")
	}
	stop()
}

func TestCommandCompleteCancel(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	if err := runCmd(completeCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	seedStore(t, &Todo{ID: "st-b", Title: "B"})
	if err := runCmd(cancelCmd, []string{"st-b"}, nil, "text"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := runCmd(completeCmd, []string{"st-missing"}, nil, "text"); err == nil {
		t.Error("expected complete not found")
	}
	stop := setOpenStoreError(t)
	if err := runCmd(cancelCmd, []string{"st-b"}, nil, "text"); err == nil {
		t.Error("expected openStore error")
	}
	stop()
}

func TestCommandDelete(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	if err := runCmd(deleteCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("delete soft: %v", err)
	}
	seedStore(t, &Todo{ID: "st-b", Title: "B"})
	if err := runCmd(deleteCmd, []string{"st-b"}, map[string]string{"soft": "false"}, "text"); err != nil {
		t.Fatalf("delete hard: %v", err)
	}
	if err := runCmd(deleteCmd, []string{"st-missing"}, nil, "text"); err == nil {
		t.Error("expected delete not found")
	}
	stop := setOpenStoreError(t)
	if err := runCmd(deleteCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected openStore error")
	}
	stop()
}

func withStore(t *testing.T, fn func(*Store)) {
	t.Helper()
	s, err := Open(todoDBPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	fn(s)
}

func TestCommandDep(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"}, &Todo{ID: "st-b", Title: "B"}, &Todo{ID: "st-c", Title: "C"})
	if err := runCmd(depAddCmd, []string{"st-b", "st-a"}, nil, "text"); err != nil {
		t.Fatalf("dep add: %v", err)
	}
	if err := runCmd(depAddCmd, []string{"st-b", "st-a"}, map[string]string{"type": "related"}, "text"); err != nil {
		t.Fatalf("dep add related: %v", err)
	}
	if err := runCmd(depAddCmd, []string{"st-b", "st-a"}, map[string]string{"type": "bad"}, "text"); err == nil {
		t.Error("expected invalid dep type error")
	}
	if err := runCmd(depAddCmd, []string{"st-missing", "st-a"}, nil, "text"); err == nil {
		t.Error("expected dep add not found")
	}
	if err := runCmd(depRemoveCmd, []string{"st-b", "st-a"}, nil, "text"); err != nil {
		t.Fatalf("dep remove: %v", err)
	}
	if err := runCmd(depAddCmd, []string{"st-b", "st-a"}, nil, "text"); err != nil {
		t.Fatalf("dep add re: %v", err)
	}
	if err := runCmd(depsCmd, []string{"st-b"}, nil, "text"); err != nil {
		t.Fatalf("deps text: %v", err)
	}
	if err := runCmd(depsCmd, []string{"st-b"}, nil, "json"); err != nil {
		t.Fatalf("deps json: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(depAddCmd, []string{"st-b", "st-a"}, nil, "text"); err == nil {
		t.Error("expected dep add openStore error")
	}
	if err := runCmd(depRemoveCmd, []string{"st-b", "st-a"}, nil, "text"); err == nil {
		t.Error("expected dep remove openStore error")
	}
	if err := runCmd(depsCmd, []string{"st-b"}, nil, "text"); err == nil {
		t.Error("expected deps openStore error")
	}
	stop()
}

func TestCommandReadyBlockedSearch(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"}, &Todo{ID: "st-b", Title: "B"})
	withStore(t, func(s *Store) {
		_ = s.AddDep(Dependency{From: "st-b", To: "st-a", Type: DepBlocks})
	})
	if err := runCmd(readyCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("ready text: %v", err)
	}
	if err := runCmd(readyCmd, []string{}, nil, "json"); err != nil {
		t.Fatalf("ready json: %v", err)
	}
	if err := runCmd(blockedCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("blocked text: %v", err)
	}
	if err := runCmd(blockedCmd, []string{}, nil, "json"); err != nil {
		t.Fatalf("blocked json: %v", err)
	}
	if err := runCmd(searchCmd, []string{"A"}, nil, "text"); err != nil {
		t.Fatalf("search: %v", err)
	}
	if err := runCmd(searchCmd, []string{"notfound"}, nil, "text"); err != nil {
		t.Fatalf("search empty: %v", err)
	}
	if err := runCmd(searchCmd, []string{""}, nil, "text"); err == nil {
		t.Error("expected empty search query error")
	}
	stop := setOpenStoreError(t)
	if err := runCmd(readyCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected ready openStore error")
	}
	stop()
}

func TestCommandGraph(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t,
		&Todo{ID: "st-a", Title: strings.Repeat("A long title to test truncate", 50), Status: StatusOpen},
		&Todo{ID: "st-b", Title: "B", Status: StatusDone},
		&Todo{ID: "st-c", Title: "C", Status: StatusInProgress},
		&Todo{ID: "st-d", Title: "D", Status: StatusCancelled},
		&Todo{ID: "st-e", Title: "E", Status: StatusBlocked},
	)
	withStore(t, func(s *Store) {
		_ = s.AddDep(Dependency{From: "st-b", To: "st-a", Type: DepBlocks})
	})
	if err := runCmd(graphCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("graph: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(graphCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected graph openStore error")
	}
	stop()
}

func TestCommandStats(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t,
		&Todo{ID: "st-a", Title: "A", Priority: PriorityP0, Type: TypeBug, Assignee: "alice"},
		&Todo{ID: "st-b", Title: "B", Priority: PriorityP1, Type: TypeFeature, Assignee: "bob"},
	)
	if err := runCmd(statsCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("stats text: %v", err)
	}
	if err := runCmd(statsCmd, []string{}, nil, "json"); err != nil {
		t.Fatalf("stats json: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(statsCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected stats openStore error")
	}
	stop()
}

func TestCommandTimeline(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	withStore(t, func(s *Store) {
		_ = s.AppendAudit(AuditEntry{TodoID: "st-a", Action: "test"})
	})
	if err := runCmd(timelineCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("timeline all text: %v", err)
	}
	if err := runCmd(timelineCmd, []string{}, nil, "json"); err != nil {
		t.Fatalf("timeline all json: %v", err)
	}
	if err := runCmd(timelineCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("timeline id: %v", err)
	}
	cleanup2 := setupCmdTest(t)
	defer cleanup2()
	if err := runCmd(timelineCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("timeline empty: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(timelineCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected timeline openStore error")
	}
	stop()
}

func TestCommandMine(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Assignee: "tester"}, &Todo{ID: "st-b", Title: "B", Assignee: "other"})
	if err := runCmd(mineCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("mine text: %v", err)
	}
	if err := runCmd(mineCmd, []string{}, nil, "json"); err != nil {
		t.Fatalf("mine json: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(mineCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected mine openStore error")
	}
	stop()
}

func TestCommandProject(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(projectCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("project show: %v", err)
	}
	if err := runCmd(projectCmd, []string{"newproj"}, nil, "text"); err != nil {
		t.Fatalf("project switch: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(projectCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected project openStore error")
	}
	stop()
}

func TestCommandRemember(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(rememberCmd, []string{"insight"}, nil, "text"); err != nil {
		t.Fatalf("remember: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(rememberCmd, []string{"insight"}, nil, "text"); err == nil {
		t.Error("expected remember openStore error")
	}
	stop()
}

func TestCommandPrime(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Priority: PriorityP0}, &Todo{ID: "st-b", Title: "B"}, &Todo{ID: "st-c", Title: "C", Assignee: "tester"})
	withStore(t, func(s *Store) {
		_ = s.AddDep(Dependency{From: "st-b", To: "st-a", Type: DepBlocks})
	})
	if err := runCmd(primeCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("prime: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(primeCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected prime openStore error")
	}
	stop()
}

func makeOldDone(t *testing.T, s *Store, id string) {
	t.Helper()
	td, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	old := time.Now().Add(-100 * time.Hour)
	td.UpdatedAt = old
	td.ClosedAt = &old
	if err := s.Update(td); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func TestCommandCompact(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Status: StatusDone})
	withStore(t, func(s *Store) { makeOldDone(t, s, "st-a") })
	if err := runCmd(compactCmd, []string{}, map[string]string{"older-than": "24h"}, "text"); err != nil {
		t.Fatalf("compact: %v", err)
	}
	seedStore(t, &Todo{ID: "st-b", Title: "B", Status: StatusDone})
	withStore(t, func(s *Store) { makeOldDone(t, s, "st-b") })
	if err := runCmd(compactCmd, []string{}, map[string]string{"older-than": "24h", "dry-run": "true"}, "text"); err != nil {
		t.Fatalf("compact dry-run: %v", err)
	}
	if err := runCmd(compactCmd, []string{}, map[string]string{"older-than": "30d"}, "text"); err == nil {
		t.Error("expected invalid duration error")
	}
	seedStore(t, &Todo{ID: "st-c", Title: "C", Status: StatusDone})
	if err := runCmd(compactCmd, []string{}, map[string]string{"older-than": "0"}, "text"); err != nil {
		t.Fatalf("compact older-than 0: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(compactCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected compact openStore error")
	}
	stop()
}

func TestCommandInit(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(initCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("init: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(initCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected init openStore error")
	}
	stop()
}

func TestCommandDoctor(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	if err := runCmd(doctorCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("doctor text: %v", err)
	}
	if err := runCmd(doctorCmd, []string{}, nil, "json"); err != nil {
		t.Fatalf("doctor json: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(doctorCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected doctor openStore error")
	}
	stop()
}

func TestCommandExport(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Priority: PriorityP0, Type: TypeBug, Status: StatusDone, Assignee: "alice", Tags: []string{"x"}, Description: "desc"})
	dir := t.TempDir()
	out := filepath.Join(dir, "out.json")
	if err := runCmd(exportCmd, []string{}, nil, "json"); err != nil {
		t.Fatalf("export json stdout: %v", err)
	}
	if err := runCmd(exportCmd, []string{}, nil, "jsonl"); err != nil {
		t.Fatalf("export jsonl: %v", err)
	}
	if err := runCmd(exportCmd, []string{}, nil, "markdown"); err != nil {
		t.Fatalf("export markdown: %v", err)
	}
	if err := runCmd(exportCmd, []string{}, map[string]string{"output": out}, "json"); err != nil {
		t.Fatalf("export file: %v", err)
	}
	if err := runCmd(exportCmd, []string{}, nil, "bad"); err == nil {
		t.Error("expected unknown format error")
	}
	stop := setOpenStoreError(t)
	if err := runCmd(exportCmd, []string{}, nil, "json"); err == nil {
		t.Error("expected export openStore error")
	}
	stop()
	orig := osWriteFileTodo
	osWriteFileTodo = func(name string, data []byte, perm os.FileMode) error { return errors.New("write error") }
	defer func() { osWriteFileTodo = orig }()
	if err := runCmd(exportCmd, []string{}, map[string]string{"output": out}, "json"); err == nil {
		t.Error("expected write file error")
	}
}

func TestCommandImport(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "t.json")
	jsonlPath := filepath.Join(dir, "t.jsonl")
	badPath := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(jsonPath, []byte(`[{"title":"I","priority":"P2","type":"task"}]`), 0644)
	_ = os.WriteFile(jsonlPath, []byte("{\"title\":\"J\",\"priority\":\"P1\",\"type\":\"bug\"}\n"), 0644)
	_ = os.WriteFile(badPath, []byte("not json"), 0644)
	if err := runCmd(importCmd, []string{jsonPath}, nil, "json"); err != nil {
		t.Fatalf("import json: %v", err)
	}
	if err := runCmd(importCmd, []string{jsonlPath}, nil, "jsonl"); err != nil {
		t.Fatalf("import jsonl: %v", err)
	}
	if err := runCmd(importCmd, []string{badPath}, nil, "json"); err == nil {
		t.Error("expected unmarshal error")
	}
	if err := runCmd(importCmd, []string{"nonexistent.json"}, nil, "json"); err == nil {
		t.Error("expected read error")
	}
	if err := runCmd(importCmd, []string{jsonPath}, nil, "bad"); err == nil {
		t.Error("expected unknown format error")
	}
	if err := runCmd(importCmd, []string{jsonPath}, nil, "json"); err != nil {
		t.Fatalf("import json output: %v", err)
	}
	stop := setOpenStoreError(t)
	if err := runCmd(importCmd, []string{jsonPath}, nil, "json"); err == nil {
		t.Error("expected import openStore error")
	}
	stop()
	orig := jsonMarshalStore
	jsonMarshalStore = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal error") }
	defer func() { jsonMarshalStore = orig }()
	if err := runCmd(importCmd, []string{jsonPath}, nil, "json"); err == nil {
		t.Error("expected import add error")
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write error") }

func TestPrintJSONDirect(t *testing.T) {
	orig := osStdoutTodo
	osStdoutTodo = &bytes.Buffer{}
	defer func() { osStdoutTodo = orig }()
	if err := printJSON(map[string]string{"k": "v"}); err != nil {
		t.Fatalf("printJSON: %v", err)
	}
	osStdoutTodo = errWriter{}
	if err := printJSON(map[string]string{"k": "v"}); err == nil {
		t.Error("expected printJSON error")
	}
}

func TestCommandJSONOutputError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := printJSONFn
	printJSONFn = func(v interface{}) error { return errors.New("printJSON error") }
	defer func() { printJSONFn = orig }()
	if err := runCmd(listCmd, []string{}, map[string]string{"all": "true"}, "json"); err == nil {
		t.Error("expected list json error")
	}
}

func setJSONMarshalStoreError(t *testing.T) func() {
	t.Helper()
	orig := jsonMarshalStore
	jsonMarshalStore = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal error") }
	return func() { jsonMarshalStore = orig }
}

func setJSONUnmarshalStoreError(t *testing.T) func() {
	t.Helper()
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func(data []byte, v interface{}) error { return errors.New("unmarshal error") }
	return func() { jsonUnmarshalStore = orig }
}

func setJSONMarshalAuditError(t *testing.T) func() {
	t.Helper()
	orig := jsonMarshalAudit
	jsonMarshalAudit = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal error") }
	return func() { jsonMarshalAudit = orig }
}

func setJSONMarshalMemoryError(t *testing.T) func() {
	t.Helper()
	orig := jsonMarshalMemory
	jsonMarshalMemory = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal error") }
	return func() { jsonMarshalMemory = orig }
}

func injectBadTodo(t *testing.T, s *Store, id string) {
	t.Helper()
	if err := s.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketTodos)).Put([]byte(id), []byte("not json"))
	}); err != nil {
		t.Fatalf("injectBadTodo: %v", err)
	}
}

func injectBadAudit(t *testing.T, s *Store) {
	t.Helper()
	if err := s.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketAudit)).Put([]byte("x\x00x"), []byte("not json"))
	}); err != nil {
		t.Fatalf("injectBadAudit: %v", err)
	}
}

func injectBadMemory(t *testing.T, s *Store) {
	t.Helper()
	if err := s.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketMems)).Put([]byte("x\x00x"), []byte("not json"))
	}); err != nil {
		t.Fatalf("injectBadMemory: %v", err)
	}
}

func injectBadDepKey(t *testing.T, s *Store) {
	t.Helper()
	if err := s.update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketDeps)).Put([]byte("a\x00b"), []byte("1"))
	}); err != nil {
		t.Fatalf("injectBadDepKey: %v", err)
	}
}

func TestStoreBadJSON(t *testing.T) {
	s := tempStore(t)
	injectBadTodo(t, s, "st-bad")
	stop := setJSONUnmarshalStoreError(t)
	if _, err := s.Get("st-bad"); err == nil {
		t.Error("expected Get unmarshal error")
	}
	if _, err := s.List(); err == nil {
		t.Error("expected List unmarshal error")
	}
	if err := s.Update(&Todo{ID: "st-bad", Title: "X", Status: StatusOpen}); err == nil {
		t.Error("expected Update unmarshal error")
	}
	if err := s.Delete("st-bad", true); err == nil {
		t.Error("expected Delete hard unmarshal error")
	}
	stop()

	s2 := tempStore(t)
	_ = s2.Add(&Todo{ID: "st-x", Title: "X"})
	stop2 := setJSONMarshalStoreError(t)
	if err := s2.Delete("st-x", false); err == nil {
		t.Error("expected Delete soft marshal error")
	}
	stop2()

	s3 := tempStore(t)
	_ = s3.Add(&Todo{ID: "st-y", Title: "Y"})
	stop3 := setJSONMarshalStoreError(t)
	if err := s3.Update(&Todo{ID: "st-y", Title: "Z", Status: StatusOpen}); err == nil {
		t.Error("expected Update marshal error")
	}
	stop3()
}

func TestAuditBadJSON(t *testing.T) {
	s := tempStore(t)
	injectBadAudit(t, s)
	entries, err := s.ListAudit("")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestMemoryBadJSON(t *testing.T) {
	s := tempStore(t)
	injectBadMemory(t, s)
	if _, err := s.ListMemories(); err == nil {
		t.Error("expected ListMemories unmarshal error")
	}
}

func TestAppendAuditMarshalError(t *testing.T) {
	s := tempStore(t)
	stop := setJSONMarshalAuditError(t)
	if err := s.AppendAudit(AuditEntry{TodoID: "x", Action: "a"}); err == nil {
		t.Error("expected AppendAudit marshal error")
	}
	stop()
}

func TestAddMemoryMarshalError(t *testing.T) {
	s := tempStore(t)
	stop := setJSONMarshalMemoryError(t)
	if err := s.AddMemory(&Memory{Insight: "x"}); err == nil {
		t.Error("expected AddMemory marshal error")
	}
	stop()
}

func TestStoreNilReceivers(t *testing.T) {
	var s *Store
	if err := s.Close(); err != nil {
		t.Errorf("Close nil: %v", err)
	}
	if err := s.update(func(tx *bolt.Tx) error { return nil }); err == nil {
		t.Error("expected update nil error")
	}
	if err := s.view(func(tx *bolt.Tx) error { return nil }); err == nil {
		t.Error("expected view nil error")
	}
}

func TestStoreDB(t *testing.T) {
	s := tempStore(t)
	if s.DB() == nil {
		t.Error("expected non-nil DB")
	}
}

func TestStoreOpenConfigDirError(t *testing.T) {
	orig := osUserConfigDirStore
	osUserConfigDirStore = func() (string, error) { return "", errors.New("config dir error") }
	defer func() { osUserConfigDirStore = orig }()
	if _, err := Open(""); err == nil {
		t.Error("expected config dir error")
	}
}

func TestStoreOpenBboltError(t *testing.T) {
	orig := bboltOpenStore
	bboltOpenStore = func(path string, mode os.FileMode, options *bolt.Options) (*bolt.DB, error) {
		return nil, errors.New("bbolt error")
	}
	defer func() { bboltOpenStore = orig }()
	if _, err := Open(filepath.Join(t.TempDir(), "todo.db")); err == nil {
		t.Error("expected bbolt error")
	}
}

func TestStoreIndexHelpers(t *testing.T) {
	s := tempStore(t)
	ids, err := s.IndexKeys("nonexistent", "key")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 ids for nonexistent bucket, got %d", len(ids))
	}
	if err := s.update(func(tx *bolt.Tx) error {
		writeIndex(tx, bucketIdxSt, "", "id")
		writeIndex(tx, "nonexistent", "key", "id")
		removeIndex(tx, bucketIdxSt, "", "id")
		removeIndex(tx, "nonexistent", "key", "id")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentActorAllBranches(t *testing.T) {
	old := todoAs
	todoAs = ""
	defer func() { todoAs = old }()
	origGit := gitUserNameFn
	gitUserNameFn = func() ([]byte, error) { return []byte("gituser\n"), nil }
	if got := currentActor(); got != "gituser" {
		t.Errorf("expected gituser, got %q", got)
	}
	gitUserNameFn = func() ([]byte, error) { return nil, errors.New("no git") }
	origCfg := osUserConfigDirTodo
	osUserConfigDirTodo = func() (string, error) { return "/home/tester", nil }
	if got := currentActor(); got != "tester" {
		t.Errorf("expected tester, got %q", got)
	}
	osUserConfigDirTodo = func() (string, error) { return "", errors.New("no config") }
	if got := currentActor(); got != "unknown" {
		t.Errorf("expected unknown, got %q", got)
	}
	gitUserNameFn = origGit
	osUserConfigDirTodo = origCfg
}

func TestCurrentProjectAllBranches(t *testing.T) {
	old := todoProject
	todoProject = "setproj"
	if got := currentProject(); got != "setproj" {
		t.Errorf("expected setproj, got %q", got)
	}
	todoProject = ""
	origGetwd := osGetwdTodo
	osGetwdTodo = func() (string, error) { return "/foo/bar", nil }
	if got := currentProject(); got != "bar" {
		t.Errorf("expected bar, got %q", got)
	}
	osGetwdTodo = func() (string, error) { return "", errors.New("no cwd") }
	if got := currentProject(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	defer func() { todoProject = old; osGetwdTodo = origGetwd }()
}

func TestStatusIconAll(t *testing.T) {
	cases := map[Status]string{
		StatusOpen:       "○",
		StatusInProgress: "●",
		StatusDone:       "✓",
		StatusCancelled:  "✗",
		StatusBlocked:    "✗",
	}
	for st, want := range cases {
		if got := statusIcon(st); got != want {
			t.Errorf("statusIcon(%q) = %q, want %q", st, got, want)
		}
	}
}

func TestPrintTodoTableCompacted(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Compacted: true, Summary: "summary"})
	if err := runCmd(listCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("list compacted: %v", err)
	}
}

func TestNotifyDirect(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_ = captureStderr(t, func() {
		notify(notifications.TypeTodoCreated, "st-1", "title", "msg", "actor")
	})
}

func TestOpenStoreDirect(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	s, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	s.Close()
}

func TestEncodeBase36Zero(t *testing.T) {
	if got := encodeBase36(0, 4); got != "0" {
		t.Errorf("expected 0, got %q", got)
	}
}

func TestGenerateIDFallback(t *testing.T) {
	resetIDState()
	orig := idSha1Sum
	idSha1Sum = func(b []byte) [20]byte { return [20]byte{1} }
	defer func() { idSha1Sum = orig }()
	id := GenerateID()
	if !IsValidID(id) {
		t.Errorf("invalid id: %q", id)
	}
}

func TestDepJSON(t *testing.T) {
	_, err := depJSON(Dependency{From: "a", To: "b", Type: DepBlocks})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuditPrefix(t *testing.T) {
	if len(auditPrefix()) != 0 {
		t.Errorf("expected empty prefix, got %v", auditPrefix())
	}
}

func TestFireHooksSuccess(t *testing.T) {
	s := tempStore(t)
	hookConfigOnce = sync.Once{}
	hookConfig = &HookConfig{Hooks: map[HookEvent][]Hook{EventPostAdd: {{Command: "true"}}}}
	fireHooks(s, EventPostAdd, &Todo{ID: "st-1", Title: "A"}, "", "", "")
}

func TestBuildEnvAllFields(t *testing.T) {
	env := buildEnv(HookContext{
		Event: EventPostAdd,
		Actor: "actor",
		Todo:  &Todo{ID: "st-1", Title: "T", Status: StatusOpen, Priority: PriorityP2, Type: TypeTask, Assignee: "a", Tags: []string{"x"}, Project: "p"},
		From:  "open",
		To:    "done",
		Note:  "note",
	})
	if len(env) == 0 {
		t.Error("expected env vars")
	}
}

func TestRunHookTimeoutCoverage(t *testing.T) {
	r := runHook(Hook{Command: "sleep 1", Timeout: 1}, HookContext{Event: EventPostAdd})
	if r.Err == nil {
		t.Error("expected timeout error")
	}
}

func TestHookValidateCoverage(t *testing.T) {
	if err := (Hook{}).Validate(); err == nil {
		t.Error("expected empty command error")
	}
	if err := (Hook{Command: "x", Timeout: -1}).Validate(); err == nil {
		t.Error("expected negative timeout error")
	}
	if err := (Hook{Command: "x", OnError: "bad"}).Validate(); err == nil {
		t.Error("expected invalid on_error error")
	}
}

func TestLoadHooksConfigInvalidCoverage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.toml")
	_ = os.WriteFile(path, []byte("not valid toml"), 0644)
	if _, err := LoadHooksConfig(path); err == nil {
		t.Error("expected decode error")
	}
	path2 := filepath.Join(t.TempDir(), "hooks2.toml")
	_ = os.WriteFile(path2, []byte("[hooks]\npost_complete = [{command = \"\"}]\n"), 0644)
	if _, err := LoadHooksConfig(path2); err == nil {
		t.Error("expected invalid hook error")
	}
}

func TestHookConfigSaveErrors(t *testing.T) {
	dir := t.TempDir()
	cfg := &HookConfig{Hooks: map[HookEvent][]Hook{}, path: filepath.Join(dir, "sub", "hooks.toml")}
	origMkdir := osMkdirAllHooks
	osMkdirAllHooks = func(path string, perm os.FileMode) error { return errors.New("mkdir error") }
	if err := cfg.Add(EventPostAdd, Hook{Command: "true"}); err == nil {
		t.Error("expected mkdir error")
	}
	osMkdirAllHooks = origMkdir
	origCreate := osCreateHooks
	osCreateHooks = func(name string) (*os.File, error) { return nil, errors.New("create error") }
	defer func() { osCreateHooks = origCreate }()
	if err := cfg.Add(EventPostAdd, Hook{Command: "true"}); err == nil {
		t.Error("expected create error")
	}
}

func TestHookCmdMore(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(hookPathCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("hook path: %v", err)
	}
	if err := runCmd(hookListCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("hook list empty: %v", err)
	}
	if err := runCmd(hookAddCmd, []string{"post_complete"}, map[string]string{"command": "echo hi", "timeout": "0", "on-error": ""}, "text"); err != nil {
		t.Fatalf("hook add: %v", err)
	}
	if err := runCmd(hookListCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("hook list defaults: %v", err)
	}
	if err := runCmd(hookTestCmd, []string{"post_complete"}, nil, "text"); err != nil {
		t.Fatalf("hook test existing: %v", err)
	}
	cleanup2 := setupCmdTest(t)
	defer cleanup2()
	if err := runCmd(hookTestCmd, []string{"post_complete"}, nil, "text"); err != nil {
		t.Fatalf("hook test no hooks: %v", err)
	}
}

func TestHookListLoadError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	dir := t.TempDir()
	badCfg := filepath.Join(dir, "hooks.toml")
	_ = os.WriteFile(badCfg, []byte("not valid toml"), 0644)
	if err := runCmd(hookListCmd, []string{}, map[string]string{"config": badCfg}, "text"); err == nil {
		t.Error("expected load error")
	}
}

func TestHookCmdTestOutputAndError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(hookAddCmd, []string{"post_complete"}, map[string]string{"command": "echo out && echo err >&2"}, "text"); err != nil {
		t.Fatalf("hook add: %v", err)
	}
	if err := runCmd(hookTestCmd, []string{"post_complete"}, nil, "text"); err != nil {
		t.Fatalf("hook test output: %v", err)
	}
	cleanup2 := setupCmdTest(t)
	defer cleanup2()
	if err := runCmd(hookAddCmd, []string{"post_complete"}, map[string]string{"command": "exit 7"}, "text"); err != nil {
		t.Fatalf("hook add error: %v", err)
	}
	if err := runCmd(hookTestCmd, []string{"post_complete"}, nil, "text"); err != nil {
		t.Fatalf("hook test error: %v", err)
	}
}

func TestHookCmdAddErrors(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	dir := t.TempDir()
	badCfg := filepath.Join(dir, "hooks.toml")
	_ = os.WriteFile(badCfg, []byte("not valid toml"), 0644)
	if err := runCmd(hookAddCmd, []string{"post_complete"}, map[string]string{"command": "echo", "config": badCfg}, "text"); err == nil {
		t.Error("expected load error")
	}
	hookConfigPath = ""
	if err := runCmd(hookAddCmd, []string{"post_complete"}, map[string]string{"command": ""}, "text"); err == nil {
		t.Error("expected add validation error")
	}
}

func TestHookCmdRemoveErrors(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	dir := t.TempDir()
	badCfg := filepath.Join(dir, "hooks.toml")
	_ = os.WriteFile(badCfg, []byte("not valid toml"), 0644)
	if err := runCmd(hookRemoveCmd, []string{"post_complete"}, map[string]string{"index": "0", "config": badCfg}, "text"); err == nil {
		t.Error("expected load error")
	}
	hookConfigPath = ""
	if err := runCmd(hookRemoveCmd, []string{"post_complete"}, map[string]string{"index": "0"}, "text"); err == nil {
		t.Error("expected remove not found")
	}
}

func TestHookCmdTestLoadError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	dir := t.TempDir()
	badCfg := filepath.Join(dir, "hooks.toml")
	_ = os.WriteFile(badCfg, []byte("not valid toml"), 0644)
	if err := runCmd(hookTestCmd, []string{"post_complete"}, map[string]string{"config": badCfg}, "text"); err == nil {
		t.Error("expected load error")
	}
}

// ── remaining statement coverage: store, query, deps, compact, id ─────────

func TestStoreUpdateNotFound(t *testing.T) {
	s := tempStore(t)
	if err := s.Update(&Todo{ID: "st-notfound", Title: "A", Status: StatusOpen}); err == nil {
		t.Error("expected update not found")
	}
}

func TestStoreDeleteSoftUnmarshalError(t *testing.T) {
	s := tempStore(t)
	injectBadTodo(t, s, "st-bad")
	if err := s.Delete("st-bad", false); err == nil {
		t.Error("expected soft delete unmarshal error")
	}
}

func TestListFilteredError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("boom") }
	defer func() { jsonUnmarshalStore = orig }()
	if _, err := s.ListFiltered(ListFilter{}); err == nil {
		t.Error("expected list filtered error")
	}
}

func TestListFilteredStatusAndType(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Status: StatusOpen, Type: TypeBug})
	_ = s.Add(&Todo{Title: "B", Status: StatusOpen, Type: TypeTask})
	out, err := s.ListFiltered(ListFilter{Status: StatusOpen, Type: TypeBug})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Title != "A" {
		t.Errorf("expected one bug, got %v", out)
	}
}

func TestReadyListError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("boom") }
	defer func() { jsonUnmarshalStore = orig }()
	if _, err := s.Ready(); err == nil {
		t.Error("expected ready error")
	}
}

func TestBlockedListError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("boom") }
	defer func() { jsonUnmarshalStore = orig }()
	if _, err := s.Blocked(); err == nil {
		t.Error("expected blocked error")
	}
}

func TestBlockedNonOpen(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Status: StatusDone})
	out, err := s.Blocked()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Errorf("expected 0 blocked, got %d", len(out))
	}
}

func TestBlockedBlockingDepsOfError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Status: StatusOpen})
	orig := getDepsFn
	getDepsFn = func(*Store, string) ([]Dependency, error) { return nil, errors.New("boom") }
	defer func() { getDepsFn = orig }()
	if _, err := s.Blocked(); err == nil {
		t.Error("expected blocked error")
	}
}

func TestComputeStatsListError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("boom") }
	defer func() { jsonUnmarshalStore = orig }()
	if _, err := s.ComputeStats(); err == nil {
		t.Error("expected compute stats error")
	}
}

func TestComputeStatsBlockedError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Status: StatusOpen})
	orig := getDepsFn
	calls := 0
	getDepsFn = func(st *Store, id string) ([]Dependency, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("boom")
		}
		return orig(st, id)
	}
	defer func() { getDepsFn = orig }()
	if _, err := s.ComputeStats(); err == nil {
		t.Error("expected compute stats blocked error")
	}
}

func TestCompactListError(t *testing.T) {
	s := tempStore(t)
	orig := listAllFn
	listAllFn = func(*Store) ([]*Todo, error) { return nil, errors.New("boom") }
	defer func() { listAllFn = orig }()
	if _, err := s.Compact(CompactOptions{}); err == nil {
		t.Error("expected compact list error")
	}
}

func TestCompactUpdateError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Status: StatusDone})
	makeOldDone(t, s, s.listIDs()[0])
	orig := updateFn
	updateFn = func(*Store, *Todo) error { return errors.New("boom") }
	defer func() { updateFn = orig }()
	if _, err := s.Compact(CompactOptions{OlderThan: 1}); err == nil {
		t.Error("expected compact update error")
	}
}

func (s *Store) listIDs() []string {
	ts, _ := s.List()
	ids := make([]string, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	return ids
}

func TestAddDepWouldCreateCycleError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	ids := s.listIDs()
	orig := getDepsFn
	getDepsFn = func(*Store, string) ([]Dependency, error) { return nil, errors.New("boom") }
	defer func() { getDepsFn = orig }()
	if err := s.AddDep(Dependency{From: ids[0], To: ids[1], Type: DepBlocks}); err == nil {
		t.Error("expected wouldCreateCycle error")
	}
}

func TestRemoveDepNotFound(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	ids := s.listIDs()
	orig := allDepTypes
	allDepTypes = []DepType{}
	defer func() { allDepTypes = orig }()
	if err := s.RemoveDep(ids[0], ids[1]); err == nil {
		t.Error("expected remove dep not found")
	}
}

func TestGetReverseDepsBadKey(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	ids := s.listIDs()
	injectBadDepKey(t, s)
	deps, err := s.GetReverseDeps(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 reverse deps, got %d", len(deps))
	}
}

func TestWouldCreateCycleError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	_ = s.AddDep(Dependency{From: "a", To: "b", Type: DepRelated})
	orig := getDepsFn
	getDepsFn = func(*Store, string) ([]Dependency, error) { return nil, errors.New("boom") }
	defer func() { getDepsFn = orig }()
	if _, err := s.wouldCreateCycle("c", "a"); err == nil {
		t.Error("expected wouldCreateCycle error")
	}
}

func TestWouldCreateCycleRecursiveError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	_ = s.Add(&Todo{Title: "C"})
	ids := s.listIDs()
	_ = s.AddDep(Dependency{From: ids[0], To: ids[1], Type: DepBlocks})
	orig := getDepsFn
	getDepsFn = func(st *Store, id string) ([]Dependency, error) {
		if id == ids[1] {
			return nil, errors.New("boom")
		}
		return orig(st, id)
	}
	defer func() { getDepsFn = orig }()
	if _, err := s.wouldCreateCycle(ids[2], ids[0]); err == nil {
		t.Error("expected recursive cycle error")
	}
}

func TestDependencyTreeDepthAndCycle(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A"})
	_ = s.Add(&Todo{Title: "B"})
	_ = s.Add(&Todo{Title: "C"})
	ids := s.listIDs()
	_ = s.AddDep(Dependency{From: ids[0], To: ids[1], Type: DepBlocks})
	_ = s.AddDep(Dependency{From: ids[0], To: ids[2], Type: DepBlocks})
	_ = s.AddDep(Dependency{From: ids[2], To: ids[1], Type: DepBlocks})
	out, err := s.DependencyTree(ids[0], 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Error("expected tree output")
	}
}

func TestEncodeBase36Padding(t *testing.T) {
	if got := encodeBase36(1, 4); got != "0001" {
		t.Errorf("expected 0001, got %q", got)
	}
}

func TestGenerateIDFallbackExhausted(t *testing.T) {
	resetIDState()
	orig := idSha1Sum
	idSha1Sum = func([]byte) [20]byte { return [20]byte{1} }
	defer func() { idSha1Sum = orig }()
	id := GenerateID()
	seenIDs[id] = struct{}{}
	id2 := GenerateID()
	if !strings.HasPrefix(id2, idPrefix) || len(id2) == 0 {
		t.Errorf("expected fallback id with prefix, got %q", id2)
	}
}

// ── remaining command-handler coverage ────────────────────────────────────

func TestCommandAddStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("put error") }
	defer func() { bucketPutFn = orig }()
	if err := runCmd(addCmd, []string{}, map[string]string{"title": "A"}, "text"); err == nil {
		t.Error("expected add store error")
	}
}

func TestCommandListMoreBranches(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Status: StatusOpen})
	if err := runCmd(listCmd, []string{}, map[string]string{"all": "true"}, "text"); err != nil {
		t.Fatalf("list all text: %v", err)
	}
	if err := runCmd(listCmd, []string{}, map[string]string{"status": "open"}, "json"); err != nil {
		t.Fatalf("list filtered json: %v", err)
	}
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("unmarshal error") }
	defer func() { jsonUnmarshalStore = orig }()
	if err := runCmd(listCmd, []string{}, map[string]string{"all": "true"}, "text"); err == nil {
		t.Error("expected list all error")
	}
	if err := runCmd(listCmd, []string{}, map[string]string{"status": "open"}, "text"); err == nil {
		t.Error("expected list filtered error")
	}
}

func TestCommandShowFull(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Status: StatusDone}, &Todo{ID: "st-b", Title: "B"}, &Todo{ID: "st-c", Title: "C"})
	withStore(t, func(s *Store) {
		makeOldDone(t, s, "st-a")
		if _, err := s.Compact(CompactOptions{OlderThan: 1}); err != nil {
			t.Fatalf("compact: %v", err)
		}
		_ = s.AddDep(Dependency{From: "st-a", To: "st-b", Type: DepBlocks})
		_ = s.AddDep(Dependency{From: "st-c", To: "st-a", Type: DepBlocks})
		_ = s.AppendAudit(AuditEntry{TodoID: "st-a", Action: "update", From: "open", To: "done", Note: "n", Actor: "actor"})
	})
	if err := runCmd(showCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("show full: %v", err)
	}
}

func TestCommandUpdateStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("put error") }
	defer func() { bucketPutFn = orig }()
	if err := runCmd(updateCmd, []string{"st-a"}, map[string]string{"title": "B"}, "text"); err == nil {
		t.Error("expected update store error")
	}
}

func TestCommandClaimStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("put error") }
	defer func() { bucketPutFn = orig }()
	if err := runCmd(claimCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected claim store error")
	}
}

func TestCommandUnclaimOpenStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	stop := setOpenStoreError(t)
	defer stop()
	if err := runCmd(unclaimCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected unclaim openStore error")
	}
}

func TestCommandUnclaimStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Assignee: "tester"})
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("put error") }
	defer func() { bucketPutFn = orig }()
	if err := runCmd(unclaimCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected unclaim store error")
	}
}

func TestCommandCompleteOpenStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	stop := setOpenStoreError(t)
	defer stop()
	if err := runCmd(completeCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected complete openStore error")
	}
}

func TestCommandCompleteStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("put error") }
	defer func() { bucketPutFn = orig }()
	if err := runCmd(completeCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected complete store error")
	}
}

func TestCommandCancelGetError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	if err := runCmd(cancelCmd, []string{"st-missing"}, nil, "text"); err == nil {
		t.Error("expected cancel not found")
	}
}

func TestCommandCancelStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := bucketPutFn
	bucketPutFn = func(b *bolt.Bucket, k, v []byte) error { return errors.New("put error") }
	defer func() { bucketPutFn = orig }()
	if err := runCmd(cancelCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected cancel store error")
	}
}

func TestCommandDepRemoveStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"}, &Todo{ID: "st-b", Title: "B"})
	withStore(t, func(s *Store) { _ = s.AddDep(Dependency{From: "st-a", To: "st-b", Type: DepBlocks}) })
	orig := bucketDeleteFn
	bucketDeleteFn = func(b *bolt.Bucket, k []byte) error { return errors.New("delete error") }
	defer func() { bucketDeleteFn = orig }()
	if err := runCmd(depRemoveCmd, []string{"st-a", "st-b"}, nil, "text"); err == nil {
		t.Error("expected dep remove store error")
	}
}

func TestCommandDepsTreeError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"}, &Todo{ID: "st-b", Title: "B"})
	withStore(t, func(s *Store) { _ = s.AddDep(Dependency{From: "st-a", To: "st-b", Type: DepBlocks}) })
	orig := getDepsFn
	getDepsFn = func(*Store, string) ([]Dependency, error) { return nil, errors.New("dep error") }
	defer func() { getDepsFn = orig }()
	if err := runCmd(depsCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected deps tree error")
	}
}

func TestCommandDepsDepthAndSeen(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"}, &Todo{ID: "st-b", Title: "B"}, &Todo{ID: "st-c", Title: "C"})
	withStore(t, func(s *Store) {
		_ = s.AddDep(Dependency{From: "st-a", To: "st-b", Type: DepBlocks})
		_ = s.AddDep(Dependency{From: "st-a", To: "st-c", Type: DepBlocks})
		_ = s.AddDep(Dependency{From: "st-c", To: "st-b", Type: DepBlocks})
	})
	if err := runCmd(depsCmd, []string{"st-a"}, map[string]string{"depth": "0"}, "text"); err != nil {
		t.Fatalf("deps depth 0: %v", err)
	}
	if err := runCmd(depsCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("deps seen: %v", err)
	}
}

func TestCommandGraphEdgesAndStyle(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"}, &Todo{ID: "st-b", Title: "B"})
	withStore(t, func(s *Store) { _ = s.AddDep(Dependency{From: "st-a", To: "st-b", Type: DepRelated}) })
	if err := runCmd(graphCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("graph: %v", err)
	}
}

func TestCommandGraphDuplicateEdge(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"}, &Todo{ID: "st-b", Title: "B"})
	withStore(t, func(s *Store) {
		_ = s.AddDep(Dependency{From: "st-a", To: "st-b", Type: DepRelated})
		_ = s.AddDep(Dependency{From: "st-a", To: "st-b", Type: DepBlocks})
	})
	if err := runCmd(graphCmd, []string{}, nil, "text"); err != nil {
		t.Fatalf("graph duplicate: %v", err)
	}
}

func TestCommandGraphListError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("unmarshal error") }
	defer func() { jsonUnmarshalStore = orig }()
	if err := runCmd(graphCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected graph list error")
	}
}

func TestCommandImportJSONOutput(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "t.json")
	_ = os.WriteFile(jsonPath, []byte(`[{"title":"I","priority":"P2","type":"task"}]`), 0644)
	oldFormat := todoFormat
	todoFormat = "json"
	defer func() { todoFormat = oldFormat }()
	if err := runCmd(importCmd, []string{jsonPath}, nil, "json"); err != nil {
		t.Fatalf("import json output: %v", err)
	}
}

func TestCommandReadyStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("unmarshal error") }
	defer func() { jsonUnmarshalStore = orig }()
	if err := runCmd(readyCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected ready store error")
	}
}

func TestCommandBlockedOpenStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	stop := setOpenStoreError(t)
	defer stop()
	if err := runCmd(blockedCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected blocked openStore error")
	}
}

func TestCommandBlockedStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("unmarshal error") }
	defer func() { jsonUnmarshalStore = orig }()
	if err := runCmd(blockedCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected blocked store error")
	}
}

func TestCommandSearchJSON(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	if err := runCmd(searchCmd, []string{"A"}, nil, "json"); err != nil {
		t.Fatalf("search json: %v", err)
	}
}

func TestCommandSearchOpenStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	stop := setOpenStoreError(t)
	defer stop()
	if err := runCmd(searchCmd, []string{"A"}, nil, "text"); err == nil {
		t.Error("expected search openStore error")
	}
}

func TestCommandStatsStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Status: StatusOpen})
	orig := getDepsFn
	getDepsFn = func(*Store, string) ([]Dependency, error) { return nil, errors.New("dep error") }
	defer func() { getDepsFn = orig }()
	if err := runCmd(statsCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected stats store error")
	}
}

func TestCommandTimelineAuditDetails(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	withStore(t, func(s *Store) {
		_ = s.AppendAudit(AuditEntry{TodoID: "st-a", Action: "update", From: "open", To: "done", Note: "note", Actor: "actor"})
	})
	if err := runCmd(timelineCmd, []string{"st-a"}, nil, "text"); err != nil {
		t.Fatalf("timeline details: %v", err)
	}
}

func TestCommandTimelineListAuditError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	withStore(t, func(s *Store) {
		_ = s.AppendAudit(AuditEntry{TodoID: "st-a", Action: "test"})
	})
	dir := t.TempDir()
	closed, _ := Open(filepath.Join(dir, "closed.db"))
	closed.Close()
	orig := openStoreFn
	openStoreFn = func() (*Store, error) { return closed, nil }
	defer func() { openStoreFn = orig }()
	if err := runCmd(timelineCmd, []string{"st-a"}, nil, "text"); err == nil {
		t.Error("expected timeline list audit error")
	}
}

func TestCommandMineStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Assignee: "tester"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("unmarshal error") }
	defer func() { jsonUnmarshalStore = orig }()
	if err := runCmd(mineCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected mine store error")
	}
}

func TestCommandProjectSwitchStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	dir := t.TempDir()
	closed, _ := Open(filepath.Join(dir, "closed.db"))
	closed.Close()
	orig := openStoreFn
	openStoreFn = func() (*Store, error) { return closed, nil }
	defer func() { openStoreFn = orig }()
	if err := runCmd(projectCmd, []string{"newproj"}, nil, "text"); err == nil {
		t.Error("expected project switch store error")
	}
}

func TestCommandRememberStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	orig := jsonMarshalMemory
	jsonMarshalMemory = func(interface{}) ([]byte, error) { return nil, errors.New("marshal error") }
	defer func() { jsonMarshalMemory = orig }()
	if err := runCmd(rememberCmd, []string{"insight"}, nil, "text"); err == nil {
		t.Error("expected remember store error")
	}
}

func TestCommandCompactStoreError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Status: StatusDone})
	withStore(t, func(s *Store) { makeOldDone(t, s, "st-a") })
	orig := jsonMarshalStore
	jsonMarshalStore = func(interface{}) ([]byte, error) { return nil, errors.New("marshal error") }
	defer func() { jsonMarshalStore = orig }()
	if err := runCmd(compactCmd, []string{}, map[string]string{"older-than": "1ns"}, "text"); err == nil {
		t.Error("expected compact store error")
	}
}

func TestCommandCompactJSON(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Status: StatusDone})
	withStore(t, func(s *Store) { makeOldDone(t, s, "st-a") })
	if err := runCmd(compactCmd, []string{}, map[string]string{"older-than": "1ns"}, "json"); err != nil {
		t.Fatalf("compact json: %v", err)
	}
}

func TestCommandDoctorListError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("unmarshal error") }
	defer func() { jsonUnmarshalStore = orig }()
	if err := runCmd(doctorCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected doctor list error")
	}
}

func TestCommandDoctorStatsError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A", Status: StatusOpen})
	orig := getDepsFn
	getDepsFn = func(*Store, string) ([]Dependency, error) { return nil, errors.New("dep error") }
	defer func() { getDepsFn = orig }()
	if err := runCmd(doctorCmd, []string{}, nil, "text"); err == nil {
		t.Error("expected doctor stats error")
	}
}

func TestCommandExportListError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	seedStore(t, &Todo{ID: "st-a", Title: "A"})
	orig := jsonUnmarshalStore
	jsonUnmarshalStore = func([]byte, interface{}) error { return errors.New("unmarshal error") }
	defer func() { jsonUnmarshalStore = orig }()
	if err := runCmd(exportCmd, []string{}, nil, "json"); err == nil {
		t.Error("expected export list error")
	}
}

func TestCommandImportJSONLLineError(t *testing.T) {
	cleanup := setupCmdTest(t)
	defer cleanup()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	_ = os.WriteFile(path, []byte(`{"title":"X"}
not json`), 0644)
	if err := runCmd(importCmd, []string{path}, nil, "jsonl"); err == nil {
		t.Error("expected jsonl line error")
	}
}

func TestListFilteredStatusFilter(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Status: StatusOpen})
	_ = s.Add(&Todo{Title: "B", Status: StatusDone})
	out, err := s.ListFiltered(ListFilter{Status: StatusOpen})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Title != "A" {
		t.Errorf("expected open A, got %v", out)
	}
}
