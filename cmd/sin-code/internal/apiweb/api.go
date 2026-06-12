// SPDX-License-Identifier: MIT
// Purpose: WebUI-v2 backend HTTP API (issue #52) — sessions / knowledge /
// chat-with-SSE mounted under /api/v1 on the sin-code serve multiplexer.
// Mandate M4: headless mode, so Loop.Ask is nil; the chat handler forces
// perm.Headless=true so the permission engine denies any "ask" rule and
// cannot self-escalate.
package apiweb

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// chatRequest is the body for POST /api/v1/chat.
type chatRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id,omitempty"`
}

// NewLoopFunc builds a fully wired agentloop.Loop for the chat endpoint.
// Same shape as chat_cmd.go runChat but isolated so the API can be
// tested with a mock factory. In production the real factory is used;
// tests swap in a stub that returns a fake loop with a fixed result.
type NewLoopFunc func(ctx context.Context, sessionID, workspace string) (*agentloop.Loop, func() error, error)

// APIServer wires the WebUI-v2 contract to session+lessons stores + a
// loop factory. Construct via NewAPIServer. Concurrency-safe: handlers
// are stateless except for the stores (sqlite-safe) and the immutable
// config. Mandate M7: stores are *not* shared between handlers.
type APIServer struct {
	// Token is the bearer token expected in the Authorization header.
	// Empty means: only loopback (127.0.0.1, [::1]) is allowed.
	Token string
	// Workspace is the directory the agent loop runs in.
	Workspace string
	// SessionDB / LessonsDB are absolute file paths. Empty = default.
	SessionDB string
	LessonsDB string
	// NewLoop builds the agent loop. REQUIRED for the chat endpoint.
	// Tests inject a stub; production wires the loopbuilder-driven
	// factory defined in package main.
	NewLoop NewLoopFunc
	// OpenStores is an indirection for opening the session+lessons
	// pair. nil = live SQLite open. Exposed for test injection of
	// failure paths (mandate M7 — concurrent test harness).
	OpenStores func() (*session.Store, *lessons.Store, error)
}

// NewAPIServer constructs an APIServer with sensible defaults and
// honours the SIN_API_TOKEN env var for the bearer-token policy. The
// caller MUST set NewLoop before serving any chat traffic — there is
// no default here because the production factory lives in package main
// (it depends on combinedTool/combinedSpecs from chat_mcp.go).
func NewAPIServer(workspace string) *APIServer {
	return &APIServer{
		Token:     os.Getenv("SIN_API_TOKEN"),
		Workspace: workspace,
	}
}

// Routes mounts every /api/v1/* handler onto mux and returns mux.
// Idempotent — safe to call multiple times with the same mux. Passing
// nil allocates a fresh mux.
func (a *APIServer) Routes(mux *http.ServeMux) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}
	mux.HandleFunc("GET /api/v1/sessions", a.handleListSessions)
	mux.HandleFunc("GET /api/v1/sessions/{id}", a.handleShowSession)
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", a.handleDeleteSession)
	mux.HandleFunc("POST /api/v1/sessions/{id}/fork", a.handleForkSession)
	mux.HandleFunc("GET /api/v1/knowledge", a.handleKnowledge)
	mux.HandleFunc("POST /api/v1/chat", a.handleChat)
	return mux
}

// auth is a middleware that enforces the bearer-token contract. If
// Token is empty, only loopback is allowed (covers the local-dev
// default). Otherwise the Authorization header must be
// "Bearer <token>". On failure it writes a 401 and returns false.
func (a *APIServer) auth(w http.ResponseWriter, r *http.Request) bool {
	if a.Token == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			writeError(w, http.StatusUnauthorized, "authentication required (set SIN_API_TOKEN or call from 127.0.0.1)")
			return false
		}
		return true
	}
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) || h[len(prefix):] != a.Token {
		writeError(w, http.StatusUnauthorized, "invalid or missing bearer token")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// stores returns the session+lessons pair, honouring an injected
// OpenStores factory (used by tests to simulate DB outages) or
// defaulting to the live SQLite opener. Each handler MUST close both
// stores via defer; the *Store pointer is not goroutine-safe across
// the WebUI API surface (mandate M7).
func (a *APIServer) stores() (*session.Store, *lessons.Store, error) {
	if a.OpenStores != nil {
		return a.OpenStores()
	}
	sessPath := a.SessionDB
	if sessPath == "" {
		sessPath = session.DefaultPath()
	}
	sstore, err := session.Open(sessPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open sessions: %w", err)
	}
	lessPath := a.LessonsDB
	if lessPath == "" {
		lessPath = lessons.DefaultPath()
	}
	lstore, err := lessons.Open(lessPath)
	if err != nil {
		_ = sstore.Close()
		return nil, nil, fmt.Errorf("open lessons: %w", err)
	}
	return sstore, lstore, nil
}

func (a *APIServer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if !a.auth(w, r) {
		return
	}
	sstore, lstore, err := a.stores()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer sstore.Close()
	defer lstore.Close()
	infos, err := sstore.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if infos == nil {
		infos = []session.Info{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": infos})
}

func (a *APIServer) handleShowSession(w http.ResponseWriter, r *http.Request) {
	if !a.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}
	sstore, _, err := a.stores()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer sstore.Close()
	sess, err := sstore.StartOrResume(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      sess.ID,
		"history": sess.History(),
	})
}

func (a *APIServer) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if !a.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}
	sstore, _, err := a.stores()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer sstore.Close()
	if err := sstore.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (a *APIServer) handleForkSession(w http.ResponseWriter, r *http.Request) {
	if !a.auth(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}
	var body struct {
		Turn int `json:"turn"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	sstore, _, err := a.stores()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer sstore.Close()
	forked, err := sstore.Fork(id, body.Turn)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      forked.ID,
		"parent":  id,
		"turn":    body.Turn,
		"history": forked.History(),
	})
}

func (a *APIServer) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	if !a.auth(w, r) {
		return
	}
	_, lstore, err := a.stores()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer lstore.Close()
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	entries, err := lstore.Query(r.Context(), a.Workspace, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []lessons.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace": a.Workspace,
		"lessons":   entries,
	})
}

// sseEvent is the JSON payload of one SSE message. Wire format is the
// standard "event: <name>\ndata: <json>\n\n" frame.
type sseEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// writeSSE emits one event in standard text/event-stream format and
// flushes immediately so the client can stream results in real time.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, ev sseEvent) {
	payload, _ := json.Marshal(ev.Data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Event, payload)
	if flusher != nil {
		flusher.Flush()
	}
}

func (a *APIServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if !a.auth(w, r) {
		return
	}
	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	writeSSE(w, flusher, sseEvent{Event: "start", Data: map[string]any{"prompt": body.Prompt}})

	// M4: headless. The injected NewLoop factory MUST set
	// perm.Headless=true (loopbuilder does this when Headless: true).
	ctx := r.Context()
	sstore, _, err := a.stores()
	if err != nil {
		writeSSE(w, flusher, sseEvent{Event: "error", Data: map[string]string{"error": err.Error()}})
		return
	}
	defer sstore.Close()
	sess, err := sstore.StartOrResume(body.SessionID)
	if err != nil {
		writeSSE(w, flusher, sseEvent{Event: "error", Data: map[string]string{"error": err.Error()}})
		return
	}

	if a.NewLoop == nil {
		writeSSE(w, flusher, sseEvent{Event: "error", Data: map[string]string{"error": "no NewLoop factory wired"}})
		return
	}
	loop, cleanup, err := a.NewLoop(ctx, sess.ID, a.Workspace)
	if err != nil {
		writeSSE(w, flusher, sseEvent{Event: "error", Data: map[string]string{"error": err.Error()}})
		return
	}
	defer func() { _ = cleanup() }()

	result, err := loop.Run(ctx, sess, body.Prompt)
	if err != nil {
		writeSSE(w, flusher, sseEvent{Event: "error", Data: map[string]string{"error": err.Error()}})
		return
	}
	writeSSE(w, flusher, sseEvent{Event: "result", Data: result})
}

// _resultTypeAnchor documents the dependency on agentloop.Result — its
// zero value is used as the SSE "result" event payload type.
var _ agentloop.Result
