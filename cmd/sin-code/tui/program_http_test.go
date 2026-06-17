// SPDX-License-Identifier: MIT
package tui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// mockSender captures tea.Msg values that would normally be delivered to
// *tea.Program via prog.Send(). It is race-safe (mandate M7).
type mockSender struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (s *mockSender) Send(msg tea.Msg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
}

func (s *mockSender) Messages() []tea.Msg {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]tea.Msg, len(s.msgs))
	copy(out, s.msgs)
	return out
}

func (s *mockSender) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = nil
}

// buildExternalMux creates an http.ServeMux that faithfully replicates the
// handler logic from runExternalMode in program.go, but routes prog.Send
// calls through a mockSender instead of a real *tea.Program. This lets us
// test the HTTP contract (status codes, JSON parsing, message conversion)
// without starting the blocking tea program loop.
func buildExternalMux(sender *mockSender) *http.ServeMux {
	mux := http.NewServeMux()

	// GET / — serves the HTML page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(externalTUIHTML))
	})

	// POST /input — receives individual keystrokes as JSON
	mux.HandleFunc("/input", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var input externalInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		msg := browserInputToMsg(input)
		if msg == nil {
			http.Error(w, "invalid input", http.StatusBadRequest)
			return
		}
		sender.Send(msg)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// POST /submit — receives full text blocks as JSON
	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, ch := range payload.Text {
			if ch == '\n' {
				sender.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
				continue
			}
			sender.Send(tea.KeyPressMsg{
				Text: string(ch),
				Code: ch,
			})
		}
		sender.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	return mux
}

// ---------------------------------------------------------------------------
// /input endpoint tests
// ---------------------------------------------------------------------------

func TestExternalInputValidKey(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/input", "application/json",
		strings.NewReader(`{"type":"key","key":"enter","code":13,"mod":0}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for valid key, got %d", resp.StatusCode)
	}

	var result map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !result["ok"] {
		t.Error("expected {\"ok\": true} in response body")
	}

	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(msgs))
	}
	kp, ok := msgs[0].(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg, got %T", msgs[0])
	}
	if kp.Code != tea.KeyEnter {
		t.Errorf("expected KeyEnter code, got %v", kp.Code)
	}
}

func TestExternalInputArrowKey(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/input", "application/json",
		strings.NewReader(`{"type":"key","key":"up","code":38,"mod":0}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for arrow key, got %d", resp.StatusCode)
	}

	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	kp, ok := msgs[0].(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg, got %T", msgs[0])
	}
	if kp.Code != tea.KeyUp {
		t.Errorf("expected KeyUp, got %v", kp.Code)
	}
}

func TestExternalInputCtrlC(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/input", "application/json",
		strings.NewReader(`{"type":"key","key":"ctrl+c","code":67,"mod":4}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for ctrl+c, got %d", resp.StatusCode)
	}

	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	kp, ok := msgs[0].(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg, got %T", msgs[0])
	}
	if kp.Mod&tea.ModCtrl == 0 {
		t.Error("expected ModCtrl to be set")
	}
}

func TestExternalInputMouseEvent(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/input", "application/json",
		strings.NewReader(`{"type":"mouse","key":"","code":0,"mod":0,"x":10,"y":5,"button":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for mouse event, got %d", resp.StatusCode)
	}

	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	mc, ok := msgs[0].(tea.MouseClickMsg)
	if !ok {
		t.Fatalf("expected MouseClickMsg, got %T", msgs[0])
	}
	if mc.X != 10 || mc.Y != 5 {
		t.Errorf("expected (10,5), got (%d,%d)", mc.X, mc.Y)
	}
}

func TestExternalInputInvalidType(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	// type "foo" is neither "key" nor "mouse" — browserInputToMsg returns nil
	resp, err := http.Post(server.URL+"/input", "application/json",
		strings.NewReader(`{"type":"foo","key":"a","code":97,"mod":0}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid type, got %d", resp.StatusCode)
	}

	if len(sender.Messages()) != 0 {
		t.Error("expected no messages sent for invalid input type")
	}
}

func TestExternalInputInvalidJSON(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/input", "application/json",
		strings.NewReader(`{invalid json`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}

	if len(sender.Messages()) != 0 {
		t.Error("expected no messages sent for invalid JSON")
	}
}

func TestExternalInputWrongMethod(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Get(server.URL + "/input")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /input, got %d", resp.StatusCode)
	}

	if len(sender.Messages()) != 0 {
		t.Error("expected no messages sent for wrong method")
	}
}

func TestExternalInputMultipleKeys(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	keys := []string{
		`{"type":"key","key":"a","code":97,"mod":0}`,
		`{"type":"key","key":"b","code":98,"mod":0}`,
		`{"type":"key","key":"backspace","code":8,"mod":0}`,
		`{"type":"key","key":"tab","code":9,"mod":0}`,
	}
	for _, body := range keys {
		resp, err := http.Post(server.URL+"/input", "application/json",
			strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for key %q, got %d", body, resp.StatusCode)
		}
		resp.Body.Close()
	}

	msgs := sender.Messages()
	if len(msgs) != 4 {
		t.Errorf("expected 4 messages, got %d", len(msgs))
	}
}

// ---------------------------------------------------------------------------
// /submit endpoint tests
// ---------------------------------------------------------------------------

func TestExternalSubmitValidText(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/submit", "application/json",
		strings.NewReader(`{"text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !result["ok"] {
		t.Error("expected {\"ok\": true} in response body")
	}

	// "hello" = 5 chars + 1 trailing Enter = 6 messages
	msgs := sender.Messages()
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages (5 chars + enter), got %d", len(msgs))
	}

	for i, ch := range "hello" {
		kp, ok := msgs[i].(tea.KeyPressMsg)
		if !ok {
			t.Fatalf("msg %d: expected KeyPressMsg, got %T", i, msgs[i])
		}
		if kp.Text != string(ch) {
			t.Errorf("msg %d: expected text %q, got %q", i, string(ch), kp.Text)
		}
		if kp.Code != ch {
			t.Errorf("msg %d: expected code %q, got %q", i, string(ch), string(kp.Code))
		}
	}

	// Last message should be the trailing Enter
	last, ok := msgs[5].(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected last msg KeyPressMsg, got %T", msgs[5])
	}
	if last.Code != tea.KeyEnter {
		t.Errorf("expected trailing KeyEnter, got %v", last.Code)
	}
}

func TestExternalSubmitMultilineText(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/submit", "application/json",
		strings.NewReader(`{"text":"line1\nline2"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// "line1\nline2" = l,i,n,e,1,Enter,l,i,n,e,2 + trailing Enter = 12 messages
	msgs := sender.Messages()
	if len(msgs) != 12 {
		t.Fatalf("expected 12 messages (11 chars incl newline + trailing enter), got %d", len(msgs))
	}

	// The 6th message (index 5) should be KeyEnter for the \n
	kp, ok := msgs[5].(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg at index 5, got %T", msgs[5])
	}
	if kp.Code != tea.KeyEnter {
		t.Errorf("expected KeyEnter at newline position, got %v", kp.Code)
	}
}

func TestExternalSubmitEmptyText(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/submit", "application/json",
		strings.NewReader(`{"text":""}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for empty text, got %d", resp.StatusCode)
	}

	// Empty text: no char loop, but the trailing Enter is still sent
	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (trailing enter only), got %d", len(msgs))
	}
	kp, ok := msgs[0].(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg, got %T", msgs[0])
	}
	if kp.Code != tea.KeyEnter {
		t.Errorf("expected KeyEnter for empty submit, got %v", kp.Code)
	}
}

func TestExternalSubmitInvalidJSON(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Post(server.URL+"/submit", "application/json",
		strings.NewReader(`{invalid`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}

	if len(sender.Messages()) != 0 {
		t.Error("expected no messages sent for invalid JSON")
	}
}

func TestExternalSubmitWrongMethod(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Get(server.URL + "/submit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET /submit, got %d", resp.StatusCode)
	}

	if len(sender.Messages()) != 0 {
		t.Error("expected no messages sent for wrong method")
	}
}

func TestExternalSubmitMissingTextField(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	// JSON without "text" field — should decode successfully with empty string
	resp, err := http.Post(server.URL+"/submit", "application/json",
		strings.NewReader(`{"other":"value"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for JSON without text field, got %d", resp.StatusCode)
	}

	// Text defaults to "" so only the trailing Enter is sent
	msgs := sender.Messages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (trailing enter for empty default), got %d", len(msgs))
	}
}

// ---------------------------------------------------------------------------
// GET / (HTML) endpoint test
// ---------------------------------------------------------------------------

func TestExternalHTMLEndpoint(t *testing.T) {
	sender := &mockSender{}
	server := httptest.NewServer(buildExternalMux(sender))
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)

	for _, substr := range []string{
		"sin-code",
		"textarea",
		"/submit",
		"/input",
		"/stream",
		"EventSource",
	} {
		if !strings.Contains(html, substr) {
			t.Errorf("expected HTML to contain %q", substr)
		}
	}
}

// ---------------------------------------------------------------------------
// browserInputToMsg unit tests (exercise the conversion used by /input)
// ---------------------------------------------------------------------------

func TestBrowserInputToMsgKeyEnter(t *testing.T) {
	msg := browserInputToMsg(externalInput{Type: "key", Key: "enter"})
	if msg == nil {
		t.Fatal("expected non-nil msg for enter key")
	}
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg, got %T", msg)
	}
	if kp.Code != tea.KeyEnter {
		t.Errorf("expected KeyEnter, got %v", kp.Code)
	}
}

func TestBrowserInputToMsgMouse(t *testing.T) {
	msg := browserInputToMsg(externalInput{Type: "mouse", X: 3, Y: 7, Button: 2})
	if msg == nil {
		t.Fatal("expected non-nil msg for mouse event")
	}
	mc, ok := msg.(tea.MouseClickMsg)
	if !ok {
		t.Fatalf("expected MouseClickMsg, got %T", msg)
	}
	if mc.X != 3 || mc.Y != 7 {
		t.Errorf("expected (3,7), got (%d,%d)", mc.X, mc.Y)
	}
}

func TestBrowserInputToMsgInvalidType(t *testing.T) {
	msg := browserInputToMsg(externalInput{Type: "gesture"})
	if msg != nil {
		t.Errorf("expected nil for invalid type, got %T", msg)
	}
}

func TestBrowserInputToMsgEmptyTypeWithKey(t *testing.T) {
	// Empty type should be treated as "key" per browserInputToMsg logic
	msg := browserInputToMsg(externalInput{Type: "", Key: "a"})
	if msg == nil {
		t.Fatal("expected non-nil msg for empty type with valid key")
	}
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		t.Fatalf("expected KeyPressMsg, got %T", msg)
	}
	if kp.Text != "a" {
		t.Errorf("expected text 'a', got %q", kp.Text)
	}
}
