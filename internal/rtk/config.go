package rtk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ConfigManager handles RTK configuration persistence
type ConfigManager struct {
	configDir string
	configFile string
	config *RTKConfig
}

// NewConfigManager creates a new config manager
func NewConfigManager(configDir string) *ConfigManager {
	if configDir == "" {
		configDir = getDefaultConfigDir()
	}

	return &ConfigManager{
		configDir: configDir,
		configFile: filepath.Join(configDir, "rtk.json"),
	}
}

// LoadConfig loads configuration from file
func (m *ConfigManager) LoadConfig() (*RTKConfig, error) {
	// Create config directory if needed
	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(m.configFile); os.IsNotExist(err) {
		// Create default config
		return m.createDefaultConfig()
	}

	// Read file
	data, err := os.ReadFile(m.configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	config := &RTKConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	m.config = config
	return config, nil
}

// SaveConfig saves configuration to file
func (m *ConfigManager) SaveConfig(config *RTKConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Create directory if needed
	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write file
	if err := os.WriteFile(m.configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	m.config = config
	return nil
}

// GetConfig returns current configuration
func (m *ConfigManager) GetConfig() *RTKConfig {
	return m.config
}

// createDefaultConfig creates and saves a default configuration
func (m *ConfigManager) createDefaultConfig() (*RTKConfig, error) {
	config := &RTKConfig{
		Enabled:         true,
		DetectBinary:    true,
		GlobalTimeout:   DefaultGlobalTimeout,
		CacheEnabled:    true,
		CacheTTL:        DefaultCacheTTL,
		StripANSI:       true,
		MetricsEnabled:  true,
		LogLevel:        DefaultLogLevel,
		ExecutionMode:   RTKExecutionModeLocal,
		Tools:           make(map[string]*RTKTool),
		RetryPolicy: &RetryPolicy{
			MaxRetries:        DefaultRetryCount,
			InitialBackoff:    1 * time.Second,
			MaxBackoff:        30 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}

	// Save to file
	if err := m.SaveConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// MergeConfig merges two configurations
func MergeConfig(base *RTKConfig, override *RTKConfig) *RTKConfig {
	if base == nil {
		base = &RTKConfig{}
	}
	if override == nil {
		return base
	}

	// Create a copy
	merged := *base

	// Override fields
	if override.BinaryPath != "" {
		merged.BinaryPath = override.BinaryPath
	}
	if override.LogLevel != "" {
		merged.LogLevel = override.LogLevel
	}
	if override.CacheTTL != 0 {
		merged.CacheTTL = override.CacheTTL
	}
	if override.GlobalTimeout != 0 {
		merged.GlobalTimeout = override.GlobalTimeout
	}
	if override.MCPServerAddress != "" {
		merged.MCPServerAddress = override.MCPServerAddress
	}
	if override.CacheDir != "" {
		merged.CacheDir = override.CacheDir
	}

	// Boolean overrides
	merged.Enabled = override.Enabled
	merged.DetectBinary = override.DetectBinary
	merged.StripANSI = override.StripANSI
	merged.CacheEnabled = override.CacheEnabled
	merged.MetricsEnabled = override.MetricsEnabled

	// Merge tools
	if override.Tools != nil {
		if merged.Tools == nil {
			merged.Tools = make(map[string]*RTKTool)
		}
		for name, tool := range override.Tools {
			merged.Tools[name] = tool
		}
	}

	return &merged
}

// ValidateConfig validates configuration
func ValidateConfig(config *RTKConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	if config.GlobalTimeout <= 0 {
		return fmt.Errorf("global timeout must be positive")
	}

	if config.CacheTTL < 0 {
		return fmt.Errorf("cache ttl cannot be negative")
	}

	if config.LogLevel == "" {
		config.LogLevel = DefaultLogLevel
	}

	return nil
}

// getDefaultConfigDir returns the default config directory
func getDefaultConfigDir() string {
	if configDir := os.Getenv("RTK_CONFIG_DIR"); configDir != "" {
		return configDir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/rtk"
	}

	return filepath.Join(home, ".config", "rtk")
}

// InitializeConfig initializes RTK configuration
func InitializeConfig() (*RTKConfig, error) {
	manager := NewConfigManager("")
	return manager.LoadConfig()
}

// GetConfigFile returns the config file path
func (m *ConfigManager) GetConfigFile() string {
	return m.configFile
}

// ResetConfig resets configuration to defaults
func (m *ConfigManager) ResetConfig() error {
	config, err := m.createDefaultConfig()
	if err != nil {
		return err
	}
	m.config = config
	return nil
}
