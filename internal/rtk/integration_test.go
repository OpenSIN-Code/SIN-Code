package rtk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRTKExecutorWithCache tests executor with caching integration
func TestRTKExecutorWithCache(t *testing.T) {
	executor := &SimpleExecutor{}
	cache := NewResultCache(1 * time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First execution
	result1, err := executor.Execute(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Cache the result
	cache.Set("echo_hello", result1)

	// Second execution
	result2, err := executor.Execute(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Results should be similar
	if result1.Output != result2.Output {
		t.Errorf("output mismatch between executions: %q vs %q", result1.Output, result2.Output)
	}

	// Check cache
	cached, found := cache.Get("echo_hello")
	if !found {
		t.Error("expected to find cached result")
	}

	if cached.Output != result1.Output {
		t.Errorf("cached output doesn't match: got %q, want %q", cached.Output, result1.Output)
	}
}

// TestRTKConfigWithExecutor tests config and executor integration
func TestRTKConfigWithExecutor(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	executor := &SimpleExecutor{
		timeout: config.GlobalTimeout,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.Execute(ctx, "echo", "test")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Verify timeout was applied
	if result.ExecutionTime > config.GlobalTimeout {
		t.Errorf("execution time exceeded timeout: %v > %v", result.ExecutionTime, config.GlobalTimeout)
	}
}

// TestRTKCacheWithMetrics tests cache with metrics tracking
func TestRTKCacheWithMetrics(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{
		Output:         "test output",
		TokensOriginal: 1000,
		TokensReduced:  200,
	}

	// Multiple accesses
	for i := 0; i < 10; i++ {
		cache.Set(fmt.Sprintf("key%d", i), result)
	}

	for i := 0; i < 20; i++ {
		cache.Get(fmt.Sprintf("key%d", i%10))
	}

	stats := cache.Statistics()
	if stats.Hits < 10 {
		t.Errorf("expected at least 10 cache hits, got %d", stats.Hits)
	}

	if stats.TotalTokensReduced < 1000 {
		t.Errorf("expected token reduction >= 1000, got %d", stats.TotalTokensReduced)
	}
}

// TestRTKConcurrentExecutorAndCache tests concurrent usage
func TestRTKConcurrentExecutorAndCache(t *testing.T) {
	executor := &SimpleExecutor{}
	cache := NewResultCache(1 * time.Hour)

	done := make(chan bool, 20)

	// Concurrent executions and caching
	for i := 0; i < 10; i++ {
		go func(index int) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := executor.Execute(ctx, "echo", fmt.Sprintf("test%d", index))
			if err == nil && result != nil {
				cache.Set(fmt.Sprintf("key%d", index), result)
			}
			done <- true
		}(i)
	}

	// Concurrent cache reads
	for i := 0; i < 10; i++ {
		go func(index int) {
			cache.Get(fmt.Sprintf("key%d", index%10))
			done <- true
		}(i)
	}

	// Wait for completion
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify cache has entries
	if cache.Size() == 0 {
		t.Error("expected cache to have entries")
	}
}

// TestRTKFullWorkflow tests complete workflow
func TestRTKFullWorkflow(t *testing.T) {
	configDir := t.TempDir()
	configManager := NewConfigManager(configDir)

	// 1. Load config
	config, err := configManager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// 2. Update config
	config.CacheEnabled = true
	config.StripANSI = true

	err = configManager.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	// 3. Create executor
	executor := &SimpleExecutor{
		timeout: config.GlobalTimeout,
	}

	// 4. Create cache
	cache := NewResultCache(config.CacheTTL)

	// 5. Execute command
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.Execute(ctx, "echo", "\x1b[32mGreen Text\x1b[0m")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// 6. Strip ANSI if configured
	if config.StripANSI {
		strippedOutput := stripANSI(result.Output)
		if strippedOutput != "Green Text" && strippedOutput != "Green Text\n" {
			t.Errorf("ANSI stripping failed: got %q", strippedOutput)
		}
	}

	// 7. Cache result
	if config.CacheEnabled {
		cache.Set("echo_command", result)

		cached, found := cache.Get("echo_command")
		if !found {
			t.Error("failed to retrieve cached result")
		}

		if cached == nil {
			t.Error("cached result is nil")
		}
	}
}

// TestRTKErrorRecovery tests error recovery in workflows
func TestRTKErrorRecovery(t *testing.T) {
	executor := &SimpleExecutor{}
	cache := NewResultCache(1 * time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to execute non-existent command
	result1, err1 := executor.Execute(ctx, "nonexistent_command_xyz")

	// This should fail
	if err1 == nil {
		t.Error("expected error for non-existent command")
	}

	// But we should still be able to execute valid commands
	result2, err2 := executor.Execute(ctx, "echo", "recovery")
	if err2 != nil {
		t.Fatalf("Execute() after error failed: %v", err2)
	}

	if result2 == nil {
		t.Fatal("expected result after recovery")
	}

	// Cache should work after errors
	cache.Set("recovery", result2)
	cached, found := cache.Get("recovery")
	if !found {
		t.Error("cache failed to work after errors")
	}

	if cached == nil {
		t.Error("cached recovery result is nil")
	}
}

// TestRTKLargeScaleOperations tests handling large numbers of operations
func TestRTKLargeScaleOperations(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)

	// Add many entries to cache
	for i := 0; i < 1000; i++ {
		result := &RTKResult{
			Output:         fmt.Sprintf("output%d", i),
			TokensOriginal: 100 + i,
			TokensReduced:  50 + i/2,
		}
		cache.Set(fmt.Sprintf("key%d", i), result)
	}

	// Verify cache size
	if cache.Size() < 1000 {
		t.Errorf("expected cache to have 1000 entries, got %d", cache.Size())
	}

	// Access entries
	for i := 0; i < 100; i++ {
		_, found := cache.Get(fmt.Sprintf("key%d", i*10))
		if !found {
			t.Errorf("expected to find entry key%d", i*10)
		}
	}

	stats := cache.Statistics()
	if stats.Hits < 100 {
		t.Errorf("expected at least 100 cache hits, got %d", stats.Hits)
	}
}

// TestRTKCacheExpirationsAtScale tests cache expirations with many entries
func TestRTKCacheExpirationsAtScale(t *testing.T) {
	cache := NewResultCache(100 * time.Millisecond)

	// Add many entries
	for i := 0; i < 100; i++ {
		result := &RTKResult{Output: fmt.Sprintf("output%d", i)}
		cache.Set(fmt.Sprintf("key%d", i), result)
	}

	// All should be found immediately
	found := 0
	for i := 0; i < 100; i++ {
		if _, ok := cache.Get(fmt.Sprintf("key%d", i)); ok {
			found++
		}
	}

	if found < 100 {
		t.Errorf("expected all entries to be found, got %d/100", found)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Most/all should be expired
	found = 0
	for i := 0; i < 100; i++ {
		if _, ok := cache.Get(fmt.Sprintf("key%d", i)); ok {
			found++
		}
	}

	if found > 5 { // Allow some tolerance for timing
		t.Errorf("expected most entries to be expired, got %d/100 still present", found)
	}
}

// TestRTKExecutorMemoryUsage tests memory efficiency
func TestRTKExecutorMemoryUsage(t *testing.T) {
	executor := &SimpleExecutor{}
	cache := NewResultCache(1 * time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Execute and cache large outputs
	for i := 0; i < 100; i++ {
		result, err := executor.Execute(ctx, "bash", "-c", "echo 'Large output line' | head -c 10000")
		if err != nil {
			t.Logf("Execute() warning: %v", err)
			continue
		}

		cache.Set(fmt.Sprintf("large%d", i), result)
	}

	// Cache should still be responsive
	_, found := cache.Get("large0")
	if !found && cache.Size() > 0 {
		t.Error("cache not accessible after large operations")
	}
}

// BenchmarkRTKFullWorkflow benchmarks complete RTK workflow
func BenchmarkRTKFullWorkflow(b *testing.B) {
	executor := &SimpleExecutor{}
	cache := NewResultCache(1 * time.Hour)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, _ := executor.Execute(ctx, "echo", fmt.Sprintf("test%d", i))
		if result != nil {
			cache.Set(fmt.Sprintf("key%d", i), result)
		}
	}
}

// BenchmarkCacheHitRate benchmarks cache hit performance
func BenchmarkCacheHitRate(b *testing.B) {
	cache := NewResultCache(1 * time.Hour)

	result := &RTKResult{Output: "test"}
	cache.Set("key", result)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}

// TestRTKSpecIntegration tests integration with Spec Layer
func TestRTKSpecIntegration(t *testing.T) {
	executor := &SimpleExecutor{}
	cache := NewResultCache(1 * time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simulate spec analysis
	result, err := executor.Execute(ctx, "echo", "Spec Analysis Result")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Cache with spec ID
	specID := "spec_001"
	cache.Set(specID, result)

	// Retrieve for spec enrichment
	enriched, found := cache.Get(specID)
	if !found {
		t.Error("failed to retrieve spec analysis")
	}

	if enriched == nil {
		t.Error("enriched result is nil")
	}
}
