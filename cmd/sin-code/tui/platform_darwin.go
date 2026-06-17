// SPDX-License-Identifier: MIT
//go:build darwin

package tui

func setupPlatformGuard() *PlatformGuard {
	return &PlatformGuard{}
}
