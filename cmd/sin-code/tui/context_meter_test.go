// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"testing"
)

func TestContextMeter_Green(t *testing.T) {
	styles := NewStyles(Themes[0])
	cm := NewContextMeter(styles, 20)
	cm.SetUsage(18000, 40000)

	if pct := cm.Percentage(); pct < 0.44 || pct > 0.46 {
		t.Errorf("Percentage() = %.2f, want ~0.45", pct)
	}
	if status := cm.Status(); status != "green" {
		t.Errorf("Status() = %q, want %q", status, "green")
	}
}

func TestContextMeter_Yellow(t *testing.T) {
	styles := NewStyles(Themes[0])
	cm := NewContextMeter(styles, 20)
	cm.SetUsage(28000, 40000)

	if status := cm.Status(); status != "yellow" {
		t.Errorf("Status() = %q, want %q", status, "yellow")
	}
}

func TestContextMeter_Red(t *testing.T) {
	styles := NewStyles(Themes[0])
	cm := NewContextMeter(styles, 20)
	cm.SetUsage(36000, 40000)

	if status := cm.Status(); status != "red" {
		t.Errorf("Status() = %q, want %q", status, "red")
	}
}

func TestContextMeter_Overfull(t *testing.T) {
	styles := NewStyles(Themes[0])
	cm := NewContextMeter(styles, 20)
	cm.SetUsage(44000, 40000)

	if pct := cm.Percentage(); pct < 1.0 {
		t.Errorf("Percentage() = %.2f, want >1.0", pct)
	}
	if status := cm.Status(); status != "red" {
		t.Errorf("Status() = %q, want %q", status, "red")
	}
	rendered := cm.Render()
	if !strings.Contains(rendered, "COMPACTED") {
		t.Errorf("overfull meter should show COMPACTED indicator")
	}
}

func TestContextMeter_Compacted(t *testing.T) {
	styles := NewStyles(Themes[0])
	cm := NewContextMeter(styles, 20)
	cm.SetUsage(20000, 40000)
	cm.SetCompacted(true)

	rendered := cm.Render()
	if !strings.Contains(rendered, "⑂") {
		t.Errorf("compacted meter should show ⑂ symbol")
	}

	cm.SetCompacted(false)
	rendered = cm.Render()
	if strings.Contains(rendered, "⑂") {
		t.Errorf("non-compacted meter should not show ⑂ symbol")
	}
}

func TestContextMeter_Render(t *testing.T) {
	styles := NewStyles(Themes[0])
	cm := NewContextMeter(styles, 20)
	cm.SetUsage(18000, 40000)

	rendered := cm.Render()
	if !strings.Contains(rendered, "[") {
		t.Errorf("rendered meter should contain opening bracket")
	}
	if !strings.Contains(rendered, "]") {
		t.Errorf("rendered meter should contain closing bracket")
	}
	if !strings.Contains(rendered, "45%") {
		t.Errorf("rendered meter should contain percentage")
	}
	if !strings.Contains(rendered, "18") {
		t.Errorf("rendered meter should contain used token count")
	}
	if !strings.Contains(rendered, "40") {
		t.Errorf("rendered meter should contain max token count")
	}
	if !strings.Contains(rendered, "█") {
		t.Errorf("rendered meter should contain filled bar character")
	}
	if !strings.Contains(rendered, "░") {
		t.Errorf("rendered meter should contain empty bar character")
	}
}
