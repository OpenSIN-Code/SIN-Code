// SPDX-License-Identifier: MIT
// Purpose: Web dashboard for tool usage telemetry (issue #369). The
// DashboardServer collects per-call statistics (call count, per-tool and
// per-server breakdowns, average latency, error count) and exposes them via a
// stdlib HTTP handler: JSON at /api/stats and a self-contained HTML page at /.
// It has no external dependencies (mandate M2) and is safe to embed behind the
// existing webui Server or to run standalone via Start.
//
// Thread-safe: all stats access takes s.mu (mandate M7).
package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// DashboardStats is the aggregated telemetry snapshot served by the dashboard.
type DashboardStats struct {
	TotalCalls  int
	ByTool      map[string]int
	ByServer    map[string]int
	AvgLatency  float64
	Errors      int
	LastUpdated time.Time
}

// DashboardServer collects tool-call telemetry and serves a web dashboard.
type DashboardServer struct {
	addr  string
	mu    sync.Mutex
	stats DashboardStats
	// totalLatency accumulates raw durations so AvgLatency stays exact.
	totalLatency time.Duration
}

// NewDashboardServer creates a dashboard bound to addr.
func NewDashboardServer(addr string) *DashboardServer {
	return &DashboardServer{
		addr: addr,
		stats: DashboardStats{
			ByTool:   make(map[string]int),
			ByServer: make(map[string]int),
		},
	}
}

// RecordCall records a single tool invocation. latency is the call duration;
// err marks the call as failed. AvgLatency is reported in milliseconds.
func (s *DashboardServer) RecordCall(tool, server string, latency time.Duration, err bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.TotalCalls++
	s.stats.ByTool[tool]++
	s.stats.ByServer[server]++
	s.totalLatency += latency
	if s.stats.TotalCalls > 0 {
		s.stats.AvgLatency = float64(s.totalLatency.Nanoseconds()) / float64(s.stats.TotalCalls) / 1e6
	}
	if err {
		s.stats.Errors++
	}
	s.stats.LastUpdated = time.Now()
}

// Stats returns a defensive copy of the current statistics.
func (s *DashboardServer) Stats() DashboardStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cloneStatsLocked()
}

func (s *DashboardServer) cloneStatsLocked() DashboardStats {
	cp := s.stats
	cp.ByTool = make(map[string]int, len(s.stats.ByTool))
	for k, v := range s.stats.ByTool {
		cp.ByTool[k] = v
	}
	cp.ByServer = make(map[string]int, len(s.stats.ByServer))
	for k, v := range s.stats.ByServer {
		cp.ByServer[k] = v
	}
	return cp
}

// RenderJSON returns the current statistics as pretty-printed JSON.
func (s *DashboardServer) RenderJSON() ([]byte, error) {
	stats := s.Stats()
	return json.MarshalIndent(stats, "", "  ")
}

// RenderHTML returns a self-contained HTML dashboard page showing the current
// statistics. No external CSS/JS is required.
func (s *DashboardServer) RenderHTML() string {
	stats := s.Stats()

	toolRows := sortedRows(stats.ByTool)
	serverRows := sortedRows(stats.ByServer)

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>SIN-Code Tool Dashboard</title>
<style>
body{font-family:system-ui,sans-serif;margin:2rem;background:#0d1117;color:#c9d1d9}
h1{color:#58a6ff}
table{border-collapse:collapse;width:100%;max-width:720px;margin-top:1rem}
th,td{border:1px solid #30363d;padding:.5rem .75rem;text-align:left}
th{background:#161b22;color:#8b949e}
.stat{display:inline-block;margin-right:2rem;padding:.75rem 1rem;background:#161b22;border-radius:6px}
.stat b{color:#58a6ff;font-size:1.4rem}
</style>
</head>
<body>
<h1>SIN-Code Tool Dashboard</h1>
`)
	fmt.Fprintf(&b, `<p class="stat">Total calls: <b>%d</b></p>`+"\n", stats.TotalCalls)
	fmt.Fprintf(&b, `<p class="stat">Avg latency: <b>%.2f ms</b></p>`+"\n", stats.AvgLatency)
	fmt.Fprintf(&b, `<p class="stat">Errors: <b>%d</b></p>`+"\n", stats.Errors)
	if !stats.LastUpdated.IsZero() {
		fmt.Fprintf(&b, `<p class="stat">Last updated: <b>%s</b></p>`+"\n", stats.LastUpdated.Format(time.RFC3339))
	}

	b.WriteString("<h2>Calls by tool</h2>\n<table><tr><th>Tool</th><th>Calls</th></tr>\n")
	if len(toolRows) == 0 {
		b.WriteString(`<tr><td colspan="2">No data</td></tr>` + "\n")
	}
	for _, r := range toolRows {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%d</td></tr>\n", escapeHTML(r.key), r.val)
	}
	b.WriteString("</table>\n")

	b.WriteString("<h2>Calls by server</h2>\n<table><tr><th>Server</th><th>Calls</th></tr>\n")
	if len(serverRows) == 0 {
		b.WriteString(`<tr><td colspan="2">No data</td></tr>` + "\n")
	}
	for _, r := range serverRows {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%d</td></tr>\n", escapeHTML(r.key), r.val)
	}
	b.WriteString("</table>\n</body>\n</html>\n")
	return b.String()
}

type kv struct {
	key string
	val int
}

func sortedRows(m map[string]int) []kv {
	out := make([]kv, 0, len(m))
	for k, v := range m {
		out = append(out, kv{key: k, val: v})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].val == out[j].val {
			return out[i].key < out[j].key
		}
		return out[i].val > out[j].val
	})
	return out
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ServeHTTP routes /api/stats to JSON and / to the HTML dashboard.
func (s *DashboardServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/stats":
		data, err := s.RenderJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	case "/", "":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = fmt.Fprint(w, s.RenderHTML())
	default:
		http.NotFound(w, r)
	}
}

// Start binds the dashboard to its configured address and serves until the
// listener is closed. It blocks the calling goroutine.
func (s *DashboardServer) Start() error {
	srv := &http.Server{Addr: s.addr, Handler: s}
	return srv.ListenAndServe()
}
