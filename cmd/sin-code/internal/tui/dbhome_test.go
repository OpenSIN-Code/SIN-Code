// SPDX-License-Identifier: MIT
// Purpose: tests for dbhome.go (issue #62 / #265). The home resolution
// must be hermetic, byte-stable, and workspace-collision-safe.
package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDBHome_UsesDBHome(t *testing.T) {
	cfg := Config{Workspace: t.TempDir(), DBHome: filepath.Join(t.TempDir(), "explicit")}
	got, err := ResolveDBHome(cfg)
	if err != nil {
		t.Fatalf("ResolveDBHome: %v", err)
	}
	if got.Home != cfg.DBHome {
		t.Errorf("Home=%q want %q", got.Home, cfg.DBHome)
	}
	if !strings.HasPrefix(got.WorkspaceKey, "000") && len(got.WorkspaceKey) != 12 {
		t.Errorf("WorkspaceKey should be 12 hex chars; got %q", got.WorkspaceKey)
	}
	if filepath.Dir(filepath.Dir(got.WorkspaceDir)) != cfg.DBHome {
		t.Errorf("WorkspaceDir=%q should be under Home=%q", got.WorkspaceDir, cfg.DBHome)
	}
	if got.SessionsPath() == "" || !strings.HasSuffix(got.SessionsPath(), "sessions.db") {
		t.Errorf("SessionsPath=%q should end with sessions.db", got.SessionsPath())
	}
	if got.LessonsPath() == "" || !strings.HasSuffix(got.LessonsPath(), "lessons.db") {
		t.Errorf("LessonsPath=%q should end with lessons.db", got.LessonsPath())
	}
}

func TestResolveDBHome_DefaultsToUserConfigDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "userCfg")
	old := userConfigDir
	userConfigDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userConfigDir = old })

	cfg := Config{Workspace: t.TempDir()} // DBHome empty
	got, err := ResolveDBHome(cfg)
	if err != nil {
		t.Fatalf("ResolveDBHome: %v", err)
	}
	wantHome := filepath.Join(home, "sin-code")
	if got.Home != wantHome {
		t.Errorf("Home=%q want %q", got.Home, wantHome)
	}
}

func TestResolveDBHome_StableAcrossConfigs(t *testing.T) {
	// Same workspace should map to the same WorkspaceKey regardless
	// of DBHome / UserConfigDir source.
	ws := t.TempDir()
	a, err := ResolveDBHome(Config{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ResolveDBHome(Config{Workspace: ws, DBHome: filepath.Join(t.TempDir(), "alt")})
	if err != nil {
		t.Fatal(err)
	}
	if a.WorkspaceKey != b.WorkspaceKey {
		t.Errorf("WorkspaceKey not stable: %q vs %q", a.WorkspaceKey, b.WorkspaceKey)
	}
}

func TestResolveDBHome_DifferentWorkspacesDifferentKeys(t *testing.T) {
	a, err := ResolveDBHome(Config{Workspace: t.TempDir(), DBHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ResolveDBHome(Config{Workspace: t.TempDir(), DBHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if a.WorkspaceKey == b.WorkspaceKey {
		t.Errorf("expected distinct keys; both %q", a.WorkspaceKey)
	}
}

func TestResolveDBHome_EmptyWorkspaceRejected(t *testing.T) {
	_, err := ResolveDBHome(Config{Workspace: ""})
	if err == nil {
		t.Error("expected error when Workspace is empty")
	}
}

func TestResolveDBHome_UserConfigDirError(t *testing.T) {
	old := userConfigDir
	userConfigDir = func() (string, error) { return "", os.ErrPermission }
	t.Cleanup(func() { userConfigDir = old })

	_, err := ResolveDBHome(Config{Workspace: t.TempDir()})
	if err == nil {
		t.Error("expected error from userConfigDir")
	}
}

func TestEnsureDBHome_CreatesDirs(t *testing.T) {
	home := t.TempDir()
	r := DBHomeResult{
		Home:         home,
		WorkspaceKey: "deadbeef0001",
		WorkspaceDir: filepath.Join(home, "workspaces", "deadbeef0001"),
	}
	if err := ensureDBHome(r); err != nil {
		t.Fatalf("ensureDBHome: %v", err)
	}
	if _, err := os.Stat(r.WorkspaceDir); err != nil {
		t.Errorf("WorkspaceDir should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "workspaces")); err != nil {
		t.Errorf("workspaces/ should exist: %v", err)
	}
}

func TestEnsureDBHome_EmptyRejects(t *testing.T) {
	err := ensureDBHome(DBHomeResult{})
	if err == nil {
		t.Error("expected error for empty WorkspaceDir")
	}
}

func TestShortSHA256_Deterministic(t *testing.T) {
	a := shortSHA256("/Users/x/dev/foo")
	b := shortSHA256("/Users/x/dev/foo")
	if a != b {
		t.Errorf("not byte-stable: %q vs %q", a, b)
	}
	if len(a) != 12 {
		t.Errorf("expected 12 hex chars; got %q (len=%d)", a, len(a))
	}
}

func TestShortSHA256_DifferentInputsDifferentKeys(t *testing.T) {
	a := shortSHA256("/Users/x/dev/foo")
	b := shortSHA256("/Users/x/dev/bar")
	if a == b {
		t.Errorf("collision: %q == %q", a, b)
	}
}

func TestShortSHA256_Lowercases(t *testing.T) {
	// macOS HFS+/APFS are case-insensitive by default — normalize.
	upper := shortSHA256("/Users/X/Dev/Foo")
	lower := shortSHA256("/users/x/dev/foo")
	if upper != lower {
		t.Errorf("case-folding required: upper=%q lower=%q", upper, lower)
	}
}

// TestShortSHA256_AlgorithmicReference matches an explicit reference
// computation so future refactors that change the digest don't drift
// silently.
func TestShortSHA256_AlgorithmicReference(t *testing.T) {
	input := "/tmp/abc"
	sum := sha256.Sum256([]byte(strings.ToLower(input)))
	want := hex.EncodeToString(sum[:])[:12]
	got := shortSHA256(input)
	if got != want {
		t.Errorf("shortSHA256(%q)=%q want %q", input, got, want)
	}
}
