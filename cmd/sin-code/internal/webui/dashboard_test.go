// SPDX-License-Identifier: MIT
package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDashboardRecordCall(t *testing.T) {
	s := NewDashboardServer("127.0.0.1:0")
	s.RecordCall("sin_edit", "sin", 5*time.Millisecond, false)
	s.RecordCall("sin_read", "sin", 10*time.Millisecond, false)
	s.RecordCall("sin_edit", "browser", 2*time.Millisecond, true)

	stats := s.Stats()
	if stats.TotalCalls != 3 {
		t.Errorf("TotalCalls = %d, want 3", stats.TotalCalls)
	}
	if stats.ByTool["sin_edit"] != 2 {
		t.Errorf("ByTool[sin_edit] = %d, want 2", stats.ByTool["sin_edit"])
	}
	if stats.ByServer["sin"] != 2 {
		t.Errorf("ByServer[sin] = %d, want 2", stats.ByServer["sin"])
	}
	if stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1", stats.Errors)
	}
	if stats.AvgLatency <= 0 {
		t.Errorf("AvgLatency = %v, want > 0", stats.AvgLatency)
	}
	if stats.LastUpdated.IsZero() {
		t.Error("LastUpdated not set")
	}
}

func TestDashboardRenderJSON(t *testing.T) {
	s := NewDashboardServer("127.0.0.1:0")
	s.RecordCall("sin_edit", "sin", 4*time.Millisecond, false)
	data, err := s.RenderJSON()
	if err != nil {
		t.Fatal(err)
	}
	var got DashboardStats
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", got.TotalCalls)
	}
	if got.ByTool["sin_edit"] != 1 {
		t.Errorf("ByTool = %v, want sin_edit=1", got.ByTool)
	}
}

func TestDashboardRenderHTML(t *testing.T) {
	s := NewDashboardServer("127.0.0.1:0")
	s.RecordCall("sin_edit", "sin", 4*time.Millisecond, false)
	html := s.RenderHTML()
	if !strings.Contains(html, "SIN-Code Tool Dashboard") {
		t.Error("HTML missing title")
	}
	if !strings.Contains(html, "sin_edit") {
		t.Error("HTML missing tool name")
	}
	if !strings.Contains(html, "<table") {
		t.Error("HTML missing table")
	}
}

func TestDashboardServeStats(t *testing.T) {
	s := NewDashboardServer("127.0.0.1:0")
	s.RecordCall("sin_test", "sin", 3*time.Millisecond, false)
	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q, want json", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	var got DashboardStats
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got.TotalCalls != 1 {
		t.Errorf("TotalCalls = %d, want 1", got.TotalCalls)
	}
}

func TestDashboardServeHTML(t *testing.T) {
	s := NewDashboardServer("127.0.0.1:0")
	s.RecordCall("sin_edit", "sin", 1*time.Millisecond, false)
	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("content-type = %q, want html", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "SIN-Code Tool Dashboard") {
		t.Error("HTML body missing title")
	}
}

func TestDashboardNotFound(t *testing.T) {
	s := NewDashboardServer("127.0.0.1:0")
	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/unknown")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDashboardErrors(t *testing.T) {
	s := NewDashboardServer("127.0.0.1:0")
	s.RecordCall("a", "x", 1*time.Millisecond, true)
	s.RecordCall("b", "x", 1*time.Millisecond, true)
	s.RecordCall("c", "x", 1*time.Millisecond, false)
	stats := s.Stats()
	if stats.Errors != 2 {
		t.Errorf("Errors = %d, want 2", stats.Errors)
	}
	if stats.TotalCalls != 3 {
		t.Errorf("TotalCalls = %d, want 3", stats.TotalCalls)
	}
}

func TestDashboardConcurrent(t *testing.T) {
	s := NewDashboardServer("127.0.0.1:0")
	ts := httptest.NewServer(s)
	defer ts.Close()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.RecordCall("tool", "srv", time.Duration(i)*time.Millisecond, i%5 == 0)
			_, _ = http.Get(ts.URL + "/api/stats")
			_ = s.RenderHTML()
		}(i)
	}
	wg.Wait()

	stats := s.Stats()
	if stats.TotalCalls != 50 {
		t.Errorf("TotalCalls = %d, want 50", stats.TotalCalls)
	}
}
