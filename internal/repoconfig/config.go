// SPDX-License-Identifier: MIT
// Purpose: per-repository loop configuration loaded from .sin-code.yml. Lets
// each repo commit its own budgets, verify mode, and disabled checks without
// touching global defaults. Missing file = zero-value = use defaults.
package repoconfig

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config mirrors .sin-code.yml. All fields optional; zero means "use default".
type Config struct {
	MaxTurns       int      `yaml:"max_turns"`
	MaxStopRejects int      `yaml:"max_stop_rejects"`
	StallThreshold int      `yaml:"stall_threshold"`
	MaxTokens      int      `yaml:"max_tokens"`
	VerifyMode     string   `yaml:"verify_mode"`    // poc|oracle|off
	DisableChecks  []string `yaml:"disable_checks"` // check names to skip, e.g. ["go vet"]
}

// FileName is the conventional per-repo config filename.
const FileName = ".sin-code.yml"

// Load reads .sin-code.yml from workspace. A missing file returns a zero
// Config and nil error — callers treat zero fields as "keep default".
func Load(workspace string) (Config, error) {
	var c Config
	b, err := os.ReadFile(filepath.Join(workspace, FileName))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, err
	}
	return c, nil
}

// IsCheckDisabled reports whether a named check is turned off for this repo.
func (c Config) IsCheckDisabled(name string) bool {
	for _, d := range c.DisableChecks {
		if d == name {
			return true
		}
	}
	return false
}
