// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestTokenBarRenderShowsBar(t *testing.T) {
	b := NewTokenBar(200000)
	b.Update(116000, 0.42, "claude-mythos-5")
	out := b.Render(NewStyles(Themes[0]), 80)
	plain := cleanANSI(out)
	if !strings.Contains(plain, "claude-mythos-5") {
		t.Errorf("expected model name in render: %q", plain)
	}
	if !strings.Contains(plain, "58%") {
		t.Errorf("expected 58%% in render: %q", plain)
	}
	if !strings.Contains(plain, "$0.42") {
		t.Errorf("expected $0.42 in render: %q", plain)
	}
	if !strings.Contains(plain, "tok") {
		t.Errorf("expected tok label in render: %q", plain)
	}
	if !strings.ContainsRune(plain, '█') && !strings.ContainsRune(plain, '░') {
		t.Errorf("expected bar glyphs in render: %q", plain)
	}
}

func TestTokenBarUpdateChangesValues(t *testing.T) {
	b := NewTokenBar(200000)
	if got := b.Percent(); got != 0 {
		t.Errorf("expected 0%% before update, got %v", got)
	}
	b.Update(100000, 0.25, "m")
	if got := b.Percent(); got != 0.5 {
		t.Errorf("expected 0.5 after update, got %v", got)
	}
	b.Update(200000, 0.5, "m")
	if got := b.Percent(); got != 1.0 {
		t.Errorf("expected 1.0 after full update, got %v", got)
	}
}

func TestTokenBarPercentCalculation(t *testing.T) {
	b := NewTokenBar(200000)
	b.Update(50000, 0, "m")
	if got := b.Percent(); got != 0.25 {
		t.Errorf("expected 0.25, got %v", got)
	}
	b.Update(0, 0, "m")
	if got := b.Percent(); got != 0 {
		t.Errorf("expected 0, got %v", got)
	}
}

func TestTokenBarIsWarningAt80(t *testing.T) {
	b := NewTokenBar(100000)
	b.Update(80000, 0, "m")
	if b.IsWarning() {
		t.Error("80% exactly should not be warning (strictly >80%)")
	}
	b.Update(81000, 0, "m")
	if !b.IsWarning() {
		t.Error("81% should be warning")
	}
	b.Update(50000, 0, "m")
	if b.IsWarning() {
		t.Error("50% should not be warning")
	}
}

func TestTokenBarIsCriticalAt95(t *testing.T) {
	b := NewTokenBar(100000)
	b.Update(95000, 0, "m")
	if b.IsCritical() {
		t.Error("95% exactly should not be critical (strictly >95%)")
	}
	b.Update(96000, 0, "m")
	if !b.IsCritical() {
		t.Error("96% should be critical")
	}
	if !b.IsWarning() {
		t.Error("96% should also be warning")
	}
}

func TestFormatTokensK(t *testing.T) {
	if got := FormatTokens(1234); got != "1.2K" {
		t.Errorf("FormatTokens(1234)=%q, want 1.2K", got)
	}
	if got := FormatTokens(1000); got != "1.0K" {
		t.Errorf("FormatTokens(1000)=%q, want 1.0K", got)
	}
	if got := FormatTokens(999999); got != "1000.0K" {
		t.Errorf("FormatTokens(999999)=%q, want 1000.0K", got)
	}
}

func TestFormatTokensM(t *testing.T) {
	if got := FormatTokens(1234567); got != "1.2M" {
		t.Errorf("FormatTokens(1234567)=%q, want 1.2M", got)
	}
	if got := FormatTokens(2000000); got != "2.0M" {
		t.Errorf("FormatTokens(2000000)=%q, want 2.0M", got)
	}
}

func TestFormatTokensSmallNumbers(t *testing.T) {
	cases := map[int]string{
		0:   "0",
		1:   "1",
		42:  "42",
		999: "999",
	}
	for n, want := range cases {
		if got := FormatTokens(n); got != want {
			t.Errorf("FormatTokens(%d)=%q, want %q", n, got, want)
		}
	}
	if got := FormatTokens(-5); got != "0" {
		t.Errorf("FormatTokens(-5)=%q, want 0", got)
	}
}

func countBarChars(s string) int {
	n := 0
	for _, r := range s {
		if r == '█' || r == '░' {
			n++
		}
	}
	return n
}

func TestTokenBarWidthScales(t *testing.T) {
	b := NewTokenBar(200000)
	b.Update(116000, 0.42, "m")
	styles := NewStyles(Themes[0])

	wide := countBarChars(cleanANSI(b.Render(styles, 200)))
	if wide != 30 {
		t.Errorf("expected bar capped at 30 chars for wide terminal, got %d", wide)
	}

	narrow := countBarChars(cleanANSI(b.Render(styles, 40)))
	if narrow < 4 || narrow > 30 {
		t.Errorf("expected bar between 4 and 30 chars for narrow terminal, got %d", narrow)
	}
	if wide <= narrow {
		t.Errorf("expected wide bar (%d) to be larger than narrow bar (%d)", wide, narrow)
	}
}

func TestTokenBarCompactionSuggestion(t *testing.T) {
	b := NewTokenBar(100000)
	b.Update(85000, 0.9, "m")
	out := cleanANSI(b.Render(NewStyles(Themes[0]), 120))
	if !strings.Contains(out, "context compaction recommended") {
		t.Errorf("expected compaction suggestion when >80%%: %q", out)
	}

	b.Update(40000, 0.2, "m")
	out2 := cleanANSI(b.Render(NewStyles(Themes[0]), 120))
	if strings.Contains(out2, "context compaction recommended") {
		t.Errorf("did not expect compaction suggestion when <80%%: %q", out2)
	}
}

func TestTokenBarConcurrentAccess(t *testing.T) {
	b := NewTokenBar(200000)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			b.Update(n*1000, float64(n)/10, "model")
			_ = b.Percent()
			_ = b.IsWarning()
			_ = b.IsCritical()
			_ = b.Render(NewStyles(Themes[0]), 80)
		}(i)
	}
	wg.Wait()
}
