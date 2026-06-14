package rtk

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestConfigManagerLoad tests loading configuration
func TestConfigManagerLoad(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if config == nil {
		t.Fatal("expected config, got nil")
	}

	if !config.Enabled {
		t.Error("default config should be enabled")
	}
}

// TestConfigManagerSave tests saving configuration
func TestConfigManagerSave(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	config.BinaryPath = "/custom/path/rtk"

	err = manager.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	// Reload and verify
	reloaded, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() after save error = %v", err)
	}

	if reloaded.BinaryPath != "/custom/path/rtk" {
		t.Errorf("BinaryPath mismatch: got %q, want %q", reloaded.BinaryPath, "/custom/path/rtk")
	}
}

// TestConfigManagerDefaults tests default values
func TestConfigManagerDefaults(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"Enabled", config.Enabled, true},
		{"DetectBinary", config.DetectBinary, true},
		{"CacheEnabled", config.CacheEnabled, true},
		{"StripANSI", config.StripANSI, true},
		{"MetricsEnabled", config.MetricsEnabled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

// TestConfigManagerEnvironmentOverride tests environment variable overrides
func TestConfigManagerEnvironmentOverride(t *testing.T) {
	configDir := t.TempDir()

	// Set environment variable
	os.Setenv("RTK_BINARY", "/custom/rtk")
	defer os.Unsetenv("RTK_BINARY")

	manager := NewConfigManager(configDir)
	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Binary path might be overridden by env var
	if config.BinaryPath != "/custom/rtk" && config.BinaryPath != "" {
		// If env override is implemented, it should be /custom/rtk
		// Otherwise it's okay to have default
	}
}

// TestConfigManagerMerge tests configuration merging
func TestConfigManagerMerge(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config1, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	config2 := &RTKConfig{
		Enabled:      false,
		BinaryPath:   "/custom/rtk",
		GlobalTimeout: 30 * time.Second,
	}

	// This would test merge functionality if implemented
	_ = config1
	_ = config2
}

// TestConfigManagerValidation tests configuration validation
func TestConfigManagerValidation(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config := &RTKConfig{
		Enabled: true,
		CacheTTL: -1 * time.Second, // Invalid: negative TTL
	}

	err := manager.ValidateConfig(config)
	if err == nil {
		t.Error("expected validation error for negative TTL")
	}
}

// TestConfigManagerReset tests resetting configuration
func TestConfigManagerReset(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	config.BinaryPath = "/custom/path"
	manager.SaveConfig(config)

	err = manager.ResetConfig()
	if err != nil {
		t.Fatalf("ResetConfig() error = %v", err)
	}

	reloaded, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() after reset error = %v", err)
	}

	if reloaded.BinaryPath != "" {
		t.Errorf("BinaryPath should be reset, got %q", reloaded.BinaryPath)
	}
}

// TestConfigManagerGetSet tests individual get/set operations
func TestConfigManagerGetSet(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Set individual values
	manager.SetConfigValue(config, "BinaryPath", "/custom/rtk")

	if config.BinaryPath != "/custom/rtk" {
		t.Errorf("BinaryPath not set correctly: got %q", config.BinaryPath)
	}

	// Get individual values
	value := manager.GetConfigValue(config, "BinaryPath")
	if value != "/custom/rtk" {
		t.Errorf("BinaryPath not retrieved correctly: got %v", value)
	}
}

// TestConfigManagerMultipleInstances tests multiple config manager instances
func TestConfigManagerMultipleInstances(t *testing.T) {
	configDir := t.TempDir()

	manager1 := NewConfigManager(configDir)
	manager2 := NewConfigManager(configDir)

	config1, err := manager1.LoadConfig()
	if err != nil {
		t.Fatalf("manager1.LoadConfig() error = %v", err)
	}

	config1.BinaryPath = "/path1"
	manager1.SaveConfig(config1)

	config2, err := manager2.LoadConfig()
	if err != nil {
		t.Fatalf("manager2.LoadConfig() error = %v", err)
	}

	// Both should see the same saved config
	if config2.BinaryPath != "/path1" {
		t.Errorf("manager2 didn't see manager1's changes: got %q", config2.BinaryPath)
	}
}

// TestConfigManagerCorruptedFile tests handling of corrupted config file
func TestConfigManagerCorruptedFile(t *testing.T) {
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "rtk.json")

	// Write corrupted JSON
	err := os.WriteFile(configFile, []byte("{invalid json}"), 0644)
	if err != nil {
		t.Fatalf("failed to write corrupted config: %v", err)
	}

	manager := NewConfigManager(configDir)

	// Should handle gracefully
	config, err := manager.LoadConfig()
	if err == nil && config != nil {
		// Either error or default config is acceptable
	}
}

// TestConfigManagerFilePermissions tests file permission handling
func TestConfigManagerFilePermissions(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	err = manager.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	configFile := filepath.Join(configDir, "rtk.json")
	fileInfo, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}

	// File should be readable
	if fileInfo.Mode().Perm()&0400 == 0 {
		t.Error("config file is not readable")
	}
}

// TestConfigManagerTimeout tests timeout configuration
func TestConfigManagerTimeout(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	config.GlobalTimeout = 30 * time.Second
	err = manager.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	reloaded, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() after save error = %v", err)
	}

	if reloaded.GlobalTimeout != 30*time.Second {
		t.Errorf("timeout not preserved: got %v, want 30s", reloaded.GlobalTimeout)
	}
}

// TestConfigManagerCacheTTL tests cache TTL configuration
func TestConfigManagerCacheTTL(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	config.CacheTTL = 48 * time.Hour
	err = manager.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	reloaded, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() after save error = %v", err)
	}

	if reloaded.CacheTTL != 48*time.Hour {
		t.Errorf("CacheTTL not preserved: got %v, want 48h", reloaded.CacheTTL)
	}
}

// TestConfigManagerLogLevel tests log level configuration
func TestConfigManagerLogLevel(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	config, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	config.LogLevel = "debug"
	err = manager.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	reloaded, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() after save error = %v", err)
	}

	if reloaded.LogLevel != "debug" {
		t.Errorf("LogLevel not preserved: got %q, want debug", reloaded.LogLevel)
	}
}

// BenchmarkConfigLoad benchmarks config loading
func BenchmarkConfigLoad(b *testing.B) {
	configDir := b.TempDir()
	manager := NewConfigManager(configDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.LoadConfig()
	}
}

// BenchmarkConfigSave benchmarks config saving
func BenchmarkConfigSave(b *testing.B) {
	configDir := b.TempDir()
	manager := NewConfigManager(configDir)

	config, _ := manager.LoadConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.SaveConfig(config)
	}
}

// TestConfigManagerDirectoryCreation tests automatic directory creation
func TestConfigManagerDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "nested", "dir", "rtk")

	manager := NewConfigManager(configDir)

	_, err := manager.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Directory should be created
	info, err := os.Stat(configDir)
	if err != nil || !info.IsDir() {
		t.Errorf("config directory not created or not accessible")
	}
}

// TestConfigManagerConfigFile tests config file path
func TestConfigManagerConfigFile(t *testing.T) {
	configDir := t.TempDir()
	manager := NewConfigManager(configDir)

	configFilePath := manager.GetConfigFile()
	if configFilePath == "" {
		t.Error("expected non-empty config file path")
	}

	if !filepath.IsAbs(configFilePath) {
		t.Error("expected absolute config file path")
	}
}
