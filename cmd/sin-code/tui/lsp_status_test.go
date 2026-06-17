// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestLSPStatusNewDefaults(t *testing.T) {
	s := NewLSPStatus()
	if s.IsRunning() {
		t.Error("expected not running by default")
	}
	if s.Server() != "" {
		t.Errorf("expected empty server, got %s", s.Server())
	}
	if s.DiagCount() != 0 {
		t.Error("expected 0 diags by default")
	}
}

func TestLSPStatusRenderOffline(t *testing.T) {
	styles := NewStyles(Themes[0])
	s := NewLSPStatus()
	out := s.Render(styles, 60)
	if !strings.Contains(out, "offline") {
		t.Errorf("expected offline in render, got: %s", out)
	}
	if !strings.Contains(out, "○") {
		t.Error("expected hollow circle for offline")
	}
}

func TestLSPStatusRenderRunningClean(t *testing.T) {
	styles := NewStyles(Themes[0])
	s := NewLSPStatus()
	s.Update("gopls", true, 0)
	out := s.Render(styles, 60)
	if !strings.Contains(out, "gopls") {
		t.Errorf("expected server name, got: %s", out)
	}
	if !strings.Contains(out, "●") {
		t.Error("expected solid circle for running")
	}
	if !strings.Contains(out, "clean") {
		t.Error("expected clean indicator when no diags")
	}
}

func TestLSPStatusRenderWithErrors(t *testing.T) {
	styles := NewStyles(Themes[0])
	s := NewLSPStatus()
	s.UpdateDetailed("gopls", true, 3, 5)
	out := s.Render(styles, 80)
	if !strings.Contains(out, "3 errors") {
		t.Errorf("expected 3 errors, got: %s", out)
	}
	if !strings.Contains(out, "5 warnings") {
		t.Errorf("expected 5 warnings, got: %s", out)
	}
}

func TestLSPStatusUpdateChangesState(t *testing.T) {
	s := NewLSPStatus()
	s.Update("pyright", true, 7)
	if !s.IsRunning() {
		t.Error("expected running after update")
	}
	if s.Server() != "pyright" {
		t.Errorf("expected pyright, got %s", s.Server())
	}
	if s.DiagCount() != 7 {
		t.Errorf("expected 7 diags, got %d", s.DiagCount())
	}
	s.Update("gopls", false, 0)
	if s.IsRunning() {
		t.Error("expected not running after offline update")
	}
}

func TestLSPStatusRenderTruncatesNarrow(t *testing.T) {
	styles := NewStyles(Themes[0])
	s := NewLSPStatus()
	s.UpdateDetailed("very-long-server-name-pyright", true, 100, 200)
	out := s.Render(styles, 10)
	if visibleWidth(out) > 10 {
		t.Errorf("expected width <= 10, got %d: %s", visibleWidth(out), out)
	}
}

func TestLSPStatusConcurrentAccess(t *testing.T) {
	s := NewLSPStatus()
	styles := NewStyles(Themes[0])
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Update("gopls", n%2 == 0, n)
			s.UpdateDetailed("gopls", n%2 == 0, n, n+1)
			_ = s.Render(styles, 60)
			_ = s.IsRunning()
			_ = s.Server()
			_ = s.DiagCount()
		}(i)
	}
	wg.Wait()
}
