// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when testgen is refactored
package testgen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

func hasGotests() bool {
	_, err := exec.LookPath("gotests")
	return err == nil
}

func runGotests(ctx context.Context, file, outFile string) error {
	cmd := exec.CommandContext(ctx, "gotests", "-all", "-w", file)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return prependMarkerIfMissing(deriveTestFileName(file))
}

func runGotestsPackage(ctx context.Context, pkg string) (string, error) {
	cmd := exec.CommandContext(ctx, "gotests", "-all", "-w", pkg)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runGoTest(ctx context.Context, pkg string) (string, bool) {
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-timeout=60s", pkg)
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

func deriveTestFileName(file string) string {
	if strings.HasSuffix(file, "_test.go") {
		return file
	}
	return strings.TrimSuffix(file, ".go") + "_test.go"
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func prependMarkerIfMissing(p string) error {
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), GeneratedMarker) {
		return nil
	}
	out := []byte(GeneratedMarker + "\n\n" + string(data))
	return os.WriteFile(p, out, filemode.Default())
}

func detectGeneratedFiles(pkg string) []string {
	var out []string
	err := filepath.WalkDir(pkg, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), GeneratedMarker) {
			out = append(out, path)
		}
		return nil
	})
	_ = err
	return out
}

func findGoFiles(pkg string) ([]string, error) {
	var files []string
	dir := pkg
	if strings.HasSuffix(dir, "/...") {
		dir = strings.TrimSuffix(dir, "/...")
	}
	if dir == "" || dir == "./" || dir == "." {
		dir = "."
	}
	recursive := strings.HasSuffix(pkg, "/...")

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
