package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestToastShowSetsMessageAndKind(t *testing.T) {
	toast := NewToast()
	if toast.Active() {
		t.Fatal("toast should be inactive initially")
	}
	toast.Show(ToastSuccess, "Tests passed (12/12)")
	if !toast.Active() {
		t.Fatal("toast should be active after Show")
	}
	if toast.Kind() != ToastSuccess {
		t.Errorf("Kind = %v, want Success", toast.Kind())
	}
	if toast.Message() != "Tests passed (12/12)" {
		t.Errorf("Message = %q, want 'Tests passed (12/12)'", toast.Message())
	}
}

func TestToastRenderShowsMessage(t *testing.T) {
	toast := NewToast()
	toast.Show(ToastSuccess, "Tests passed (12/12)")
	out := toast.Render(NewStyles(Themes[0]), 120)
	if out == "" {
		t.Fatal("Render returned empty for active toast")
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "Tests passed") {
		t.Errorf("render missing message, got %q", plain)
	}
	if !strings.Contains(plain, "✓") {
		t.Errorf("render missing success icon, got %q", plain)
	}
}

func TestToastAutoDismissAfterTicks(t *testing.T) {
	toast := NewToast()
	toast.Show(ToastInfo, "hello")
	if !toast.Active() {
		t.Fatal("should be active after Show")
	}
	for i := 0; i < ToastTTL; i++ {
		toast.Tick()
	}
	if toast.Active() {
		t.Fatal("toast should auto-dismiss after TTL ticks")
	}
	if toast.Render(NewStyles(Themes[0]), 80) != "" {
		t.Error("Render should be empty after auto-dismiss")
	}
}

func TestToastActiveState(t *testing.T) {
	toast := NewToast()
	if toast.Active() {
		t.Error("should be inactive initially")
	}
	toast.Show(ToastWarning, "careful")
	if !toast.Active() {
		t.Error("should be active after Show")
	}
	toast.Tick()
	if !toast.Active() {
		t.Error("should still be active after one tick")
	}
}

func TestToastDifferentKindsRenderDifferentColors(t *testing.T) {
	styles := NewStyles(Themes[0])
	kinds := []ToastKind{ToastInfo, ToastSuccess, ToastWarning, ToastError}
	outs := make(map[ToastKind]string)
	for _, k := range kinds {
		toast := NewToast()
		toast.Show(k, "msg")
		o := toast.Render(styles, 120)
		if !strings.Contains(o, "\x1b[") {
			t.Errorf("kind %v render should contain ANSI color codes, got %q", k, o)
		}
		outs[k] = o
	}
	if outs[ToastSuccess] == outs[ToastError] {
		t.Error("success and error renders should differ")
	}
	if outs[ToastInfo] == outs[ToastWarning] {
		t.Error("info and warning renders should differ")
	}
}

func TestToastConcurrent(t *testing.T) {
	toast := NewToast()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				toast.Show(ToastKind(n%4), "concurrent msg")
				_ = toast.Active()
				_ = toast.Kind()
				_ = toast.Message()
				_ = toast.Render(NewStyles(Themes[0]), 80)
				toast.Tick()
			}
		}(i)
	}
	wg.Wait()
}

func TestToastTruncatesLongMessage(t *testing.T) {
	toast := NewToast()
	long := strings.Repeat("x", 200)
	toast.Show(ToastError, long)
	out := toast.Render(NewStyles(Themes[0]), 60)
	if out == "" {
		t.Fatal("render should not be empty")
	}
	plain := stripANSI(out)
	if strings.Contains(plain, strings.Repeat("x", 200)) {
		t.Error("long message should be truncated, not rendered in full")
	}
}
