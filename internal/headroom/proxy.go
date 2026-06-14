// internal/headroom/proxy.go
package headroom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"
)

// Proxy is an HTTP reverse proxy that sits in front of an upstream
// OpenAI-compatible API. It intercepts chat-completion requests, compresses
// the message contents through the configured Compressor, and forwards the
// reduced payload upstream. This gives "zero code change" compression for any
// client that points its base URL at the proxy.
type Proxy struct {
	config     Config
	compressor *Compressor
	upstream   *url.URL
	reverse    *httputil.ReverseProxy
	server     *http.Server
	requests   int64
	compressed int64
}

// chatRequest is the minimal shape of an OpenAI chat-completion body that the
// proxy needs to read and rewrite. Unknown fields are preserved via the raw map.
type chatRequest struct {
	Messages []map[string]interface{} `json:"messages"`
}

// NewProxy builds a proxy that forwards to the given upstream base URL
// (for example "https://api.openai.com"). The compressor is used to shrink
// message contents before they are forwarded.
func NewProxy(cfg Config, compressor *Compressor, upstreamBaseURL string) (*Proxy, error) {
	u, err := url.Parse(upstreamBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid upstream URL %q: %w", upstreamBaseURL, err)
	}
	p := &Proxy{
		config:     cfg,
		compressor: compressor,
		upstream:   u,
	}
	p.reverse = httputil.NewSingleHostReverseProxy(u)
	// Preserve the original director but ensure the Host header targets upstream.
	originalDirector := p.reverse.Director
	p.reverse.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = u.Host
	}
	return p, nil
}

// Handler returns the http.Handler that performs interception + forwarding.
func (p *Proxy) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", p.handleHealth)
	mux.HandleFunc("/", p.handleProxy)
	return mux
}

func (p *Proxy) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "ok",
		"requests":   atomic.LoadInt64(&p.requests),
		"compressed": atomic.LoadInt64(&p.compressed),
	})
}

// handleProxy intercepts the request body, compresses chat messages when
// possible, and forwards everything to the upstream reverse proxy.
func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&p.requests, 1)

	// Only attempt to rewrite JSON chat-completion POST bodies.
	if r.Method == http.MethodPost && r.Body != nil && isChatCompletionPath(r.URL.Path) {
		// Read the body once; we must always restore it so the reverse proxy
		// can forward it whether or not we rewrote anything.
		raw, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err == nil {
			body := raw
			if newBody, ok := p.compressBody(r.Context(), raw); ok {
				body = newBody
				atomic.AddInt64(&p.compressed, 1)
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
			r.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
		}
	}

	p.reverse.ServeHTTP(w, r)
}

// compressBody compresses each message's string content in the given raw JSON
// body and returns the re-encoded body. The second return value indicates
// whether a usable rewritten body was produced (false means "use the original").
func (p *Proxy) compressBody(ctx context.Context, raw []byte) ([]byte, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	// Decode into a generic map so we preserve all unknown fields.
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, false
	}

	msgsRaw, ok := generic["messages"].([]interface{})
	if !ok || len(msgsRaw) == 0 {
		return nil, false
	}

	changed := false
	for _, m := range msgsRaw {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := msg["content"].(string)
		if !ok || content == "" {
			continue
		}
		compressed, _, err := p.compressor.CompressContent(ctx, content)
		if err == nil && compressed != "" && compressed != content {
			msg["content"] = compressed
			changed = true
		}
	}

	if !changed {
		return nil, false
	}

	out, err := json.Marshal(generic)
	if err != nil {
		return nil, false
	}
	return out, true
}

// Start runs the proxy HTTP server on the given address (e.g. ":8787").
// It blocks until the server stops; run it in a goroutine for non-blocking use.
func (p *Proxy) Start(addr string) error {
	p.server = &http.Server{
		Addr:              addr,
		Handler:           p.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return p.server.ListenAndServe()
}

// Shutdown gracefully stops the proxy server.
func (p *Proxy) Shutdown(ctx context.Context) error {
	if p.server == nil {
		return nil
	}
	return p.server.Shutdown(ctx)
}

// isChatCompletionPath reports whether the path looks like an OpenAI-style
// chat or completion endpoint that carries a messages array.
func isChatCompletionPath(path string) bool {
	switch {
	case path == "/v1/chat/completions",
		path == "/chat/completions",
		path == "/v1/messages",
		path == "/v1/completions":
		return true
	default:
		return false
	}
}
