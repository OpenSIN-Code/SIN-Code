// SPDX-License-Identifier: MIT
//go:build !darwin && !linux && !windows

package tui
// sin-debt: shrink, upgrade: when a second other-clipboard function is needed, merge into a shared file

// CopyToClipboard is a no-op on unsupported platforms.
func CopyToClipboard(text string) error {
	return nil
}
