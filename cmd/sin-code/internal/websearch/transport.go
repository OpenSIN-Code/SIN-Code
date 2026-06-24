// SPDX-License-Identifier: MIT
// Purpose: HTTP transport abstraction for the web-search engine (issue #381).
// HTTPDoer lets tests inject an httptest.Server-backed client without
// touching the real network. The default stdlibDoer wraps http.Client.
package websearch

import (
	"net/http"
	"time"
)

// sin-debt: yagni, upgrade: when a second implementation lands, remove this marker
// HTTPDoer is the minimal HTTP client surface the providers need.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// stdlibDoer wraps http.Client and implements HTTPDoer.
type stdlibDoer struct {
	client *http.Client
}

// sin-debt: yagni, upgrade: when a second HTTPDoer implementation lands, remove this marker
// NewStdlibDoer returns an HTTPDoer backed by net/http with the given
// timeout. A timeout <= 0 means no timeout.
func NewStdlibDoer(timeout time.Duration) HTTPDoer {
	c := &http.Client{}
	if timeout > 0 {
		c.Timeout = timeout
	}
	return &stdlibDoer{client: c}
}

func (d *stdlibDoer) Do(req *http.Request) (*http.Response, error) {
	return d.client.Do(req)
}
