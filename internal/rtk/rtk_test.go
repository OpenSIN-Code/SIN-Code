package rtk

import (
	"context"
	"testing"
	"time"
)

// TestRTKExecutorCreation tests executor creation
func TestRTKExecutorCreation(t *testing.T) {
	config := &RTKConfig{
		Enabled:       true,
		GlobalTimeout: 30 * time.Second,
	}

	executor, err := NewSimpleExecutor(config)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	if executor == nil {
		t.Fatal("Executor is nil")
	}

	if executor.GetConfig() == nil {
		t.Fatal("Config is nil")
	}
}

// TestDefaultConfig tests default configuration
func TestDefaultConfig(t *testing.T) {
	executor, err := NewSimpleExecutor(nil)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	config := executor.GetConfig()
	if !config.Enabled {
		t.Fatal("Config should be enabled by default")
	}

	if !config.DetectBinary {
		t.Fatal("Binary detection should be enabled by default")
	}

	if !config.StripANSI {
		t.Fatal("ANSI stripping should be enabled by default")
	}

	if !config.CacheEnabled {
		t.Fatal("Cache should be enabled by default")
	}
}

// TestRTKBinaryDetection tests binary detection
func TestRTKBinaryDetection(t *testing.T) {
	config := &RTKConfig{
		Enabled:      true,
		DetectBinary: true,
	}

	executor, err := NewSimpleExecutor(config)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := executor.Detect(ctx)
	// Detection may fail if rtk is not installed, which is OK
	if info != nil {
		if info.Path == "" {
			t.Fatal("Binary path is empty")
		}
	}
}

// TestRTKToolCreation tests tool creation
func TestRTKToolCreation(t *testing.T) {
	tool := &RTKTool{
		Name:        "test_tool",
		Kind:        RTKToolKindValidator,
		Description: "Test tool",
		Args:        []string{"--help"},
		Timeout:     30 * time.Second,
		Enabled:     true,
	}

	if tool.Name != "test_tool" {
		t.Fatal("Tool name not set correctly")
	}

	if tool.Kind != RTKToolKindValidator {
		t.Fatal("Tool kind not set correctly")
	}

	if tool.Timeout != 30*time.Second {
		t.Fatal("Tool timeout not set correctly")
	}
}

// TestRTKToolRegistry tests tool registry
func TestRTKToolRegistry(t *testing.T) {
	registry := NewRTKToolRegistry()

	tool1 := &RTKTool{Name: "tool1", Enabled: true}
	tool2 := &RTKTool{Name: "tool2", Enabled: true}

	registry.Register(tool1)
	registry.Register(tool2)

	// Test Get
	retrieved, ok := registry.Get("tool1")
	if !ok {
		t.Fatal("Tool not found in registry")
	}

	if retrieved.Name != "tool1" {
		t.Fatal("Retrieved tool has wrong name")
	}

	// Test List
	tools := registry.List()
	if len(tools) != 2 {
		t.Fatalf("Expected 2 tools, got %d", len(tools))
	}
}

// TestResultCache tests result caching
func TestResultCache(t *testing.T) {
	cache := NewResultCache(1 * time.Second)
	defer cache.Stop()

	result := &RTKResult{
		Name:      "test",
		Status:    RTKStatusSuccess,
		ExitCode:  0,
		Timestamp: time.Now(),
	}

	cache.Set("key1", result)

	// Retrieve
	retrieved, found := cache.Get("key1")
	if !found {
		t.Fatal("Result not found in cache")
	}

	if retrieved.Name != "test" {
		t.Fatal("Retrieved result has wrong name")
	}

	// Test expiration
	time.Sleep(1100 * time.Millisecond)

	_, found = cache.Get("key1")
	if found {
		t.Fatal("Expired result should not be found")
	}
}

// TestCacheSize tests cache size tracking
func TestCacheSize(t *testing.T) {
	cache := NewResultCache(1 * time.Hour)
	defer cache.Stop()

	if cache.Size() != 0 {
		t.Fatal("Cache should be empty initially")
	}

	for i := 0; i < 10; i++ {
		result := &RTKResult{Name: "test"}
		cache.Set(string(rune(i)), result)
	}

	if cache.Size() != 10 {
		t.Fatalf("Cache size should be 10, got %d", cache.Size())
	}

	cache.Clear()
	if cache.Size() != 0 {
		t.Fatal("Cache should be empty after clear")
	}
}

// TestRTKConfig tests configuration validation
func TestRTKConfigValidation(t *testing.T) {
	config := &RTKConfig{
		GlobalTimeout: 0,
	}

	err := ValidateConfig(config)
	if err == nil {
		t.Fatal("Should fail with zero timeout")
	}

	config.GlobalTimeout = 30 * time.Second
	err = ValidateConfig(config)
	if err != nil {
		t.Fatalf("Should pass with valid timeout: %v", err)
	}
}

// TestConfigMerge tests configuration merging
func TestConfigMerge(t *testing.T) {
	base := &RTKConfig{
		Enabled:       true,
		GlobalTimeout: 30 * time.Second,
		LogLevel:      "info",
	}

	override := &RTKConfig{
		GlobalTimeout: 60 * time.Second,
		LogLevel:      "debug",
	}

	merged := MergeConfig(base, override)

	if merged.GlobalTimeout != 60*time.Second {
		t.Fatal("Timeout not overridden")
	}

	if merged.LogLevel != "debug" {
		t.Fatal("LogLevel not overridden")
	}
}

// TestMCPToolHandler tests MCP tool handler
func TestMCPToolHandler(t *testing.T) {
	config := &RTKConfig{Enabled: true}
	executor, _ := NewSimpleExecutor(config)

	handler := NewMCPToolHandler(executor)

	tool := &RTKTool{
		Name:        "test_mcp_tool",
		Description: "Test MCP tool",
		Kind:        RTKToolKindValidator,
		Enabled:     true,
	}

	err := handler.RegisterTool(tool)
	if err != nil {
		t.Fatalf("Failed to register tool: %v", err)
	}

	// Validate tool
	err = handler.ValidateTool("test_mcp_tool")
	if err != nil {
		t.Fatalf("Tool validation failed: %v", err)
	}

	// Get definitions
	defs := handler.GetToolDefinitions()
	if len(defs) == 0 {
		t.Fatal("No tool definitions returned")
	}
}

// TestANSIStripping tests ANSI color code stripping
func TestANSIStripping(t *testing.T) {
	text := "\x1b[31mRed Text\x1b[0m"
	clean := StripANSI(text)

	if clean != "Red Text" {
		t.Fatalf("ANSI not stripped correctly: %q", clean)
	}
}

// TestTokenCounting tests token counting
func TestTokenCounting(t *testing.T) {
	text := "Hello world" // 11 characters
	tokens := CountTokens(text)

	// Expecting roughly 3 tokens (11 chars / 4 chars per token)
	if tokens != 3 && tokens != 2 && tokens != 4 {
		t.Fatalf("Token count unexpected: %d", tokens)
	}
}

// TestMetrics tests metrics collection
func TestMetrics(t *testing.T) {
	config := &RTKConfig{Enabled: true}
	executor, _ := NewSimpleExecutor(config)

	metrics := executor.GetMetrics()
	if metrics == nil {
		t.Fatal("Metrics is nil")
	}

	if metrics.TotalExecutions != 0 {
		t.Fatal("Metrics should start at zero")
	}
}

// TestSpecIndexingIntegration tests spec integration
func TestSpecIndexingIntegration(t *testing.T) {
	config := &RTKConfig{Enabled: true}
	executor, _ := NewSimpleExecutor(config)

	integration := NewSpecIndexingIntegration(executor)
	if integration == nil {
		t.Fatal("Integration is nil")
	}

	// Test cache retrieval
	_, found := integration.GetCachedAnalysis("nonexistent")
	if found {
		t.Fatal("Should not find nonexistent cache")
	}
}

// TestGenerateRTKCommandForSpec tests spec command generation
func TestGenerateRTKCommandForSpec(t *testing.T) {
	tool, err := GenerateRTKCommandForSpec("goal", "auth")
	if err != nil {
		t.Fatalf("Failed to generate command: %v", err)
	}

	if tool == nil {
		t.Fatal("Tool is nil")
	}

	if tool.Name == "" {
		t.Fatal("Tool name is empty")
	}

	if tool.Tags["spec_kind"] != "goal" {
		t.Fatal("spec_kind tag not set")
	}

	if tool.Tags["spec_namespace"] != "auth" {
		t.Fatal("spec_namespace tag not set")
	}
}

// TestTokenReductionReport tests token reduction reporting
func TestTokenReductionReport(t *testing.T) {
	config := &RTKConfig{
		Enabled:      true,
		StripANSI:    true,
		MetricsEnabled: true,
	}

	executor, _ := NewSimpleExecutor(config)
	integration := NewSpecIndexingIntegration(executor)

	report := integration.CalculateTokenReductionReport()
	if report == nil {
		t.Fatal("Report is nil")
	}

	if report.ToolsUsed == nil {
		t.Fatal("ToolsUsed is nil")
	}
}

// BenchmarkRTKExecutorCreation benchmarks executor creation
func BenchmarkRTKExecutorCreation(b *testing.B) {
	config := &RTKConfig{Enabled: true}

	for i := 0; i < b.N; i++ {
		NewSimpleExecutor(config)
	}
}

// BenchmarkCacheOperation benchmarks cache operations
func BenchmarkCacheOperation(b *testing.B) {
	cache := NewResultCache(1 * time.Hour)
	defer cache.Stop()

	result := &RTKResult{Name: "test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("key", result)
		cache.Get("key")
	}
}

// BenchmarkANSIStripping benchmarks ANSI stripping
func BenchmarkANSIStripping(b *testing.B) {
	text := "\x1b[31m\x1b[1mHello\x1b[0m \x1b[32mWorld\x1b[0m"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StripANSI(text)
	}
}
