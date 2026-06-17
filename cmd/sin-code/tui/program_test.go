// SPDX-License-Identifier: MIT
package tui

import (
	"net/http"
	"runtime"
	"strings"
	"testing"
)

func TestProgramOptionsDefaults(t *testing.T) {
	opts := ProgramOptions{}
	if opts.ExternalMode {
		t.Error("default ExternalMode should be false")
	}
	if opts.Port != 0 {
		t.Errorf("default Port should be 0, got %d", opts.Port)
	}
	if opts.Hostname != "" {
		t.Errorf("default Hostname should be empty, got %q", opts.Hostname)
	}
	if opts.MDNS {
		t.Error("default MDNS should be false")
	}
	if opts.Sigusr2Reload {
		t.Error("default Sigusr2Reload should be false")
	}
}

func TestReloadMsg(t *testing.T) {
	var msg any = ReloadMsg{}
	rm, ok := msg.(ReloadMsg)
	if !ok {
		t.Fatalf("expected ReloadMsg, got %T", msg)
	}
	if rm != (ReloadMsg{}) {
		t.Errorf("expected zero-value ReloadMsg, got %+v", rm)
	}
}

func TestReloadCmd(t *testing.T) {
	cmd := ReloadCmd()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(ReloadMsg); !ok {
		t.Errorf("expected ReloadMsg from cmd, got %T", msg)
	}
}

func TestHandleReload(t *testing.T) {
	m := NewModel()
	HandleReload(m)
	if m.NotificationBanner == nil {
		t.Fatal("expected banner to be set after reload")
	}
	if m.NotificationBanner.Title != "Reloaded" {
		t.Errorf("expected banner title 'Reloaded', got %q", m.NotificationBanner.Title)
	}
	if m.NotificationBanner.Type != "info" {
		t.Errorf("expected banner type 'info', got %q", m.NotificationBanner.Type)
	}
	if !strings.HasPrefix(m.NotificationBanner.ID, "reload-") {
		t.Errorf("expected banner ID prefix 'reload-', got %q", m.NotificationBanner.ID)
	}
	found := false
	for _, n := range m.Notifications {
		if n.ID == m.NotificationBanner.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected reload notification to be appended to Notifications")
	}
}

func TestExternalTUIHTML(t *testing.T) {
	if externalTUIHTML == "" {
		t.Fatal("expected non-empty HTML template")
	}
	if !strings.Contains(externalTUIHTML, "sin-code") {
		t.Error("expected HTML to contain 'sin-code'")
	}
	if !strings.Contains(externalTUIHTML, "EventSource") {
		t.Error("expected HTML to contain EventSource for SSE streaming")
	}
	if !strings.Contains(externalTUIHTML, "/stream") {
		t.Error("expected HTML to reference the /stream endpoint")
	}
}

func TestReloadSignal(t *testing.T) {
	sig := reloadSignal()
	switch runtime.GOOS {
	case "darwin", "linux":
		if sig == nil {
			t.Errorf("expected non-nil SIGUSR2 signal on %s", runtime.GOOS)
		}
	default:
		if sig != nil {
			t.Errorf("expected nil signal on unsupported OS %s, got %v", runtime.GOOS, sig)
		}
	}
}

func TestRenderExternalFrame(t *testing.T) {
	m := NewModel()
	frame := renderExternalFrame(m)
	if frame == "" {
		t.Fatal("expected non-empty external frame")
	}
	if !strings.Contains(frame, "external mode") {
		t.Error("expected frame to mention 'external mode'")
	}
	if !strings.Contains(frame, m.ViewKind.String()) {
		t.Error("expected frame to contain current view name")
	}
}

func TestWriteSSEFrame(t *testing.T) {
	rw := &sseRecorder{}
	writeSSEFrame(rw, "line1\nline2\nline3")
	joined := strings.Join(rw.writes, "")
	if !strings.Contains(joined, "data: line1\n") {
		t.Errorf("expected 'data: line1\\n' in output, got %q", joined)
	}
	if !strings.Contains(joined, "data: line2\n") {
		t.Errorf("expected 'data: line2\\n' in output, got %q", joined)
	}
	if !strings.Contains(joined, "data: line3\n") {
		t.Errorf("expected 'data: line3\\n' in output, got %q", joined)
	}
	if !strings.HasSuffix(joined, "\n\n") {
		t.Errorf("expected output to end with blank line (\\n\\n), got %q", joined)
	}
}

type sseRecorder struct {
	writes []string
	hdr    http.Header
}

func (r *sseRecorder) Header() http.Header {
	if r.hdr == nil {
		r.hdr = http.Header{}
	}
	return r.hdr
}

func (r *sseRecorder) Write(p []byte) (int, error) {
	r.writes = append(r.writes, string(p))
	return len(p), nil
}

func (r *sseRecorder) WriteHeader(int) {}
