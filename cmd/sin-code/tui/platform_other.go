// SPDX-License-Identifier: MIT
//go:build !darwin && !linux && !windows

package tui

func setupPlatformGuard() *PlatformGuard {
	return &PlatformGuard{}
}
