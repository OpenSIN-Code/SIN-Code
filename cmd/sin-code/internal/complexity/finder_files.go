// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when complexity analyzer is refactored
package complexity

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// selectSkipDirs are directory base names that never contain first-party
// source files. They are build artifacts, test fixtures, VCS metadata, or
// vendored dependencies.
var selectSkipDirs = map[string]bool{
	"testdata":      true,
	"build":         true,
	"node_modules":  true,
	".git":          true,
	"vendor":        true,
	"dist":          true,
	"target":        true,
	"out":           true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".pytest_cache": true,
	".mypy_cache":   true,
}

// selectFiles returns the .go files to analyze. If sinceRef is set, it analyzes
// every .go file in directories touched by the diff.
func selectFiles(root, sinceRef string) ([]string, error) {
	if sinceRef != "" {
		cmd := exec.Command("git", "diff", "--name-only", sinceRef)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git diff --name-only %s: %w", sinceRef, err)
		}
		dirs := make(map[string]struct{})
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" || !strings.HasSuffix(line, ".go") {
				continue
			}
			dirs[filepath.Dir(line)] = struct{}{}
		}
		var files []string
		for dir := range dirs {
			entries, err := os.ReadDir(filepath.Join(root, dir))
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				n := e.Name()
				if strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
					files = append(files, filepath.Join(root, dir, n))
				}
			}
		}
		return files, nil
	}

	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if selectSkipDirs[name] {
				return filepath.SkipDir
			}
			if path != root {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") {
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return files, nil
}

func groupByDir(files []string) map[string][]string {
	out := make(map[string][]string)
	for _, f := range files {
		dir := filepath.Dir(f)
		out[dir] = append(out[dir], f)
	}
	return out
}
