// SPDX-License-Identifier: MIT
// Purpose: regression tests for the WebUI-v2 backend API (issue #52).
// Covers: auth (token required vs localhost fallback), session CRUD,
// fork semantics, knowledge list, chat endpoint SSE contract with a
// mock loop factory. Race-safe (mandate M7).
package apiweb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

// newTestAPIServer wires a fresh APIServer to a temp session+lessons DB
// and a real httptest.Server. Tests inject custom NewLoop and OpenStores
// factories on the returned api to exercise happy-path and error paths.
func newTestAPIServer(t *testing.T, token string) (*APIServer, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	api := NewAPIServer(dir)
	api.Token = token
	api.SessionDB = filepath.Join(dir, "sessions.db")
	api.LessonsDB = filepath.Join(dir, "lessons.db")
	mux := http.NewServeMux()
	api.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return api, srv
}

// happyChatLoopFactory returns a NewLoopFunc whose loop returns
// (result, err) from Run without exercising the rest of the agent loop.
func happyChatLoopFactory(result *agentloop.Result, err error) NewLoopFunc {
	return func(ctx context.Context, sessionID, workspace string) (*agentloop.Loop, func() error, error) {
		return &agentloop.Loop{
			RunOverride: func(ctx context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
				return result, err
			},
		}, func() error { return nil }, nil
	}
}

// loopbuilderLoopFactory is a no-op factory used by tests that do not
// exercise the chat endpoint. It returns nil loop + nil cleanup + nil
// err; the chat handler will then write an error SSE event and return.
func loopbuilderLoopFactory(ctx context.Context, sessionID, workspace string) (*agentloop.Loop, func() error, error) {
	return &agentloop.Loop{}, func() error { return nil }, nil
}

func doJSON(t *testing.T, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

type sseLine struct {
	Event string
	Data  string
}

// parseSSE is a tolerant SSE parser for tests: it reads "event: X
// data: Y" frames separated by blank lines. Malformed lines ignored.
func parseSSE(t *testing.T, r io.Reader) []sseLine {
	t.Helper()
	var out []sseLine
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var cur sseLine
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if cur.Event != "" || cur.Data != "" {
				out = append(out, cur)
				cur = sseLine{}
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.Data = strings.TrimPrefix(line, "data: ")
		}
	}
	if cur.Event != "" || cur.Data != "" {
		out = append(out, cur)
	}
	return out
}

// ─── auth ──────────────────────────────────────────────────────────────

func TestAPIAuth_TokenRequired_RejectsMissing(t *testing.T) {
	_, srv := newTestAPIServer(t, "secret-token")
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAPIAuth_TokenRequired_AcceptsBearer(t *testing.T) {
	_, srv := newTestAPIServer(t, "secret-token")
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "secret-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestAPIAuth_TokenRequired_RejectsWrong(t *testing.T) {
	_, srv := newTestAPIServer(t, "secret-token")
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "wrong", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAPIAuth_LocalhostAllowed_NoToken(t *testing.T) {
	_, srv := newTestAPIServer(t, "")
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for loopback, got %d", resp.StatusCode)
	}
}

func TestAPIAuth_NonLoopback_NoToken(t *testing.T) {
	api := NewAPIServer(t.TempDir())
	mux := http.NewServeMux()
	api.Routes(mux)
	// Wrap mux to fake a non-loopback RemoteAddr.
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = "10.0.0.5:12345"
		mux.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(wrapped)
	defer srv.Close()
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 for non-loopback, got %d", resp.StatusCode)
	}
}

// ─── session CRUD ──────────────────────────────────────────────────────

func TestAPI_ListSessions_Empty(t *testing.T) {
	_, srv := newTestAPIServer(t, "tok")
	resp, body := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "tok", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		Sessions []session.Info `json:"sessions"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("json: %v body=%s", err, body)
	}
	if got.Sessions == nil {
		t.Fatal("sessions should be [] not null on empty store")
	}
}

func TestAPI_ShowSession_RoundTrip(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	sstore, err := session.Open(api.SessionDB)
	if err != nil {
		t.Fatal(err)
	}
	defer sstore.Close()
	sess, err := sstore.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}
	msgs := []session.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	if err := sess.SaveHistory(msgs); err != nil {
		t.Fatal(err)
	}

	resp, body := doJSON(t, "GET", srv.URL+"/api/v1/sessions/"+sess.ID, "tok", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		ID      string            `json:"id"`
		History []session.Message `json:"history"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.ID != sess.ID || len(got.History) != 2 {
		t.Fatalf("round-trip mismatch: id=%q hist=%v", got.ID, got.History)
	}
}

func TestAPI_ShowSession_NotFound(t *testing.T) {
	_, srv := newTestAPIServer(t, "tok")
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions/nope-9999", "tok", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestAPI_DeleteSession(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	sstore, _ := session.Open(api.SessionDB)
	sess, _ := sstore.StartOrResume("")
	sstore.Close()

	resp, body := doJSON(t, "DELETE", srv.URL+"/api/v1/sessions/"+sess.ID, "tok", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", resp.StatusCode, body)
	}

	// Follow-up GET must 404.
	resp2, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions/"+sess.ID, "tok", nil)
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 after delete, got %d", resp2.StatusCode)
	}
}

// ─── fork semantics ────────────────────────────────────────────────────

func TestAPI_ForkSession_TruncatesAtTurn(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	sstore, _ := session.Open(api.SessionDB)
	src, _ := sstore.StartOrResume("")
	msgs := []session.Message{
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
	}
	if err := src.SaveHistory(msgs); err != nil {
		t.Fatal(err)
	}
	sstore.Close()

	resp, body := doJSON(t, "POST", srv.URL+"/api/v1/sessions/"+src.ID+"/fork", "tok", map[string]int{"turn": 2})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		ID      string            `json:"id"`
		Parent  string            `json:"parent"`
		Turn    int               `json:"turn"`
		History []session.Message `json:"history"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("json: %v body=%s", err, body)
	}
	if got.Parent != src.ID || got.Turn != 2 {
		t.Fatalf("parent/turn wrong: %+v", got)
	}
	if len(got.History) != 2 {
		t.Fatalf("fork history should be 2, got %d (%v)", len(got.History), got.History)
	}
	if got.History[0].Content != "u1" || got.History[1].Content != "a1" {
		t.Fatalf("fork content wrong: %v", got.History)
	}
	if got.ID == src.ID {
		t.Fatal("fork must produce a new id")
	}
}

func TestAPI_ForkSession_ClampsOvershoot(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	sstore, _ := session.Open(api.SessionDB)
	src, _ := sstore.StartOrResume("")
	_ = src.SaveHistory([]session.Message{
		{Role: "user", Content: "u1"},
	})
	sstore.Close()
	resp, body := doJSON(t, "POST", srv.URL+"/api/v1/sessions/"+src.ID+"/fork", "tok", map[string]int{"turn": 9999})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		History []session.Message `json:"history"`
	}
	_ = json.Unmarshal(body, &got)
	if len(got.History) != 1 {
		t.Fatalf("overshoot must clamp to len(history), got %d", len(got.History))
	}
}

func TestAPI_ForkSession_BadJSON(t *testing.T) {
	_, srv := newTestAPIServer(t, "tok")
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/sessions/whatever/fork", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

// ─── knowledge ─────────────────────────────────────────────────────────

func TestAPI_Knowledge_Empty(t *testing.T) {
	_, srv := newTestAPIServer(t, "tok")
	resp, body := doJSON(t, "GET", srv.URL+"/api/v1/knowledge", "tok", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		Workspace string          `json:"workspace"`
		Lessons   []lessons.Entry `json:"lessons"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.Lessons == nil {
		t.Fatal("lessons should be [] not null on empty store")
	}
}

func TestAPI_Knowledge_ListsEntries(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	lstore, _ := lessons.Open(api.LessonsDB)
	defer lstore.Close()
	ctx := context.Background()
	if err := lstore.Record(ctx, lessons.Entry{
		Type:      lessons.TypeConstraint,
		Workspace: api.Workspace,
		Context:   map[string]any{"k": "v"},
		Lesson:    "do not rm -rf",
	}); err != nil {
		t.Fatal(err)
	}

	resp, body := doJSON(t, "GET", srv.URL+"/api/v1/knowledge", "tok", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		Lessons []lessons.Entry `json:"lessons"`
	}
	_ = json.Unmarshal(body, &got)
	if len(got.Lessons) != 1 {
		t.Fatalf("want 1 lesson, got %d (%v)", len(got.Lessons), got.Lessons)
	}
	if got.Lessons[0].Lesson != "do not rm -rf" {
		t.Fatalf("wrong lesson: %v", got.Lessons[0])
	}
}

// ─── chat / SSE ────────────────────────────────────────────────────────

func TestAPI_Chat_MissingPrompt(t *testing.T) {
	_, srv := newTestAPIServer(t, "tok")
	resp, _ := doJSON(t, "POST", srv.URL+"/api/v1/chat", "tok", map[string]string{"prompt": "  "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestAPI_Chat_BadJSON(t *testing.T) {
	_, srv := newTestAPIServer(t, "tok")
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/chat", strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

// TestAPI_Chat_StreamsSSE_HappyPath exercises the SSE contract end-to-end
// against a mock loop factory. Stream begins with event:start, terminates
// with event:result whose data matches the stable JSON contract.
func TestAPI_Chat_StreamsSSE_HappyPath(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.NewLoop = happyChatLoopFactory(&agentloop.Result{
		SessionID: "sess-XYZ", Summary: "all good", Verified: true, Turns: 3,
	}, nil)

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/chat", strings.NewReader(`{"prompt":"hello"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("want text/event-stream, got %q", ct)
	}
	events := parseSSE(t, resp.Body)
	if len(events) < 2 {
		t.Fatalf("want >=2 events, got %d (%v)", len(events), events)
	}
	if events[0].Event != "start" {
		t.Fatalf("first event must be start, got %q", events[0].Event)
	}
	last := events[len(events)-1]
	if last.Event != "result" {
		t.Fatalf("last event must be result, got %q", last.Event)
	}
	var got agentloop.Result
	if err := json.Unmarshal([]byte(last.Data), &got); err != nil {
		t.Fatalf("result data is not JSON: %v data=%q", err, last.Data)
	}
	if got.SessionID != "sess-XYZ" || got.Summary != "all good" || !got.Verified || got.Turns != 3 {
		t.Fatalf("result payload wrong: %+v", got)
	}
}

func TestAPI_Chat_StreamsSSE_Error(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.NewLoop = happyChatLoopFactory(nil, fmt.Errorf("boom"))

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/chat", strings.NewReader(`{"prompt":"hello"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := parseSSE(t, resp.Body)
	if len(events) < 2 {
		t.Fatalf("want >=2 events, got %d", len(events))
	}
	if events[len(events)-1].Event != "error" {
		t.Fatalf("last event must be error on failure, got %q", events[len(events)-1].Event)
	}
}

// TestAPI_Chat_FactoryError covers the case where NewLoop itself fails.
// The handler must still emit the SSE contract with a final "error" event.
func TestAPI_Chat_FactoryError(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.NewLoop = func(ctx context.Context, sessionID, workspace string) (*agentloop.Loop, func() error, error) {
		return nil, nil, fmt.Errorf("loop factory down")
	}
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/chat", strings.NewReader(`{"prompt":"hello"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := parseSSE(t, resp.Body)
	if len(events) < 2 || events[len(events)-1].Event != "error" {
		t.Fatalf("expected error SSE, got %v", events)
	}
}

// TestAPI_Chat_ResumesSession verifies that a non-empty session_id
// resumes the existing session rather than starting a new one.
func TestAPI_Chat_ResumesSession(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	sstore, _ := session.Open(api.SessionDB)
	existing, _ := sstore.StartOrResume("")
	sstore.Close()
	api.NewLoop = happyChatLoopFactory(&agentloop.Result{
		SessionID: existing.ID, Summary: "ok", Verified: true, Turns: 1,
	}, nil)

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/chat",
		strings.NewReader(fmt.Sprintf(`{"prompt":"hi","session_id":%q}`, existing.ID)))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := parseSSE(t, resp.Body)
	last := events[len(events)-1]
	var got agentloop.Result
	_ = json.Unmarshal([]byte(last.Data), &got)
	if got.SessionID != existing.ID {
		t.Fatalf("want session=%q got %q", existing.ID, got.SessionID)
	}
}

// ─── error paths / injection coverage ──────────────────────────────────

func TestAPI_StoresFailure_ListSessions(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		return nil, nil, fmt.Errorf("simulated db outage")
	}
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestAPI_StoresFailure_ShowSession(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		return nil, nil, fmt.Errorf("simulated db outage")
	}
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/sessions/whatever", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestAPI_StoresFailure_DeleteSession(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		return nil, nil, fmt.Errorf("simulated db outage")
	}
	resp, _ := doJSON(t, "DELETE", srv.URL+"/api/v1/sessions/whatever", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestAPI_StoresFailure_ForkSession(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		return nil, nil, fmt.Errorf("simulated db outage")
	}
	resp, _ := doJSON(t, "POST", srv.URL+"/api/v1/sessions/whatever/fork", "tok", map[string]int{"turn": 1})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestAPI_StoresFailure_Knowledge(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		return nil, nil, fmt.Errorf("simulated db outage")
	}
	resp, _ := doJSON(t, "GET", srv.URL+"/api/v1/knowledge", "tok", nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

// TestAPI_StoresFailure_Chat covers the openStores failure branch inside
// the SSE handler — it must still emit at least the start event followed
// by an error event so clients can react predictably.
func TestAPI_StoresFailure_Chat(t *testing.T) {
	api, srv := newTestAPIServer(t, "tok")
	api.OpenStores = func() (*session.Store, *lessons.Store, error) {
		return nil, nil, fmt.Errorf("simulated db outage")
	}
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/chat", strings.NewReader(`{"prompt":"hi"}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := parseSSE(t, resp.Body)
	if len(events) < 2 || events[0].Event != "start" || events[1].Event != "error" {
		t.Fatalf("expected start+error, got %v", events)
	}
}

// TestAPI_NewAPIServer_PicksEnvToken covers the env-token read path.
func TestAPI_NewAPIServer_PicksEnvToken(t *testing.T) {
	t.Setenv("SIN_API_TOKEN", "")
	api := NewAPIServer(t.TempDir())
	if api.Token != "" {
		t.Fatalf("expected empty token, got %q", api.Token)
	}
}

// TestAPIServer_RoutesNilMux ensures Routes(nil) allocates a fresh mux.
func TestAPIServer_RoutesNilMux(t *testing.T) {
	api := NewAPIServer(t.TempDir())
	mux := api.Routes(nil)
	if mux == nil {
		t.Fatal("Routes(nil) must allocate a mux")
	}
}

// TestAPI_DefaultNewLoopFactoryStub exercises the no-op stub factory;
// production wiring lives in package main (serve_api_loop.go).
func TestAPI_DefaultNewLoopFactoryStub(t *testing.T) {
	loop, cleanup, err := loopbuilderLoopFactory(context.Background(), "sess", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if loop == nil || cleanup == nil {
		t.Fatal("loop and cleanup must be non-nil")
	}
	_ = cleanup()
}
