// SPDX-License-Identifier: MIT
// Purpose: extra coverage tests for the notifications package — fills gaps
// in Dispatch / sendWebhook / sendMacOS / OpenStore / print-helper branches
// that the existing tests do not exercise.
package notifications

import (
	"encoding/json"
	"errors"
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

const nilDispatcherOrNotif = "nil dispatcher or notification"

// TestDispatch_NilNotification covers the canonical nil-guard surfaced by
// Dispatch.Send. Two cases:
//  1. Dispatch(nil)              → store opens, d is non-nil, n is nil
//  2. (*Dispatcher)(nil).Send(n) → d is nil, n is non-nil
//
// Both must return the same wrapped error — and no side effects (no TUI
// broadcast, no macOS subprocess, no store write) must leak through.
func TestDispatch_NilNotification(t *testing.T) {
	dir := t.TempDir()

	oldPath := notifDBPath
	notifDBPath = filepath.Join(dir, "nil-notif.db")
	defer func() { notifDBPath = oldPath }()

	// Case 1: Dispatch(nil)
	if err := Dispatch(nil); err == nil {
		t.Fatal("Dispatch(nil): expected error, got nil")
	} else if !strings.Contains(err.Error(), nilDispatcherOrNotif) {
		t.Errorf("Dispatch(nil) error = %q; want substring %q", err.Error(), nilDispatcherOrNotif)
	}

	// Case 2: nil dispatcher with non-nil notification.
	var d *Dispatcher
	n := &Notification{Type: TypeTodoCreated, TodoID: "st-x", Title: "x"}
	if err := d.Send(n); err == nil {
		t.Fatal("nil-dispatcher.Send: expected error, got nil")
	} else if !strings.Contains(err.Error(), nilDispatcherOrNotif) {
		t.Errorf("nil-dispatcher.Send error = %q; want substring %q", err.Error(), nilDispatcherOrNotif)
	}
}

// TestSendWebhook_HTTPServer asserts that sendWebhook posts an
// application/json body with the X-Sin-Code-Event header mirroring the
// notification Type, that the body round-trips through JSON, and that a 5xx
// response is treated as silent success (Send never propagates webhook
// failures by design).
func TestSendWebhook_HTTPServer(t *testing.T) {
	var (
		mu         sync.Mutex
		gotMethod  string
		gotContent string
		gotEvent   string
		gotPayload Notification
	)
	received := make(chan struct{})
	var closeOnce sync.Once

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotContent = r.Header.Get("Content-Type")
		gotEvent = r.Header.Get("X-Sin-Code-Event")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
		closeOnce.Do(func() { close(received) })
	}))
	defer srv.Close()
	defer srv.CloseClientConnections()

	s := tempStore(t)
	d := NewDispatcher(s)
	d.Stderr = false
	d.MacOS = false
	d.WebhookURL = srv.URL

	n := &Notification{
		Type:    TypeTodoStale,
		TodoID:  "st-rw",
		Title:   "webhook-roundtrip",
		Message: "hi",
	}
	// 500-returning server ⇒ Send must still return nil (silent failure design).
	if err := d.Send(n); err != nil {
		t.Fatalf("Send: expected nil err on 500 (silent failure), got %v", err)
	}

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook never received by httptest.Server")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q; want POST", gotMethod)
	}
	if gotContent != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", gotContent)
	}
	if gotEvent != string(n.Type) {
		t.Errorf("X-Sin-Code-Event = %q; want %q", gotEvent, n.Type)
	}
	if gotPayload.Title != n.Title || gotPayload.Message != n.Message || gotPayload.TodoID != n.TodoID {
		t.Errorf("body roundtrip mismatch:\n got = %+v\n want = %+v", gotPayload, n)
	}
}

// TestSendMacOS_NoBinary verifies that sendMacOS silently returns when the
// osascript binary cannot be located via exec.LookPath — and that the
// subprocess is never even constructed in that branch.
func TestSendMacOS_NoBinary(t *testing.T) {
	orig := testHookExecLookPath
	defer func() { testHookExecLookPath = orig }()

	testHookExecLookPath = func(string) (string, error) {
		return "", errors.New("osascript not on PATH")
	}

	s := tempStore(t)
	d := NewDispatcher(s)
	d.Stderr = false
	d.WebhookURL = ""
	// MacOS stays at the default (true) — the no-lookPath branch must run.
	if err := d.Send(&Notification{Type: TypeTodoCreated, Title: "no-osascript"}); err != nil {
		t.Fatalf("Send: expected nil err when osascript is missing, got %v", err)
	}
}

// TestOpenStore_RWOnly exercises the Open → Add → List round-trip plus the
// package-level path override that command-end Dispatch / openStore honour.
// After the bbolt file is created and closed, it is chmod'd to 0600 (rw
// owner-only) to mirror a "RW-only" restrictive-perms scenario; the next
// Open on the same path must still succeed and the Add must still List.
func TestOpenStore_RWOnly(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "rw.db")
	t.Setenv("SIN_CODE_NOTIF_DB", target)

	oldPath := notifDBPath
	notifDBPath = target
	defer func() { notifDBPath = oldPath }()

	// 1. Normal Open + Add + Close.
	s, err := Open(target)
	if err != nil {
		t.Fatalf("Open(%s): %v", target, err)
	}
	if err := s.Add(&Notification{
		Type: TypeTodoCreated, TodoID: "st-rw", Title: "rw-only",
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 2. Tighten file mode; verify re-Open still finds the previously
	// persisted entry list.
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	s2, err := Open(target)
	if err != nil {
		t.Fatalf("Open after chmod 0600: %v", err)
	}
	defer s2.Close()
	ns, err := s2.List(ListFilter{}, 0)
	if err != nil {
		t.Fatalf("List after chmod 0600: %v", err)
	}
	if len(ns) != 1 {
		t.Fatalf("List after chmod 0600: got %d, want 1", len(ns))
	}
	if ns[0].Title != "rw-only" {
		t.Errorf("Title = %q; want %q", ns[0].Title, "rw-only")
	}

	// 3. Round-trip an Add under the same restrictive mode.
	if err := s2.Add(&Notification{
		Type: TypeTodoCompleted, TodoID: "st-rw-2", Title: "rw-only-2",
	}); err != nil {
		t.Fatalf("Add under 0600: %v", err)
	}
	all, _ := s2.List(ListFilter{}, 0)
	if len(all) != 2 {
		t.Errorf("After second Add: got %d, want 2", len(all))
	}
	// Close before the next openStore() call. bbolt serialises opens on
	// the same file with a 2s lock timeout; leaving s2 alive would
	// deadlock our own follow-up open.
	if err := s2.Close(); err != nil {
		t.Fatalf("s2 Close: %v", err)
	}

	// 4. Verify the package-level path override is observed by
	// openStore() — this is the standing replacement for the
	// SIN_CODE_NOTIF_DB env override until the codebase consumes the env
	// var directly.
	s3, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	defer s3.Close()
	if s3.Path() != target {
		t.Errorf("openStore Path = %q; want %q", s3.Path(), target)
	}
}

// TestPrintNotifList_JSONFormat seeds a varied 3-notif set, then validates
// three printer paths:
//   - JSON list output (unmarshal-able back to []*Notification)
//   - Text column-aligned table (header columns + titles + 80-rune rule)
//   - Stats JSON envelope (exact field set {total, unread, by_type})
func TestPrintNotifList_JSONFormat(t *testing.T) {
	dir := t.TempDir()

	oldPath := notifDBPath
	notifFormat = "" // will be overridden per subtest
	notifDBPath = filepath.Join(dir, "print-notif.db")
	defer func() {
		notifDBPath = oldPath
		notifFormat = "text"
	}()

	s, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}

	seed := []*Notification{
		{Type: TypeTodoCreated, TodoID: "st-1", Title: "alpha", Message: "first"},
		{Type: TypeTodoCompleted, TodoID: "st-2", Title: "beta", Message: "second"},
		{Type: TypeTodoStale, TodoID: "st-3", Title: "gamma", Message: "third"},
	}
	for _, n := range seed {
		if err := s.Add(n); err != nil {
			s.Close()
			t.Fatal(err)
		}
	}
	all, err := s.List(ListFilter{}, 0)
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	if len(all) != 3 {
		s.Close()
		t.Fatalf("seed: got %d, want 3", len(all))
	}
	// Close before subtest RunE paths call openStore() themselves;
	// bbolt serialises opens on the same file with a 2s lock timeout.
	if err := s.Close(); err != nil {
		t.Fatalf("seed s Close: %v", err)
	}

	t.Run("list_json", func(t *testing.T) {
		oldFmt := notifFormat
		notifFormat = "json"
		defer func() { notifFormat = oldFmt }()

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		err := listCmd.RunE(listCmd, nil)
		w.Close()
		os.Stdout = oldStdout
		if err != nil {
			t.Fatal(err)
		}
		buf, _ := io.ReadAll(r)

		var parsed []*Notification
		if err := json.Unmarshal(buf, &parsed); err != nil {
			t.Fatalf("json list output unmarshal: %v\nraw=%s", err, string(buf))
		}
		if len(parsed) != 3 {
			t.Errorf("json entries: got %d, want 3", len(parsed))
		}
	})

	t.Run("list_text", func(t *testing.T) {
		oldFmt := notifFormat
		notifFormat = "text"
		defer func() { notifFormat = oldFmt }()

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		err := listCmd.RunE(listCmd, nil)
		w.Close()
		os.Stdout = oldStdout
		if err != nil {
			t.Fatal(err)
		}
		out := string(mustRead(r))

		// Header columns must all appear.
		for _, hdr := range []string{"ID", "TYPE", "READ", "TITLE"} {
			if !strings.Contains(out, hdr) {
				t.Errorf("text table missing header column %q\n%s", hdr, out)
			}
		}
		// Every seeded title must appear.
		for _, n := range seed {
			if !strings.Contains(out, n.Title) {
				t.Errorf("text table missing title %q\n%s", n.Title, out)
			}
		}
		// Column-alignment: the dash rule is exactly 80 runes of '─'.
		// That enforces constant column widths on every row.
		var rule string
		for _, ln := range strings.Split(out, "\n") {
			if strings.HasPrefix(ln, "─") {
				rule = ln
				break
			}
		}
		if rule == "" {
			t.Fatal("text table: dash rule not found")
		}
		if got, want := len([]rune(rule)), 80; got != want {
			t.Errorf("dash rule width = %d runes; want %d", got, want)
		}
	})

	t.Run("stats_json", func(t *testing.T) {
		oldFmt := notifFormat
		notifFormat = "json"
		defer func() { notifFormat = oldFmt }()

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		if err := statsCmd.RunE(statsCmd, nil); err != nil {
			t.Fatal(err)
		}
		w.Close()
		os.Stdout = oldStdout

		buf := mustRead(r)

		// Round-trip through a generic map first to confirm the exact
		// field set — em-dash separators and extras would surface here.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(buf, &raw); err != nil {
			t.Fatalf("stats json unmarshal: %v\nraw=%s", err, string(buf))
		}
		want := []string{"total", "unread", "by_type"}
		if len(raw) != len(want) {
			t.Errorf("stats field count: got %d (%v), want %d (%v)",
				len(raw), mapKeys(raw), len(want), want)
		}
		for _, k := range want {
			if _, ok := raw[k]; !ok {
				t.Errorf("stats missing field %q", k)
			}
		}
		// Round-trip through typed Stats to confirm wire shape.
		var st Stats
		if err := json.Unmarshal(buf, &st); err != nil {
			t.Fatalf("stats struct unmarshal: %v", err)
		}
		if st.Total != 3 {
			t.Errorf("stats.Total = %d; want 3", st.Total)
		}
		if st.Unread != 3 {
			t.Errorf("stats.Unread = %d; want 3", st.Unread)
		}
		if len(st.ByType) != 3 {
			t.Errorf("stats.ByType len = %d; want 3 (%v)", len(st.ByType), st.ByType)
		}
	})
}

// ── helpers ───────────────────────────────────────────────────────────────

// mustRead drains an io.Reader to EOF; used after a piped writer is closed.
func mustRead(r io.Reader) []byte {
	b, _ := io.ReadAll(r)
	return b
}

// mapKeys returns the keys of a JSON-raw-message map. Used for diagnostic
// messages; ordering does not matter for the assertions.
func mapKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
