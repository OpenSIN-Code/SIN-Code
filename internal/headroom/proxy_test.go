// internal/headroom/proxy_test.go
package headroom

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewProxy_InvalidURL(t *testing.T) {
	cfg := DefaultConfig()
	comp := NewCompressor(cfg)
	if _, err := NewProxy(cfg, comp, "://bad-url"); err == nil {
		t.Error("expected error for invalid upstream URL")
	}
}

func TestIsChatCompletionPath(t *testing.T) {
	cases := map[string]bool{
		"/v1/chat/completions": true,
		"/chat/completions":    true,
		"/v1/messages":         true,
		"/v1/completions":      true,
		"/v1/models":           false,
		"/health":              false,
		"/":                    false,
	}
	for path, want := range cases {
		if got := isChatCompletionPath(path); got != want {
			t.Errorf("isChatCompletionPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestProxy_HealthEndpoint(t *testing.T) {
	cfg := DefaultConfig()
	comp := NewCompressor(cfg)
	proxy, err := NewProxy(cfg, comp, "https://api.openai.com")
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid health JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
}

func TestProxy_ForwardsToUpstream(t *testing.T) {
	// Upstream test server records what it received and replies with a fixed body.
	var received string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		received = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	// Disabled compressor => passthrough, so we test the plumbing deterministically.
	cfg := DefaultConfig()
	cfg.Enabled = false
	comp := NewCompressor(cfg)

	proxy, err := NewProxy(cfg, comp, upstream.URL)
	if err != nil {
		t.Fatalf("NewProxy failed: %v", err)
	}

	front := httptest.NewServer(proxy.Handler())
	defer front.Close()

	reqBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello world"}]}`
	resp, err := http.Post(front.URL+"/v1/chat/completions", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), `"ok":true`) {
		t.Errorf("unexpected upstream response: %s", respBody)
	}
	if !strings.Contains(received, "hello world") {
		t.Errorf("upstream did not receive the forwarded message: %s", received)
	}
}

func TestProxy_CompressBodyNoMessages(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	comp := NewCompressor(cfg)
	proxy, _ := NewProxy(cfg, comp, "https://api.openai.com")

	if _, ok := proxy.compressBody(context.Background(), []byte(`{"model":"gpt-4"}`)); ok {
		t.Error("compressBody should return ok=false when there are no messages")
	}
}
