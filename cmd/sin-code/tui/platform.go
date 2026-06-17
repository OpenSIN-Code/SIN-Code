// SPDX-License-Identifier: MIT
package tui

// PlatformGuard sets up platform-specific signal handling before the TUI
// starts. On Windows, this installs a Ctrl+C handler that restores the
// terminal before exiting. On Unix, it is a no-op (Ctrl+C is handled by
// Bubbletea's tea.Quit).
type PlatformGuard struct {
	cleanup func()
}

// SetupPlatformGuard initializes the platform-specific guard.
// Returns a guard whose Cleanup method must be called on exit.
func SetupPlatformGuard() *PlatformGuard {
	return setupPlatformGuard()
}

// Cleanup restores the terminal state. Safe to call multiple times
// and on a nil guard.
func (g *PlatformGuard) Cleanup() {
	if g != nil && g.cleanup != nil {
		g.cleanup()
		g.cleanup = nil
	}
}
