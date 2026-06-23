// SPDX-License-Identifier: MIT
//go:build coverage
// Purpose: coverage tests for the remaining branches in the notifications package.
package notifications

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// withTempDB sets notifDBPath to a temp DB and returns a cleanup function that
// restores the previous value and closes the store if it was opened by the
// command under test.
func withTempDB(t *testing.T) func() {
	t.Helper()
	old := notifDBPath
	dir := t.TempDir()
	notifDBPath = filepath.Join(dir, "notifications.db")
	return func() { notifDBPath = old }
}

func withInvalidDB(t *testing.T) func() {
	t.Helper()
	old := notifDBPath
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	notifDBPath = filepath.Join(blocker, "sub", "notifications.db")
	return func() { notifDBPath = old }
}

func seedDB(t *testing.T) []string {
	t.Helper()
	s, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "st-1", Title: "A", Message: "msg A"})
	_ = s.Add(&Notification{Type: TypeTodoCompleted, TodoID: "st-2", Title: "B", Message: "msg B"})
	ns, _ := s.List(ListFilter{}, 0)
	ids := make([]string, len(ns))
	for i, n := range ns {
		ids[i] = n.ID
	}
	return ids
}

// ── openStore / Dispatch helpers ───────────────────────────────────────────

func TestOpenStore(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	s, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}

func TestOpenStoreError(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	_, err := openStore()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDispatch(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	n := &Notification{Type: TypeTodoCreated, TodoID: "st-1", Title: "dispatch", Message: "m"}
	if err := Dispatch(n); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchWebhookAndFlags(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	oldWebhook := notifWebhook
	oldNoMac := notifNoMac
	oldNoStderr := notifNoStderr
	notifWebhook = "http://example.com/webhook"
	notifNoMac = true
	notifNoStderr = true
	defer func() {
		notifWebhook = oldWebhook
		notifNoMac = oldNoMac
		notifNoStderr = oldNoStderr
	}()

	if err := Dispatch(&Notification{Type: TypeTodoCreated, Title: "flags"}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchOpenError(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	if err := Dispatch(&Notification{Type: TypeTodoCreated, Title: "x"}); err == nil {
		t.Fatal("expected error")
	}
}

// ── print helpers ──────────────────────────────────────────────────────────

func TestPrintNotifListNonEmpty(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	printNotifList([]*Notification{
		{ID: "nt-1", Type: TypeTodoCreated, Title: "A", Read: true},
		{ID: "nt-2", Type: TypeTodoCompleted, Title: "B"},
	})
	w.Close()
	os.Stdout = old
	buf, _ := io.ReadAll(r)
	out := string(buf)
	if !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Errorf("output: %s", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("expected read marker, got: %s", out)
	}
}

// ── CLI commands ───────────────────────────────────────────────────────────

func TestListCmd(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	_ = seedDB(t)
	cmd := listCmd
	_ = cmd.Flags().Set("limit", "10")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestListCmdJSON(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	_ = seedDB(t)
	old := notifFormat
	notifFormat = "json"
	defer func() { notifFormat = old }()
	cmd := listCmd
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestListCmdUnread(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	ids := seedDB(t)
	{
		s, err := openStore()
		if err != nil {
			t.Fatal(err)
		}
		_ = s.MarkRead(ids[0])
		_ = s.Close()
	}
	cmd := listCmd
	_ = cmd.Flags().Set("unread", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestListCmdOpenError(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	if err := listCmd.RunE(listCmd, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestReadCmd(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	ids := seedDB(t)
	cmd := readCmd
	if err := cmd.RunE(cmd, []string{ids[0]}); err != nil {
		t.Fatal(err)
	}
}

func TestReadCmdErrors(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	if err := readCmd.RunE(readCmd, []string{"nt-1"}); err == nil {
		t.Fatal("expected error")
	}
	cleanup2 := withTempDB(t)
	defer cleanup2()
	if err := readCmd.RunE(readCmd, []string{"nt-zzzzzz"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUnreadCmd(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	ids := seedDB(t)
	{
		s, err := openStore()
		if err != nil {
			t.Fatal(err)
		}
		_ = s.MarkRead(ids[0])
		_ = s.Close()
	}
	cmd := unreadCmd
	if err := cmd.RunE(cmd, []string{ids[0]}); err != nil {
		t.Fatal(err)
	}
}

func TestUnreadCmdErrors(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	if err := unreadCmd.RunE(unreadCmd, []string{"nt-1"}); err == nil {
		t.Fatal("expected error")
	}
	cleanup2 := withTempDB(t)
	defer cleanup2()
	if err := unreadCmd.RunE(unreadCmd, []string{"nt-zzzzzz"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDismissCmd(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	ids := seedDB(t)
	cmd := dismissCmd
	if err := cmd.RunE(cmd, []string{ids[0]}); err != nil {
		t.Fatal(err)
	}
}

func TestDismissCmdErrors(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	if err := dismissCmd.RunE(dismissCmd, []string{"nt-1"}); err == nil {
		t.Fatal("expected error")
	}
	cleanup2 := withTempDB(t)
	defer cleanup2()
	if err := dismissCmd.RunE(dismissCmd, []string{"nt-zzzzzz"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestListenCmd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := listenCmd
	cmd.SetContext(ctx)
	done := make(chan error, 1)
	go func() {
		done <- cmd.RunE(cmd, nil)
	}()
	SendTUI(&Notification{Type: TypeTodoCreated, Title: "listen"})
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listen command did not stop")
	}
}

func TestClearCmd(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	_ = seedDB(t)
	cmd := clearCmd
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestClearCmdError(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	if err := clearCmd.RunE(clearCmd, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestPruneCmd(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	s, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "old", Created: time.Now().Add(-200 * time.Hour)})
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "y", Title: "new"})
	_ = s.Close()
	cmd := pruneCmd
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPruneCmdError(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	if err := pruneCmd.RunE(pruneCmd, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestStatsCmd(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	_ = seedDB(t)
	cmd := statsCmd
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStatsCmdJSON(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	_ = seedDB(t)
	old := notifFormat
	notifFormat = "json"
	defer func() { notifFormat = old }()
	cmd := statsCmd
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStatsCmdError(t *testing.T) {
	cleanup := withInvalidDB(t)
	defer cleanup()
	if err := statsCmd.RunE(statsCmd, nil); err == nil {
		t.Fatal("expected error")
	}
}

// ── dispatch edge cases ────────────────────────────────────────────────────

func TestDispatcherNil(t *testing.T) {
	var d *Dispatcher
	if err := d.Send(&Notification{Type: TypeTodoCreated}); err == nil {
		t.Fatal("expected error for nil dispatcher")
	}
}

func TestDispatcherWebhookNewRequestError(t *testing.T) {
	orig := testHookHTTPNewRequest
	testHookHTTPNewRequest = func(string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("bad request")
	}
	defer func() { testHookHTTPNewRequest = orig }()

	d := NewDispatcher(tempStore(t))
	d.Stderr = false
	d.MacOS = false
	d.WebhookURL = "http://example.com"
	if err := d.Send(&Notification{Type: TypeTodoCreated, Title: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherWebhookHTTPDoError(t *testing.T) {
	d := NewDispatcher(tempStore(t))
	d.Stderr = false
	d.MacOS = false
	d.WebhookURL = "http://example.com"
	d.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport failed")
		}),
	}
	if err := d.Send(&Notification{Type: TypeTodoCreated, Title: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherMacOSNoosascript(t *testing.T) {
	orig := testHookExecLookPath
	testHookExecLookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}
	defer func() { testHookExecLookPath = orig }()

	d := NewDispatcher(tempStore(t))
	d.Stderr = false
	d.WebhookURL = ""
	if err := d.Send(&Notification{Type: TypeTodoCreated, Title: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherNilHTTPClient(t *testing.T) {
	d := NewDispatcher(tempStore(t))
	d.Stderr = false
	d.MacOS = false
	d.WebhookURL = "http://example.com"
	d.HTTPClient = nil
	if err := d.Send(&Notification{Type: TypeTodoCreated, Title: "x"}); err != nil {
		t.Fatal(err)
	}
}

// roundTripperFunc adapts a function to http.RoundTripper for tests.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// ── helpers / store error branches ─────────────────────────────────────────

func TestDefaultConfigDirError(t *testing.T) {
	orig := testHookUserConfigDir
	testHookUserConfigDir = func() (string, error) {
		return "", errors.New("no config dir")
	}
	defer func() { testHookUserConfigDir = orig }()

	if _, err := Open(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestBoltOpenError(t *testing.T) {
	orig := testHookBoltOpen
	testHookBoltOpen = func(string, os.FileMode, *bolt.Options) (*bolt.DB, error) {
		return nil, errors.New("bolt open failed")
	}
	defer func() { testHookBoltOpen = orig }()

	if _, err := Open(filepath.Join(t.TempDir(), "n.db")); err == nil {
		t.Fatal("expected error")
	}
}

func TestAddJSONMarshalError(t *testing.T) {
	orig := testHookJSONMarshalStore
	testHookJSONMarshalStore = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	defer func() { testHookJSONMarshalStore = orig }()

	s := tempStore(t)
	if err := s.Add(&Notification{Type: TypeTodoCreated, Title: "x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestMarkReadOnClosedDB(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x"})
	n, _ := s.List(ListFilter{}, 0)
	_ = s.Close()
	if err := s.MarkRead(n[0].ID); err == nil {
		t.Fatal("expected error")
	}
}

func TestMarkUnreadOnClosedDB(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x"})
	n, _ := s.List(ListFilter{}, 0)
	_ = s.Close()
	if err := s.MarkUnread(n[0].ID); err == nil {
		t.Fatal("expected error")
	}
}

func TestDismissOnClosedDB(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x"})
	n, _ := s.List(ListFilter{}, 0)
	_ = s.Close()
	if err := s.Dismiss(n[0].ID); err == nil {
		t.Fatal("expected error")
	}
}

func TestClearOnClosedDB(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x"})
	_ = s.Close()
	if err := s.Clear(); err == nil {
		t.Fatal("expected error")
	}
}

func TestPruneOnClosedDB(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x"})
	_ = s.Close()
	if _, err := s.Prune(24 * time.Hour); err == nil {
		t.Fatal("expected error")
	}
}

func TestCountOnClosedDB(t *testing.T) {
	s := tempStore(t)
	_ = s.Close()
	if _, err := s.Count(); err == nil {
		t.Fatal("expected error")
	}
}

func TestCountUnreadOnClosedDB(t *testing.T) {
	s := tempStore(t)
	_ = s.Close()
	if _, err := s.CountUnread(); err == nil {
		t.Fatal("expected error")
	}
}

func TestComputeStatsOnClosedDB(t *testing.T) {
	s := tempStore(t)
	_ = s.Close()
	if _, err := s.ComputeStats(); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateNotifJSONMarshalError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x"})
	orig := testHookJSONMarshalStore
	testHookJSONMarshalStore = func(any) ([]byte, error) {
		return nil, errors.New("marshal failed")
	}
	defer func() { testHookJSONMarshalStore = orig }()

	n, _ := s.List(ListFilter{}, 0)
	if err := s.MarkRead(n[0].ID); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteIndexNilBucket(t *testing.T) {
	dir := t.TempDir()
	db, err := bolt.Open(filepath.Join(dir, "x.db"), 0o644, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_ = db.Update(func(tx *bolt.Tx) error {
		_, _ = tx.CreateBucketIfNotExists([]byte("existing"))
		writeIndex(tx, "missing", "k", "id")
		writeIndex(tx, "existing", "", "id")
		return nil
	})
}

func TestListCorruptValue(t *testing.T) {
	s := tempStore(t)
	_ = s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketNotifs)).Put(key(time.Now().UTC(), "bad"), []byte("not json"))
	})
	ns, err := s.List(ListFilter{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 0 {
		t.Fatalf("expected 0, got %d", len(ns))
	}
}

func TestJsonMarshalDirect(t *testing.T) {
	b, err := jsonMarshal(&Notification{Type: TypeTodoCreated, Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "todo_created") {
		t.Errorf("unexpected: %s", string(b))
	}
}

func TestListWithTypeAndLimit(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, TodoID: "x", Title: "a"})
	_ = s.Add(&Notification{Type: TypeTodoCompleted, TodoID: "y", Title: "b"})
	ns, _ := s.List(ListFilter{Type: TypeTodoCreated}, 1)
	if len(ns) != 1 {
		t.Fatalf("expected 1, got %d", len(ns))
	}
	if ns[0].Type != TypeTodoCreated {
		t.Errorf("unexpected type: %s", ns[0].Type)
	}
}

func TestStoreCloseNil(t *testing.T) {
	var s *Store
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreGetNotFound(t *testing.T) {
	s := tempStore(t)
	_, err := s.Get("nt-missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStorePath(t *testing.T) {
	s := tempStore(t)
	if s.Path() == "" {
		t.Error("expected path")
	}
}

func TestClearBucketDeleteError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x"})
	_ = s.Close()
	if err := s.Clear(); err == nil {
		t.Fatal("expected error")
	}
}

// ── remaining coverage tests ─────────────────────────────────────────────────

func makeClosedStore(t *testing.T) *Store {
	s := tempStore(t)
	_ = s.Close()
	return s
}

func TestListCmdListError(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	orig := testHookOpenStore
	testHookOpenStore = func(path string) (*Store, error) {
		return makeClosedStore(t), nil
	}
	defer func() { testHookOpenStore = orig }()
	if err := listCmd.RunE(listCmd, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestMarkUnreadAlreadyUnread(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x", ID: "nt-1"})
	if err := s.MarkUnread("nt-1"); err != nil {
		t.Fatal(err)
	}
}

func TestDismissAlreadyRead(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x", ID: "nt-1", Read: true})
	if err := s.Dismiss("nt-1"); err != nil {
		t.Fatal(err)
	}
}

func TestReadCmdMarkReadError(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	orig := testHookOpenStore
	testHookOpenStore = func(path string) (*Store, error) {
		return makeClosedStore(t), nil
	}
	defer func() { testHookOpenStore = orig }()
	if err := readCmd.RunE(readCmd, []string{"nt-1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUnreadCmdMarkUnreadError(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	orig := testHookOpenStore
	testHookOpenStore = func(path string) (*Store, error) {
		return makeClosedStore(t), nil
	}
	defer func() { testHookOpenStore = orig }()
	if err := unreadCmd.RunE(unreadCmd, []string{"nt-1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDismissCmdDismissError(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	orig := testHookOpenStore
	testHookOpenStore = func(path string) (*Store, error) {
		return makeClosedStore(t), nil
	}
	defer func() { testHookOpenStore = orig }()
	if err := dismissCmd.RunE(dismissCmd, []string{"nt-1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestClearCmdClearError(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	orig := testHookOpenStore
	testHookOpenStore = func(path string) (*Store, error) {
		return makeClosedStore(t), nil
	}
	defer func() { testHookOpenStore = orig }()
	if err := clearCmd.RunE(clearCmd, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestPruneCmdPruneError(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	orig := testHookOpenStore
	testHookOpenStore = func(path string) (*Store, error) {
		return makeClosedStore(t), nil
	}
	defer func() { testHookOpenStore = orig }()
	if err := pruneCmd.RunE(pruneCmd, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestStatsCmdComputeStatsError(t *testing.T) {
	cleanup := withTempDB(t)
	defer cleanup()
	orig := testHookOpenStore
	testHookOpenStore = func(path string) (*Store, error) {
		return makeClosedStore(t), nil
	}
	defer func() { testHookOpenStore = orig }()
	if err := statsCmd.RunE(statsCmd, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenCreateBucketError(t *testing.T) {
	orig := testHookCreateBucket
	testHookCreateBucket = func(*bolt.Tx, []byte) (*bolt.Bucket, error) {
		return nil, errors.New("bucket failed")
	}
	defer func() { testHookCreateBucket = orig }()
	if _, err := Open(filepath.Join(t.TempDir(), "n.db")); err == nil {
		t.Fatal("expected error")
	}
}

func TestAddPutError(t *testing.T) {
	orig := testHookAddPut
	testHookAddPut = func(*bolt.Bucket, []byte, []byte) error { return errors.New("put failed") }
	defer func() { testHookAddPut = orig }()

	s := tempStore(t)
	if err := s.Add(&Notification{Type: TypeTodoCreated, Title: "x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetUnmarshalError(t *testing.T) {
	s := tempStore(t)
	_ = s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketNotifs)).Put(key(time.Now().UTC(), "bad"), []byte("not json"))
	})
	_, err := s.Get("bad")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateNotifUnmarshalError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x", ID: "nt-ok"})
	_ = s.db.Update(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(bucketNotifs)).Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			_ = c.Delete()
		}
		return tx.Bucket([]byte(bucketNotifs)).Put(key(time.Now().UTC(), "nt-ok"), []byte("not json"))
	})
	if err := s.MarkRead("nt-ok"); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateNotifIDMismatch(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x", ID: "nt-1"})
	if err := s.MarkRead("nt-2"); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateNotifPutError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x", ID: "nt-1"})
	orig := testHookUpdatePut
	testHookUpdatePut = func(*bolt.Bucket, []byte, []byte) error { return errors.New("put failed") }
	defer func() { testHookUpdatePut = orig }()
	if err := s.MarkRead("nt-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateNotifDeleteUnreadError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x", ID: "nt-1"})
	orig := testHookUpdateDeleteUnread
	testHookUpdateDeleteUnread = func(*bolt.Bucket, []byte) error { return errors.New("delete failed") }
	defer func() { testHookUpdateDeleteUnread = orig }()
	if err := s.MarkRead("nt-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPruneDeleteError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "old", ID: "nt-old", Created: time.Now().Add(-200 * time.Hour)})
	orig := testHookPruneDelete
	testHookPruneDelete = func(*bolt.Cursor) error { return errors.New("delete failed") }
	defer func() { testHookPruneDelete = orig }()
	if _, err := s.Prune(24 * time.Hour); err == nil {
		t.Fatal("expected error")
	}
}

func TestPruneUnmarshalError(t *testing.T) {
	s := tempStore(t)
	_ = s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketNotifs)).Put(key(time.Now().Add(-200*time.Hour).UTC(), "bad"), []byte("not json"))
	})
	n, err := s.Prune(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestPruneCountIncrement(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "old", Created: time.Now().Add(-200 * time.Hour)})
	n, err := s.Prune(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
}

func TestPruneDismissed(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "d", Dismissed: true})
	n, err := s.Prune(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}
}

func TestClearForEachError(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Notification{Type: TypeTodoCreated, Title: "x"})
	orig := testHookClearDelete
	testHookClearDelete = func(*bolt.Bucket, []byte) error { return errors.New("delete failed") }
	defer func() { testHookClearDelete = orig }()
	if err := s.Clear(); err == nil {
		t.Fatal("expected error")
	}
}

func TestListenCmdNilContextAndClosedChannel(t *testing.T) {
	tuiChanMu.Lock()
	oldCh := tuiChan
	tuiChan = make(chan *Notification, 100)
	close(tuiChan)
	tuiChanMu.Unlock()
	defer func() {
		tuiChanMu.Lock()
		tuiChan = oldCh
		tuiChanMu.Unlock()
	}()
	cmd := listenCmd
	cmd.SetContext(nil)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestListenCmdEncodeError(t *testing.T) {
	tuiChanMu.Lock()
	oldCh := tuiChan
	tuiChan = make(chan *Notification, 100)
	tuiChanMu.Unlock()
	defer func() {
		tuiChanMu.Lock()
		tuiChan = oldCh
		tuiChanMu.Unlock()
	}()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()
	_ = w.Close()

	cmd := listenCmd
	cmd.SetContext(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- cmd.RunE(cmd, nil)
	}()
	SendTUI(&Notification{Type: TypeTodoCreated, Title: "encode-err"})
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listen command did not stop")
	}
	// drain the pipe to avoid goroutine leak
	_, _ = io.ReadAll(r)
}
