package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestTransitionStartSetsActive(t *testing.T) {
	tr := NewTransition()
	if tr.Active() {
		t.Fatal("transition should be inactive initially")
	}
	tr.Start(TransitionSlideLeft)
	if !tr.Active() {
		t.Fatal("transition should be active after Start")
	}
	if tr.Kind() != TransitionSlideLeft {
		t.Errorf("Kind = %v, want SlideLeft", tr.Kind())
	}
}

func TestTransitionTickAdvancesProgress(t *testing.T) {
	tr := NewTransition()
	tr.Start(TransitionFade)
	if p := tr.Progress(); p != 0 {
		t.Errorf("initial Progress = %v, want 0", p)
	}
	tr.Tick()
	if p := tr.Progress(); p <= 0 {
		t.Errorf("Progress after one tick = %v, want > 0", p)
	}
}

func TestTransitionProgressZeroToOneOverTicks(t *testing.T) {
	tr := NewTransition()
	tr.Start(TransitionSlideLeft)
	if p := tr.Progress(); p != 0 {
		t.Errorf("Progress = %v, want 0", p)
	}
	for i := 0; i < transitionTotalTicks-1; i++ {
		tr.Tick()
	}
	if p := tr.Progress(); p != 0.8 {
		t.Errorf("Progress after 4 ticks = %v, want 0.8", p)
	}
	tr.Tick()
	if tr.Active() {
		t.Error("transition should auto-deactivate after total ticks")
	}
	if p := tr.Progress(); p != 1.0 {
		t.Errorf("Progress after completion = %v, want 1.0", p)
	}
}

func TestTransitionRenderNoneReturnsContent(t *testing.T) {
	tr := NewTransition()
	content := "hello\nworld"
	out := tr.Render(content, NewStyles(Themes[0]), 80, 24)
	if out != content {
		t.Errorf("Render with no transition = %q, want %q", out, content)
	}
}

func TestTransitionRenderFadeDimsContent(t *testing.T) {
	tr := NewTransition()
	tr.Start(TransitionFade)
	content := "hello world"
	out := tr.Render(content, NewStyles(Themes[0]), 80, 24)
	if out == content {
		t.Error("fade render should differ from content")
	}
	if !strings.Contains(out, "\x1b[2m") {
		t.Errorf("fade render should contain faint ANSI \\x1b[2m, got %q", out)
	}
}

func TestTransitionAutoDeactivatesAfterComplete(t *testing.T) {
	tr := NewTransition()
	tr.Start(TransitionSlideRight)
	for i := 0; i < transitionTotalTicks; i++ {
		tr.Tick()
	}
	if tr.Active() {
		t.Error("transition should be inactive after completing")
	}
	if tr.Kind() != TransitionNone {
		t.Errorf("Kind = %v, want None", tr.Kind())
	}
	out := tr.Render("content", NewStyles(Themes[0]), 80, 24)
	if out != "content" {
		t.Errorf("Render after completion = %q, want content", out)
	}
}

func TestTransitionConcurrentTick(t *testing.T) {
	tr := NewTransition()
	tr.Start(TransitionSlideLeft)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				tr.Tick()
				_ = tr.Active()
				_ = tr.Progress()
				_ = tr.Kind()
				_ = tr.Render("x\ny", NewStyles(Themes[0]), 80, 24)
				if j%100 == 0 {
					tr.Start(TransitionSlideRight)
				}
			}
		}()
	}
	wg.Wait()
}

func TestTransitionSlideProducesDifferentOutput(t *testing.T) {
	tr := NewTransition()
	tr.Start(TransitionSlideLeft)
	content := "line one\nline two"
	out := tr.Render(content, NewStyles(Themes[0]), 80, 24)
	if out == content {
		t.Error("slide render should differ from content while active")
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "line one") {
		t.Errorf("slide render should still contain content text, got %q", plain)
	}
}
