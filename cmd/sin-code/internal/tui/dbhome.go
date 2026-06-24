// SPDX-License-Identifier: MIT
// Purpose: Resolve the runtime-DB home directory. The legacy TUI agent
// wrote its lessons.db and sessions.db directly inside the workspace
// at "<workspace>/.sin-code/" — a recipe for accidental commits. The
// proper home is os.UserConfigDir()/sin-code/workspaces/<ws-hash>/ so:
//
//   - Two projects do not collide on the same lessons/sessions row.
//   - SQLite files are never inside a git working tree.
//   - CI / packs / home dir backups pick them up automatically.
//
// Tests override Config.DBHome; production callers set it to "" and
// the function composes UserConfigDir() + a stable sha256-prefix12 of
// absWorkspace(Workspace).
//
// Issue #62 / #265.
package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sin-debt: delete, upgrade: remove when test no longer needs this override
// userConfigDir is a package-level hook for os.UserConfigDir so tests
// can hermetic-mock the platform-specific user directory.
var userConfigDir = os.UserConfigDir

// DBHomeResult is the resolved directory layout for one runtime DB
// home. Both Join methods are pure filepath operations; they do not
// touch the filesystem so they are safe for hot-path callers.
type DBHomeResult struct {
	// Home is the parent directory that holds "workspaces/<key>/…".
	Home string
	// WorkspaceKey is the 12-char hex hash of absWorkspace(Workspace).
	WorkspaceKey string
	// WorkspaceDir is Home/WorkspaceKey. Sessions and lessons live
	// inside this directory and nowhere else.
	WorkspaceDir string
}

// ResolveDBHome returns the runtime DB home for the given config.
//
//	cfg.DBHome == ""  →  os.UserConfigDir()/sin-code
//	cfg.DBHome != ""  →  cfg.DBHome (used by tests and portable setups)
//
// WorkspaceKey is a sha256-prefix12 of absWorkspace(cfg.Workspace).
// Two workspaces with the same absolute path collapse to the same
// key, which is the property callers want — "same project = same DB".
func ResolveDBHome(cfg Config) (DBHomeResult, error) {
	home := cfg.DBHome
	if home == "" {
		base, err := userConfigDir()
		if err != nil {
			return DBHomeResult{}, fmt.Errorf("tui: resolve user config dir: %w", err)
		}
		home = filepath.Join(base, "sin-code")
	}

	ws := cfg.Workspace
	if ws == "" {
		return DBHomeResult{}, errors.New("tui: ResolveDBHome: Config.Workspace is empty")
	}
	abs, err := filepath.Abs(ws)
	if err != nil {
		return DBHomeResult{}, fmt.Errorf("tui: abs workspace: %w", err)
	}
	key := shortSHA256(abs)

	out := DBHomeResult{
		Home:         home,
		WorkspaceKey: key,
		WorkspaceDir: filepath.Join(home, "workspaces", key),
	}
	return out, nil
}

// SessionsPath returns the canonical path to sessions.db for the
// resolved DB home. Empty string when the resolution failed — callers
// should treat that as a hard error.
func (r DBHomeResult) SessionsPath() string {
	if r.WorkspaceDir == "" {
		return ""
	}
	return filepath.Join(r.WorkspaceDir, "sessions.db")
}

// LessonsPath returns the canonical path to lessons.db for the
// resolved DB home. Empty string when the resolution failed.
func (r DBHomeResult) LessonsPath() string {
	if r.WorkspaceDir == "" {
		return ""
	}
	return filepath.Join(r.WorkspaceDir, "lessons.db")
}

// shortSHA256 returns the first 12 hex chars of SHA-256(input).
// Collision risk: ~4e9 workspaces before 1% birthday probability.
func shortSHA256(input string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(input)))
	return hex.EncodeToString(sum[:])[:12]
}

// ensureDBHome creates the workspace directory under Home if missing.
// Permission is 0o755 (matches DefaultPath for the lessons + session
// packages so umask + chmod line up).
func ensureDBHome(r DBHomeResult) error {
	if r.WorkspaceDir == "" {
		return errors.New("tui: ensureDBHome: empty WorkspaceDir")
	}
	if err := os.MkdirAll(filepath.Dir(r.WorkspaceDir), 0o755); err != nil {
		return fmt.Errorf("tui: mkdir home: %w", err)
	}
	if err := os.MkdirAll(r.WorkspaceDir, 0o755); err != nil {
		return fmt.Errorf("tui: mkdir workspace dir: %w", err)
	}
	return nil
}
