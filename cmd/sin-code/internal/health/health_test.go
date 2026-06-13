// SPDX-License-Identifier: MIT
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	checker := NewChecker("test-version")
	checker.RegisterCheck("test", func(ctx context.Context) Check {
		return Check{
			Status:  StatusHealthy,
			Message: "test ok",
		}
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	checker.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != StatusHealthy {
		t.Errorf("expected status healthy, got %s", resp.Status)
	}
	if resp.Version != "test-version" {
		t.Errorf("expected version test-version, got %s", resp.Version)
	}
	if resp.Checks["test"].Status != StatusHealthy {
		t.Errorf("expected test check healthy, got %s", resp.Checks["test"].Status)
	}
}

func TestHealthHandlerUnhealthy(t *testing.T) {
	checker := NewChecker("test-version")
	checker.RegisterCheck("test", func(ctx context.Context) Check {
		return Check{
			Status:  StatusUnhealthy,
			Message: "test failed",
		}
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	checker.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestLivenessHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/live", nil)
	w := httptest.NewRecorder()

	LivenessHandler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %s", w.Body.String())
	}
}

func TestReadinessHandler(t *testing.T) {
	checker := NewChecker("test-version")
	checker.RegisterCheck("test", func(ctx context.Context) Check {
		return Check{Status: StatusHealthy}
	})

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	ReadinessHandler(checker).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRuntimeInfo(t *testing.T) {
	info := RuntimeInfo()

	if _, ok := info["go_version"]; !ok {
		t.Error("missing go_version")
	}
	if _, ok := info["num_cpu"]; !ok {
		t.Error("missing num_cpu")
	}
	if _, ok := info["memory"]; !ok {
		t.Error("missing memory")
	}
}

func TestInfoHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/info", nil)
	w := httptest.NewRecorder()

	InfoHandler("test-version").ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var info map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if info["version"] != "test-version" {
		t.Errorf("expected version test-version, got %v", info["version"])
	}
}
