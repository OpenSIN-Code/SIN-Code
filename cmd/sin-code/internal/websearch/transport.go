// SPDX-License-Identifier: MIT
// Purpose: stdlib HTTP transport abstraction for the websearch engine.
// Lets the five providers share one injectable roundtripper so tests
// can spin an httptest.Server and capture outbound requests without
// touching the production net/http layer.
package websearch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// httpRequest is the request shape the engine net/http layer gets
// after providers build it. Kept distinct from the public Provider
// API so future schema changes do not ripple into engine.go.
type httpRequest struct {
	method string
	url    string
	header http.Header
	body   []byte
}

// httpResponse is the response shape net/http returns.
type httpResponse struct {
	status int
	header http.Header
	body   []byte
}

// stdlibDoer is the production HTTPDoer implementation backed by
// net/http.Client. Zero value uses http.DefaultClient with a 15s
// timeout (matches the legacy MCP server default).
type stdlibDoer struct {
	client *http.Client
}

// newStdlibDoer returns a stdlibDoer with sensible defaults. Tests can
// pass nil client to fall back to http.DefaultClient.
func newStdlibDoer(c *http.Client) *stdlibDoer {
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	return &stdlibDoer{client: c}
}

func (s *stdlibDoer) Do(req *httpRequest) (*httpResponse, error) {
	if req == nil {
		return nil, errors.New("websearch: nil request")
	}
	var body io.Reader
	if len(req.body) > 0 {
		body = bytes.NewReader(req.body)
	}
	r, err := http.NewRequestWithContext(context.Background(), req.method, req.url, body)
	if err != nil {
		return nil, err
	}
	if req.header != nil {
		r.Header = req.header.Clone()
	}
	if r.Header.Get("User-Agent") == "" {
		r.Header.Set("User-Agent", "sin-code/1.0 (+native websearch)")
	}
	resp, err := s.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &httpResponse{
		status: resp.StatusCode,
		header: resp.Header.Clone(),
		body:   buf,
	}, nil
}
