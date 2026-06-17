// SPDX-License-Identifier: MIT
package tui

import "testing"

func TestPlatformGuardSetup(t *testing.T) {
	guard := SetupPlatformGuard()
	if guard == nil {
		t.Fatal("expected non-nil guard")
	}
	guard.Cleanup()
	guard.Cleanup()
}

func TestPlatformGuardNilCleanup(t *testing.T) {
	var g *PlatformGuard
	g.Cleanup()
}

func TestPlatformGuardTypeSafety(t *testing.T) {
	guard := SetupPlatformGuard()
	if guard == nil {
		t.Fatal("SetupPlatformGuard returned nil")
	}
	guard.Cleanup()
	guard.Cleanup()
	var nilGuard *PlatformGuard
	nilGuard.Cleanup()
}
