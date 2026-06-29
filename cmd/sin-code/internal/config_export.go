// SPDX-License-Identifier: MIT
// Purpose: exported wrappers around the unexported config helpers in
// config.go, following the same LoadMergedConfig/loadMergedConfig pattern.
// These allow the cmd/sin-code/internal/config subpackage (and its tests)
// to exercise the full config API surface without duplicating logic.
package internal

// sin-debt: shrink, upgrade: inline wrappers when config subpackage is merged into internal package

// ConfigKV is an exported key-value pair used by ConfigPairs for display.
type ConfigKV struct {
	Key   string
	Value string
}

// DefaultConfig returns the default configuration (delegates to defaultConfig).
func DefaultConfig() SinCodeConfig { return defaultConfig() }

// ConfigDir returns the configuration directory path (delegates to configDir).
func ConfigDir() string { return configDir() }

// GetConfigValue returns the string value for a config key (delegates to getConfigValue).
func GetConfigValue(key string) (string, error) { return getConfigValue(key) }

// GetConfigValueFromCfg returns the value for a key from the given config struct
// (delegates to getConfigValueFrom).
func GetConfigValueFromCfg(key string, cfg SinCodeConfig) (string, error) {
	return getConfigValueFrom(key, cfg)
}

// SetConfigValue sets a config value in the user config file (delegates to setConfigValue).
func SetConfigValue(key, value string) error { return setConfigValue(key, value) }

// SetConfigValueInCfg sets a config value in the given config struct without saving
// (delegates to setConfigValueIn).
func SetConfigValueInCfg(key, value string, cfg *SinCodeConfig) error {
	return setConfigValueIn(key, value, cfg)
}

// ValidateConfig validates the given config and returns a list of issue strings
// (delegates to validateConfig).
func ValidateConfig(cfg SinCodeConfig) []string { return validateConfig(cfg) }

// MaskSecret masks a secret string for display (delegates to maskSecret).
func MaskSecret(s string) string { return maskSecret(s) }

// SaveConfig saves the config to the user config path (delegates to saveConfig).
func SaveConfig(cfg SinCodeConfig) error { return saveConfig(cfg) }

// ConfigPairs returns all config key-value pairs, optionally masking secrets
// (delegates to configPairs, converting to exported ConfigKV).
func ConfigPairs(cfg SinCodeConfig, mask bool) []ConfigKV {
	pairs := configPairs(cfg, mask)
	out := make([]ConfigKV, len(pairs))
	for i, p := range pairs {
		out[i] = ConfigKV{Key: p.Key, Value: p.Value}
	}
	return out
}

// LoadConfigFromPath loads a config from a specific file path
// (delegates to loadConfigFrom, returning only the Cfg field).
func LoadConfigFromPath(path string) (SinCodeConfig, error) {
	cfr, err := loadConfigFrom(path)
	return cfr.Cfg, err
}
