// SPDX-License-Identifier: MIT
// Purpose: project identity from git remote — stable across clones and
// CI checkouts. The hash is the dedup key for cross-project promotion.
// Docs: project.doc.md
package instinct

import (
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Project identifies the repo an instinct belongs to.
type Project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Remote string `json:"remote,omitempty"`
	Path   string `json:"path,omitempty"`
}

var credRe = regexp.MustCompile(`://[^@/]+@`)

// DetectProject resolves the current project from a git remote (preferred)
// or the working-directory path as a fallback. Stable across clones.
func DetectProject(dir string) Project {
	if dir == "" {
		dir = "."
	}
	abs, _ := filepath.Abs(dir)

	if remote := gitRemote(dir); remote != "" {
		norm := normalizeRemote(remote)
		return Project{
			ID:     hash12(norm),
			Name:   repoNameFromRemote(norm),
			Remote: norm,
			Path:   abs,
		}
	}
	top := gitToplevel(dir)
	if top == "" {
		top = abs
	}
	return Project{
		ID:   hash12(top),
		Name: filepath.Base(top),
		Path: top,
	}
}

func gitRemote(dir string) string {
	out, err := runGit(dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func gitToplevel(dir string) string {
	out, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.Output()
	return string(b), err
}

// normalizeRemote strips credentials and trailing .git so SSH/HTTPS of
// the same repo collapse to one identity.
func normalizeRemote(remote string) string {
	r := strings.TrimSpace(remote)
	r = credRe.ReplaceAllString(r, "://")
	if strings.HasPrefix(r, "git@") {
		r = strings.TrimPrefix(r, "git@")
		r = strings.Replace(r, ":", "/", 1)
	}
	r = strings.TrimPrefix(r, "https://")
	r = strings.TrimPrefix(r, "http://")
	r = strings.TrimPrefix(r, "ssh://")
	r = strings.TrimSuffix(r, ".git")
	r = strings.TrimSuffix(r, "/")
	return strings.ToLower(r)
}

func repoNameFromRemote(norm string) string {
	parts := strings.Split(norm, "/")
	if len(parts) == 0 {
		return norm
	}
	return parts[len(parts)-1]
}

func hash12(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:12]
}
