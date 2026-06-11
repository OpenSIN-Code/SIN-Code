// SPDX-License-Identifier: MIT
// Purpose: tests for the webui server lifecycle + accessors.

package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServer_AddrAndHandler(t *testing.T) {
	s, err := NewServer(Config{Port: 0, TodoDB: "", NotifDB: ""})
	if err != nil {
		t.Fatal(err)
	}
	if s.Handler() == nil {
		t.Fatal("nil handler")
	}
	addr := s.Addr()
	if addr == "" {
		t.Fatal("empty addr")
	}
}

func TestServer_HandleIndex_NoCrash(t *testing.T) {
	s, _ := NewServer(Config{Port: 0, TodoDB: "", NotifDB: ""})
	rec := httptest.NewRecorder()
	s.handleIndex(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code == 0 {
		t.Fatal("no response code set")
	}
}

func TestServer_HandleAgentsJSON(t *testing.T) {
	s, _ := NewServer(Config{Port: 0, TodoDB: "", NotifDB: ""})
	rec := httptest.NewRecorder()
	s.handleAgentsJSON(rec, httptest.NewRequest("GET", "/api/agents", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
}

func TestServer_RenderErrorsOnUnknownTemplate(t *testing.T) {
	s, _ := NewServer(Config{Port: 0, TodoDB: "", NotifDB: ""})
	rec := httptest.NewRecorder()
	s.render(rec, "does-not-exist", pageData{})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: %d", rec.Code)
	}
}

func TestDefaultAgentConfigs_NonEmpty(t *testing.T) {
	defaults := defaultAgentConfigs()
	if len(defaults) == 0 {
		t.Fatal("no default agent configs")
	}
}

func TestDefaultTodoDB_Idempotent(t *testing.T) {
	// defaultTodoDB is a pure function (no I/O) — calling it twice
	// must return the same path.
	a := defaultTodoDB()
	b := defaultTodoDB()
	if a != b {
		t.Fatalf("defaultTodoDB not deterministic: %q vs %q", a, b)
	}
}
