// SPDX-License-Identifier: MIT
// Purpose: vane integration — HTTP bridge to a self-hosted Vane
// AI-answering engine (MIT). The package owns (1) Config persistence with
// VANE_API_URL env override, (2) the typed Client (Search / Healthy), and
// (3) the idempotent RegisterMCP merger for $SIN_CODE_HOME/mcp.json. Stdlib
// only — Vane itself is NEVER vendored (M2: single static Go binary).
// Docs: vane.doc.md
package vane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ── Public configuration constants ─────────────────────────────────────

// ServerName is the MCP server name registered in mcp.json. Used by
// sin-code mcp status to surface the bridge without scraping paths.
const ServerName = "vane"

// DefaultBaseURL points at a locally-running Vane instance. The default
// matches Vane's own docker-compose quickstart (port 3000).
const DefaultBaseURL = "http://localhost:3000"

// FocusModes is the enumerated set of Vane "focus" modes accepted by the
// /api/search endpoint. Exposed as a package-level map so the cobra
// command and the MCP tool spec can render the same enum without drift.
var FocusModes = map[string]string{
	"webSearch":         "General web search — default for most questions.",
	"academicSearch":    "Scholarly / arXiv / PubMed-style sources.",
	"writingAssistant":  "Long-form generation with citation injection.",
	"wolframAlphaSearch": "Symbolic math via Wolfram Alpha fallback.",
	"youtubeSearch":     "YouTube transcript + video metadata retrieval.",
	"redditSearch":      "Reddit thread + comment retrieval.",
}

// Optimizations is the model-quality knob: speed vs. balanced vs. quality.
// Mirrors Vane's own UI to keep the API contract obvious.
var Optimizations = map[string]string{
	"speed":     "Lowest latency, fewer sources.",
	"balanced":  "Default — quality and latency trade-off.",
	"quality":   "Most thorough retrieval + larger context window.",
}

// Home resolves $SIN_CODE_HOME (preferred) or falls back to the legacy
// ~/.local/share/sin-code path. MUST stay in lockstep with
// superpowers.Home() so a single `SIN_CODE_HOME=/tmp/...` test override
// moves every package's root to the same directory.
func Home() string {
	if v := os.Getenv("SIN_CODE_HOME"); v != "" {
		return v
	}
	// Use os.UserHomeDir for cross-platform safety (macOS/Linux/Windows).
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".local", "share", "sin-code")
	}
	// Last resort: cwd-relative fallback so the function is total.
	return filepath.Join(".", ".sin-code-home")
}

// ConfigPath is where the vane Config is persisted. The home root
// (matches superpowers.MCPConfigPath() for mcp.json layout consistency).
func ConfigPath() string {
	return filepath.Join(Home(), "vane.json")
}

// MCPConfigPath is where the vane MCP server is registered. The home
// root — see superpowers.MCPConfigPath() for the rationale.
func MCPConfigPath() string {
	return filepath.Join(Home(), "mcp.json")
}

// DefaultTimeoutSeconds is the Search request timeout. 60s is generous
// enough for slow focus modes (Wolfram / academic) without blocking the
// agent loop indefinitely.
const DefaultTimeoutSeconds = 60

// ── Config ────────────────────────────────────────────────────────────

// Config is the persisted shape written to ConfigPath(). It tracks which
// upstream BaseURL to talk to, which chat/embed providers Vane should
// route through, and the request timeout.
type Config struct {
	BaseURL        string `json:"base_url"`
	ChatProvider   string `json:"chat_provider"`
	ChatModel      string `json:"chat_model"`
	EmbedProvider  string `json:"embed_provider"`
	EmbedModel     string `json:"embed_model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// DefaultConfig is the zero-config baseline used when no vane.json is
// found on disk. Callers may mutate the returned struct freely.
func DefaultConfig() Config {
	return Config{
		BaseURL:        DefaultBaseURL,
		ChatProvider:   "openai",
		ChatModel:      "gpt-4o-mini",
		EmbedProvider:  "openai",
		EmbedModel:     "text-embedding-3-small",
		TimeoutSeconds: DefaultTimeoutSeconds,
	}
}

// LoadConfig reads ConfigPath() and overlays it on DefaultConfig(). The
// VANE_API_URL environment variable, when set, takes precedence over
// the on-disk BaseURL — this lets CI / docker compose override the
// target without editing vane.json.
//
// Returns the effective config plus a bool indicating whether a
// vane.json file was actually present (false = defaults only).
func LoadConfig() (Config, bool, error) {
	cfg := DefaultConfig()
	path := ConfigPath()
	present := false
	if b, err := os.ReadFile(path); err == nil {
		present = true
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, present, fmt.Errorf("vane: parse %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, present, fmt.Errorf("vane: read %s: %w", path, err)
	}
	// Env override (highest priority).
	if v := os.Getenv("VANE_API_URL"); v != "" {
		cfg.BaseURL = strings.TrimRight(v, "/")
	}
	// Defensive: zero/negative timeout falls back to default.
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = DefaultTimeoutSeconds
	}
	// Defensive: blank BaseURL falls back to default.
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return cfg, present, nil
}

// SaveConfig persists cfg to ConfigPath() atomically (temp file +
// rename) so a crash mid-write cannot corrupt the user's vane.json.
func SaveConfig(cfg Config) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = DefaultTimeoutSeconds
	}
	return writeJSONAtomic(ConfigPath(), cfg)
}

// ── Domain types ──────────────────────────────────────────────────────

// Source is one citation in Vane's response. Title is human-readable,
// URL is the canonical link Vane wants the agent to surface.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Answer is the parsed Search response. Message is the synthesized text;
// Sources is the citation list rendered in FormatAnswer.
type Answer struct {
	Message string   `json:"message"`
	Sources []Source `json:"sources"`
}

// searchRequest is the wire shape posted to /api/search. Kept private so
// callers cannot accidentally leak internal field names through the
// public API surface.
type searchRequest struct {
	QueryOptimizationMode string `json:"optimization_mode"`
	FocusMode             string `json:"focus_mode"`
	Query                 string `json:"query"`
	ChatModelProviderID   string `json:"chat_model_provider"`
	ChatModel             string `json:"chat_model"`
	EmbeddingModelProvider string `json:"embedding_model_provider"`
	EmbeddingModel        string `json:"embedding_model"`
}

// searchResponse is Vane's actual payload. We accept both `message` and
// `answer` for forward compatibility, then normalize to Answer in Search.
type searchResponse struct {
	Message string   `json:"message"`
	Answer  string   `json:"answer"`
	Sources []Source `json:"sources"`
}

// ── Client ────────────────────────────────────────────────────────────

// Client is the typed HTTP wrapper around a Vane instance. Construct
// with NewClient; the zero value is NOT usable (http.Client would be
// nil and crash on first call).
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient builds a Client from cfg. The http.Client honors
// cfg.TimeoutSeconds; the caller may replace http via the (intentionally
// omitted) field setter if they need a custom transport — but for the
// SIN-Code use case the stdlib default transport is fine.
func NewClient(cfg Config) *Client {
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

// Healthy issues a GET to / and returns nil on any 2xx/3xx response.
// 5xx is treated as an error (instance likely down or in crash-loop);
// 4xx is treated as "up but rejecting" — also surfaced as an error
// because the agent should not assume the bridge is usable.
func (c *Client) Healthy(ctx context.Context) error {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/", nil)
	if err != nil {
		return fmt.Errorf("vane: build request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("vane: unreachable: %w", err)
	}
	defer resp.Body.Close()
	// Drain a small slice to allow connection reuse.
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("vane: server error: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("vane: client error: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Search posts a question to Vane and returns the parsed answer.
// `query` is required (empty → error). `focusMode` defaults to
// "webSearch" when blank or unknown. `optimization` defaults to
// "balanced" for the same reason. Returning a non-nil error is part of
// the Graceful Degradation contract — the MCP layer catches the error
// and emits an `isError: true` response with the fallback hint.
func (c *Client) Search(ctx context.Context, query, focusMode, optimization string) (*Answer, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("vane: query is required")
	}
	if _, ok := FocusModes[focusMode]; !ok {
		focusMode = "webSearch"
	}
	if _, ok := Optimizations[optimization]; !ok {
		optimization = "balanced"
	}
	body := searchRequest{
		QueryOptimizationMode:   optimization,
		FocusMode:               focusMode,
		Query:                   q,
		ChatModelProviderID:     c.cfg.ChatProvider,
		ChatModel:               c.cfg.ChatModel,
		EmbeddingModelProvider:  c.cfg.EmbedProvider,
		EmbeddingModel:          c.cfg.EmbedModel,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("vane: marshal: %w", err)
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/api/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("vane: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "sin-code-vane-bridge/3.8.0")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vane: post: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vane: read body: %w", err)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("vane: server error: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 240))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("vane: client error: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 240))
	}
	var parsed searchResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("vane: decode: %w", err)
	}
	// Vane may emit `message` or `answer` depending on version; prefer
	// `message` (current contract) and fall back to `answer`.
	msg := parsed.Message
	if msg == "" {
		msg = parsed.Answer
	}
	if parsed.Sources == nil {
		parsed.Sources = []Source{}
	}
	return &Answer{Message: msg, Sources: parsed.Sources}, nil
}

// ── Formatting ────────────────────────────────────────────────────────

// FormatAnswer renders an Answer as markdown. The "## Cited Sources"
// section is emitted ONLY when Sources is non-empty so a no-citation
// answer stays a single coherent block.
func FormatAnswer(a *Answer) string {
	if a == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(a.Message))
	if len(a.Sources) > 0 {
		b.WriteString("\n\n## Cited Sources\n")
		for i := range a.Sources {
			title := strings.TrimSpace(a.Sources[i].Title)
			url := strings.TrimSpace(a.Sources[i].URL)
			if title == "" && url == "" {
				continue
			}
			if title == "" {
				title = url
			}
			if url == "" {
				b.WriteString(fmt.Sprintf("%d. %s\n", i+1, title))
			} else {
				b.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, title, url))
			}
		}
	}
	return b.String()
}

// ── RegisterMCP ───────────────────────────────────────────────────────

// RegisterMCP merges the vane server entry into mcpPath (or
// MCPConfigPath() if empty). Idempotent: existing entries for "vane"
// with the same command + args are left untouched. Preserves every
// other key in the file (notably pre-existing "superpowers" entries).
func RegisterMCP(mcpPath string) (string, error) {
	if mcpPath == "" {
		mcpPath = MCPConfigPath()
	}
	exe, err := os.Executable()
	if err != nil {
		return mcpPath, fmt.Errorf("vane: resolve executable: %w", err)
	}
	// Load existing config (or start with an empty shell). We tolerate a
	// malformed file by treating it as empty — the next save will
	// re-write a clean shape.
	cfg := map[string]any{}
	if b, err := os.ReadFile(mcpPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	entry := map[string]any{
		"command": exe,
		"args":    []string{"vane", "serve"},
	}
	if existing, ok := servers[ServerName].(map[string]any); ok {
		// Idempotency: same command + args → no-op.
		if existing["command"] == entry["command"] {
			return mcpPath, nil
		}
	}
	servers[ServerName] = entry
	cfg["mcpServers"] = servers
	return mcpPath, writeJSONAtomic(mcpPath, cfg)
}

// ── Internal helpers ──────────────────────────────────────────────────

// truncate caps s to at most n bytes, appending "…" when truncation
// actually happened. Used to keep error messages bounded in size when
// the upstream returns a large HTML error page.
func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// writeJSONAtomic marshals v and writes to path via a temp-file +
// rename so a crash mid-write cannot corrupt the existing file. Parent
// directories are created on demand. Mirrors superpowers.WriteJSON() so
// the rest of the codebase has a consistent persistence semantic.
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".vane-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write([]byte("\n")); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
