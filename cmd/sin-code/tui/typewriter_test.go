// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTypewriterBuffer_Append(t *testing.T) {
	tb := NewTypewriterBuffer(DefaultTypewriterConfig())
	tb.Append("hello")
	if tb.Len() != 5 {
		t.Errorf("expected len 5, got %d", tb.Len())
	}
	tb.Append(" world")
	if tb.Len() != 11 {
		t.Errorf("expected len 11, got %d", tb.Len())
	}
	if tb.Full() != "hello world" {
		t.Errorf("expected full text 'hello world', got %q", tb.Full())
	}
}

func TestTypewriterBuffer_Tick(t *testing.T) {
	tb := NewTypewriterBuffer(TypewriterConfig{
		Enabled:     true,
		CharsPerSec: 100,
		BatchSize:   5,
	})
	tb.Append("0123456789")
	tb.Tick()
	if tb.RevealedCount() < 5 {
		t.Errorf("expected at least 5 revealed after tick, got %d", tb.RevealedCount())
	}
	tb.Tick()
	if tb.RevealedCount() < 10 {
		t.Errorf("expected at least 10 revealed after two ticks, got %d", tb.RevealedCount())
	}
	tb.Tick()
	if !tb.IsComplete() {
		t.Errorf("expected complete after enough ticks, revealed=%d len=%d", tb.RevealedCount(), tb.Len())
	}
}

func TestTypewriterBuffer_Visible(t *testing.T) {
	tb := NewTypewriterBuffer(TypewriterConfig{
		Enabled:     true,
		CharsPerSec: 10,
		BatchSize:   3,
	})
	tb.Append("Hello, World!")
	tb.Tick()
	visible := tb.Visible()
	if len(visible) == 0 {
		t.Error("expected non-empty visible text after tick")
	}
	if visible == "Hello, World!" {
		t.Error("expected partial text, not full text")
	}
	if !strings.HasPrefix("Hello, World!", visible) {
		t.Errorf("visible %q should be a prefix of full text", visible)
	}
}

func TestTypewriterBuffer_Complete(t *testing.T) {
	tb := NewTypewriterBuffer(TypewriterConfig{
		Enabled:     true,
		CharsPerSec: 10,
		BatchSize:   3,
	})
	tb.Append("Hello, World!")
	tb.Complete()
	if tb.Visible() != "Hello, World!" {
		t.Errorf("expected full text after Complete, got %q", tb.Visible())
	}
	if !tb.IsComplete() {
		t.Error("expected IsComplete true after Complete")
	}
}

func TestTypewriterBuffer_Reset(t *testing.T) {
	tb := NewTypewriterBuffer(DefaultTypewriterConfig())
	tb.Append("some text")
	tb.Tick()
	tb.Reset()
	if tb.Len() != 0 {
		t.Errorf("expected len 0 after reset, got %d", tb.Len())
	}
	if tb.RevealedCount() != 0 {
		t.Errorf("expected revealed 0 after reset, got %d", tb.RevealedCount())
	}
	if tb.Visible() != "" {
		t.Errorf("expected empty visible after reset, got %q", tb.Visible())
	}
	if tb.IsComplete() {
		t.Error("expected not complete after reset")
	}
}

func TestTypewriterBuffer_Disabled(t *testing.T) {
	tb := NewTypewriterBuffer(TypewriterConfig{
		Enabled:     true,
		CharsPerSec: 0, // 0 = instant
		BatchSize:   1,
	})
	tb.Append("instant reveal")
	tb.Tick()
	if tb.Visible() != "instant reveal" {
		t.Errorf("expected instant full reveal when CharsPerSec=0, got %q", tb.Visible())
	}
	if !tb.IsComplete() {
		t.Error("expected complete when CharsPerSec=0")
	}

	// Also test !Enabled
	tb2 := NewTypewriterBuffer(TypewriterConfig{
		Enabled:     false,
		CharsPerSec: 100,
		BatchSize:   1,
	})
	tb2.Append("disabled effect")
	tb2.Tick()
	if tb2.Visible() != "disabled effect" {
		t.Errorf("expected instant full reveal when disabled, got %q", tb2.Visible())
	}
}

func TestTypewriterBuffer_Concurrent(t *testing.T) {
	tb := NewTypewriterBuffer(DefaultTypewriterConfig())
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			tb.Append("chunk")
		}()
		go func() {
			defer wg.Done()
			tb.Tick()
			_ = tb.Visible()
			_ = tb.IsComplete()
		}()
	}
	wg.Wait()
	if tb.Len() != 250 {
		t.Errorf("expected len 250 after 50 concurrent appends of 5 chars, got %d", tb.Len())
	}
}

func TestTypewriterBuffer_TimeBasedReveal(t *testing.T) {
	cfg := TypewriterConfig{
		Enabled:     true,
		CharsPerSec: 1000,
		BatchSize:   1,
	}
	tb := NewTypewriterBuffer(cfg)
	tb.Append(strings.Repeat("X", 500))
	// Simulate a 50ms tick
	time.Sleep(50 * time.Millisecond)
	tb.Tick()
	revealed := tb.RevealedCount()
	// At 1000 chars/sec, 50ms should reveal ~50 chars, but minimum batch is 1
	// So we expect at least 1 and definitely less than 500
	if revealed < 1 {
		t.Errorf("expected at least 1 char revealed, got %d", revealed)
	}
	if revealed >= 500 {
		t.Errorf("expected less than 500 chars revealed after short tick, got %d", revealed)
	}
	// Complete and verify
	tb.Complete()
	if tb.RevealedCount() != 500 {
		t.Errorf("expected 500 after Complete, got %d", tb.RevealedCount())
	}
}
