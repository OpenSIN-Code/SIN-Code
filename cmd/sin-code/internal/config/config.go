// SPDX-License-Identifier: MIT
// Purpose: thin re-export package around the canonical internal.SinCodeConfig
// so that downstream callers (e.g. cmd/sin-code/fusion_cmd.go, which is
// package main in cmd/sin-code/) can refer to the same configuration via the
// `cmd/sin-code/internal/config` import path. The single source of truth
// remains cmd/sin-code/internal/config.go (package internal); this aliasing
// is purely a path convenience.
package config

import (
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
)

// SinCodeConfig is the canonical user + project merged configuration shape.
// Aliased to internal.SinCodeConfig so the two import paths expose the same
// type and field set — toggling defaultConfig / getConfigValue / etc. stays
// in one place.
type SinCodeConfig = internal.SinCodeConfig

// ConfigKV is an exported key-value pair for config display.
type ConfigKV = internal.ConfigKV

// LoadMergedConfig delegates to internal.LoadMergedConfig.
func LoadMergedConfig() (SinCodeConfig, error) {
	return internal.LoadMergedConfig()
}

// LoadFromFile loads a config from a specific file path.
func LoadFromFile(path string) (SinCodeConfig, error) {
	return internal.LoadConfigFromPath(path)
}

// Get returns the string value for a config key (loads from user config file).
func Get(key string) (string, error) {
	return internal.GetConfigValue(key)
}

// GetFrom returns the value for a key from the given config struct (no file I/O).
func GetFrom(key string, cfg SinCodeConfig) (string, error) {
	return internal.GetConfigValueFromCfg(key, cfg)
}

// Set sets a config value in the user config file (persists to disk).
func Set(key, value string) error {
	return internal.SetConfigValue(key, value)
}

// SetIn sets a config value in the given config struct without saving.
func SetIn(key, value string, cfg *SinCodeConfig) error {
	return internal.SetConfigValueInCfg(key, value, cfg)
}

// Validate validates the given config and returns a list of issue strings.
func Validate(cfg SinCodeConfig) []string {
	return internal.ValidateConfig(cfg)
}

// Path returns the configuration directory path.
func Path() string {
	return internal.ConfigDir()
}

// Default returns the default configuration.
func Default() SinCodeConfig {
	return internal.DefaultConfig()
}

// MaskSecret masks a secret string for display.
func MaskSecret(s string) string {
	return internal.MaskSecret(s)
}

// Pairs returns all config key-value pairs, optionally masking secrets.
func Pairs(cfg SinCodeConfig, mask bool) []ConfigKV {
	return internal.ConfigPairs(cfg, mask)
}

// Save saves the config to the user config path.
func Save(cfg SinCodeConfig) error {
	return internal.SaveConfig(cfg)
}
