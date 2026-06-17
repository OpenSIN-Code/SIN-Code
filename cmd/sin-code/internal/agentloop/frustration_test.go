// SPDX-License-Identifier: MIT
// Purpose: tests for frustration detection and adaptive UX (issue #271, M7).
package agentloop

import (
	"sync"
	"testing"
	"time"
)

func TestFrustration_NormalKeyword_Mild(t *testing.T) {
	d := NewFrustrationDetector()
	now := time.Now()
	level := d.Track("why is this not working?", now)
	if level != FrustrationMild {
		t.Errorf("normal keyword 'why' should trigger mild, got %s", level)
	}
}

func TestFrustration_StrongKeyword_MildOrHigh(t *testing.T) {
	d := NewFrustrationDetector()
	now := time.Now()
	level := d.Track("wtf is going on", now)
	if level < FrustrationMild {
		t.Errorf("strong keyword 'wtf' should trigger at least mild, got %s", level)
	}
}

func TestFrustration_StrongKeywordPlusCaps_High(t *testing.T) {
	d := NewFrustrationDetector()
	now := time.Now()
	level := d.Track("WTF IS GOING ON HERE", now)
	if level != FrustrationHigh {
		t.Errorf("strong keyword + caps lock should trigger high, got %s (score=%d)", level, d.Score())
	}
}

func TestFrustration_NoKeywords_None(t *testing.T) {
	d := NewFrustrationDetector()
	now := time.Now()
	level := d.Track("please help me write a function", now)
	if level != FrustrationNone {
		t.Errorf("benign message should be none, got %s", level)
	}
}

func TestFrustration_RepetitionDetection(t *testing.T) {
	d := NewFrustrationDetector()
	base := time.Now()
	msg := "why doesn't this work"
	d.Track(msg, base)
	d.Track(msg, base.Add(1*time.Second))
	level := d.Track(msg, base.Add(2*time.Second))
	if level < FrustrationHigh {
		t.Errorf("3x repetition should trigger high, got %s (score=%d)", level, d.Score())
	}
}

func TestFrustration_RepetitionBelowThreshold(t *testing.T) {
	d := NewFrustrationDetector()
	base := time.Now()
	msg := "why doesn't this work"
	d.Track(msg, base)
	level := d.Track(msg, base.Add(1*time.Second))
	if level != FrustrationMild {
		t.Errorf("2x repetition with keyword should be mild (keyword only), got %s (score=%d)", level, d.Score())
	}
}

func TestFrustration_CapsLockDetection(t *testing.T) {
	d := NewFrustrationDetector()
	now := time.Now()
	level := d.Track("STOP DOING THAT WRONG THING", now)
	if level < FrustrationMild {
		t.Errorf("caps lock + keyword should trigger at least mild, got %s (score=%d)", level, d.Score())
	}
}

func TestFrustration_CapsLockNotTriggered_ShortMessage(t *testing.T) {
	d := NewFrustrationDetector()
	now := time.Now()
	level := d.Track("NO", now)
	if level != FrustrationMild {
		t.Errorf("short 'NO' has keyword but caps should not count (< 5 alpha), got %s (score=%d)", level, d.Score())
	}
}

func TestFrustration_RapidRetriesDetection(t *testing.T) {
	d := NewFrustrationDetector()
	base := time.Now()
	d.Track("try again", base)
	d.Track("try again", base.Add(10*time.Second))
	level := d.Track("try again", base.Add(20*time.Second))
	if level < FrustrationMild {
		t.Errorf("3 rapid retries in 30s should trigger at least mild, got %s (score=%d)", level, d.Score())
	}
}

func TestFrustration_RapidRetriesNotTriggered_SlowPace(t *testing.T) {
	d := NewFrustrationDetector()
	base := time.Now()
	d.Track("try this", base)
	d.Track("try this", base.Add(40*time.Second))
	level := d.Track("try this", base.Add(50*time.Second))
	if d.Score() > 0 && level == FrustrationHigh {
		t.Errorf("slow pace should not trigger rapid-retry signal, got %s (score=%d)", level, d.Score())
	}
}

func TestFrustration_LevelTransitions(t *testing.T) {
	d := NewFrustrationDetector()
	base := time.Now()
	if d.Level() != FrustrationNone {
		t.Fatal("initial level should be none")
	}
	d.Track("please help", base)
	if d.Level() != FrustrationNone {
		t.Errorf("after benign message, level should be none, got %s", d.Level())
	}
	d.Track("why is this wrong", base.Add(5*time.Second))
	if d.Level() != FrustrationMild {
		t.Errorf("after keyword, level should be mild, got %s", d.Level())
	}
	d.Track("WTF THIS IS BROKEN STOP", base.Add(10*time.Second))
	if d.Level() != FrustrationHigh {
		t.Errorf("after strong keyword + caps, level should be high, got %s", d.Level())
	}
}

func TestFrustration_Reset(t *testing.T) {
	d := NewFrustrationDetector()
	now := time.Now()
	d.Track("WTF THIS IS COMPLETELY WRONG", now)
	if d.Level() != FrustrationHigh {
		t.Fatalf("expected high before reset, got %s", d.Level())
	}
	d.Reset()
	if d.Level() != FrustrationNone {
		t.Errorf("after reset, level should be none, got %s", d.Level())
	}
	if d.Score() != 0 {
		t.Errorf("after reset, score should be 0, got %d", d.Score())
	}
	d.Track("please help", now)
	if d.Level() != FrustrationNone {
		t.Errorf("after reset + benign message, level should be none, got %s", d.Level())
	}
}

func TestFrustration_SlidingWindowExpiry(t *testing.T) {
	d := NewFrustrationDetector()
	base := time.Now()
	d.Track("why is this wrong", base)
	d.Track("why is this wrong", base.Add(1*time.Second))
	d.Track("why is this wrong", base.Add(2*time.Second))
	if d.Level() != FrustrationHigh {
		t.Fatalf("expected high from repetition, got %s (score=%d)", d.Level(), d.Score())
	}
	level := d.Track("please help nicely", base.Add(65*time.Second))
	if level != FrustrationNone {
		t.Errorf("after window expiry + benign message, level should be none, got %s (score=%d)", level, d.Score())
	}
}

func TestFrustration_SystemPromptSuffix(t *testing.T) {
	d := NewFrustrationDetector()
	if d.SystemPromptSuffix() != "" {
		t.Error("none level should produce empty suffix")
	}
	now := time.Now()
	d.Track("why is this wrong", now)
	if d.SystemPromptSuffix() == "" {
		t.Error("mild level should produce non-empty suffix")
	}
	if d.Level() != FrustrationMild {
		t.Fatalf("expected mild, got %s", d.Level())
	}
	d.Track("WTF THIS IS STILL WRONG STOP IT", now.Add(5*time.Second))
	if d.Level() != FrustrationHigh {
		t.Fatalf("expected high, got %s", d.Level())
	}
	suffix := d.SystemPromptSuffix()
	if suffix == "" {
		t.Error("high level should produce non-empty suffix")
	}
}

func TestFrustration_Metadata(t *testing.T) {
	d := NewFrustrationDetector()
	now := time.Now()
	d.Track("WTF STOP", now)
	meta := d.Metadata()
	if meta["frustration_level"] != "high" && meta["frustration_level"] != "mild" {
		t.Errorf("metadata should reflect frustration, got %v", meta["frustration_level"])
	}
	if frustrated, ok := meta["frustrated"].(bool); !ok || !frustrated {
		t.Errorf("metadata should mark frustrated=true, got %v", meta["frustrated"])
	}
	d.Reset()
	meta = d.Metadata()
	if meta["frustration_level"] != "none" {
		t.Errorf("after reset metadata should be none, got %v", meta["frustration_level"])
	}
	if frustrated, ok := meta["frustrated"].(bool); !ok || frustrated {
		t.Errorf("after reset metadata should mark frustrated=false, got %v", meta["frustrated"])
	}
}

func TestFrustration_IsFrustrated(t *testing.T) {
	d := NewFrustrationDetector()
	if d.IsFrustrated() {
		t.Error("new detector should not be frustrated")
	}
	now := time.Now()
	d.Track("why is this wrong", now)
	if !d.IsFrustrated() {
		t.Error("after keyword, should be frustrated")
	}
	d.Reset()
	if d.IsFrustrated() {
		t.Error("after reset, should not be frustrated")
	}
}

func TestFrustration_RaceSafe(t *testing.T) {
	d := NewFrustrationDetector()
	base := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ts := base.Add(time.Duration(n) * time.Second)
			switch n % 5 {
			case 0:
				d.Track("why is this wrong", ts)
			case 1:
				d.Track("WTF STOP DOING THAT", ts)
			case 2:
				d.Track("please help me", ts)
			case 3:
				_ = d.Level()
				_ = d.Score()
				_ = d.SystemPromptSuffix()
				_ = d.Metadata()
			case 4:
				d.Reset()
			}
		}(i)
	}
	wg.Wait()
}

func TestFrustration_NilSafe(t *testing.T) {
	var d *FrustrationDetector
	if d.Level() != FrustrationNone {
		t.Error("nil detector Level() should return none")
	}
	if d.Track("anything", time.Now()) != FrustrationNone {
		t.Error("nil detector Track() should return none")
	}
	if d.SystemPromptSuffix() != "" {
		t.Error("nil detector should return empty suffix")
	}
	if d.IsFrustrated() {
		t.Error("nil detector should not be frustrated")
	}
	d.Reset()
}

func TestFrustrationLevel_String(t *testing.T) {
	cases := []struct {
		level FrustrationLevel
		want  string
	}{
		{FrustrationNone, "none"},
		{FrustrationMild, "mild"},
		{FrustrationHigh, "high"},
		{FrustrationLevel(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.level.String(); got != c.want {
			t.Errorf("%d.String() = %q, want %q", c.level, got, c.want)
		}
	}
}
