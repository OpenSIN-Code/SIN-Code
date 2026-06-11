// SPDX-License-Identifier: MIT
// Purpose: Shared test helper to skip permission-denial tests in
// environments where discretionary file permissions are not enforced
// (root, containers/sandboxes with CAP_DAC_OVERRIDE, some CI runners).

//go:build !windows

package internal

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var (
	permProbeOnce sync.Once
	permEnforced  bool
)

// skipIfRoot skips tests that rely on file permissions being enforced.
// Statt euid zu prüfen (ungenügend: CAP_DAC_OVERRIDE kann Berechtigungen
// selbst für Nicht-Root-Benutzer umgehen), wird das tatsächliche Verhalten
// einmal per Probe ermittelt: Datei mit Mode 0000 erstellen und versuchen,
// sie zu öffnen.
func skipIfRoot(t *testing.T) {
	t.Helper()
	permProbeOnce.Do(func() {
		dir, err := os.MkdirTemp("", "permprobe")
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)
		probe := filepath.Join(dir, "probe")
		if err := os.WriteFile(probe, []byte("x"), 0000); err != nil {
			return
		}
		f, err := os.Open(probe)
		if err != nil {
			permEnforced = true
			return
		}
		f.Close()
	})
	if !permEnforced {
		t.Skip("file permissions are not enforced in this environment (root or CAP_DAC_OVERRIDE)")
	}
}
