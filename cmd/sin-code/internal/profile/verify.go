// SPDX-License-Identifier: MIT
// Verify gate for the profile renderer. The renderer is byte-stable
// per (target, source bytes) pair; this package exposes the hash
// function so CI (`sin-code profile verify`) can refuse to merge
// whenever a per-agent mirror drifts off the source.
package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// HashSource returns the canonical SHA-256 of the rendered output for
// `tgt` against `body`. The hash is bit-identical across machines for
// the same (tgt, body) pair; CI depends on this property.
func HashSource(tgt Target, body string) (string, error) {
	rendered, err := Render(tgt, body)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(rendered))
	return hex.EncodeToString(sum[:]), nil
}

// Result is the outcome of one Verify invocation.
type Result struct {
	Target  Target
	Path    string
	Found   bool   // file exists on disk
	Match   bool   // file exists AND contents match the expected render
	WantSHA string // expected SHA-256 of rendered output
	GotSHA  string // actual SHA-256 of disk content (empty if missing)
}

// Verify compares the on-disk files for every target with the
// expected Render(tgt, body) bytes. Sources for `body` is the same
// bytes the writer would emit for `tgt`.
//
// All targets are checked even after the first mismatch (the loop is
// fast and binary reporters love full tables). Returns nil if every
// match row is true; returns a *DriftError otherwise.
//
// `base` is the repo root (writer's cwd). Tests may pass t.TempDir().
// `body` is the source markdown (already loaded by the caller).
func Verify(base, body string) ([]Result, error) {
	out := make([]Result, 0, len(Targets))
	anyDrift := false
	for _, name := range TargetNames() {
		tgt := Targets[name]
		resolved, err := Resolve(tgt, base)
		if err != nil {
			return out, err
		}
		res := Result{Target: tgt, Path: resolved}

		rendered, err := Render(tgt, body)
		if err != nil {
			return out, err
		}
		want := sha256.Sum256([]byte(rendered))
		res.WantSHA = hex.EncodeToString(want[:])

		got, err := os.ReadFile(resolved)
		if errors.Is(err, os.ErrNotExist) {
			out = append(out, res)
			anyDrift = true
			continue
		}
		if err != nil {
			return out, fmt.Errorf("profile: read %q: %w", resolved, err)
		}
		res.Found = true
		g := sha256.Sum256(got)
		res.GotSHA = hex.EncodeToString(g[:])
		res.Match = res.WantSHA == res.GotSHA
		if !res.Match {
			anyDrift = true
		}
		out = append(out, res)
	}
	if anyDrift {
		return out, &DriftError{Results: out}
	}
	return out, nil
}

// DriftError is returned by Verify when at least one target's on-disk
// file does not match its expected Render(tgt, body) bytes. The caller
// (CLI) prints the table; tests assert on the populated Results slice.
type DriftError struct {
	Results []Result
}

func (e *DriftError) Error() string {
	var b strings.Builder
	b.WriteString("profile: per-agent mirrors are out of sync with the source:\n")
	for _, r := range e.Results {
		if r.Match {
			continue
		}
		switch {
		case !r.Found:
			fmt.Fprintf(&b, "  - %s (%s): MISSING %s\n", r.Target.Name, r.Target.Format, r.Path)
		default:
			fmt.Fprintf(&b, "  - %s (%s): DRIFT want_sha=%s got_sha=%s at %s\n",
				r.Target.Name, r.Target.Format, shortSHA(r.WantSHA), shortSHA(r.GotSHA), r.Path)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderAll computes the rendered bytes for every target (no IO).
// Returns a map keyed by Target.Name. Useful for `--dry-run` inspection
// and for tests that pin per-target golden output.
//
// RenderAll returns the sorted (alphabetical by target name) keys
// alongside the map so callers can iterate deterministically.
func RenderAll(body string) (map[string]string, []string, error) {
	out := make(map[string]string, len(Targets))
	keys := TargetNames()
	for _, name := range keys {
		rendered, err := Render(Targets[name], body)
		if err != nil {
			return out, keys, err
		}
		out[name] = rendered
	}
	return out, keys, nil
}

// List returns the sorted, human-readable target table the CLI prints
// for `profile list --json` and `profile --help`.
type ListEntry struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Format      string `json:"format"`
	InstallPath string `json:"install_path"`
}

// ListTable returns ListEntry rows in alphabetical order.
func ListTable() []ListEntry {
	names := TargetNames()
	out := make([]ListEntry, 0, len(names))
	for _, n := range names {
		t := Targets[n]
		out = append(out, ListEntry{
			Name:        t.Name,
			DisplayName: t.DisplayName,
			Format:      t.Format,
			InstallPath: strings.ReplaceAll(t.InstallPath, "<skill>", ProfileSkill),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// shortSHA returns the first 12 hex chars of a full sha256. Used in
// human-readable error output; full hex is still surfaced via
// Result.WantSHA / Result.GotSHA.
func shortSHA(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

// LoadSource reads the canonical source markdown. The path is resolved
// relative to `base` if not absolute. os.ErrNotExist is returned with
// no wrapping so callers can recognize it.
func LoadSource(base string) (string, error) {
	p := DefaultSourcePath
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("profile: source missing at %s (run from repo root)", p)
	}
	if err != nil {
		return "", fmt.Errorf("profile: read source %q: %w", p, err)
	}
	return string(data), nil
}

// WriteAll renders `body` for every registered target and writes each
// result to disk under `base`. Returns the list of paths that were
// written. Errors stop on the first failure (the binary is headless
// during CI — partial mirrors are worse than no mirrors).
func WriteAll(base, body string) ([]string, error) {
	keys := TargetNames()
	written := make([]string, 0, len(keys))
	for _, name := range keys {
		tgt := Targets[name]
		resolved, err := Resolve(tgt, base)
		if err != nil {
			return written, err
		}
		rendered, err := Render(tgt, body)
		if err != nil {
			return written, err
		}
		if err := atomicWrite(resolved, []byte(rendered)); err != nil {
			return written, fmt.Errorf("profile: write %s: %w", resolved, err)
		}
		written = append(written, resolved)
	}
	return written, nil
}

// WriteSelected renders `body` for the named target (or "all") and
// writes the result to disk under `base`.
func WriteSelected(base, body, name string) ([]string, error) {
	if name == "all" {
		return WriteAll(base, body)
	}
	tgt, ok := Targets[name]
	if !ok {
		return nil, fmt.Errorf("profile: unknown target %q (registered: %v)", name, TargetNames())
	}
	resolved, err := Resolve(tgt, base)
	if err != nil {
		return nil, err
	}
	rendered, err := Render(tgt, body)
	if err != nil {
		return nil, err
	}
	if err := atomicWrite(resolved, []byte(rendered)); err != nil {
		return nil, fmt.Errorf("profile: write %s: %w", resolved, err)
	}
	return []string{resolved}, nil
}

// atomicWrite writes `data` to `path` via temp-file rename so a partial
// rewrite can never be observed by the agent mid-write. Mirrors
// skilldist.atomicWrite (byte semantics). The parent directory is
// created with the same 0755 mode skilldist uses; permission drift
// here would surface as a "directory not found" error on CI.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sin-profile-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
