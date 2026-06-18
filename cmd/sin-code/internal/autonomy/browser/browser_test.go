// SPDX-License-Identifier: MIT
// Purpose: tests for the browser-automation primitives (issue #382).
package browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBrowser_Fetch_Stub(t *testing.T) {
	stub := &StubTransport{
		Response: Response{StatusCode: 200, Body: []byte("hello")},
	}
	b := NewBrowser(stub)
	resp, err := b.Fetch(context.Background(), Request{URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "hello" {
		t.Fatalf("resp = %+v", resp)
	}
	if stub.LastReq.URL != "https://example.com" {
		t.Errorf("last req URL = %q", stub.LastReq.URL)
	}
	if stub.LastReq.Headers["User-Agent"] == "" {
		t.Error("default User-Agent not set")
	}
}

func TestBrowser_Fetch_EmptyURL(t *testing.T) {
	b := NewBrowser(&StubTransport{})
	if _, err := b.Fetch(context.Background(), Request{URL: ""}); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestBrowser_Fetch_DefaultMethod(t *testing.T) {
	stub := &StubTransport{Response: Response{StatusCode: 200}}
	b := NewBrowser(stub)
	_, _ = b.Fetch(context.Background(), Request{URL: "https://x.com"})
	if stub.LastReq.Method != http.MethodGet {
		t.Fatalf("method = %q; want GET", stub.LastReq.Method)
	}
}

func TestBrowser_Navigate(t *testing.T) {
	stub := &StubTransport{
		Response: Response{StatusCode: 200, Body: []byte("<html>hi</html>")},
	}
	b := NewBrowser(stub)
	page, err := b.Navigate(context.Background(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if page.URL != "https://example.com" {
		t.Errorf("page URL = %q", page.URL)
	}
	if string(page.Response.Body) != "<html>hi</html>" {
		t.Errorf("body = %q", page.Response.Body)
	}
}

func TestBrowser_Navigate_Error(t *testing.T) {
	stub := &StubTransport{Err: errors.New("boom")}
	b := NewBrowser(stub)
	if _, err := b.Navigate(context.Background(), "https://x.com"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBrowser_Render_Stub(t *testing.T) {
	stub := &StubTransport{
		Response: Response{StatusCode: 200, Body: []byte("<html>rendered</html>")},
	}
	b := NewBrowser(stub)
	res, err := b.Render(context.Background(), "https://example.com", RenderOpts{
		Width:  1280,
		Height: 720,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.HTML != "<html>rendered</html>" {
		t.Fatalf("html = %q", res.HTML)
	}
	if res.URL != "https://example.com" {
		t.Errorf("url = %q", res.URL)
	}
	if res.Duration <= 0 {
		t.Error("duration should be positive")
	}
}

func TestBrowser_StdlibTransport_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "yes")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("real"))
	}))
	defer srv.Close()

	b := NewBrowser(nil) // uses stdlibTransport
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := b.Fetch(ctx, Request{URL: srv.URL, Method: http.MethodGet})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d; want 202", resp.StatusCode)
	}
	if string(resp.Body) != "real" {
		t.Errorf("body = %q", resp.Body)
	}
	if resp.Headers["X-Custom"] != "yes" {
		t.Errorf("header missing: %+v", resp.Headers)
	}
}

func TestStubTransport_Error(t *testing.T) {
	stub := &StubTransport{Err: errors.New("network down")}
	b := NewBrowser(stub)
	_, err := b.Fetch(context.Background(), Request{URL: "https://x.com"})
	if err == nil || err.Error() != "network down" {
		t.Fatalf("err = %v", err)
	}
}
