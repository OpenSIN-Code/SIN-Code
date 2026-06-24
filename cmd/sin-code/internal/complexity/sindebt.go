// SPDX-License-Identifier: MIT
// Purpose: sin-debt marker parser — respects the "sin-debt:" annotation format (issue #177).
package complexity

import (
	"bufio"
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

// ParseMarkers walks root and returns every "sin-debt:" marker; also accepts the hash-style form.
// mapped by cleaned relative path. Only regular files are scanned.
func ParseMarkers(root string) (map[string][]Marker, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]Marker)
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !info.Mode().IsRegular() {
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
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var markers []Marker
	scanner := bufio.NewScanner(f)
	lineNo := 1
	for scanner.Scan() {
		line := scanner.Text()
		if m := sinDebtRE.FindStringSubmatch(line); m != nil {
			reason := strings.TrimSpace(m[1])
			markers = append(markers, Marker{
				Path:   path,
				Line:   lineNo,
				Reason: reason,
			})
		}
		lineNo++
	}
	return markers, scanner.Err()
}

// markerFor returns the first marker that falls inside [start, end] or within a
// few preceding lines of the node.
func markerFor(markers map[string][]Marker, path string, start, end int) string {
	const contextLines = 5
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
