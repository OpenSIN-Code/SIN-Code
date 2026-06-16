// SPDX-License-Identifier: MIT
// Purpose: git-worktree isolation primitives. Wraps `git worktree add /
// list / remove / lock` with a strict, race-clean surface so callers
// never accidentally write outside the repo or lose uncommitted work.
//
// Complements `claude --worktree <name>` (Anthropic Claude Code v2.0+)
// and the per-subagent `isolation: worktree` frontmatter. Used by issue
// #194 part 2 (worktree-from-flag) and feeds into #1 of the v3.20.0
// roadmap (worktree-isolated parallel agents).
package isolation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Info describes a single git worktree as returned by `git worktree list
// --porcelain`.
type Info struct {
	Path      string // absolute on-disk path
	Branch    string // refs/heads/<name> or "(detached)"
	Commit    string // HEAD sha
	Locked    bool   // true if `git worktree lock` is set
	IsCurrent bool
	IsBare    bool
}

// ErrNotARepo is returned when the supplied path is not a git work tree.
type ErrNotARepo struct{ Path string }

func (e ErrNotARepo) Error() string {
	return fmt.Sprintf("isolation: %q is not a git repository", e.Path)
}

// ErrInvalidName is returned when the worktree name is empty, contains
// path-traversal characters, or otherwise rejects the strict whitelist
// below.
type ErrInvalidName struct{ Name string }

func (e ErrInvalidName) Error() string {
	return fmt.Sprintf("isolation: invalid worktree name %q", e.Name)
}

// ErrAlreadyExists is returned when the target worktree path is already
// checked out by another worktree.
type ErrAlreadyExists struct{ Path string }

func (e ErrAlreadyExists) Error() string {
	return fmt.Sprintf("isolation: worktree path %q already in use", e.Path)
}

// ErrRefusal is returned when an operation is refused because of dirty
// state (uncommitted / untracked / unpushed changes) that would be lost.
type ErrRefusal struct {
	Path   string
	Reason string
}

func (e ErrRefusal) Error() string {
	return fmt.Sprintf("isolation: refusing operation on %q: %s", e.Path, e.Reason)
}

// repoRoot resolves the toplevel of the git work tree at `cwd`. It
// fails with ErrNotARepo if `cwd` is not inside a git repo.
func repoRoot(cwd string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", ErrNotARepo{Path: cwd}
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", ErrNotARepo{Path: cwd}
	}
	return root, nil
}

// sanitizeName enforces a strict whitelist on worktree branch names:
// no path traversal, no slashes, no leading dot, no NUL. It is mirrored
// by git's own check_ref_format rules.
func sanitizeName(name string) error {
	if name == "" {
		return ErrInvalidName{Name: name}
	}
	if name == "." || name == ".." {
		return ErrInvalidName{Name: name}
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "-") {
		return ErrInvalidName{Name: name}
	}
	for _, r := range name {
		if r == 0 || r == '/' || r == '\\' || r == ':' || r < 0x20 || r == 0x7f {
			return ErrInvalidName{Name: name}
		}
	}
	return nil
}

// Create provisions a new git worktree at `<repoRoot>/.sin-code/
// worktrees/<name>` based on the repo's current HEAD. The new branch
// `worktree-<name>` is created from HEAD. Returns the absolute
// worktree-on-disk path on success.
//
// The worktree is automatically registered with `git worktree add` so
// it shows up in `git worktree list` and can be cleaned up with
// `git worktree remove`. The on-disk path uses the .sin-code prefix so
// `cmd/sin-code/.gitignore` keeps it out of git's view (mandate M3 —
// isolation metadata must never leak into the user's repo).
func Create(repoRootDir, name string) (string, error) {
	if err := sanitizeName(name); err != nil {
		return "", err
	}
	root, err := repoRoot(repoRootDir)
	if err != nil {
		return "", err
	}
	wtDir := filepath.Join(root, ".sin-code", "worktrees", name)
	if _, err := os.Stat(wtDir); err == nil {
		return "", ErrAlreadyExists{Path: wtDir}
	}
	if err := os.MkdirAll(filepath.Dir(wtDir), 0o755); err != nil {
		return "", fmt.Errorf("isolation: mkdir parent: %w", err)
	}
	branch := "worktree-" + name
	cmd := exec.Command("git", "worktree", "add", "-b", branch, wtDir, "HEAD")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("isolation: git worktree add: %s: %w",
			strings.TrimSpace(string(out)), err)
	}
	// Lock so `git worktree remove --force` cannot race a cleanup
	// timer that another process is running.
	if err := Lock(root, wtDir); err != nil {
		return wtDir, fmt.Errorf("isolation: worktree provisioned but lock failed: %w", err)
	}
	return wtDir, nil
}

// List returns every worktree registered in the repo rooted at `cwd`.
func List(cwd string) ([]Info, error) {
	root, err := repoRoot(cwd)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("isolation: git worktree list: %w", err)
	}
	return parseWorktreePorcelain(strings.TrimRight(string(out), "\n"))
}

func parseWorktreePorcelain(out string) ([]Info, error) {
	var out2 []Info
	var cur Info
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur = Info{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "HEAD "):
			cur.Commit = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(line, "branch ")
		case line == "locked":
			cur.Locked = true
		case line == "detached":
			cur.Branch = "(detached)"
		case line == "bare":
			cur.IsBare = true
		case line == "":
			if cur.Path != "" {
				out2 = append(out2, cur)
				cur = Info{}
			}
		}
	}
	if cur.Path != "" {
		out2 = append(out2, cur)
	}
	return out2, nil
}

// Remove deletes the worktree at `worktreePath` from the repo rooted
// at `cwd`. It refuses with ErrRefusal if the worktree has uncommitted
// or untracked changes; pass force=true to bypass (e.g. abandon).
//
// A worktree that is locked (Create() auto-locks while the agent runs)
// is refused when force=false; force=true passes `--force --force` to
// `git worktree remove`, which overrides both dirty + lock state.
func Remove(cwd, worktreePath string, force bool) error {
	root, err := repoRoot(cwd)
	if err != nil {
		return err
	}
	if equal, err := samePath(worktreePath, root); err == nil && equal {
		return ErrRefusal{Path: worktreePath, Reason: "refuses to remove main worktree"}
	} else if err != nil {
		// best-effort fall back to lexical equality if EvalSymlinks fails
		if filepath.Clean(worktreePath) == filepath.Clean(root) {
			return ErrRefusal{Path: worktreePath, Reason: "refuses to remove main worktree"}
		}
	}
	if !force {
		// Check locked state via List — git worktree remove without
		// --force --force refuses a locked entry regardless of dirt.
		if infos, lerr := List(root); lerr == nil {
			for _, i := range infos {
				if i.Path == worktreePath && i.Locked {
					return ErrRefusal{
						Path:   worktreePath,
						Reason: "worktree is locked (pass force=true)",
					}
				}
			}
		}
		dirty, why, err := HasUncommitted(worktreePath)
		if err != nil {
			return err
		}
		if dirty {
			return ErrRefusal{Path: worktreePath, Reason: why}
		}
	}
	cmd := exec.Command("git", "worktree", "remove")
	if force {
		// `-f -f` overrides both dirty and locked state.
		cmd.Args = append(cmd.Args, "--force", "--force")
	}
	cmd.Args = append(cmd.Args, worktreePath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("isolation: git worktree remove: %s: %w",
			strings.TrimSpace(string(out)), err)
	}
	// Best-effort cleanup of the .sin-code parent dir if empty.
	_ = os.Remove(filepath.Join(root, ".sin-code", "worktrees",
		filepath.Base(worktreePath)))
	return nil
}

// samePath returns true if `a` and `b` refer to the same on-disk
// directory after EvalSymlinks (handles macOS's /var → /private/var
// alias which would otherwise defeat string-comparison).
func samePath(a, b string) (bool, error) {
	ca, err := filepath.EvalSymlinks(a)
	if err != nil {
		ca = filepath.Clean(a)
	}
	cb, err := filepath.EvalSymlinks(b)
	if err != nil {
		cb = filepath.Clean(b)
	}
	return ca == cb, nil
}

// Lock acquires the per-worktree lock `git worktree lock` recognises.
// Use to prevent cleanup timers from removing a worktree that is still
// in use (mirrors Claude Code's behaviour).
func Lock(cwd, worktreePath string) error {
	root, err := repoRoot(cwd)
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "worktree", "lock", worktreePath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("isolation: git worktree lock: %s: %w",
			strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Unlock reverses Lock.
func Unlock(cwd, worktreePath string) error {
	root, err := repoRoot(cwd)
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "worktree", "unlock", worktreePath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("isolation: git worktree unlock: %s: %w",
			strings.TrimSpace(string(out)), err)
	}
	return nil
}

// HasUncommitted returns (true, reason, nil) if `path` has any
// uncommitted, untracked, or staged-but-not-yet-committed changes; it
// returns (false, "", nil) when the tree is clean. Errors propagate
// from git itself.
func HasUncommitted(path string) (bool, string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", fmt.Errorf("isolation: git status: %w", err)
	}
	raw := strings.TrimRight(string(out), "\n")
	if raw == "" {
		return false, "", nil
	}
	return true, "worktree has modifications: " + firstLine(raw), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
