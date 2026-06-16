// SPDX-License-Identifier: MIT
// Purpose: deterministic fault-injection HTTP server for the test harness.
//
// NewFaultServer returns an httptest.Server with one route per finding
// category so each browser test can navigate to a known-bad (or known-good)
// page and assert on a single class of problem in isolation.
package cdp

import (
	"net/http"
	"net/http/httptest"
	"time"
)

// NewFaultServer returns an httptest server that deterministically reproduces
// every finding category the recorder must catch. Each route triggers exactly
// one class of problem so tests can assert on it in isolation.
func NewFaultServer() *httptest.Server {
	mux := http.NewServeMux()

	// 1. Console error + uncaught exception (ReferenceError).
	mux.HandleFunc("/console-error", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><body><h1>console</h1><script>
			console.error("boom: deterministic console error");
			thisSymbolDoesNotExist();
		</script></body></html>`))
	})

	// 2. HTTP 500 on a fetched subresource.
	mux.HandleFunc("/server-500.js", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`// 500`))
	})
	mux.HandleFunc("/http-500", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><body><h1>500</h1>
			<script src="/server-500.js"></script></body></html>`))
	})

	// 3. Failed request (resource that never resolves -> loadingFailed).
	// Points at port 1 which nothing listens on -> immediate connection refused.
	mux.HandleFunc("/net-fail", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><body><h1>fail</h1>
			<img src="http://127.0.0.1:1/missing.png"></body></html>`))
	})

	// 4. CSP violation (inline script blocked by a strict policy -> Audits issue).
	mux.HandleFunc("/csp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Security-Policy", "script-src 'none'")
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><body><h1>csp</h1>
			<script>console.log("blocked by csp")</script></body></html>`))
	})

	// 5. Slow LCP (delayed hero image -> poor LCP vital when InstallVitals is active).
	mux.HandleFunc("/slow-img.png", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(1200 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		w.Write(onePxPNG)
	})
	mux.HandleFunc("/slow-lcp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><body>
			<img src="/slow-img.png" style="width:600px;height:400px"></body></html>`))
	})

	// 6. Clean page (control: must produce zero error findings).
	mux.HandleFunc("/clean", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><body><h1>all good</h1>
			<script>console.log("nothing wrong here")</script></body></html>`))
	})

	return httptest.NewServer(mux)
}
