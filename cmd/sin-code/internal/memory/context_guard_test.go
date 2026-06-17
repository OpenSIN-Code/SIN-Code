// SPDX-License-Identifier: MIT
// Purpose: tests for the context guard — level transitions, warning
// and compaction thresholds, message formatting, and race-free
// concurrency (mandate M7).
package memory

import (
	"strings"
	"sync"
	"testing"
)

func TestContextGuardGreen(t *testing.T) {
	g := NewContextGuard(10000)
	g.Update(5000) // 50%
	if g.Level() != GuardGreen {
		t.Errorf("expected green at 50%%, got %s", g.Level())
	}
	if g.ShouldWarn() {
		t.Error("should not warn at green")
	}
	if g.ShouldCompact() {
		t.Error("should not compact at green")
	}
}

func TestContextGuardYellow(t *testing.T) {
	g := NewContextGuard(10000)
	g.Update(6500) // 65%
	if g.Level() != GuardYellow {
		t.Errorf("expected yellow at 65%%, got %s", g.Level())
	}
	if !g.ShouldWarn() {
		t.Error("should warn at yellow")
	}
	if g.ShouldCompact() {
		t.Error("should not compact at yellow")
	}
}

func TestContextGuardOrange(t *testing.T) {
	g := NewContextGuard(10000)
	g.Update(8500) // 85%
	if g.Level() != GuardOrange {
		t.Errorf("expected orange at 85%%, got %s", g.Level())
	}
	if !g.ShouldWarn() {
		t.Error("should warn at orange")
	}
	if !g.ShouldCompact() {
		t.Error("should compact at orange")
	}
}

func TestContextGuardRed(t *testing.T) {
	g := NewContextGuard(10000)
	g.Update(9800) // 98%
	if g.Level() != GuardRed {
		t.Errorf("expected red at 98%%, got %s", g.Level())
	}
	if !g.ShouldCompact() {
		t.Error("should compact at red")
	}
}

func TestContextGuardBoundaryTransitions(t *testing.T) {
	g := NewContextGuard(10000)
	// Exactly at boundaries.
	g.Update(5999)
	if g.Level() != GuardGreen {
		t.Errorf("5999/10000 should be green, got %s", g.Level())
	}
	g.Update(6000)
	if g.Level() != GuardYellow {
		t.Errorf("6000/10000 should be yellow, got %s", g.Level())
	}
	g.Update(7999)
	if g.Level() != GuardYellow {
		t.Errorf("7999/10000 should be yellow, got %s", g.Level())
	}
	g.Update(8000)
	if g.Level() != GuardOrange {
		t.Errorf("8000/10000 should be orange, got %s", g.Level())
	}
	g.Update(9499)
	if g.Level() != GuardOrange {
		t.Errorf("9499/10000 should be orange, got %s", g.Level())
	}
	g.Update(9500)
	if g.Level() != GuardRed {
		t.Errorf("9500/10000 should be red, got %s", g.Level())
	}
}

func TestContextGuardMessage(t *testing.T) {
	g := NewContextGuard(10000)
	g.Update(7000) // 70%
	msg := g.Message()
	if !strings.Contains(msg, "70%") {
		t.Errorf("message should contain percentage: %s", msg)
	}
	if !strings.Contains(msg, "yellow") {
		t.Errorf("message should contain level: %s", msg)
	}
}

func TestContextGuardMessageRed(t *testing.T) {
	g := NewContextGuard(10000)
	g.Update(9900)
	msg := g.Message()
	if !strings.Contains(msg, "red") {
		t.Errorf("message should contain 'red': %s", msg)
	}
	if !strings.Contains(msg, "compact immediately") {
		t.Errorf("red message should mention compaction: %s", msg)
	}
}

func TestContextGuardZeroOrNegative(t *testing.T) {
	g := NewContextGuard(0)
	// maxTokens clamped to 1 → any usage is 100%+.
	g.Update(1)
	if g.Level() != GuardRed {
		t.Errorf("expected red with 0 max, got %s", g.Level())
	}
	g.Update(-5)
	if g.Used() != 0 {
		t.Errorf("negative usage should clamp to 0, got %d", g.Used())
	}
}

func TestContextGuardRaceFree(t *testing.T) {
	g := NewContextGuard(10000)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			g.Update(n * 500)
		}(i)
		go func() {
			defer wg.Done()
			_ = g.Level()
			_ = g.ShouldWarn()
			_ = g.ShouldCompact()
			_ = g.Message()
			_ = g.Used()
			_ = g.Max()
		}()
	}
	wg.Wait()
}

func TestContextGuardUsedAndMax(t *testing.T) {
	g := NewContextGuard(5000)
	g.Update(3000)
	if g.Used() != 3000 {
		t.Errorf("Used should be 3000, got %d", g.Used())
	}
	if g.Max() != 5000 {
		t.Errorf("Max should be 5000, got %d", g.Max())
	}
}
