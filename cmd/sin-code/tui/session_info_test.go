// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestSessionInfoRenderShowsInfo(t *testing.T) {
	s := NewSessionInfo()
	s.Update("a1b2c3", "claude-mythos-5", 12, true)
	out := cleanANSI(s.Render(NewStyles(Themes[0]), 80))
	if !strings.Contains(out, "session: a1b2c3") {
		t.Errorf("expected session id: %q", out)
	}
	if !strings.Contains(out, "turns: 12") {
		t.Errorf("expected turns: %q", out)
	}
	if !strings.Contains(out, "verified:") {
		t.Errorf("expected verified label: %q", out)
	}
	if !strings.Contains(out, "model: claude-mythos-5") {
		t.Errorf("expected model: %q", out)
	}
}

func TestSessionInfoUpdateChangesValues(t *testing.T) {
	s := NewSessionInfo()
	s.Update("aaa", "m1", 1, false)
	out1 := cleanANSI(s.Render(NewStyles(Themes[0]), 80))
	if !strings.Contains(out1, "m1") || !strings.Contains(out1, "turns: 1") {
		t.Errorf("initial values missing: %q", out1)
	}
	s.Update("bbb", "m2", 99, true)
	out2 := cleanANSI(s.Render(NewStyles(Themes[0]), 80))
	if !strings.Contains(out2, "m2") || !strings.Contains(out2, "turns: 99") {
		t.Errorf("updated values missing: %q", out2)
	}
	if strings.Contains(out2, "m1") {
		t.Errorf("stale model still present after update: %q", out2)
	}
}

func TestSessionInfoVerifiedStatusColors(t *testing.T) {
	styles := NewStyles(Themes[0])
	s := NewSessionInfo()

	s.Update("sid", "m", 3, true)
	rawTrue := s.Render(styles, 80)
	plainTrue := cleanANSI(rawTrue)
	if !strings.Contains(plainTrue, "✓") || strings.Contains(plainTrue, "✗") {
		t.Errorf("verified=true should show ✓ not ✗: %q", plainTrue)
	}

	s.Update("sid", "m", 3, false)
	rawFalse := s.Render(styles, 80)
	plainFalse := cleanANSI(rawFalse)
	if !strings.Contains(plainFalse, "✗") || strings.Contains(plainFalse, "✓") {
		t.Errorf("verified=false should show ✗ not ✓: %q", plainFalse)
	}

	if rawTrue == rawFalse {
		t.Error("verified true/false renders should differ (color + symbol)")
	}
}

func TestSessionInfoWidthTruncation(t *testing.T) {
	s := NewSessionInfo()
	longModel := strings.Repeat("x", 60)
	s.Update("sid", longModel, 5, true)
	styles := NewStyles(Themes[0])
	out := s.Render(styles, 30)
	if w := lipgloss.Width(out); w > 30 {
		t.Errorf("rendered width %d exceeds 30", w)
	}
	plain := cleanANSI(out)
	if strings.Contains(plain, longModel) {
		t.Errorf("full long model should be truncated: %q", plain)
	}
}

func TestSessionInfoConcurrentAccess(t *testing.T) {
	s := NewSessionInfo()
	styles := NewStyles(Themes[0])
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Update("sid", "m", n, n%2 == 0)
			_ = s.Render(styles, 80)
			_ = s.SessionID()
			_ = s.Turns()
			_ = s.Verified()
		}(i)
	}
	wg.Wait()
}
