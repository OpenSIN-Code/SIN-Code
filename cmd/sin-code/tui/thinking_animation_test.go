package tui

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestThinkingTickAdvancesFrame(t *testing.T) {
	anim := NewThinkingAnimation()
	initial := anim.Frame()
	anim.Tick()
	if anim.Frame() != initial+1 {
		t.Errorf("expected frame %d, got %d", initial+1, anim.Frame())
	}
	anim.Tick()
	if anim.Frame() != initial+2 {
		t.Errorf("expected frame %d, got %d", initial+2, anim.Frame())
	}
}

func TestThinkingRenderShowsFrame(t *testing.T) {
	anim := NewThinkingAnimation()
	anim.SetFrame(0)
	out0 := anim.Render(testStyles())
	anim.SetFrame(1)
	out1 := anim.Render(testStyles())
	if out0 == out1 {
		t.Error("expected different output for different frames")
	}
	stripped := stripANSI(out0)
	if !strings.Contains(stripped, "Thinking") {
		t.Error("expected 'Thinking' text in output")
	}
}

func TestThinkingStartStopResets(t *testing.T) {
	anim := NewThinkingAnimation()
	anim.Tick()
	anim.Tick()
	anim.Start()
	if anim.Frame() != 0 {
		t.Errorf("expected frame 0 after Start, got %d", anim.Frame())
	}
	if anim.Elapsed() > time.Second {
		t.Error("expected near-zero elapsed after Start")
	}
	anim.Tick()
	anim.Tick()
	anim.Stop()
	if anim.Frame() != 0 {
		t.Errorf("expected frame 0 after Stop, got %d", anim.Frame())
	}
	if anim.Elapsed() != 0 {
		t.Errorf("expected zero elapsed after Stop, got %v", anim.Elapsed())
	}
}

func TestThinkingElapsedTracking(t *testing.T) {
	anim := NewThinkingAnimation()
	anim.SetStart(time.Now().Add(-3 * time.Second))
	elapsed := anim.Elapsed()
	if elapsed < 2*time.Second || elapsed > 4*time.Second {
		t.Errorf("expected ~3s elapsed, got %v", elapsed)
	}
	out := anim.Render(testStyles())
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "3s") {
		t.Errorf("expected elapsed time '3s' in output, got: %s", stripped)
	}
}

func TestThinkingFrameCycling(t *testing.T) {
	anim := NewThinkingAnimation()
	for i := 0; i < len(thinkingFrames); i++ {
		anim.Tick()
	}
	if anim.Frame() != 0 {
		t.Errorf("expected frame 0 after %d ticks, got %d", len(thinkingFrames), anim.Frame())
	}
}

func TestThinkingConcurrentTick(t *testing.T) {
	anim := NewThinkingAnimation()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			anim.Tick()
			_ = anim.Render(testStyles())
			_ = anim.Elapsed()
		}()
	}
	wg.Wait()
	if anim.Frame() < 0 || anim.Frame() >= len(thinkingFrames) {
		t.Errorf("frame out of range after concurrent ticks: %d", anim.Frame())
	}
}

func TestThinkingBrailleMode(t *testing.T) {
	anim := NewThinkingAnimation()
	anim.SetBraille(true)
	anim.SetFrame(0)
	out := anim.Render(testStyles())
	stripped := stripANSI(out)
	found := false
	for _, f := range thinkingBrailleFrames {
		if strings.Contains(stripped, f) {
			found = true
		}
	}
	if !found {
		t.Error("expected braille frame character in output")
	}
	anim.Tick()
	anim.Tick()
	for i := 0; i < len(thinkingBrailleFrames)-2; i++ {
		anim.Tick()
	}
	if anim.Frame() != 0 {
		t.Errorf("expected frame 0 after full braille cycle, got %d", anim.Frame())
	}
}
