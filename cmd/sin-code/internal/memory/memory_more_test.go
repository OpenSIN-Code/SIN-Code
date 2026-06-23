// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for the memory package. These hit the
// error branches and edge cases that require package-level hooks.
// Docs: memory.doc.md
package memory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// roundTripperFunc is a thin adapter so tests can inject arbitrary HTTP
// responses without standing up a real server.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNoopEmbedding(t *testing.T) {
	v, err := NoopEmbedding("anything")
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Fatalf("expected nil vector, got %v", v)
	}
}

func TestGetenvDefault(t *testing.T) {
	if got := getenv("SIN_CODE_MEMORY_TEST_DEFAULT", "fallback"); got != "fallback" {
		t.Fatalf("expected default, got %q", got)
	}
}

func TestOpenDefaultDirError(t *testing.T) {
	orig := osUserConfigDir
	osUserConfigDir = func() (string, error) { return "", errors.New("no config dir") }
	t.Cleanup(func() { osUserConfigDir = orig })
	if _, err := Open(""); err == nil || !strings.Contains(err.Error(), "no config dir") {
		t.Fatalf("expected config dir error, got %v", err)
	}
}

func TestOpenMkdirError(t *testing.T) {
	orig := osMkdirAll
	osMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir boom") }
	t.Cleanup(func() { osMkdirAll = orig })
	if _, err := Open(filepath.Join(t.TempDir(), "sub", "memory.db")); err == nil || !strings.Contains(err.Error(), "mkdir") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestOpenBoltOpenError(t *testing.T) {
	orig := boltOpen
	boltOpen = func(string, os.FileMode, *bolt.Options) (*bolt.DB, error) { return nil, errors.New("open boom") }
	t.Cleanup(func() { boltOpen = orig })
	if _, err := Open(filepath.Join(t.TempDir(), "memory.db")); err == nil || !strings.Contains(err.Error(), "open boom") {
		t.Fatalf("expected bolt open error, got %v", err)
	}
}

func TestOpenInitError(t *testing.T) {
	orig := dbUpdate
	dbUpdate = func(*bolt.DB, func(*bolt.Tx) error) error { return errors.New("init boom") }
	t.Cleanup(func() { dbUpdate = orig })
	if _, err := Open(filepath.Join(t.TempDir(), "memory.db")); err == nil || !strings.Contains(err.Error(), "init boom") {
		t.Fatalf("expected init error, got %v", err)
	}
}

func TestCloseNil(t *testing.T) {
	var s *Store
	if err := s.Close(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestAddMarshalError(t *testing.T) {
	s := tempStore(t)
	orig := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal boom") }
	t.Cleanup(func() { jsonMarshal = orig })
	if err := s.Add(&Memory{Insight: "x"}); err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestAddPutError(t *testing.T) {
	s := tempStore(t)
	orig := putErrHook
	putErrHook = func(name string) error {
		if name == bucketMems {
			return errors.New("put boom")
		}
		return nil
	}
	t.Cleanup(func() { putErrHook = orig })
	if err := s.Add(&Memory{Insight: "x"}); err == nil || !strings.Contains(err.Error(), "put boom") {
		t.Fatalf("expected put error, got %v", err)
	}
}

func TestAddEmbeddingPutError(t *testing.T) {
	old, oldDim := GetEmbedder()
	defer SetEmbedder(old, oldDim)
	SetEmbedder(func(string) ([]float32, error) { return []float32{0.1}, nil }, 1)

	s := tempStore(t)
	orig := putErrHook
	putErrHook = func(name string) error {
		if name == bucketEmbeddings {
			return errors.New("emb put boom")
		}
		return nil
	}
	t.Cleanup(func() { putErrHook = orig })
	if err := s.Add(&Memory{Insight: "x"}); err == nil || !strings.Contains(err.Error(), "emb put boom") {
		t.Fatalf("expected embedding put error, got %v", err)
	}
}

func TestAddAuditError(t *testing.T) {
	s := tempStore(t)
	orig := putErrHook
	putErrHook = func(name string) error {
		if name == bucketAudit {
			return errors.New("audit put boom")
		}
		return nil
	}
	t.Cleanup(func() { putErrHook = orig })
	if err := s.Add(&Memory{Insight: "x"}); err == nil || !strings.Contains(err.Error(), "audit put boom") {
		t.Fatalf("expected audit put error, got %v", err)
	}
}

func TestAuditNextSeqError(t *testing.T) {
	s := tempStore(t)
	orig := nextSeqErrHook
	nextSeqErrHook = func(name string) error {
		if name == bucketAudit {
			return errors.New("seq boom")
		}
		return nil
	}
	t.Cleanup(func() { nextSeqErrHook = orig })
	if err := s.Add(&Memory{Insight: "x"}); err == nil || !strings.Contains(err.Error(), "seq boom") {
		t.Fatalf("expected audit seq error, got %v", err)
	}
}

func TestGetUnmarshalError(t *testing.T) {
	s := tempStore(t)
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketMems)).Put(memKey("bad"), []byte("not json"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("bad"); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected unmarshal error, got %v", err)
	}
}

func TestListViewError(t *testing.T) {
	s := tempStore(t)
	orig := dbView
	dbView = func(*bolt.DB, func(*bolt.Tx) error) error { return errors.New("view boom") }
	t.Cleanup(func() { dbView = orig })
	if _, err := s.List(ListFilter{}); err == nil || !strings.Contains(err.Error(), "view boom") {
		t.Fatalf("expected view error, got %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := tempStore(t)
	if err := s.Delete("missing", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.Delete("missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound hard, got %v", err)
	}
}

func TestDeleteSoftUnmarshalError(t *testing.T) {
	s := tempStore(t)
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketMems)).Put(memKey("bad"), []byte("not json"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("bad", false); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected unmarshal error, got %v", err)
	}
}

func TestDeleteMemoryPutError(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "soft"}
	if err := s.Add(m); err != nil {
		t.Fatal(err)
	}
	orig := putErrHook
	putErrHook = func(name string) error {
		if name == bucketMems {
			return errors.New("put boom")
		}
		return nil
	}
	t.Cleanup(func() { putErrHook = orig })
	if err := s.Delete(m.ID, false); err == nil || !strings.Contains(err.Error(), "put boom") {
		t.Fatalf("expected put error, got %v", err)
	}
}

func TestDeleteAuditError(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "soft"}
	if err := s.Add(m); err != nil {
		t.Fatal(err)
	}
	orig := putErrHook
	putErrHook = func(name string) error {
		if name == bucketAudit {
			return errors.New("audit put boom")
		}
		return nil
	}
	t.Cleanup(func() { putErrHook = orig })
	if err := s.Delete(m.ID, false); err == nil || !strings.Contains(err.Error(), "audit put boom") {
		t.Fatalf("expected audit error, got %v", err)
	}
}

func TestDeleteHardDeleteError(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "hard"}
	if err := s.Add(m); err != nil {
		t.Fatal(err)
	}
	orig := deleteErrHook
	deleteErrHook = func(name string) error {
		if name == bucketMems {
			return errors.New("delete boom")
		}
		return nil
	}
	t.Cleanup(func() { deleteErrHook = orig })
	if err := s.Delete(m.ID, true); err == nil || !strings.Contains(err.Error(), "delete boom") {
		t.Fatalf("expected delete error, got %v", err)
	}
}

func TestDeleteHardWithLinks(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	if err := s.Add(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(b); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLink(Link{From: a.ID, To: b.ID, Rel: "references"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(a.ID, true); err != nil {
		t.Fatal(err)
	}
	links, _ := s.GetLinks(a.ID)
	if len(links) != 0 {
		t.Fatalf("expected links cleaned up, got %d", len(links))
	}
}

func TestAddLinkPutError(t *testing.T) {
	s := tempStore(t)
	orig := putErrHook
	putErrHook = func(name string) error {
		if name == bucketLinks {
			return errors.New("link put boom")
		}
		return nil
	}
	t.Cleanup(func() { putErrHook = orig })
	if err := s.AddLink(Link{From: "a", To: "b", Rel: "references"}); err == nil || !strings.Contains(err.Error(), "link put boom") {
		t.Fatalf("expected link put error, got %v", err)
	}
}

func TestRemoveLinkDeleteError(t *testing.T) {
	s := tempStore(t)
	orig := deleteErrHook
	deleteErrHook = func(name string) error {
		if name == bucketLinks {
			return errors.New("link delete boom")
		}
		return nil
	}
	t.Cleanup(func() { deleteErrHook = orig })
	if err := s.RemoveLink("a", "b"); err == nil || !strings.Contains(err.Error(), "link delete boom") {
		t.Fatalf("expected link delete error, got %v", err)
	}
}

func TestGetLinksMalformedKey(t *testing.T) {
	s := tempStore(t)
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketLinks)).Put([]byte("bad"), []byte("x"))
	}); err != nil {
		t.Fatal(err)
	}
	links, err := s.GetLinks("nope")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("expected no links, got %d", len(links))
	}
}

func TestGetLinksDuplicate(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	if err := s.Add(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(b); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLink(Link{From: a.ID, To: b.ID, Rel: "references"}); err != nil {
		t.Fatal(err)
	}
	orig := getLinksDuplicateKey
	getLinksDuplicateKey = string(linkKey(a.ID, b.ID))
	t.Cleanup(func() { getLinksDuplicateKey = orig })
	links, _ := s.GetLinks(a.ID)
	if len(links) != 0 {
		t.Fatalf("expected duplicate skipped, got %d", len(links))
	}
}

func TestGetLinksViewError(t *testing.T) {
	s := tempStore(t)
	orig := dbView
	dbView = func(*bolt.DB, func(*bolt.Tx) error) error { return errors.New("view boom") }
	t.Cleanup(func() { dbView = orig })
	if _, err := s.GetLinks("x"); err == nil || !strings.Contains(err.Error(), "view boom") {
		t.Fatalf("expected view error, got %v", err)
	}
}

func TestStatsViewError(t *testing.T) {
	s := tempStore(t)
	orig := dbView
	dbView = func(*bolt.DB, func(*bolt.Tx) error) error { return errors.New("view boom") }
	t.Cleanup(func() { dbView = orig })
	if _, err := s.Stats(); err == nil || !strings.Contains(err.Error(), "view boom") {
		t.Fatalf("expected view error, got %v", err)
	}
}

func TestSearchLimitZero(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "hello world"})
	results, err := s.Search("hello", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected default limit results")
	}
}

func TestSearchListError(t *testing.T) {
	s := tempStore(t)
	orig := dbView
	dbView = func(*bolt.DB, func(*bolt.Tx) error) error { return errors.New("view boom") }
	t.Cleanup(func() { dbView = orig })
	if _, err := s.Search("hello", "", 10); err == nil || !strings.Contains(err.Error(), "view boom") {
		t.Fatalf("expected view error, got %v", err)
	}
}

func TestSearchEmbeddingError(t *testing.T) {
	old, oldDim := GetEmbedder()
	defer SetEmbedder(old, oldDim)
	SetEmbedder(func(string) ([]float32, error) { return nil, errors.New("embed boom") }, 0)

	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "hello world"})
	results, err := s.Search("hello", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected fallback results")
	}
}

func TestSearchEmbeddingNoVector(t *testing.T) {
	old, oldDim := GetEmbedder()
	defer SetEmbedder(old, oldDim)
	SetEmbedder(func(text string) ([]float32, error) {
		if strings.Contains(text, "query") {
			return []float32{1.0, 0.0}, nil
		}
		return nil, nil
	}, 2)

	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "something else"})
	results, err := s.Search("query", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

func TestSearchLimitTruncate(t *testing.T) {
	s := tempStore(t)
	for i := 0; i < 5; i++ {
		_ = s.Add(&Memory{Insight: "item " + string(rune('a'+i))})
	}
	results, err := s.Search("item", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestGraphDefaultDepth(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	c := &Memory{Insight: "C"}
	for _, m := range []*Memory{a, b, c} {
		if err := s.Add(m); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.AddLink(Link{From: a.ID, To: b.ID, Rel: "extends"})
	_ = s.AddLink(Link{From: b.ID, To: c.ID, Rel: "references"})
	tree, err := s.Graph(a.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(tree))
	}
}

func TestGraphGetLinksError(t *testing.T) {
	s := tempStore(t)
	orig := dbView
	dbView = func(*bolt.DB, func(*bolt.Tx) error) error { return errors.New("view boom") }
	t.Cleanup(func() { dbView = orig })
	if _, err := s.Graph("x", 1); err == nil || !strings.Contains(err.Error(), "view boom") {
		t.Fatalf("expected view error, got %v", err)
	}
}

func TestPrimeTopKZero(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "go modules"})
	text, err := s.Prime("modules", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "go modules") {
		t.Fatalf("expected prime text, got %q", text)
	}
}

func TestPrimeSearchError(t *testing.T) {
	s := tempStore(t)
	orig := dbView
	dbView = func(*bolt.DB, func(*bolt.Tx) error) error { return errors.New("view boom") }
	t.Cleanup(func() { dbView = orig })
	if _, err := s.Prime("x", "", 5); err == nil || !strings.Contains(err.Error(), "view boom") {
		t.Fatalf("expected view error, got %v", err)
	}
}

func TestEmbedNewRequestError(t *testing.T) {
	e := &Embedder{BaseURL: "://bad", Model: "m", APIKey: "k", HTTP: &http.Client{}}
	if _, err := e.EmbedOne(context.Background(), "hello"); err == nil {
		t.Fatal("expected new request error")
	}
}

func TestEmbedHTTPDoError(t *testing.T) {
	e := &Embedder{BaseURL: "https://x", Model: "m", APIKey: "k", HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network boom")
	})}}
	if _, err := e.EmbedOne(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "network boom") {
		t.Fatalf("expected network error, got %v", err)
	}
}

func TestEmbedDecodeError(t *testing.T) {
	e := &Embedder{BaseURL: "https://x", Model: "m", APIKey: "k", HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("not json"))}, nil
	})}}
	if _, err := e.EmbedOne(context.Background(), "hello"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestEmbedCountMismatch(t *testing.T) {
	e := &Embedder{BaseURL: "https://x", Model: "m", APIKey: "k", HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		body := `{"data":[{"embedding":[0.1],"index":0},{"embedding":[0.2],"index":1}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	if _, err := e.EmbedOne(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "expected 1") {
		t.Fatalf("expected count mismatch error, got %v", err)
	}
}

func TestEmbedBadIndex(t *testing.T) {
	e := &Embedder{BaseURL: "https://x", Model: "m", APIKey: "k", HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		body := `{"data":[{"embedding":[0.1],"index":5}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	if _, err := e.EmbedOne(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "bad embedding index") {
		t.Fatalf("expected bad index error, got %v", err)
	}
}

func TestEmbedStatusCodeError(t *testing.T) {
	e := &Embedder{BaseURL: "https://x", Model: "m", APIKey: "k", HTTP: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("server error"))}, nil
	})}}
	if _, err := e.EmbedOne(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}

func TestListSearchFilter(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Memory{Insight: "hello world"})
	_ = s.Add(&Memory{Insight: "goodbye world"})
	got, _ := s.List(ListFilter{Search: "hello"})
	if len(got) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(got))
	}
}

func TestAddAutoIDAndTimestamps(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "auto"}
	if err := s.Add(m); err != nil {
		t.Fatal(err)
	}
	if m.ID == "" {
		t.Fatal("ID not set")
	}
	if m.Created.IsZero() || m.Updated.IsZero() {
		t.Fatal("timestamps not set")
	}
}

func TestGetenvSet(t *testing.T) {
	t.Setenv("SIN_CODE_MEMORY_TEST_SET", "value")
	if got := getenv("SIN_CODE_MEMORY_TEST_SET", "fallback"); got != "value" {
		t.Fatalf("expected value, got %q", got)
	}
}

func TestSetupNIMEmbedderWithServer(t *testing.T) {
	old, oldDim := GetEmbedder()
	t.Cleanup(func() { SetEmbedder(old, oldDim) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintln(w, `{"data":[{"embedding":[0.1,0.2,0.3],"index":0}],"usage":{"prompt_tokens":1,"total_tokens":2}}`)
	}))
	defer srv.Close()

	t.Setenv("SIN_NIM_API_KEY", "test")
	t.Setenv("SIN_NIM_BASE_URL", srv.URL)
	SetupNIMEmbedder()

	fn, dim := GetEmbedder()
	if fn == nil {
		t.Fatal("embedder not set")
	}
	if dim != 4096 {
		t.Fatalf("expected dim 4096, got %d", dim)
	}
	vec, err := fn("hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3-dim vector, got %d", len(vec))
	}
}

func TestComputeEmbeddingCache(t *testing.T) {
	old, oldDim := GetEmbedder()
	defer SetEmbedder(old, oldDim)
	calls := 0
	SetEmbedder(func(text string) ([]float32, error) {
		calls++
		return []float32{0.1, 0.2}, nil
	}, 2)

	s := tempStore(t)
	if _, err := s.computeEmbedding("x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.computeEmbedding("x"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 embedder call (cache hit), got %d", calls)
	}
}

func TestListUnmarshalError(t *testing.T) {
	s := tempStore(t)
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketMems)).Put(memKey("bad"), []byte("not json"))
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 results, got %d", len(got))
	}
}

func TestSearchSemanticLimitTruncate(t *testing.T) {
	old, oldDim := GetEmbedder()
	defer SetEmbedder(old, oldDim)
	SetEmbedder(func(string) ([]float32, error) { return []float32{1.0, 0.0}, nil }, 2)

	s := tempStore(t)
	for i := 0; i < 5; i++ {
		_ = s.Add(&Memory{Insight: fmt.Sprintf("item %d", i)})
	}
	results, err := s.Search("item", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestDeleteHardAuditError(t *testing.T) {
	s := tempStore(t)
	m := &Memory{Insight: "hard"}
	if err := s.Add(m); err != nil {
		t.Fatal(err)
	}
	orig := putErrHook
	putErrHook = func(name string) error {
		if name == bucketAudit {
			return errors.New("audit put boom")
		}
		return nil
	}
	t.Cleanup(func() { putErrHook = orig })
	if err := s.Delete(m.ID, true); err == nil || !strings.Contains(err.Error(), "audit put boom") {
		t.Fatalf("expected audit error, got %v", err)
	}
}

func TestAppendAuditFromTo(t *testing.T) {
	s := tempStore(t)
	err := s.db.Update(func(tx *bolt.Tx) error {
		return s.appendAudit(tx, "id", "link", "from", "to")
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddingStatusEnabled(t *testing.T) {
	old, oldDim := GetEmbedder()
	defer SetEmbedder(old, oldDim)
	SetEmbedder(func(string) ([]float32, error) { return []float32{0.1}, nil }, 1)

	s := tempStore(t)
	enabled, dim := s.EmbeddingStatus()
	if !enabled || dim != 1 {
		t.Fatalf("expected enabled dim=1, got enabled=%v dim=%d", enabled, dim)
	}
}

func TestPutWithErrRealError(t *testing.T) {
	s := tempStore(t)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMems))
		err := putWithErr(bucketMems, b, []byte("k"), []byte("v"))
		if err == nil {
			t.Fatal("expected put error in read-only tx")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeleteWithErrRealError(t *testing.T) {
	s := tempStore(t)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMems))
		err := deleteWithErr(bucketMems, b, []byte("k"))
		if err == nil {
			t.Fatal("expected delete error in read-only tx")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNextSeqWithErrRealError(t *testing.T) {
	s := tempStore(t)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketAudit))
		_, err := nextSeqWithErr(bucketAudit, b)
		if err == nil {
			t.Fatal("expected next sequence error in read-only tx")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenBucketCreationError(t *testing.T) {
	orig := createBucketErrHook
	createBucketErrHook = func() error { return errors.New("bucket boom") }
	t.Cleanup(func() { createBucketErrHook = orig })
	if _, err := Open(filepath.Join(t.TempDir(), "memory.db")); err == nil || !strings.Contains(err.Error(), "bucket boom") {
		t.Fatalf("expected bucket creation error, got %v", err)
	}
}

func TestCreateBucketIfNotExistsRealError(t *testing.T) {
	s := tempStore(t)
	err := s.db.Update(func(tx *bolt.Tx) error {
		return createBucketIfNotExists(tx, []byte{})
	})
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}
}

func TestDecodeEmbeddingInvalid(t *testing.T) {
	if got := decodeEmbedding([]byte{1}); got != nil {
		t.Fatalf("expected nil for invalid length, got %v", got)
	}
	if got := decodeEmbedding(nil); got != nil {
		t.Fatalf("expected nil for empty, got %v", got)
	}
}

func TestStatsCountsAll(t *testing.T) {
	old, oldDim := GetEmbedder()
	defer SetEmbedder(old, oldDim)
	SetEmbedder(func(string) ([]float32, error) { return []float32{0.1}, nil }, 1)

	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	if err := s.Add(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(b); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLink(Link{From: a.ID, To: b.ID, Rel: "references"}); err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats["total"] != 2 {
		t.Fatalf("expected total 2, got %d", stats["total"])
	}
	if stats["links"] != 1 {
		t.Fatalf("expected links 1, got %d", stats["links"])
	}
	if stats["embeddings"] != 2 {
		t.Fatalf("expected embeddings 2, got %d", stats["embeddings"])
	}
}

func TestAddPresets(t *testing.T) {
	s := tempStore(t)
	m := &Memory{ID: "preset", Insight: "preset", Created: time.Unix(123, 0), Tags: []string{"  a ", "a", "b"}}
	if err := s.Add(m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "preset" {
		t.Fatalf("expected ID preset, got %q", m.ID)
	}
	if m.Created.Unix() != 123 {
		t.Fatalf("created not preserved")
	}
	if len(m.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(m.Tags))
	}
}

func TestDeleteHardLinksDeleteError(t *testing.T) {
	s := tempStore(t)
	a := &Memory{Insight: "A"}
	b := &Memory{Insight: "B"}
	if err := s.Add(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(b); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLink(Link{From: a.ID, To: b.ID, Rel: "references"}); err != nil {
		t.Fatal(err)
	}
	orig := deleteErrHook
	deleteErrHook = func(name string) error {
		if name == bucketLinks {
			return errors.New("links delete boom")
		}
		return nil
	}
	t.Cleanup(func() { deleteErrHook = orig })
	if err := s.Delete(a.ID, true); err == nil || !strings.Contains(err.Error(), "links delete boom") {
		t.Fatalf("expected links delete error, got %v", err)
	}
}
