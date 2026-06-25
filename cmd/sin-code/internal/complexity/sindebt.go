// SPDX-License-Identifier: MIT
// Purpose: sin-debt marker parser — respects the "sin-debt:" annotation format (issue #177).
package complexity

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Marker is a single "sin-debt:" annotation.
type Marker struct {
	Path   string
	Line   int
	Reason string
}

var sinDebtRE = regexp.MustCompile(`(?:^|\s)(?://|#)\s*sin-debt:\s*(?P<reason>[^,\n\r]+?)(?:\s*,\s*upgrade:\s*(?P<upgrade>.+?))?\s*$`)

// skipDirs are directory base names that never contain source-level sin-debt
// markers. They are build artifacts, vendored modules, or VCS metadata.
var skipDirs = map[string]bool{
	".git":          true,
	"node_modules":  true,
	"vendor":        true,
	"build":         true,
	"dist":          true,
	"target":        true,
	"out":           true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".pytest_cache": true,
	".mypy_cache":   true,
}

// maxMarkerFileBytes is the per-file read cap; files above this are skipped
// with no error. Matches the sindept package's 2 MiB default.
const maxMarkerFileBytes = 2 << 20

// ParseMarkers walks root and returns every "sin-debt:" marker; also accepts the hash-style form.
// mapped by cleaned relative path. Only regular source files are scanned.
func ParseMarkers(root string) (map[string][]Marker, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]Marker)
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > maxMarkerFileBytes {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		markers, err := parseMarkerFile(path)
		if err != nil {
			return err
		}
		if len(markers) > 0 {
			out[rel] = markers
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func parseMarkerFile(path string) ([]Marker, error) {
	data, err := os.ReadFile(path) // #nosec G304 — input is a CLI path
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, nil // binary file — skip
	}
	var markers []Marker
	for lineNo, line := range strings.Split(string(data), "\n") {
		if m := sinDebtRE.FindStringSubmatch(line); m != nil {
			reason := strings.TrimSpace(m[1])
			markers = append(markers, Marker{
				Path:   path,
				Line:   lineNo + 1,
				Reason: reason,
			})
		}
	}
	return markers, nil
}

// markerFor returns the first marker that falls inside [start, end] or within a
// few preceding lines of the node. The 10-line window matches the CEO audit's
// approvedBySinDebt scanner so the two systems stay consistent.
func markerFor(markers map[string][]Marker, path string, start, end int) string {
	const contextLines = 10
	for _, m := range markers[path] {
		if m.Line >= start-contextLines && m.Line <= end {
			return m.Reason
		}
	}
	return ""
}

// markerForLine returns the first marker on the exact line.
func markerForLine(markers map[string][]Marker, path string, line int) string {
	for _, m := range markers[path] {
		if m.Line == line {
			return m.Reason
		}
	}
	return ""
}

// markerForPath returns the first marker anywhere in the file.
func markerForPath(markers map[string][]Marker, path string) string {
	for _, m := range markers[path] {
		return m.Reason
	}
	return ""
}
