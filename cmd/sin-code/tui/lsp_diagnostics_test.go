// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func sampleDiags() []Diagnostic {
	return []Diagnostic{
		{File: "b.go", Line: 15, Col: 3, Severity: "warning", Message: "unused variable: bar", Source: "gopls"},
		{File: "a.go", Line: 42, Col: 10, Severity: "error", Message: "undefined: foo", Source: "gopls"},
		{File: "a.go", Line: 8, Col: 1, Severity: "info", Message: "consider using bytes.Buffer", Source: "gopls"},
		{File: "a.go", Line: 22, Col: 5, Severity: "hint", Message: "could simplify", Source: "gopls"},
	}
}

func TestLSPDiagnosticsNewIsEmpty(t *testing.T) {
	d := NewLSPDiagnostics()
	if d.Count() != 0 {
		t.Errorf("expected 0 diags, got %d", d.Count())
	}
	if d.ErrorCount() != 0 || d.WarningCount() != 0 {
		t.Error("expected zero counts on new")
	}
	if s := d.Selected(); s != nil {
		t.Error("expected nil selected on empty")
	}
}

func TestLSPDiagnosticsUpdateCounts(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update(sampleDiags())
	if d.Count() != 4 {
		t.Errorf("expected 4, got %d", d.Count())
	}
	if d.ErrorCount() != 1 {
		t.Errorf("expected 1 error, got %d", d.ErrorCount())
	}
	if d.WarningCount() != 1 {
		t.Errorf("expected 1 warning, got %d", d.WarningCount())
	}
	if d.InfoCount() != 1 {
		t.Errorf("expected 1 info, got %d", d.InfoCount())
	}
	if d.HintCount() != 1 {
		t.Errorf("expected 1 hint, got %d", d.HintCount())
	}
}

func TestLSPDiagnosticsSortedByFileThenLine(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update(sampleDiags())
	all := d.All()
	if len(all) != 4 {
		t.Fatalf("expected 4, got %d", len(all))
	}
	if all[0].File != "a.go" || all[0].Line != 8 {
		t.Errorf("expected a.go:8 first, got %s:%d", all[0].File, all[0].Line)
	}
	if all[1].File != "a.go" || all[1].Line != 22 {
		t.Errorf("expected a.go:22 second, got %s:%d", all[1].File, all[1].Line)
	}
	if all[2].File != "a.go" || all[2].Line != 42 {
		t.Errorf("expected a.go:42 third, got %s:%d", all[2].File, all[2].Line)
	}
	if all[3].File != "b.go" {
		t.Errorf("expected b.go last, got %s", all[3].File)
	}
}

func TestLSPDiagnosticsErrorsSortBeforeWarningsSameLine(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update([]Diagnostic{
		{File: "x.go", Line: 10, Col: 1, Severity: "warning", Message: "w", Source: "gopls"},
		{File: "x.go", Line: 10, Col: 1, Severity: "error", Message: "e", Source: "gopls"},
	})
	all := d.All()
	if all[0].Severity != "error" {
		t.Errorf("expected error first, got %s", all[0].Severity)
	}
	if all[1].Severity != "warning" {
		t.Errorf("expected warning second, got %s", all[1].Severity)
	}
}

func TestLSPDiagnosticsRenderEmpty(t *testing.T) {
	d := NewLSPDiagnostics()
	styles := NewStyles(Themes[0])
	out := d.Render(styles, 60, 20)
	if !strings.Contains(out, "No diagnostics") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestLSPDiagnosticsRenderSummary(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update(sampleDiags())
	styles := NewStyles(Themes[0])
	out := d.Render(styles, 80, 20)
	if !strings.Contains(out, "1 errors") {
		t.Errorf("expected error count in summary, got: %s", out)
	}
	if !strings.Contains(out, "1 warnings") {
		t.Errorf("expected warning count in summary, got: %s", out)
	}
}

func TestLSPDiagnosticsRenderContainsMessages(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update(sampleDiags())
	styles := NewStyles(Themes[0])
	out := d.Render(styles, 120, 30)
	if !strings.Contains(out, "undefined: foo") {
		t.Error("expected error message in render")
	}
	if !strings.Contains(out, "unused variable: bar") {
		t.Error("expected warning message in render")
	}
}

func TestLSPDiagnosticsNavigation(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update(sampleDiags())
	if s := d.Selected(); s == nil || s.Message != "consider using bytes.Buffer" {
		t.Errorf("expected first (a.go:8) selected, got %+v", s)
	}
	d.MoveDown()
	if s := d.Selected(); s == nil || s.Line != 22 {
		t.Errorf("expected a.go:22 after MoveDown, got %+v", s)
	}
	d.MoveDown()
	d.MoveDown()
	if s := d.Selected(); s == nil || s.File != "b.go" {
		t.Errorf("expected b.go after 3 MoveDown, got %+v", s)
	}
	d.MoveDown()
	if s := d.Selected(); s == nil || s.File != "b.go" {
		t.Error("MoveDown should clamp at last")
	}
	d.MoveUp()
	d.MoveUp()
	d.MoveUp()
	if s := d.Selected(); s == nil || s.Line != 8 {
		t.Errorf("expected back at a.go:8, got %+v", s)
	}
	d.MoveUp()
	if s := d.Selected(); s == nil || s.Line != 8 {
		t.Error("MoveUp should clamp at zero")
	}
}

func TestLSPDiagnosticsSelectionClampedOnShrink(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update(sampleDiags())
	for i := 0; i < 10; i++ {
		d.MoveDown()
	}
	if s := d.Selected(); s == nil || s.File != "b.go" {
		t.Error("expected at last")
	}
	d.Update([]Diagnostic{{File: "only.go", Line: 1, Col: 1, Severity: "error", Message: "x", Source: "gopls"}})
	if s := d.Selected(); s == nil || s.File != "only.go" {
		t.Errorf("expected selection clamped to only.go, got %+v", s)
	}
}

func TestLSPDiagnosticsUpdateEmptyClearsCounts(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update(sampleDiags())
	if d.ErrorCount() != 1 {
		t.Fatal("expected 1 error before clear")
	}
	d.Update(nil)
	if d.Count() != 0 || d.ErrorCount() != 0 || d.WarningCount() != 0 {
		t.Errorf("expected all zero after empty update, got count=%d err=%d warn=%d", d.Count(), d.ErrorCount(), d.WarningCount())
	}
	if s := d.Selected(); s != nil {
		t.Error("expected nil selected after clear")
	}
}

func TestLSPDiagnosticsRenderGlyphs(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update([]Diagnostic{
		{File: "a.go", Line: 1, Col: 1, Severity: "error", Message: "e", Source: "gopls"},
		{File: "a.go", Line: 2, Col: 1, Severity: "warning", Message: "w", Source: "gopls"},
		{File: "a.go", Line: 3, Col: 1, Severity: "info", Message: "i", Source: "gopls"},
		{File: "a.go", Line: 4, Col: 1, Severity: "hint", Message: "h", Source: "gopls"},
	})
	styles := NewStyles(Themes[0])
	out := d.Render(styles, 120, 30)
	if !strings.Contains(out, "🔴") {
		t.Error("expected error glyph")
	}
	if !strings.Contains(out, "🟡") {
		t.Error("expected warning glyph")
	}
	if !strings.Contains(out, "🔵") {
		t.Error("expected info glyph")
	}
	if !strings.Contains(out, "⚪") {
		t.Error("expected hint glyph")
	}
}

func TestLSPDiagnosticsConcurrentAccess(t *testing.T) {
	d := NewLSPDiagnostics()
	styles := NewStyles(Themes[0])
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			d.Update(sampleDiags())
			_ = d.Render(styles, 80, 20)
			_ = d.Selected()
			_ = d.ErrorCount()
			_ = d.WarningCount()
			_ = d.All()
			d.MoveUp()
			d.MoveDown()
		}(i)
	}
	wg.Wait()
}

func TestLSPDiagnosticsAllReturnsCopy(t *testing.T) {
	d := NewLSPDiagnostics()
	d.Update(sampleDiags())
	all := d.All()
	if len(all) != 4 {
		t.Fatal("expected 4")
	}
	all[0].Message = "mutated"
	again := d.All()
	if again[0].Message == "mutated" {
		t.Error("All() should return a copy, not the internal slice")
	}
}
