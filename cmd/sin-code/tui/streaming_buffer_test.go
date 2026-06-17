// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func TestStreamingBufferAppend(t *testing.T) {
	b := NewStreamingBuffer()
	b.Append("hello")
	if b.Len() != 5 {
		t.Errorf("expected len 5, got %d", b.Len())
	}
	b.Append(" world")
	if b.Len() != 11 {
		t.Errorf("expected len 11, got %d", b.Len())
	}
}

func TestStreamingBufferAppendEmpty(t *testing.T) {
	b := NewStreamingBuffer()
	b.Append("")
	if b.Len() != 0 {
		t.Errorf("expected len 0 for empty append, got %d", b.Len())
	}
}

func TestStreamingBufferRenderPartial(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(5)
	b.Append("HelloWorld1234567890")
	b.Tick()
	rendered := b.Render(testStyles(), 80)
	if !strings.HasPrefix(rendered, "Hello") {
		t.Errorf("expected partial text starting with Hello, got %q", rendered)
	}
	if strings.Contains(rendered, "1234567890") {
		t.Errorf("should not reveal all text after one tick, got %q", rendered)
	}
	if b.Pending() != 15 {
		t.Errorf("expected 15 pending, got %d", b.Pending())
	}
}

func TestStreamingBufferCompleteShowsAll(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(5)
	b.Append("HelloWorld1234567890")
	b.Tick()
	b.Complete()
	rendered := b.Render(testStyles(), 80)
	if !strings.Contains(rendered, "HelloWorld1234567890") {
		t.Errorf("expected all text after Complete, got %q", rendered)
	}
	if b.Pending() != 0 {
		t.Errorf("expected 0 pending after Complete, got %d", b.Pending())
	}
	if !b.IsCompleted() {
		t.Error("expected completed flag set")
	}
}

func TestStreamingBufferReset(t *testing.T) {
	b := NewStreamingBuffer()
	b.Append("some text")
	b.Tick()
	b.Reset()
	if b.Len() != 0 {
		t.Errorf("expected len 0 after reset, got %d", b.Len())
	}
	if b.Revealed() != 0 {
		t.Errorf("expected revealed 0 after reset, got %d", b.Revealed())
	}
	if b.Pending() != 0 {
		t.Errorf("expected pending 0 after reset, got %d", b.Pending())
	}
	rendered := b.Render(testStyles(), 80)
	if rendered != "" && rendered != streamingCursorRune {
		t.Errorf("expected empty or cursor-only render after reset, got %q", rendered)
	}
}

func TestStreamingBufferPendingCount(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(10)
	b.Append("0123456789ABCDEFGHIJ")
	if b.Pending() != 20 {
		t.Errorf("expected 20 pending before tick, got %d", b.Pending())
	}
	b.Tick()
	if b.Pending() != 10 {
		t.Errorf("expected 10 pending after one tick, got %d", b.Pending())
	}
	b.Tick()
	if b.Pending() != 0 {
		t.Errorf("expected 0 pending after two ticks, got %d", b.Pending())
	}
}

func TestStreamingBufferCursorAtEnd(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(50)
	b.Append("hello")
	b.Tick()
	b.SetCursorBlinkForTest(true)
	rendered := b.Render(testStyles(), 80)
	if !strings.Contains(rendered, streamingCursorRune) {
		t.Errorf("expected cursor rune %q in render, got %q", streamingCursorRune, rendered)
	}
	if !strings.HasPrefix(rendered, "hello") {
		t.Errorf("expected text before cursor, got %q", rendered)
	}
}

func TestStreamingBufferCursorBlinkOff(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(50)
	b.Append("hello")
	b.Tick()
	b.SetCursorBlinkForTest(false)
	rendered := b.Render(testStyles(), 80)
	if strings.Contains(rendered, streamingCursorRune) {
		t.Errorf("expected no cursor when blink off, got %q", rendered)
	}
	if !strings.Contains(rendered, "hello") {
		t.Errorf("expected text still visible, got %q", rendered)
	}
}

func TestStreamingBufferRevealRate(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(7)
	b.Append("0123456789012345678901234567890")
	b.Tick()
	if b.Revealed() != 7 {
		t.Errorf("expected revealed 7 after one tick with rate 7, got %d", b.Revealed())
	}
	b.Tick()
	if b.Revealed() != 14 {
		t.Errorf("expected revealed 14 after two ticks, got %d", b.Revealed())
	}
}

func TestStreamingBufferRevealRateClamped(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(0)
	b.Append("hello")
	b.Tick()
	if b.Revealed() != 1 {
		t.Errorf("expected revealed 1 with clamped rate 1, got %d", b.Revealed())
	}
}

func TestStreamingBufferConcurrentAppend(t *testing.T) {
	b := NewStreamingBuffer()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			b.Append("chunk")
			_ = b.Pending()
			_ = b.Len()
		}(i)
	}
	wg.Wait()
	if b.Len() != 250 {
		t.Errorf("expected len 250 after 50 concurrent appends of 5 chars, got %d", b.Len())
	}
}

func TestStreamingBufferConcurrentTickAndAppend(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(10)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			b.Append("data123456")
		}()
		go func() {
			defer wg.Done()
			b.Tick()
			_ = b.Render(testStyles(), 40)
			_ = b.Pending()
		}()
	}
	wg.Wait()
}

func TestStreamingBufferWrapLongLine(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(100)
	b.Append(strings.Repeat("A", 30))
	b.Tick()
	b.Complete()
	rendered := b.Render(testStyles(), 10)
	lines := strings.Split(rendered, "\n")
	if len(lines) < 3 {
		t.Errorf("expected wrapping to produce multiple lines, got %d: %q", len(lines), rendered)
	}
}

func TestStreamingBufferMaxLines(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(100)
	b.SetMaxLines(2)
	b.Append("line1\nline2\nline3\nline4")
	b.Tick()
	b.Complete()
	rendered := b.Render(testStyles(), 80)
	lines := strings.Split(rendered, "\n")
	if len(lines) > 2 {
		t.Errorf("expected max 2 lines with scroll, got %d: %q", len(lines), rendered)
	}
	if !strings.Contains(rendered, "line4") {
		t.Errorf("expected last line to be visible, got %q", rendered)
	}
}

func TestStreamingBufferMultilineText(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(100)
	b.Append("first line\nsecond line\nthird line")
	b.Tick()
	b.Complete()
	rendered := b.Render(testStyles(), 80)
	if !strings.Contains(rendered, "first line") {
		t.Errorf("expected first line, got %q", rendered)
	}
	if !strings.Contains(rendered, "third line") {
		t.Errorf("expected third line, got %q", rendered)
	}
}

func TestStreamingBufferFullText(t *testing.T) {
	b := NewStreamingBuffer()
	b.SetRevealRate(5)
	b.Append("HelloWorld")
	b.Tick()
	if b.FullText() != "HelloWorld" {
		t.Errorf("expected full text preserved, got %q", b.FullText())
	}
}

func (b *StreamingBuffer) SetCursorBlinkForTest(on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cursorBlink = on
}
