// SPDX-License-Identifier: MIT
//go:build linux

package tui

func setupPlatformGuard() *PlatformGuard {
	return &PlatformGuard{
		cleanup: func() {},
	}
}
