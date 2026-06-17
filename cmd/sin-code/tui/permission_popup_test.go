package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestPermissionPopupSetRequestStores(t *testing.T) {
	p := NewPermissionPopup()
	if p.Active() {
		t.Fatal("popup should be inactive initially")
	}
	p.SetRequest(PermissionRequest{Tool: "sin_bash", Args: "ls -la", Risk: RiskMedium})
	if !p.Active() {
		t.Fatal("popup should be active after SetRequest")
	}
	req := p.Request()
	if req.Tool != "sin_bash" || req.Args != "ls -la" || req.Risk != RiskMedium {
		t.Errorf("Request = %+v, want {sin_bash, ls -la, medium}", req)
	}
}

func TestPermissionPopupRenderShowsToolAndArgs(t *testing.T) {
	p := NewPermissionPopup()
	p.SetRequest(PermissionRequest{Tool: "sin_bash", Args: "rm -rf /tmp/test", Risk: RiskHigh})
	out := p.Render(NewStyles(Themes[0]), 120, 40)
	if out == "" {
		t.Fatal("Render returned empty for active popup")
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "sin_bash") {
		t.Errorf("render missing tool name, got %q", plain)
	}
	if !strings.Contains(plain, "rm -rf /tmp/test") {
		t.Errorf("render missing args, got %q", plain)
	}
}

func TestPermissionPopupRenderShowsRiskLevel(t *testing.T) {
	p := NewPermissionPopup()
	p.SetRequest(PermissionRequest{Tool: "sin_bash", Args: "x", Risk: RiskHigh})
	out := p.Render(NewStyles(Themes[0]), 120, 40)
	plain := stripANSI(out)
	if !strings.Contains(plain, "HIGH") {
		t.Errorf("render missing risk level HIGH, got %q", plain)
	}
}

func TestPermissionPopupActiveDismiss(t *testing.T) {
	p := NewPermissionPopup()
	p.SetRequest(PermissionRequest{Tool: "sin_edit", Risk: RiskLow})
	if !p.Active() {
		t.Fatal("should be active after SetRequest")
	}
	p.Dismiss()
	if p.Active() {
		t.Fatal("should be inactive after Dismiss")
	}
	if p.Render(NewStyles(Themes[0]), 80, 24) != "" {
		t.Error("Render should be empty after Dismiss")
	}
}

func TestPermissionPopupHighRiskHasColorCodes(t *testing.T) {
	p := NewPermissionPopup()
	p.SetRequest(PermissionRequest{Tool: "sin_bash", Args: "reboot", Risk: RiskHigh})
	out := p.Render(NewStyles(Themes[0]), 120, 40)
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("high-risk render should contain ANSI color codes, got %q", out)
	}
	p2 := NewPermissionPopup()
	p2.SetRequest(PermissionRequest{Tool: "sin_bash", Args: "reboot", Risk: RiskCritical})
	out2 := p2.Render(NewStyles(Themes[0]), 120, 40)
	if !strings.Contains(out2, "\x1b[") {
		t.Errorf("critical-risk render should contain ANSI color codes, got %q", out2)
	}
}

func TestPermissionPopupEmptyRequest(t *testing.T) {
	p := NewPermissionPopup()
	p.SetRequest(PermissionRequest{Tool: "", Args: "", Risk: RiskLow})
	out := p.Render(NewStyles(Themes[0]), 120, 40)
	plain := stripANSI(out)
	if !strings.Contains(plain, "(unknown)") {
		t.Errorf("empty tool should render (unknown), got %q", plain)
	}
}

func TestPermissionPopupConcurrent(t *testing.T) {
	p := NewPermissionPopup()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				p.SetRequest(PermissionRequest{Tool: "sin_bash", Args: "x", Risk: RiskLevel(n % 4)})
				_ = p.Active()
				_ = p.Request()
				_ = p.Render(NewStyles(Themes[0]), 80, 24)
				if j%50 == 0 {
					p.Dismiss()
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestPermissionPopupWidthAdjusts(t *testing.T) {
	p := NewPermissionPopup()
	p.SetRequest(PermissionRequest{Tool: "sin_bash", Args: "echo hi", Risk: RiskLow})
	out60 := p.Render(NewStyles(Themes[0]), 60, 24)
	out200 := p.Render(NewStyles(Themes[0]), 200, 24)
	if out60 == out200 {
		t.Error("render at width 60 and 200 should differ")
	}
}

func TestRiskFromTool(t *testing.T) {
	cases := []struct {
		tool, detail string
		want         RiskLevel
	}{
		{"sin_bash", "ls", RiskHigh},
		{"sin_bash", "rm -rf /", RiskCritical},
		{"sin_write", "write file", RiskMedium},
		{"sin_edit", "edit file", RiskMedium},
		{"sin_scout", "read-only", RiskLow},
		{"sin_git_commit", "commit", RiskHigh},
	}
	for _, c := range cases {
		if got := RiskFromTool(c.tool, c.detail); got != c.want {
			t.Errorf("RiskFromTool(%q,%q) = %v, want %v", c.tool, c.detail, got, c.want)
		}
	}
}
