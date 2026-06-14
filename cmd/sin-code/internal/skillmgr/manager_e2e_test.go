// SPDX-License-Identifier: MIT
// Purpose: E2E integration test that installs the websearch skill from GitHub
// and verifies the built binary is runnable. Run with: go test -tags=e2e ./...
//go:build e2e

package skillmgr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func listDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{"<error: " + err.Error() + ">"}
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestInstallWebsearchFromGitHub clones web_search_bundle via the skill manager
// and verifies the sin-websearch binary is built and runnable. This is a real
// network test and therefore gated behind the e2e build tag.
func TestInstallWebsearchFromGitHub(t *testing.T) {
	skillsDir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", skillsDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	st, err := Install(ctx, "websearch")
	if err != nil {
		t.Fatalf("install websearch: %v", err)
	}
	if !st.Installed {
		t.Fatalf("websearch not installed")
	}
	t.Logf("websearch status: installed=%v runnable=%v detail=%q", st.Installed, st.Runnable, st.Detail)
	if !st.Runnable {
		t.Fatalf("websearch not runnable: %s", st.Detail)
	}

	bin := filepath.Join(skillsDir, "web_search_bundle", "sin-websearch")
	if _, err := os.Stat(bin); err != nil {
		t.Logf("skills dir contents: %v", listDir(filepath.Join(skillsDir, "web_search_bundle")))
		t.Fatalf("binary not found at %s: %v", bin, err)
	}

	out, err := exec.CommandContext(ctx, bin, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("binary --help failed: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Fatalf("binary --help produced no output")
	}
}
