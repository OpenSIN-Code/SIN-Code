// SPDX-License-Identifier: MIT
// Purpose: version/build info and background update-check logic.
// Split from main.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

var Version = internal.Version // Re-export from internal/version.go; set at build time via -ldflags

// checkUpdateFn is the network probe used by checkUpdate. It is a package
// variable so tests can stub it out and stay fully hermetic (no GitHub calls).
var checkUpdateFn = internal.CheckUpdateAvailable

// updateCheckDisabled reports whether the background update check is
// disabled via environment:
//   - SIN_CODE_NO_UPDATE_CHECK / NO_UPDATE_CHECK: explicit user opt-out
//   - SIN_CODE_OFFLINE: generic offline switch
func updateCheckDisabled() bool {
	for _, key := range []string{"SIN_CODE_NO_UPDATE_CHECK", "NO_UPDATE_CHECK", "SIN_CODE_OFFLINE"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

func checkUpdate() {
	// Only run when invoked with no args or --version/-v.
	if len(os.Args) > 1 {
		first := os.Args[1]
		if first != "--version" && first != "-v" {
			return
		}
	}

	if updateCheckDisabled() {
		return
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	stampDir := filepath.Join(configDir, "sin")
	stampPath := filepath.Join(stampDir, ".last-update-check")

	if info, err := os.Stat(stampPath); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return
		}
	}

	// Touch the stamp file immediately so repeated invocations don't hammer GitHub.
	os.MkdirAll(stampDir, 0755)
	os.WriteFile(stampPath, []byte(time.Now().Format(time.RFC3339)), filemode.Default())

	// Query GitHub with a short timeout so the CLI stays responsive.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type result struct {
		version string
		has     bool
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		v, h, e := checkUpdateFn()
		ch <- result{v, h, e}
	}()

	select {
	case <-ctx.Done():
		return
	case res := <-ch:
		if res.err != nil || !res.has {
			return
		}
		fmt.Printf("\n🔄 A new version of sin-code is available: %s → %s\n", Version, res.version)
		fmt.Println("   Run 'sin-code self-update' to install.")
	}
}
