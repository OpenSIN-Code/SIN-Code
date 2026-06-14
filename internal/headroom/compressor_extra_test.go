// internal/headroom/compressor_extra_test.go
package headroom

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCompressor_DisabledPassthrough(t *testing.T) {
	cfg := DefaultConfig() // disabled by default
	comp := NewCompressor(cfg)

	content := "some content"
	out, result, err := comp.CompressContent(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != content {
		t.Errorf("disabled compressor should pass through, got %q", out)
	}
	if result != nil {
		t.Errorf("disabled compressor should return nil result, got %+v", result)
	}
}

func TestCompressor_LearnRecordsLesson(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Mode = ModeProxy // proxy mode needs no external backend for LearnFromFailure
	cfg.LearnFromFailures = true

	comp := NewCompressor(cfg)

	store, err := NewLessonStore(filepath.Join(t.TempDir(), "lessons.json"))
	if err != nil {
		t.Fatalf("NewLessonStore failed: %v", err)
	}
	comp.SetLessonStore(store)

	if err := comp.LearnFromFailure(context.Background(), "build failed: missing import in foo.go"); err != nil {
		t.Fatalf("LearnFromFailure failed: %v", err)
	}

	if store.Count() != 1 {
		t.Errorf("expected 1 recorded lesson, got %d", store.Count())
	}
	if comp.Lessons() != store {
		t.Error("Lessons() should return the attached store")
	}
}

func TestCompressor_LearnDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Mode = ModeProxy
	cfg.LearnFromFailures = false

	comp := NewCompressor(cfg)
	store, _ := NewLessonStore(filepath.Join(t.TempDir(), "lessons.json"))
	comp.SetLessonStore(store)

	if err := comp.LearnFromFailure(context.Background(), "irrelevant"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.Count() != 0 {
		t.Errorf("learning disabled should record nothing, got %d", store.Count())
	}
}

func TestTruncatePattern(t *testing.T) {
	short := "abc"
	if truncatePattern(short) != short {
		t.Error("short pattern should be unchanged")
	}

	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	if len(truncatePattern(string(long))) != 280 {
		t.Errorf("long pattern should be truncated to 280, got %d", len(truncatePattern(string(long))))
	}
}

func TestUpdateStats(t *testing.T) {
	cfg := DefaultConfig()
	comp := NewCompressor(cfg)

	comp.updateStats(&CompressionResult{OriginalTokens: 100, CompressedTokens: 40})
	stats := comp.GetStats()
	if stats.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", stats.TotalRequests)
	}
	if stats.TotalOriginalTokens != 100 || stats.TotalCompressedTokens != 40 {
		t.Errorf("token totals incorrect: %+v", stats)
	}
	expectedSavings := (1 - 40.0/100.0) * 100
	if stats.AverageSavings != expectedSavings {
		t.Errorf("expected savings %.2f, got %.2f", expectedSavings, stats.AverageSavings)
	}
}
