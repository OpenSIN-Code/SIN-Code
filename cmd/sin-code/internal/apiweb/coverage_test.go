// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests to reach 100% statement coverage.
package apiweb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// nonFlusherRecorder is an http.ResponseWriter that does NOT implement
// http.Flusher, exercising the chat handler's streaming-unsupported path.
type nonFlusherRecorder struct {
	rec *httptest.ResponseRecorder
}

func (n nonFlusherRecorder) Header() http.Header     { return n.rec.Header() }
func (n nonFlusherRecorder) Write(b []byte) (int, error) { return n.rec.Write(b) }
func (n nonFlusherRecorder) WriteHeader(code int)        { n.rec.WriteHeader(code) }

// nonFlusherHandler wraps a handler with a non-flushing ResponseWriter.
type nonFlusherHandler struct{ h http.Handler }

func (n nonFlusherHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := httptest.NewRecorder()
	n.h.ServeHTTP(nonFlusherRecorder{rec}, r)
	// Copy recorded state back to the real writer.
	for k, v := range rec.Header() {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.Code)
	_, _ = w.Write(rec.Body.Bytes())
}

// newTestAPIServerWithAPI is like newTestAPIServer but leaves DB paths empty
// so the default-path branches can be exercised.
func newTestAPIServerWithAPI(t *testing.T, token string) (*APIServer, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	api := NewAPIServer(dir)
	api.Token = token
	api.SessionDB = ""
	api.LessonsDB = ""
	mux := http.NewServeMux()
	api.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return api, srv
}

// ─── auth edge cases ───────────────────────────────────────────────────

func TestAPIAuth_MalformedRemoteAddr(t *testing.T) {
	api := NewAPIServer(t.TempDir())
	api.Token = "" // loopback-only
	mux := http.NewServeMux()
	api.Routes(mux)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = "not-a-valid-addr"
		mux.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(wrapped)
	defer srv.Close()
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

// ─── default store open error paths ────────────────────────────────────

func TestAPI_DefaultSessionPath_OpenError(t *testing.T) {
	_, srv := newTestAPIServerWithAPI(t, "tok")
	old := sessionOpenHook
	sessionOpenHook = func(path string) (*session.Store, error) {
		return nil, fmt.Errorf("session open failed")
	}
	defer func() { sessionOpenHook = old }()

	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestAPI_DefaultLessonsPath_OpenError(t *testing.T) {
	_, srv := newTestAPIServerWithAPI(t, "tok")
	old := lessonsOpenHook
	lessonsOpenHook = func(path string) (*lessons.Store, error) {
		return nil, fmt.Errorf("lessons open failed")
	}
	defer func() { lessonsOpenHook = old }()

	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/knowledge", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// ─── handler-specific auth failures ────────────────────────────────────

func TestAPI_ShowSession_AuthRejects(t *testing.T) {
	_, srv := newTestAPIServerWithAPI(t, "tok")
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions/abc", "wrong", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAPI_ShowSession_MissingID(t *testing.T) {
	api, _ := newTestAPIServerWithAPI(t, "tok")
	req := httptest.NewRequest("GET", "/api/v1/sessions/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	api.handleShowSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestAPI_DeleteSession_AuthRejects(t *testing.T) {
	_, srv := newTestAPIServerWithAPI(t, "tok")
	resp, _ := doJSON(t, "DELETE", srv.URL+"/api/v1/sessions/abc", "wrong", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAPI_DeleteSession_MissingID(t *testing.T) {
	api, _ := newTestAPIServerWithAPI(t, "tok")
	req := httptest.NewRequest("DELETE", "/api/v1/sessions/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	api.handleDeleteSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestAPI_ForkSession_AuthRejects(t *testing.T) {
	_, srv := newTestAPIServerWithAPI(t, "tok")
	resp, _ := doJSON(t, "POST", srv.URL+"/api/v1/sessions/abc/fork", "wrong", map[string]int{"turn": 1})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAPI_ForkSession_MissingID(t *testing.T) {
	api, _ := newTestAPIServerWithAPI(t, "tok")
	req := httptest.NewRequest("POST", "/api/v1/sessions/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	api.handleForkSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestAPI_Knowledge_AuthRejects(t *testing.T) {
	_, srv := newTestAPIServerWithAPI(t, "tok")
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/knowledge", "wrong", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAPI_Chat_AuthRejects(t *testing.T) {
	_, srv := newTestAPIServerWithAPI(t, "tok")
	resp, _ := doJSON(t, "POST", srv.URL+"/api/v1/chat", "wrong", map[string]string{"prompt": "hi"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

// ─── store method error injection ──────────────────────────────────────

func TestAPI_ListSessions_StoreError(t *testing.T) {
	api, srv := newTestAPIServerWithAPI(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		s, _ := session.Open(filepath.Join(t.TempDir(), "s.db"))
		l, _ := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
		return s, l, nil
	}
	old := sessionListHook
	sessionListHook = func(s *session.Store) ([]session.Info, error) {
		return nil, fmt.Errorf("list failed")
	}
	defer func() { sessionListHook = old }()

	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestAPI_DeleteSession_StoreError(t *testing.T) {
	api, srv := newTestAPIServerWithAPI(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		s, _ := session.Open(filepath.Join(t.TempDir(), "s.db"))
		l, _ := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
		return s, l, nil
	}
	old := sessionDeleteHook
	sessionDeleteHook = func(s *session.Store, id string) error {
		return fmt.Errorf("delete failed")
	}
	defer func() { sessionDeleteHook = old }()

	resp, _ := doJSON(t, "DELETE", srv.URL+"/api/v1/sessions/abc", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestAPI_ForkSession_StoreError(t *testing.T) {
	api, srv := newTestAPIServerWithAPI(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		s, _ := session.Open(filepath.Join(t.TempDir(), "s.db"))
		l, _ := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
		return s, l, nil
	}
	old := sessionForkHook
	sessionForkHook = func(s *session.Store, src string, turn int) (*session.Session, error) {
		return nil, fmt.Errorf("fork failed")
	}
	defer func() { sessionForkHook = old }()

	resp, _ := doJSON(t, "POST", srv.URL+"/api/v1/sessions/abc/fork", "tok", map[string]int{"turn": 1})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestAPI_Knowledge_Limit(t *testing.T) {
	api, srv := newTestAPIServerWithAPI(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		s, _ := session.Open(filepath.Join(t.TempDir(), "s.db"))
		l, _ := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
		return s, l, nil
	}
	old := lessonsQueryHook
	called := false
	lessonsQueryHook = func(s *lessons.Store, ctx context.Context, workspace string, limit int) ([]lessons.Entry, error) {
		called = true
		if limit != 7 {
			t.Fatalf("want limit 7, got %d", limit)
		}
		return []lessons.Entry{}, nil
	}
	defer func() { lessonsQueryHook = old }()

	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/knowledge?limit=7", "tok", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if !called {
		t.Fatal("lessonsQueryHook was not called")
	}
}

func TestAPI_Knowledge_QueryError(t *testing.T) {
	api, srv := newTestAPIServerWithAPI(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		s, _ := session.Open(filepath.Join(t.TempDir(), "s.db"))
		l, _ := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
		return s, l, nil
	}
	old := lessonsQueryHook
	lessonsQueryHook = func(s *lessons.Store, ctx context.Context, workspace string, limit int) ([]lessons.Entry, error) {
		return nil, fmt.Errorf("query failed")
	}
	defer func() { lessonsQueryHook = old }()

	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/knowledge", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// ─── chat error paths ──────────────────────────────────────────────────

func TestAPI_Chat_FlusherNotOK(t *testing.T) {
	api, _ := newTestAPIServerWithAPI(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		s, _ := session.Open(filepath.Join(t.TempDir(), "s.db"))
		l, _ := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
		return s, l, nil
	}

	wrapped := httptest.NewServer(nonFlusherHandler{api.Routes(nil)})
	defer wrapped.Close()

	req, _ := http.NewRequest("POST", wrapped.URL+"/api/v1/chat", strings.NewReader(`{"prompt":"hi"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestAPI_Chat_StartOrResumeError(t *testing.T) {
	api, srv := newTestAPIServerWithAPI(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		s, _ := session.Open(filepath.Join(t.TempDir(), "s.db"))
		l, _ := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
		return s, l, nil
	}
	api.NewLoop = happyChatLoopFactory(&agentloop.Result{}, nil)

	old := sessionStartOrResumeHook
	sessionStartOrResumeHook = func(s *session.Store, id string) (*session.Session, error) {
		return nil, fmt.Errorf("resume failed")
	}
	defer func() { sessionStartOrResumeHook = old }()

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/chat", strings.NewReader(`{"prompt":"hi"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	events := parseSSE(t, resp.Body)
	last := events[len(events)-1]
	if last.Event != "error" {
		t.Fatalf("want error event, got %q", last.Event)
	}
}

func TestAPI_Chat_NoNewLoop(t *testing.T) {
	api, srv := newTestAPIServerWithAPI(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		s, _ := session.Open(filepath.Join(t.TempDir(), "s.db"))
		l, _ := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
		return s, l, nil
	}
	api.NewLoop = nil

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/chat", strings.NewReader(`{"prompt":"hi"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("no NewLoop factory wired")) {
		t.Fatalf("expected no NewLoop error, got %s", body)
	}
}
