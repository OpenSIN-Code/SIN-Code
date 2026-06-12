// SPDX-License-Identifier: MIT
// Purpose: tests for the notifications package: store CRUD, dispatch, indexes, prune.
package notifications

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "notif.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenAndClose(t *testing.T) {
	s := tempStore(t)
	if s.Path() == "" {
		t.Error("Path should be set")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// A path whose parent "directory" is actually a file fails with ENOTDIR on
// every platform regardless of user privileges (unlike absolute paths under
// "/", which are creatable in containers/CI running with a writable root).
func TestOpenInvalidDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(blocker, "sub", "notif.db")); err == nil {
		t.Error("expected error for invalid dir")
	}
}

func TestOpenEmptyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open empty: %v", err)
	}
	defer s.Close()
	if _, err := os.Stat(s.Path()); err != nil {
		t.Errorf("file should exist: %v", err)
	}
}

func TestAddAndGet(t *testing.T) {
	s := tempStore(t)
	n := &Notification{Type: TypeTodoCreated, TodoID: "st-aaaa", Title: "Hello", Message: "world"}
	if err := s.Add(n); err != nil {
		t.Fatal(err)
	}
	if n.ID == "" {
		t.Error("ID should be set")
	}
	if n.Created.IsZero() {
		t.Error("Created should be set")
	}
	got, err := s.Get(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Hello" || got.TodoID != "st-aaaa" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

func TestAddValidation(t *testing.T) {
	s := tempStore(t)
	if err := s.Add(nil); err == nil {
		t.Error("expected error for nil")
	}
	if err := s.Add(&Notification{}); err == nil {
		t.Error("expected error for missing type")
	}
}

func TestListFilters(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "st-aaaa", Title: "1"})
	_ = s.Add(&Notification{Type: TypeTodoCompleted, TodoID: "st-aaaa", Title: "2"})
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "st-bbbb", Title: "3"})

	all, _ := s.List(ListFilter{}, 0)
	if len(all) != 3 {
		t.Errorf("all: got %d, want 3", len(all))
	}

	byType, _ := s.List(ListFilter{Type: TypeTodoCreated}, 0)
	if len(byType) != 2 {
		t.Errorf("by type: got %d, want 2", len(byType))
	}

	byTodo, _ := s.List(ListFilter{TodoID: "st-aaaa"}, 0)
	if len(byTodo) != 2 {
		t.Errorf("by todo: got %d, want 2", len(byTodo))
	}
}

func TestListLimit(t *testing.T) {
	s := tempStore(t)
	for i := 0; i < 10; i++ {
		_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x"})
	}
	limited, _ := s.List(ListFilter{}, 3)
	if len(limited) != 3 {
		t.Errorf("limit: got %d, want 3", len(limited))
	}
}

func TestListUnreadFilter(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x"})
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "y", Title: "y"})
	ns, _ := s.List(ListFilter{}, 0)
	if err := s.MarkRead(ns[0].ID); err != nil {
		t.Fatal(err)
	}
	unread, _ := s.List(ListFilter{Unread: true}, 0)
	if len(unread) != 1 {
		t.Errorf("unread: got %d, want 1", len(unread))
	}
}

func TestListNotDismissed(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x"})
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "y", Title: "y"})
	ns, _ := s.List(ListFilter{}, 0)
	_ = s.Dismiss(ns[0].ID)
	active, _ := s.List(ListFilter{NotDismissed: true}, 0)
	if len(active) != 1 {
		t.Errorf("not-dismissed: got %d, want 1", len(active))
	}
}

func TestMarkReadAndUnread(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x"})
	ns, _ := s.List(ListFilter{}, 0)
	id := ns[0].ID
	if err := s.MarkRead(id); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(id)
	if !got.Read {
		t.Error("expected Read=true")
	}
	if err := s.MarkUnread(id); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Get(id)
	if got.Read {
		t.Error("expected Read=false")
	}
}

func TestMarkReadNoop(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x"})
	ns, _ := s.List(ListFilter{}, 0)
	id := ns[0].ID
	_ = s.MarkRead(id)
	_ = s.MarkRead(id)
	got, _ := s.Get(id)
	if !got.Read {
		t.Error("should be read")
	}
}

func TestMarkReadNotFound(t *testing.T) {
	s := tempStore(t)
	if err := s.MarkRead("nt-zzzzzz"); err == nil {
		t.Error("expected error for missing")
	}
}

func TestDismiss(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x"})
	ns, _ := s.List(ListFilter{}, 0)
	id := ns[0].ID
	if err := s.Dismiss(id); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(id)
	if !got.Dismissed || !got.Read {
		t.Error("Dismiss should set both Dismissed and Read")
	}
}

func TestClear(t *testing.T) {
	s := tempStore(t)
	for i := 0; i < 5; i++ {
		_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x"})
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Count()
	if c != 0 {
		t.Errorf("after clear: got %d, want 0", c)
	}
}

func TestPrune(t *testing.T) {
	s := tempStore(t)
	old := &Notification{Type: TypeTodoCreated, TodoID: "x", Title: "old", Created: time.Now().Add(-100 * time.Hour)}
	_ = s.Add(old)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "y", Title: "new"})
	n, err := s.Prune(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned: got %d, want 1", n)
	}
}

func TestPruneDefault(t *testing.T) {
	s := tempStore(t)
	if _, err := s.Prune(0); err != nil {
		t.Fatal(err)
	}
}

func TestCountAndUnread(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "1"})
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "y", Title: "2"})
	total, _ := s.Count()
	if total != 2 {
		t.Errorf("total: got %d", total)
	}
	unread, _ := s.CountUnread()
	if unread != 2 {
		t.Errorf("unread: got %d", unread)
	}
	ns, _ := s.List(ListFilter{}, 0)
	_ = s.MarkRead(ns[0].ID)
	unread, _ = s.CountUnread()
	if unread != 1 {
		t.Errorf("unread after read: got %d", unread)
	}
}

func TestComputeStats(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "1"})
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "y", Title: "2"})
	_ = s.Add(&Notification{Type: TypeTodoCompleted, TodoID: "z", Title: "3"})
	st, _ := s.ComputeStats()
	if st.Total != 3 {
		t.Errorf("total: got %d", st.Total)
	}
	if st.ByType[TypeTodoCreated] != 2 {
		t.Errorf("created: got %d", st.ByType[TypeTodoCreated])
	}
	if st.ByType[TypeTodoCompleted] != 1 {
		t.Errorf("completed: got %d", st.ByType[TypeTodoCompleted])
	}
	if st.Unread != 3 {
		t.Errorf("unread: got %d", st.Unread)
	}
}

func TestGenerateIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		n := &Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x"}
		_ = tempStore(t).Add(n)
		if seen[n.ID] {
			t.Errorf("duplicate id: %s", n.ID)
		}
		seen[n.ID] = true
	}
}

func TestKeyOrderAndSort(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "B", Created: now.Add(1 * time.Second)})
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "A", Created: now})
	ns, _ := s.List(ListFilter{}, 0)
	if len(ns) < 2 {
		t.Fatal("expected 2")
	}
	if !ns[0].Created.After(ns[1].Created) {
		t.Error("expected newest first")
	}
}

func TestTUIBroadcasterChannel(t *testing.T) {
	ch := TUIBroadcaster()
drain:
	for {
		select {
		case <-ch:
		default:
			break drain
		}
	}
	d := NewDispatcher(tempStore(t))
	d.Stderr = false
	d.MacOS = false
	n := &Notification{Type: TypeTodoCreated, TodoID: "x", Title: "TUI test"}
	if err := d.Send(n); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-ch:
		if got.Title != "TUI test" {
			t.Errorf("got %q", got.Title)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for TUI channel")
	}
}

func TestTUIBroadcasterNonBlocking(t *testing.T) {
	for i := 0; i < 200; i++ {
		SendTUI(&Notification{Type: TypeTodoCreated, Title: "flood"})
	}
}

func TestDispatcherWebhook(t *testing.T) {
	var got *Notification
	var mu sync.Mutex
	ready := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var n Notification
		_ = json.Unmarshal(body, &n)
		mu.Lock()
		got = &n
		mu.Unlock()
		w.WriteHeader(200)
		select {
		case <-ready:
		default:
			close(ready)
		}
	}))
	defer srv.Close()
	defer srv.CloseClientConnections()

	s := tempStore(t)
	d := NewDispatcher(s)
	d.Stderr = false
	d.MacOS = false
	d.WebhookURL = srv.URL
	_ = d.Send(&Notification{Type: TypeTodoCreated, TodoID: "st-x", Title: "webhook"})

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook never received")
	}
	mu.Lock()
	defer mu.Unlock()
	if got == nil || got.Title != "webhook" {
		t.Errorf("webhook payload: %+v", got)
	}
}

func TestDispatcherStderr(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	s := tempStore(t)
	d := NewDispatcher(s)
	d.MacOS = false
	_ = d.Send(&Notification{Type: TypeTodoCreated, Title: "stderr-test"})
	w.Close()
	os.Stderr = old
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if !strings.Contains(string(buf[:n]), "stderr-test") {
		t.Errorf("stderr: %s", string(buf[:n]))
	}
}

func TestEscape(t *testing.T) {
	cases := map[string]string{
		`hello`:      `hello`,
		`with"quote`: `with\"quote`,
		"with\nline": `with  line`,
		`back\slash`: `back\\slash`,
	}
	for in, want := range cases {
		if got := escape(in); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDispatchNil(t *testing.T) {
	d := NewDispatcher(nil)
	if err := d.Send(nil); err == nil {
		t.Error("expected error for nil notification")
	}
}

func TestListDedupeByID(t *testing.T) {
	s := tempStore(t)
	n := &Notification{Type: TypeTodoCreated, TodoID: "x", Title: "x", ID: "nt-fixed"}
	_ = s.Add(n)
	_ = s.Add(n)
	all, _ := s.List(ListFilter{}, 0)
	count := 0
	for _, x := range all {
		if x.ID == "nt-fixed" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("dedupe: got %d", count)
	}
}

func TestPrintNotifListEmpty(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	printNotifList(nil)
	w.Close()
	os.Stdout = old
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if !strings.Contains(string(buf[:n]), "no notifications") {
		t.Errorf("empty list: %s", string(buf[:n]))
	}
}

func TestPrintJSONNotification(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	_ = printJSON(&Notification{Type: TypeTodoCreated, Title: "json"})
	w.Close()
	os.Stdout = old
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if !bytes.Contains(buf[:n], []byte(`"title": "json"`)) {
		t.Errorf("json output: %s", string(buf[:n]))
	}
}
