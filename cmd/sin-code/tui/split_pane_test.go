// SPDX-License-Identifier: MIT
package tui

import (
	"sync"
	"testing"
)

func TestSplitPaneDefaults(t *testing.T) {
	sp := NewSplitPane()
	if sp.Active() {
		t.Error("expected inactive by default")
	}
	if sp.SideKind() != PaneFileViewer {
		t.Errorf("expected default side PaneFileViewer, got %v", sp.SideKind())
	}
}

func TestSplitPaneToggle(t *testing.T) {
	sp := NewSplitPane()
	if sp.Active() {
		t.Fatal("expected inactive")
	}
	sp.Toggle()
	if !sp.Active() {
		t.Error("expected active after toggle")
	}
	sp.Toggle()
	if sp.Active() {
		t.Error("expected inactive after second toggle")
	}
}

func TestSplitPaneSetActive(t *testing.T) {
	sp := NewSplitPane()
	sp.SetActive(true)
	if !sp.Active() {
		t.Error("expected active")
	}
	sp.SetActive(false)
	if sp.Active() {
		t.Error("expected inactive")
	}
}

func TestSplitPaneSetSide(t *testing.T) {
	sp := NewSplitPane()
	sp.SetSide(PaneDAG)
	if sp.SideKind() != PaneDAG {
		t.Errorf("expected PaneDAG, got %v", sp.SideKind())
	}
	sp.SetSide(PaneDiff)
	if sp.SideKind() != PaneDiff {
		t.Errorf("expected PaneDiff, got %v", sp.SideKind())
	}
}

func TestSplitPaneSideWidthNormal(t *testing.T) {
	sp := NewSplitPane()
	w := sp.SideWidth(100)
	if w != 40 {
		t.Errorf("expected 40, got %d", w)
	}
}

func TestSplitPaneSideWidthMin(t *testing.T) {
	sp := NewSplitPane()
	w := sp.SideWidth(50)
	if w != 30 {
		t.Errorf("expected 30 (min), got %d", w)
	}
}

func TestSplitPaneSideWidthMax(t *testing.T) {
	sp := NewSplitPane()
	w := sp.SideWidth(200)
	if w != 60 {
		t.Errorf("expected 60 (max), got %d", w)
	}
}

func TestSplitPaneSideWidthSmallTerminal(t *testing.T) {
	sp := NewSplitPane()
	w := sp.SideWidth(25)
	expected := 25 - 20
	if w != expected {
		t.Errorf("expected %d, got %d", expected, w)
	}
}

func TestSplitPaneMainWidth(t *testing.T) {
	sp := NewSplitPane()
	mw := sp.MainWidth(100)
	if mw != 59 {
		t.Errorf("expected 59 (100-40-1), got %d", mw)
	}
}

func TestSplitPaneConcurrentAccess(t *testing.T) {
	sp := NewSplitPane()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sp.Toggle()
			sp.SetSide(PaneKind(n % 5))
			_ = sp.Active()
			_ = sp.SideKind()
			_ = sp.SideWidth(100)
			_ = sp.MainWidth(100)
		}(i)
	}
	wg.Wait()
}

func TestPaneKindString(t *testing.T) {
	tests := []struct {
		kind PaneKind
		want string
	}{
		{PaneNone, "None"},
		{PaneChat, "Chat"},
		{PaneFileViewer, "FileViewer"},
		{PaneDAG, "DAG"},
		{PaneDiff, "Diff"},
	}
	for _, tc := range tests {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("PaneKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}
