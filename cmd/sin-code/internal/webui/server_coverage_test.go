// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for webui error paths and branches.
package webui

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/health"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

func setHook[T any](t *testing.T, hook *T, val T) {
	old := *hook
	*hook = val
	t.Cleanup(func() { *hook = old })
}

type fakeDirEntry struct {
	name  string
	isDir bool
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return f.isDir }
func (f fakeDirEntry) Type() os.FileMode          { return 0 }
func (f fakeDirEntry) Info() (os.FileInfo, error) { return nil, errors.New("no info") }

type fakeListener struct {
	addr net.Addr
	err  error
}

func (f *fakeListener) Accept() (net.Conn, error) { return nil, f.err }
func (f *fakeListener) Close() error              { return nil }
func (f *fakeListener) Addr() net.Addr            { return f.addr }

func TestStartEnvHost(t *testing.T) {
	t.Setenv("SIN_CODE_WEBUI_HOST", "127.0.0.1")
	setHook(t, &netListenHook, func(network, address string) (net.Listener, error) {
		return nil, errors.New("listen boom")
	})
	if err := Start(0); err == nil {
		t.Fatal("expected error")
	}
}

func TestStartDefaultHost(t *testing.T) {
	t.Setenv("SIN_CODE_WEBUI_HOST", "")
	setHook(t, &netListenHook, func(network, address string) (net.Listener, error) {
		return nil, errors.New("listen boom")
	})
	if err := Start(0); err == nil {
		t.Fatal("expected error")
	}
}

func TestStartWithNewServerError(t *testing.T) {
	setHook(t, &templateFSSubHook, func() (fs.FS, error) { return nil, errors.New("boom") })
	if err := StartWith(Config{Port: 0}); err == nil {
		t.Fatal("expected error")
	}
}

func TestStartWithListenError(t *testing.T) {
	setHook(t, &netListenHook, func(network, address string) (net.Listener, error) {
		return nil, errors.New("listen boom")
	})
	if err := StartWith(Config{Port: 0}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewServerLoadTemplatesError(t *testing.T) {
	setHook(t, &templateFSSubHook, func() (fs.FS, error) { return nil, errors.New("boom") })
	_, err := NewServer(Config{Port: 0})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAddrWithAddrSet(t *testing.T) {
	srv, err := NewServer(Config{Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	srv.addr_ = "custom:123"
	if got := srv.Addr(); got != "custom:123" {
		t.Fatalf("Addr = %q", got)
	}
}

func TestHealthCheckTemplateNil(t *testing.T) {
	srv := &Server{health: health.NewChecker("webui")}
	srv.templates = nil
	srv.setupHealthChecks()
	resp := srv.health.Check(context.Background())
	if got := resp.Checks["templates"].Status; got != health.StatusUnhealthy {
		t.Fatalf("templates status = %q", got)
	}
}

func TestHealthCheckTodoDBEmpty(t *testing.T) {
	srv := &Server{health: health.NewChecker("webui"), todoDB: ""}
	srv.setupHealthChecks()
	resp := srv.health.Check(context.Background())
	if got := resp.Checks["todo_db"].Status; got != health.StatusDegraded {
		t.Fatalf("todo_db status = %q", got)
	}
}

func TestHealthChecksHealthy(t *testing.T) {
	srv, err := NewServer(Config{Port: 0, TodoDB: filepath.Join(t.TempDir(), "todo.db")})
	if err != nil {
		t.Fatal(err)
	}
	resp := srv.health.Check(context.Background())
	if got := resp.Checks["templates"].Status; got != health.StatusHealthy {
		t.Fatalf("templates status = %q", got)
	}
	if got := resp.Checks["todo_db"].Status; got != health.StatusHealthy {
		t.Fatalf("todo_db status = %q", got)
	}
}

func TestLoadTemplatesSubError(t *testing.T) {
	setHook(t, &templateFSSubHook, func() (fs.FS, error) { return nil, errors.New("boom") })
	_, err := loadTemplates()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadTemplatesParseError(t *testing.T) {
	setHook(t, &parseFSHook, func(t *template.Template, f fs.FS) (*template.Template, error) {
		return nil, errors.New("boom")
	})
	_, err := loadTemplates()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSafeHTMLFunc(t *testing.T) {
	tmpl, err := loadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err = tmpl.New("safe").Parse(`{{ safeHTML "<b>hi</b>" }}`)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "safe", nil); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "<b>hi</b>" {
		t.Fatalf("safeHTML output = %q", got)
	}
}

func TestRenderTemplateNotFound(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	rr := httptest.NewRecorder()
	srv.render(rr, "missing.html", pageData{})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestRenderCloneError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &templateCloneHook, func(t *template.Template) (*template.Template, error) {
		return nil, errors.New("boom")
	})
	rr := httptest.NewRecorder()
	srv.render(rr, "index.html", pageData{})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestRenderParseError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &templateParseHook, func(t *template.Template, text string) (*template.Template, error) {
		return nil, errors.New("boom")
	})
	rr := httptest.NewRecorder()
	srv.render(rr, "index.html", pageData{})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestRenderExecuteError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &templateExecHook, func(t *template.Template, wr io.Writer, name string, data interface{}) error {
		return errors.New("boom")
	})
	rr := httptest.NewRecorder()
	srv.render(rr, "index.html", pageData{})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestOrchestratorRunBadForm(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/orchestrator/run", strings.NewReader("%"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleOrchestratorRun(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestOrchestratorRunError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &orchestratorRunFunc, func(ctx context.Context, prompt string) (*orchestrator.Result, error) {
		return nil, errors.New("boom")
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/orchestrator/run", strings.NewReader("prompt=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleOrchestratorRun(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "boom") {
		t.Fatal("expected error in body")
	}
}

func TestTodosPageOpenError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &todoOpenHook, func(path string) (*todo.Store, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	srv.handleTodosPage(rr, httptest.NewRequest(http.MethodGet, "/todos", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "boom") {
		t.Fatal("expected error in body")
	}
}

func TestTodosPageListError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0, TodoDB: filepath.Join(t.TempDir(), "todo.db")})
	setHook(t, &todoListHook, func(s *todo.Store) ([]*todo.Todo, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	srv.handleTodosPage(rr, httptest.NewRequest(http.MethodGet, "/todos", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "boom") {
		t.Fatal("expected error in body")
	}
}

func TestTodosPageDoneStatus(t *testing.T) {
	db := filepath.Join(t.TempDir(), "todo.db")
	srv, _ := NewServer(Config{Port: 0, TodoDB: db})
	store, err := todo.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Add(&todo.Todo{Title: "Done", Status: todo.StatusDone, Priority: todo.PriorityP2, Type: todo.TypeTask})
	store.Close()

	rr := httptest.NewRecorder()
	srv.handleTodosPage(rr, httptest.NewRequest(http.MethodGet, "/todos", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestTodosAddBadForm(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/todos/add", strings.NewReader("%"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleTodosAdd(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestTodosAddEmptyTitle(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/todos/add", strings.NewReader("title="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleTodosAdd(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestTodosAddOpenError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &todoOpenHook, func(path string) (*todo.Store, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/todos/add", strings.NewReader("title=hi"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleTodosAdd(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestTodosAddDefaultsAndAddError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0, TodoDB: filepath.Join(t.TempDir(), "todo.db")})
	setHook(t, &todoAddHook, func(s *todo.Store, t *todo.Todo) error { return errors.New("boom") })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/todos/add", strings.NewReader("title=hi"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleTodosAdd(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "boom") {
		t.Fatal("expected error in body")
	}
}

func TestTodoDetailOpenError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &todoOpenHook, func(path string) (*todo.Store, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/todos/x", nil)
	srv.handleTodoDetail(rr, req)
	if rr.Code == 0 {
		t.Fatal("no response")
	}
}

func TestTodoDetailNotFound(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0, TodoDB: filepath.Join(t.TempDir(), "todo.db")})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/todos/missing", nil)
	srv.handleTodoDetail(rr, req)
	if rr.Code == 0 {
		t.Fatal("no response")
	}
}

func TestNotificationsPageOpenError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &notifOpenHook, func(path string) (*notifications.Store, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	srv.handleNotificationsPage(rr, httptest.NewRequest(http.MethodGet, "/notifications", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestNotificationsPageListError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0, NotifDB: filepath.Join(t.TempDir(), "notif.db")})
	setHook(t, &notifListHook, func(s *notifications.Store, f notifications.ListFilter, limit int) ([]*notifications.Notification, error) {
		return nil, errors.New("boom")
	})
	rr := httptest.NewRecorder()
	srv.handleNotificationsPage(rr, httptest.NewRequest(http.MethodGet, "/notifications", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestNotificationsPageUnread(t *testing.T) {
	db := filepath.Join(t.TempDir(), "notif.db")
	srv, _ := NewServer(Config{Port: 0, NotifDB: db})
	store, err := notifications.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Add(&notifications.Notification{Type: notifications.TypeTodoCreated, TodoID: "x", Title: "hi"})
	store.Close()

	rr := httptest.NewRecorder()
	srv.handleNotificationsPage(rr, httptest.NewRequest(http.MethodGet, "/notifications", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Notifications") {
		t.Fatal("expected Notifications heading")
	}
}

func TestEfmPageError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &readDirHook, func(string) ([]os.DirEntry, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	srv.handleEfmPage(rr, httptest.NewRequest(http.MethodGet, "/efm", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "boom") {
		t.Fatal("expected error in body")
	}
}

func TestEfmDetailError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &readDirHook, func(string) ([]os.DirEntry, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	srv.handleEfmDetail(rr, httptest.NewRequest(http.MethodGet, "/efm/x", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "boom") {
		t.Fatal("expected error in body")
	}
}

func TestEfmDetailMatchName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	metaDir := filepath.Join(dir, ".local", "state", "sin-code", "efm")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stackPath := "/tmp/somestack.yml"
	meta := `{"stack":"/tmp/somestack.yml","started":"` + time.Now().Format(time.RFC3339) + `","expires":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `","runtime":"docker"}`
	if err := os.WriteFile(filepath.Join(metaDir, efmMetaKey(stackPath)), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &execCommandRunner, func(name string, args ...string) ([]byte, error) { return nil, nil })

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/efm/somestack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "somestack") {
		t.Fatalf("body = %s", string(body))
	}
}

func TestNotificationsJSONOpenError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &notifOpenHook, func(path string) (*notifications.Store, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	srv.handleNotificationsJSON(rr, httptest.NewRequest(http.MethodGet, "/api/notifications.json", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestNotificationsJSONListError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0, NotifDB: filepath.Join(t.TempDir(), "notif.db")})
	setHook(t, &notifListHook, func(s *notifications.Store, f notifications.ListFilter, limit int) ([]*notifications.Notification, error) {
		return nil, errors.New("boom")
	})
	rr := httptest.NewRecorder()
	srv.handleNotificationsJSON(rr, httptest.NewRequest(http.MethodGet, "/api/notifications.json", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestNotificationsJSONNil(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0, NotifDB: filepath.Join(t.TempDir(), "notif.db")})
	setHook(t, &notifListHook, func(s *notifications.Store, f notifications.ListFilter, limit int) ([]*notifications.Notification, error) {
		return nil, nil
	})
	rr := httptest.NewRecorder()
	srv.handleNotificationsJSON(rr, httptest.NewRequest(http.MethodGet, "/api/notifications.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

func TestTodosJSONOpenError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0})
	setHook(t, &todoOpenHook, func(path string) (*todo.Store, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	srv.handleTodosJSON(rr, httptest.NewRequest(http.MethodGet, "/api/todos.json", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestTodosJSONListError(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0, TodoDB: filepath.Join(t.TempDir(), "todo.db")})
	setHook(t, &todoListHook, func(s *todo.Store) ([]*todo.Todo, error) { return nil, errors.New("boom") })
	rr := httptest.NewRecorder()
	srv.handleTodosJSON(rr, httptest.NewRequest(http.MethodGet, "/api/todos.json", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestTodosJSONNil(t *testing.T) {
	srv, _ := NewServer(Config{Port: 0, TodoDB: filepath.Join(t.TempDir(), "todo.db")})
	setHook(t, &todoListHook, func(s *todo.Store) ([]*todo.Todo, error) { return nil, nil })
	rr := httptest.NewRecorder()
	srv.handleTodosJSON(rr, httptest.NewRequest(http.MethodGet, "/api/todos.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

func TestDefaultTodoDBEnv(t *testing.T) {
	t.Setenv("SIN_CODE_TODO_DB", "/custom/todo.db")
	if got := defaultTodoDB(); got != "/custom/todo.db" {
		t.Fatalf("defaultTodoDB = %q", got)
	}
}

func TestDefaultTodoDBConfigError(t *testing.T) {
	t.Setenv("SIN_CODE_TODO_DB", "")
	setHook(t, &userConfigDirHook, func() (string, error) { return "", errors.New("boom") })
	if got := defaultTodoDB(); got != "todo.db" {
		t.Fatalf("defaultTodoDB = %q", got)
	}
}

func TestDefaultNotifDBEnv(t *testing.T) {
	t.Setenv("SIN_CODE_NOTIF_DB", "/custom/notif.db")
	if got := defaultNotifDB(); got != "/custom/notif.db" {
		t.Fatalf("defaultNotifDB = %q", got)
	}
}

func TestDefaultNotifDBConfigError(t *testing.T) {
	t.Setenv("SIN_CODE_NOTIF_DB", "")
	setHook(t, &userConfigDirHook, func() (string, error) { return "", errors.New("boom") })
	if got := defaultNotifDB(); got != "notifications.db" {
		t.Fatalf("defaultNotifDB = %q", got)
	}
}

func TestEfmMetaDirUserConfig(t *testing.T) {
	t.Setenv("HOME", "")
	tmp := t.TempDir()
	setHook(t, &userConfigDirHook, func() (string, error) { return tmp, nil })
	if got := efmMetaDir(); got != filepath.Join(tmp, "sin-code", "efm") {
		t.Fatalf("efmMetaDir = %q", got)
	}
}

func TestEfmMetaDirFallback(t *testing.T) {
	t.Setenv("HOME", "")
	tmp := t.TempDir()
	setHook(t, &userConfigDirHook, func() (string, error) { return "", errors.New("boom") })
	setHook(t, &osTempDirHook, func() string { return tmp })
	if got := efmMetaDir(); got != filepath.Join(tmp, "sin-code-efm") {
		t.Fatalf("efmMetaDir = %q", got)
	}
}

func TestDetectContainerRuntimeLinux(t *testing.T) {
	setHook(t, &goosHook, func() string { return "linux" })
	setHook(t, &lookPathHook, func(string) (string, error) { return "/usr/bin/docker", nil })
	if got := detectContainerRuntime(); got != "docker" {
		t.Fatalf("runtime = %q", got)
	}
}

func TestDetectContainerRuntimeLinuxNone(t *testing.T) {
	setHook(t, &goosHook, func() string { return "linux" })
	setHook(t, &lookPathHook, func(string) (string, error) { return "", errors.New("not found") })
	if got := detectContainerRuntime(); got != "" {
		t.Fatalf("runtime = %q", got)
	}
}

func TestDiscoverEfmStacksReadDirError(t *testing.T) {
	setHook(t, &readDirHook, func(string) ([]os.DirEntry, error) { return nil, errors.New("boom") })
	_, _, err := discoverEfmStacks()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDiscoverEfmStacksSkipEntries(t *testing.T) {
	setHook(t, &readDirHook, func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{fakeDirEntry{name: "dir", isDir: true}, fakeDirEntry{name: "file.txt", isDir: false}}, nil
	})
	stacks, _, err := discoverEfmStacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 0 {
		t.Fatalf("expected 0 stacks, got %d", len(stacks))
	}
}

func TestDiscoverEfmStacksReadFileError(t *testing.T) {
	setHook(t, &readDirHook, func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{fakeDirEntry{name: "abc.meta", isDir: false}}, nil
	})
	setHook(t, &readFileHook, func(string) ([]byte, error) { return nil, errors.New("boom") })
	stacks, _, err := discoverEfmStacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 0 {
		t.Fatalf("expected 0 stacks, got %d", len(stacks))
	}
}

func TestDiscoverEfmStacksInvalidJSON(t *testing.T) {
	setHook(t, &readDirHook, func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{fakeDirEntry{name: "abc.meta", isDir: false}}, nil
	})
	setHook(t, &readFileHook, func(string) ([]byte, error) { return []byte("not json"), nil })
	stacks, _, err := discoverEfmStacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 0 {
		t.Fatalf("expected 0 stacks, got %d", len(stacks))
	}
}

func TestDiscoverEfmStacksRunning(t *testing.T) {
	setHook(t, &readDirHook, func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{fakeDirEntry{name: "abc.meta", isDir: false}}, nil
	})
	setHook(t, &readFileHook, func(string) ([]byte, error) {
		return []byte(`{"stack":"/tmp/abc.yml","started":"` + time.Now().Format(time.RFC3339) + `","expires":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `","runtime":"docker"}`), nil
	})
	setHook(t, &goosHook, func() string { return "linux" })
	setHook(t, &lookPathHook, func(string) (string, error) { return "/usr/bin/docker", nil })
	setHook(t, &execCommandRunner, func(name string, args ...string) ([]byte, error) { return []byte("Up 5 minutes"), nil })
	stacks, _, err := discoverEfmStacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d", len(stacks))
	}
	if stacks[0].Status != "running" {
		t.Fatalf("status = %q", stacks[0].Status)
	}
}

func TestDiscoverEfmStacksEmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".local", "state", "sin-code", "efm"), 0o755); err != nil {
		t.Fatal(err)
	}
	stacks, _, err := discoverEfmStacks()
	if err != nil {
		t.Fatal(err)
	}
	if stacks == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(stacks) != 0 {
		t.Fatalf("expected 0 stacks, got %d", len(stacks))
	}
}

func TestListenAndServeListenError(t *testing.T) {
	srv, err := NewServer(Config{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	setHook(t, &netListenHook, func(network, address string) (net.Listener, error) {
		return nil, errors.New("listen boom")
	})
	if err := srv.ListenAndServe(); err == nil {
		t.Fatal("expected error")
	}
}

func TestListenAndServeOpenBrowser(t *testing.T) {
	srv, err := NewServer(Config{Host: "127.0.0.1", Port: 0, OpenBrowser: true})
	if err != nil {
		t.Fatal(err)
	}
	urlCh := make(chan string, 1)
	setHook(t, &openBrowserHook, func(target string) error {
		urlCh <- target
		return nil
	})
	setHook(t, &netListenHook, netListenHook)

	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe() }()

	select {
	case target := <-urlCh:
		if target == "" {
			t.Fatal("empty target")
		}
	case <-time.After(time.Second):
		t.Fatal("openBrowser not called")
	}

	time.Sleep(50 * time.Millisecond)
	_ = srv.httpServer.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ListenAndServe: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestListenAndServeServeError(t *testing.T) {
	srv, err := NewServer(Config{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	setHook(t, &netListenHook, func(network, address string) (net.Listener, error) {
		return &fakeListener{
			addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0},
			err:  errors.New("accept boom"),
		}, nil
	})
	if err := srv.ListenAndServe(); err == nil {
		t.Fatal("expected error")
	}
}

func TestListenAndServeStopSignal(t *testing.T) {
	srv, err := NewServer(Config{Host: "127.0.0.1", Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	setHook(t, &signalNotifyHook, func(c chan<- os.Signal, sigs ...os.Signal) {
		go func() {
			time.Sleep(50 * time.Millisecond)
			c <- os.Interrupt
		}()
	})
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestOpenInBrowserWindows(t *testing.T) {
	setHook(t, &goosHook, func() string { return "windows" })
	setHook(t, &browserExecHook, func(name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	})
	_ = openInBrowser("http://127.0.0.1:1")
}

func TestOpenInBrowserLinux(t *testing.T) {
	setHook(t, &goosHook, func() string { return "linux" })
	setHook(t, &browserExecHook, func(name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	})
	_ = openInBrowser("http://127.0.0.1:1")
}

func TestTemplateSubPanic(t *testing.T) {
	setHook(t, &templateFSSubHook, func() (fs.FS, error) { return nil, errors.New("boom") })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = templateSub()
}

func TestStaticSubPanic(t *testing.T) {
	setHook(t, &staticFSSubHook, func() (fs.FS, error) { return nil, errors.New("boom") })
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = staticSub()
}
