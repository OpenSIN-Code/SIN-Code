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

// LoadMergedConfig delegates to internal.LoadMergedConfig.
func LoadMergedConfig() (SinCodeConfig, error) {
	return internal.LoadMergedConfig()
}
