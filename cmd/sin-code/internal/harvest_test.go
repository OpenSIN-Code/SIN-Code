// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the harvest subcommand.
package internal

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHarvestURLFetch_Success(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"message":"hello"}`)
	}))
	defer server.Close()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := harvestURLFetch(server.URL, "GET", 5, "text")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("harvestURLFetch failed: %v", err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "hello") {
		t.Errorf("expected output to contain 'hello', got %q", string(out))
	}
}

func TestHarvestURLFetch_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "test")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"data": 123}`)
	}))
	defer server.Close()

	oldStdout := os.Stdout
	pr, pw, _ := os.Pipe()
	os.Stdout = pw

	err := harvestURLFetch(server.URL, "GET", 5, "json")
	pw.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("harvestURLFetch failed: %v", err)
	}
	out, _ := io.ReadAll(pr)
	if !strings.Contains(string(out), `"status": 200`) {
		t.Errorf("expected JSON output to contain status 200, got %q", string(out))
	}
}

func TestHarvestURLFetch_Cache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"count":`+fmt.Sprintf("%d", callCount)+`}`)
	}))
	defer server.Close()

	// Clear cache first
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "sin-code", "harvest")
	os.RemoveAll(cacheDir)

	// First fetch should hit server
	err := harvestURLFetch(server.URL, "GET", 5, "text")
	if err != nil {
		t.Fatalf("first harvestURLFetch failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected server to be called once, got %d", callCount)
	}

	// Second fetch should hit cache
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	err = harvestURLFetch(server.URL, "GET", 5, "text")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("second harvestURLFetch failed: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected cache hit (server call count still 1), got %d", callCount)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "[CACHED") {
		t.Errorf("expected cached output to contain [CACHED], got %q", string(out))
	}
}

func TestHarvestURLFetch_Error(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := harvestURLFetch("http://localhost:1", "GET", 1, "text")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("expected no error for failing URL (error is printed), got %v", err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "ERROR:") {
		t.Errorf("expected output to contain 'ERROR:', got %q", string(out))
	}
}

func TestHarvestURLFetch_InvalidURL(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := harvestURLFetch("not-a-valid-url", "GET", 5, "text")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("expected no error (error printed), got %v", err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "ERROR:") {
		t.Errorf("expected output to contain 'ERROR:', got %q", string(out))
	}
}

func TestHarvestURLFetch_PostMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"ok": true}`)
	}))
	defer server.Close()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := harvestURLFetch(server.URL, "POST", 5, "text")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("harvestURLFetch failed: %v", err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "ok") {
		t.Errorf("expected output to contain 'ok', got %q", string(out))
	}
}

func TestHarvestURLFetch_CacheJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"data":"cached"}`)
	}))
	defer server.Close()

	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "sin-code", "harvest")
	os.RemoveAll(cacheDir)

	err := harvestURLFetch(server.URL, "GET", 5, "text")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	err = harvestURLFetch(server.URL, "GET", 5, "json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("cached json fetch failed: %v", err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), `"cached": true`) {
		t.Errorf("expected JSON output to contain cached=true, got %q", string(out))
	}
}

func TestHarvestURLFetch_ErrorJSON(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := harvestURLFetch("http://localhost:1", "GET", 1, "json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("expected no error for failing URL in json mode, got %v", err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), `"error"`) {
		t.Errorf("expected JSON output to contain error field, got %q", string(out))
	}
}

func TestHarvestURLFetch_InvalidURLJSON(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := harvestURLFetch("not-a-valid-url", "GET", 5, "json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("expected no error for invalid URL in json mode, got %v", err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), `"error"`) {
		t.Errorf("expected JSON output to contain error field, got %q", string(out))
	}
}

func TestHarvestURLFetch_MissingURL(t *testing.T) {
	harvestURL = ""
	err := HarvestCmd.RunE(HarvestCmd, []string{})
	if err == nil {
		t.Error("expected error when --url is missing")
	}
}

func TestHarvestURLFetch_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "internal error")
	}))
	defer server.Close()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := harvestURLFetch(server.URL, "GET", 5, "json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("harvestURLFetch failed: %v", err)
	}
	out, _ := io.ReadAll(r)
	// With the circuitbreaker RoundTripper wrapping the transport
	// (#72), 5xx responses are converted into a transport-level error
	// BEFORE harvest.go can record `result.Status = resp.StatusCode`.
	// Either the literal `"status": 500` (pre-breaker) or the new
	// `"error"`-string carrying `circuitbreaker: HTTP 500` is
	// acceptable — both prove "5xx surfaced as an error".
	s := string(out)
	if !strings.Contains(s, `"status": 500`) && !strings.Contains(s, "circuitbreaker: HTTP 500") {
		t.Errorf("expected JSON to surface 5xx (status 500 OR circuitbreaker error), got %q", s)
	}
}

// errBody is a response body that always fails when read.
type errBody struct{}

func (errBody) Read(_ []byte) (int, error) { return 0, fmt.Errorf("simulated body read error") }
func (errBody) Close() error               { return nil }

func TestHarvestURLFetch_ReadBodyError(t *testing.T) {
	oldClient := harvestHTTPClient
	harvestHTTPClient = func(timeout int) *http.Client {
		return &http.Client{
			Transport: roundTripperFn(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       errBody{},
				}, nil
			}),
		}
	}
	defer func() { harvestHTTPClient = oldClient }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := harvestURLFetch("http://example.com", "GET", 5, "json")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("harvestURLFetch failed: %v", err)
	}
	out, _ := io.ReadAll(r)
	s := string(out)
	if !strings.Contains(s, "read body:") {
		t.Errorf("expected JSON output to contain 'read body:' error, got %q", s)
	}
}

// roundTripperFn adapts a function to http.RoundTripper.
type roundTripperFn func(*http.Request) (*http.Response, error)

func (f roundTripperFn) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
