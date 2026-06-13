// SPDX-License-Identifier: MIT
// Purpose: tests for the sin-code webui server.
package webui

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	todoDB := filepath.Join(dir, "todo.db")
	notifDB := filepath.Join(dir, "notif.db")
	s, err := NewServer(Config{
		Host:    "127.0.0.1",
		Port:    0,
		TodoDB:  todoDB,
		NotifDB: notifDB,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func TestServerStart(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "sin-code") {
		t.Errorf("body missing brand, got: %s", string(body)[:min(200, len(body))])
	}
}

func TestServerOrchestratorPage(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/orchestrator")
	if err != nil {
		t.Fatalf("GET /orchestrator: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "<form") {
		t.Error("expected <form> in orchestrator page")
	}
	if !strings.Contains(s, "name=\"prompt\"") {
		t.Error("expected prompt input")
	}
	if !strings.Contains(s, "Available Agents") {
		t.Error("expected Available Agents section")
	}
}

func TestServerOrchestratorRun(t *testing.T) {
	_, ts := newTestServer(t)
	form := strings.NewReader("prompt=Add+a+hello+world+function")
	resp, err := http.Post(ts.URL+"/orchestrator/run", "application/x-www-form-urlencoded", form)
	if err != nil {
		t.Fatalf("POST /orchestrator/run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body: %s", resp.StatusCode, string(body)[:min(500, len(body))])
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "Plan") {
		t.Errorf("expected Plan section, got: %s", s[:min(500, len(s))])
	}
}

func TestServerOrchestratorRunEmptyPrompt(t *testing.T) {
	_, ts := newTestServer(t)
	form := strings.NewReader("prompt=")
	resp, err := http.Post(ts.URL+"/orchestrator/run", "application/x-www-form-urlencoded", form)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Prompt is required") {
		t.Error("expected 'Prompt is required' error")
	}
}

func TestServerTodosAPI(t *testing.T) {
	dir := t.TempDir()
	todoDB := filepath.Join(dir, "todo.db")
	notifDB := filepath.Join(dir, "notif.db")
	s, err := NewServer(Config{Host: "127.0.0.1", Port: 0, TodoDB: todoDB, NotifDB: notifDB})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	store, err := todo.Open(todoDB)
	if err != nil {
		t.Fatalf("open todo: %v", err)
	}
	for i := 0; i < 3; i++ {
		t1 := &todo.Todo{
			Title:    "Test todo " + string(rune('A'+i)),
			Priority: todo.PriorityP2,
			Type:     todo.TypeTask,
		}
		if err := store.Add(t1); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	store.Close()

	resp, err := http.Get(ts.URL + "/api/todos.json")
	if err != nil {
		t.Fatalf("GET /api/todos.json: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	var items []*todo.Todo
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("unmarshal: %v: %s", err, string(body))
	}
	if len(items) != 3 {
		t.Errorf("expected 3 todos, got %d", len(items))
	}
}

func TestServerTodosPage(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/todos")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Todos") {
		t.Error("expected Todos heading")
	}
}

func TestServerTodosAdd(t *testing.T) {
	dir := t.TempDir()
	todoDB := filepath.Join(dir, "todo.db")
	notifDB := filepath.Join(dir, "notif.db")
	s, err := NewServer(Config{Host: "127.0.0.1", Port: 0, TodoDB: todoDB, NotifDB: notifDB})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	form := strings.NewReader("title=Test+via+webui&priority=P1&type=feature")
	resp, err := http.Post(ts.URL+"/todos/add", "application/x-www-form-urlencoded", form)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 303 or 200", resp.StatusCode)
	}

	store, err := todo.Open(todoDB)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	ts2, _ := store.List()
	if len(ts2) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(ts2))
	}
	if ts2[0].Title != "Test via webui" {
		t.Errorf("title = %q", ts2[0].Title)
	}
}

func TestServerNotificationsAPI(t *testing.T) {
	dir := t.TempDir()
	todoDB := filepath.Join(dir, "todo.db")
	notifDB := filepath.Join(dir, "notif.db")
	s, err := NewServer(Config{Host: "127.0.0.1", Port: 0, TodoDB: todoDB, NotifDB: notifDB})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	store, err := notifications.Open(notifDB)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := 0; i < 2; i++ {
		n := &notifications.Notification{
			Type:    notifications.TypeTodoCreated,
			TodoID:  "st-1",
			Title:   "Hello " + string(rune('A'+i)),
			Message: "msg",
		}
		if err := store.Add(n); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	store.Close()

	resp, err := http.Get(ts.URL + "/api/notifications.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var items []*notifications.Notification
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("unmarshal: %v: %s", err, string(body))
	}
	if len(items) != 2 {
		t.Errorf("expected 2 notifications, got %d", len(items))
	}
}

func TestServerNotificationsPage(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/notifications")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Notifications") {
		t.Error("expected Notifications heading")
	}
}

func TestServerEfmPage(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/efm")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "EFM Stacks") {
		t.Error("expected EFM Stacks heading")
	}
}

func TestServerAgentsJSON(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/orchestrator/agents.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	var items []map[string]interface{}
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("unmarshal: %v: %s", err, string(body))
	}
	if len(items) < 5 {
		t.Errorf("expected at least 5 agents, got %d", len(items))
	}
}

func TestServerStaticFiles(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/static/style.css")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) < 100 {
		t.Errorf("css body too small: %d bytes", len(body))
	}
	if !strings.Contains(string(body), ":root") {
		t.Error("expected :root selector in CSS")
	}
}

func TestServerTodoDetail(t *testing.T) {
	dir := t.TempDir()
	todoDB := filepath.Join(dir, "todo.db")
	notifDB := filepath.Join(dir, "notif.db")
	s, err := NewServer(Config{Host: "127.0.0.1", Port: 0, TodoDB: todoDB, NotifDB: notifDB})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	store, err := todo.Open(todoDB)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t1 := &todo.Todo{Title: "Detail me", Priority: todo.PriorityP0, Type: todo.TypeBug}
	if err := store.Add(t1); err != nil {
		t.Fatalf("add: %v", err)
	}
	store.Close()

	resp, err := http.Get(ts.URL + "/todos/" + t1.ID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Detail me") {
		t.Error("expected todo title in detail page")
	}
}

func TestServerAddr(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServer(Config{
		Host:    "127.0.0.1",
		Port:    0,
		TodoDB:  filepath.Join(dir, "todo.db"),
		NotifDB: filepath.Join(dir, "notif.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	addr := s.Addr()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Errorf("addr = %q", addr)
	}
}

func TestServerEmbedFSPresent(t *testing.T) {
	f, err := staticSub().Open("style.css")
	if err != nil {
		t.Fatalf("static/style.css not embedded: %v", err)
	}
	defer f.Close()
}

func TestLoadTemplates(t *testing.T) {
	tmpl, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates: %v", err)
	}
	for _, name := range []string{"base.html", "index.html", "orchestrator.html", "todos.html", "todo_detail.html", "notifications.html", "efm.html"} {
		if tmpl.Lookup(name) == nil {
			t.Errorf("template %s not loaded", name)
		}
	}
}

func TestServerRealListen(t *testing.T) {
	ln, err := listenOn("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if ln.Addr().Network() != "tcp" {
		t.Errorf("network = %q", ln.Addr().Network())
	}
}

func TestServerEfmDetail(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/efm/somestack")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestWriteJSONAndError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, map[string]string{"hello": "world"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	rr2 := httptest.NewRecorder()
	writeJSONError(rr2, http.StatusInternalServerError, errSentinel)
	if rr2.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", rr2.Code)
	}
}

var errSentinel = io.EOF

func TestEfmMetaDir(t *testing.T) {
	dir := efmMetaDir()
	if dir == "" {
		t.Error("empty efmMetaDir")
	}
}

func TestDiscoverEfmStacksEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stacks, _, err := discoverEfmStacks()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if stacks == nil {
		t.Error("expected non-nil empty slice")
	}
}

func TestDiscoverEfmStacksWithMeta(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	metaDir := filepath.Join(dir, ".local", "state", "sin-code", "efm")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"stack":"/tmp/docker-compose.yml","started":"` + time.Now().Format(time.RFC3339) + `","expires":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `","runtime":"docker"}`
	if err := os.WriteFile(filepath.Join(metaDir, efmMetaKey("/tmp/docker-compose.yml")), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	stacks, _, err := discoverEfmStacks()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(stacks) != 1 {
		t.Errorf("expected 1 stack, got %d", len(stacks))
	}
}

func TestRenderBasePage(t *testing.T) {
	s, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	s.render(rr, "index.html", pageData{Title: "Home", Active: "home"})
	body := rr.Body.String()
	if !strings.Contains(body, "<!doctype html>") {
		t.Error("missing doctype")
	}
	if !strings.Contains(body, "sin-code") {
		t.Error("missing brand")
	}
}

func TestServerHandleIndex(t *testing.T) {
	s, _ := newTestServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s.handleIndex(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d", rr.Code)
	}
}

func TestDetectRuntimeEmpty(t *testing.T) {
	_ = detectContainerRuntime()
}

func TestOpenInBrowserDoesNotPanic(t *testing.T) {
	_ = openInBrowser("http://127.0.0.1:1")
}

func TestServerListenAndServeGraceful(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServer(Config{
		Host:    "127.0.0.1",
		Port:    0,
		TodoDB:  filepath.Join(dir, "todo.db"),
		NotifDB: filepath.Join(dir, "notif.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.ln = ln
	s.addr_ = ln.Addr().String()
	s.httpServer = &http.Server{Handler: s.mux}

	done := make(chan struct{})
	go func() {
		_ = s.httpServer.Serve(ln)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	_ = s.httpServer.Close()
	<-done
}
