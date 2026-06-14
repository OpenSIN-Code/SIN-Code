// SPDX-License-Identifier: MIT
// Purpose: HTTP integration — wraps an http.RoundTripper so every
// outbound request goes through a Breaker. 5xx responses are treated
// as failures (server-likely-down, agent should back off); transport
// errors (DNS, TCP, TLS, timeout, EOF) are likewise failures. 2xx /
// 3xx / 4xx are successes — a flood of 404s would otherwise cause
// healthy upstreams to be falsely tripped.
// Docs: breaker.doc.md
package circuitbreaker

import (
	"errors"
	"fmt"
	"net/http"
)

// RoundTripper wraps inner so every request goes through breaker. A nil
// breaker returns inner unchanged (lets callers build a "debug: breaker
// disabled" config without branching call sites). A nil inner defaults
// to http.DefaultTransport.
func RoundTripper(inner http.RoundTripper, breaker *Breaker) http.RoundTripper {
	if breaker == nil {
		return inner
	}
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &breakerRoundTripper{inner: inner, breaker: breaker}
}

// breakerRoundTripper is the http.RoundTripper adapter. We intentionally
// implement RoundTripper (not http.Client) so the wrapper composes with
// existing http.Client structures (Timeout, CheckRedirect, Jar) — the
// caller is expected to set c.Transport = RoundTripper(c.Transport, b).
type breakerRoundTripper struct {
	inner   http.RoundTripper
	breaker *Breaker
}

// RoundTrip dispatches the request through the breaker. Failure
// classification:
//   - inner.RoundTrip error (DNS/TLS/TCP/timeout/EOF) → failure
//   - HTTP 5xx → failure (response body is drained + closed so the
//     caller never sees a leaked net.Conn)
//   - HTTP 2xx / 3xx / 4xx → success (4xx is caller-error, not server-
//     health-error)
//
// When the breaker rejects, RoundTrip returns ErrBreakerOpen WITHOUT
// calling inner — this is the "fast fail" property the caller wants.
//
//nolint:bodyclose // the http.RoundTripper contract: returned *http.Response is closed by the caller.
func (r *breakerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Wrap closure captures the response so the breaker can inspect
	// the status code post-call. Body (if any) is buffered lazily —
	// we only need to KNOW statusCode; the caller will read it
	// themselves if RoundTrip returns a *http.Response.
	var resp *http.Response
	err := r.breaker.Execute(func() error {
		var innerErr error
		resp, innerErr = r.inner.RoundTrip(req)
		return classifyHTTPError(resp, innerErr)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// classifyHTTPError maps the (resp, err) pair from an HTTP round trip
// into the breaker's failure/success verdict.
//
// Returns nil → success; non-nil → failure.
//
// The wrapped error returned here is also what propagates to Execute's
// caller as the original error — so callers downstream see the canonical
// "5xx" message rather than a synthetic breaker error.
func classifyHTTPError(resp *http.Response, err error) error {
	if err != nil {
		// Transport-level failure: must count. Wrap with our prefix so
		// the caller's error message identifies the breaker.
		return fmt.Errorf("circuitbreaker: transport error: %w", err)
	}
	if resp == nil {
		return errors.New("circuitbreaker: nil response")
	}
	if resp.StatusCode >= 500 {
		// Drain + close so the connection can be reused by the
		// underlying transport. We MUST NOT leak the body — that's
		// how the http.Client bodyclose linter rule bites us in CI.
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("circuitbreaker: HTTP %d (5xx treated as failure)", resp.StatusCode)
	}
	return nil
}
