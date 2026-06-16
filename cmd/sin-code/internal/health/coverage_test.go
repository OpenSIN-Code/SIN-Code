// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests to reach 100% statement coverage.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckerVersion(t *testing.T) {
	c := NewChecker("v1.2.3")
	if got := c.Version(); got != "v1.2.3" {
		t.Fatalf("Version: want v1.2.3, got %q", got)
	}
}

func TestCheckDegraded(t *testing.T) {
	c := NewChecker("v")
	c.RegisterCheck("degraded-check", func(ctx context.Context) Check {
		return Check{Status: StatusDegraded, Message: "slow"}
	})
	resp := c.Check(context.Background())
	if resp.Status != StatusDegraded {
		t.Fatalf("want degraded, got %s", resp.Status)
	}
}

func TestReadinessHandlerUnhealthy(t *testing.T) {
	c := NewChecker("v")
	c.RegisterCheck("bad", func(ctx context.Context) Check {
		return Check{Status: StatusUnhealthy, Message: "down"}
	})

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	ReadinessHandler(c).ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "not ready" {
		t.Fatalf("unexpected body: %v", body)
	}
}
