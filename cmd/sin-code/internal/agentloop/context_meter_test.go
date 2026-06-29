// SPDX-License-Identifier: MIT
package agentloop

import (
	"strings"
	"sync"
	"testing"
)

func TestContextMeter_Update(t *testing.T) {
	m := NewContextMeter(100000)
	m.Update(50000)
	used, max, pct := m.Usage()
	if used != 50000 || max != 100000 {
		t.Fatalf("got %d/%d", used, max)
	}
	if pct != 50 {
		t.Fatalf("pct = %.1f, want 50", pct)
	}
}

func TestContextMeter_ShouldWarn(t *testing.T) {
	m := NewContextMeter(100000)
	m.Update(79000)
	if m.ShouldWarn() {
		t.Error("should not warn at 79%")
	}
	m.Update(81000)
	if !m.ShouldWarn() {
		t.Error("should warn at 81%")
	}
}

func TestContextMeter_ShouldCompact(t *testing.T) {
	m := NewContextMeter(100000)
	m.Update(89000)
	if m.ShouldCompact() {
		t.Error("should not compact at 89%")
	}
	m.Update(91000)
	if !m.ShouldCompact() {
		t.Error("should compact at 91%")
	}
}

func TestContextMeter_String(t *testing.T) {
	m := NewContextMeter(1000000)
	m.Update(620000)
	s := m.String()
	if !strings.Contains(s, "62%") {
		t.Errorf("String = %q, want 62%%", s)
	}
	if !strings.Contains(s, "620k/1000k") {
		t.Errorf("String = %q, want 620k/1000k", s)
	}
}

func TestContextMeter_DefaultMax(t *testing.T) {
	m := NewContextMeter(0)
	_, max, _ := m.Usage()
	if max != 128000 {
		t.Fatalf("default max = %d, want 128000", max)
	}
}

func TestContextMeter_RaceFree(t *testing.T) {
	m := NewContextMeter(100000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); m.Update(50000) }()
		go func() { defer wg.Done(); m.Usage() }()
	}
	wg.Wait()
}

func TestContextMeter_SetMax(t *testing.T) {
	m := NewContextMeter(100000)
	m.SetMax(200000)
	_, max, _ := m.Usage()
	if max != 200000 {
		t.Fatalf("max = %d, want 200000", max)
	}
}
