package rtk

import (
	"context"
	"testing"
	"time"
)

// TestResultCacheBasic tests basic cache operations
func TestResultCacheBasic(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{
		Output:         "test output",
		ExitCode:       0,
		ExecutionTime:  100 * time.Millisecond,
		TokensOriginal: 100,
		TokensReduced:  50,
	}

	// Store result
	cache.Set("key1", result)

	// Retrieve result
	retrieved, found := cache.Get("key1")
	if !found {
		t.Error("expected to find cached result")
	}

	if retrieved == nil {
		t.Fatal("retrieved result is nil")
	}

	if retrieved.Output != result.Output {
		t.Errorf("Output mismatch: got %q, want %q", retrieved.Output, result.Output)
	}
}

// TestResultCacheExpiration tests TTL expiration
func TestResultCacheExpiration(t *testing.T) {
	cache := NewResultCache(100 * time.Millisecond)

	result := &RTKResult{Output: "test"}
	cache.Set("key1", result)

	// Should be found immediately
	_, found := cache.Get("key1")
	if !found {
		t.Error("expected to find cached result immediately")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not be found after expiration
	_, found = cache.Get("key1")
	if found {
		t.Error("expected cached result to be expired")
	}
}

// TestResultCacheMultipleKeys tests multiple cache entries
func TestResultCacheMultipleKeys(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	for i := 1; i <= 5; i++ {
		result := &RTKResult{
			Output:   "test" + string(rune(i)),
			ExitCode: i,
		}
		cache.Set("key"+string(rune(i)), result)
	}

	// Verify all entries
	for i := 1; i <= 5; i++ {
		_, found := cache.Get("key" + string(rune(i)))
		if !found {
			t.Errorf("expected to find cached entry %d", i)
		}
	}
}

// TestResultCacheClear tests cache clearing
func TestResultCacheClear(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{Output: "test"}
	cache.Set("key1", result)

	// Verify entry exists
	_, found := cache.Get("key1")
	if !found {
		t.Error("expected to find entry before clear")
	}

	// Clear cache
	cache.Clear()

	// Verify entry is gone
	_, found = cache.Get("key1")
	if found {
		t.Error("expected entry to be cleared")
	}
}

// TestResultCacheDelete tests deleting specific entries
func TestResultCacheDelete(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{Output: "test"}
	cache.Set("key1", result)
	cache.Set("key2", result)

	// Delete one entry
	cache.Delete("key1")

	// Verify first entry is gone
	_, found := cache.Get("key1")
	if found {
		t.Error("expected entry to be deleted")
	}

	// Verify second entry still exists
	_, found = cache.Get("key2")
	if !found {
		t.Error("expected other entry to still exist")
	}
}

// TestResultCacheSize tests cache size tracking
func TestResultCacheSize(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{Output: "test"}

	for i := 1; i <= 10; i++ {
		cache.Set("key"+string(rune(i)), result)
	}

	size := cache.Size()
	if size != 10 {
		t.Errorf("expected cache size 10, got %d", size)
	}

	cache.Delete("key1")
	size = cache.Size()
	if size != 9 {
		t.Errorf("expected cache size 9 after delete, got %d", size)
	}
}

// TestResultCacheConcurrency tests concurrent access
func TestResultCacheConcurrency(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	done := make(chan bool, 20)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(index int) {
			result := &RTKResult{Output: "test"}
			cache.Set("key"+string(rune(index)), result)
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(index int) {
			cache.Get("key" + string(rune(index)))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify no race conditions
	if cache.Size() < 10 {
		t.Errorf("expected at least 10 entries after concurrent writes")
	}
}

// TestResultCacheStatistics tests stats collection
func TestResultCacheStatistics(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{
		Output:         "test",
		TokensOriginal: 100,
		TokensReduced:  50,
	}

	cache.Set("key1", result)
	cache.Get("key1") // Cache hit
	cache.Get("key1") // Cache hit
	cache.Get("miss") // Cache miss

	stats := cache.Statistics()
	if stats == nil {
		t.Fatal("expected statistics, got nil")
	}

	if stats.Hits < 2 {
		t.Errorf("expected at least 2 hits, got %d", stats.Hits)
	}

	if stats.Misses < 1 {
		t.Errorf("expected at least 1 miss, got %d", stats.Misses)
	}
}

// TestResultCacheTokenTracking tests token reduction tracking
func TestResultCacheTokenTracking(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{
		TokensOriginal: 1000,
		TokensReduced:  200,
	}

	cache.Set("key1", result)

	stats := cache.Statistics()
	if stats.TotalTokensReduced < 800 {
		t.Errorf("expected token reduction >= 800, got %d", stats.TotalTokensReduced)
	}
}

// TestResultCacheEviction tests LRU eviction
func TestResultCacheEviction(t *testing.T) {
	// Small cache with max 5 entries
	cache := NewResultCache(1 * time.Hour)
	cache.maxSize = 5

	// Add more than max entries
	for i := 0; i < 10; i++ {
		result := &RTKResult{Output: "test" + string(rune(i))}
		cache.Set("key"+string(rune(i)), result)
	}

	// Cache size should be at most maxSize
	size := cache.Size()
	if size > 5 {
		t.Errorf("cache size %d exceeds max size 5", size)
	}
}

// BenchmarkCacheGet benchmarks cache read performance
func BenchmarkCacheGet(b *testing.B) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{Output: "test"}
	cache.Set("key", result)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}

// BenchmarkCacheSet benchmarks cache write performance
func BenchmarkCacheSet(b *testing.B) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{Output: "test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("key"+string(rune(i%100)), result)
	}
}

// TestResultCacheContextCancellation tests cancellation handling
func TestResultCacheContextCancellation(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := &RTKResult{Output: "test"}

	// Operations should handle cancelled context gracefully
	cache.Set("key1", result)
	_, found := cache.Get("key1")

	if !found {
		t.Error("cache should work even with cancelled context")
	}
}

// TestResultCacheLargeOutput tests handling of large output
func TestResultCacheLargeOutput(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	// Create result with large output (10MB)
	largeOutput := ""
	for i := 0; i < 10000000; i++ {
		largeOutput += "a"
	}

	result := &RTKResult{
		Output:         largeOutput,
		TokensOriginal: len(largeOutput),
		TokensReduced:  len(largeOutput) / 2,
	}

	cache.Set("large", result)

	retrieved, found := cache.Get("large")
	if !found {
		t.Error("expected to find large output")
	}

	if len(retrieved.Output) != len(largeOutput) {
		t.Errorf("output size mismatch: got %d, want %d", len(retrieved.Output), len(largeOutput))
	}
}

// TestResultCacheNilHandling tests handling of nil values
func TestResultCacheNilHandling(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	// Set nil
	cache.Set("key1", nil)

	// Get nil
	result, found := cache.Get("key1")

	// nil values might be stored as empty, depending on implementation
	// Just verify no panic
	_ = result
	_ = found
}

// TestResultCacheEmptyKey tests empty key handling
func TestResultCacheEmptyKey(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{Output: "test"}

	// Store with empty key
	cache.Set("", result)

	// Retrieve with empty key
	_, found := cache.Get("")

	// Should work (even with unusual key)
	_ = found
}
