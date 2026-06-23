// SPDX-License-Identifier: MIT
// Purpose: helpers (paths, mkdir) for notifications package.
package notifications

import (
	"os"
	"path/filepath"
)

func defaultConfigDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "sin-code"), nil
}

func dirOf(p string) string {
	return filepath.Dir(p)
}

func mkdirAll(p string, perm os.FileMode) error {
	return os.MkdirAll(p, perm)
}
