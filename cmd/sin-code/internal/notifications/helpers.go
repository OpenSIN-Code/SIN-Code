// SPDX-License-Identifier: MIT
// Purpose: helpers (paths, mkdir) for notifications package.
package notifications

import (
	"os"
	"path/filepath"
)

var (
	// testHookUserConfigDir lets tests inject config-dir failures.
	testHookUserConfigDir = os.UserConfigDir
)

func defaultConfigDir() (string, error) {
	cfg, err := testHookUserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "sin-code"), nil
}

// sin-debt: shrink, upgrade: inline when callers are consolidated
func dirOf(p string) string {
	return filepath.Dir(p)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated
func mkdirAll(p string, perm os.FileMode) error {
	return os.MkdirAll(p, perm)
}
