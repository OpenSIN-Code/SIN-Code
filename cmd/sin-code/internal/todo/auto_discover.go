// SPDX-License-Identifier: MIT
// Purpose: AutoDiscover scans source files for TODO/FIXME/HACK/XXX markers
// and converts them into todos (issue #330). Supports Go, Python, JS, and
// Rust comment styles.
package todo

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TodoMarker is a TODO/FIXME/HACK/XXX marker found in source code.
type TodoMarker struct {
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Type     string   `json:"type"`
	Text     string   `json:"text"`
	Priority Priority `json:"priority"`
}

// AutoDiscover scans code for TODO/FIXME markers.
type AutoDiscover struct {
	mu sync.Mutex
}

// NewAutoDiscover creates a new scanner.
func NewAutoDiscover() *AutoDiscover {
	return &AutoDiscover{}
}

var markerTypes = []string{"TODO", "FIXME", "HACK", "XXX"}

var markerCommentPrefixes = []string{"//", "#", "/*", "*"}

// markerPriority maps a marker type to a todo priority.
func markerPriority(markerType string) Priority {
	switch markerType {
	case "FIXME":
		return PriorityP1
	case "TODO":
		return PriorityP2
	case "XXX":
		return PriorityP2
	case "HACK":
		return PriorityP3
	default:
		return PriorityP2
	}
}

// ScanFile reads a single file and returns all markers found.
func (d *AutoDiscover) ScanFile(path string) ([]TodoMarker, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var markers []TodoMarker
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		mt, text, ok := parseMarkerLine(sc.Text())
		if !ok {
			continue
		}
		markers = append(markers, TodoMarker{
			File:     path,
			Line:     lineNum,
			Type:     mt,
			Text:     text,
			Priority: markerPriority(mt),
		})
	}
	return markers, sc.Err()
}

// ScanDir walks a directory tree up to maxDepth and returns all markers.
// A maxDepth <= 0 means unlimited. Directories named .git, node_modules,
// vendor, dist, and build are always skipped.
func (d *AutoDiscover) ScanDir(root string, maxDepth int) ([]TodoMarker, error) {
	var all []TodoMarker
	return all, filepath.WalkDir(root, func(path string, dent os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if dent.IsDir() {
			base := dent.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" ||
				base == "dist" || base == "build" || base == ".sin-code" {
				return filepath.SkipDir
			}
			if maxDepth > 0 {
				rel, _ := filepath.Rel(root, path)
				if rel != "." {
					depth := strings.Count(rel, string(filepath.Separator)) + 1
					if depth >= maxDepth {
						return filepath.SkipDir
					}
				}
			}
			return nil
		}
		if !isSourceFile(path) {
			return nil
		}
		markers, err := d.ScanFile(path)
		if err != nil {
			return nil
		}
		all = append(all, markers...)
		return nil
	})
}

// MarkersToTodos converts markers into todos with ExternalRef set to
// "file:<path>:<line>". FIXME markers become bug type; others become task.
func (d *AutoDiscover) MarkersToTodos(markers []TodoMarker) []*Todo {
	todos := make([]*Todo, 0, len(markers))
	for _, m := range markers {
		title := m.Text
		if title == "" {
			title = m.Type
		}
		td := &Todo{
			Title:       title,
			Type:        TypeTask,
			Priority:    m.Priority,
			Status:      StatusOpen,
			ExternalRef: fmt.Sprintf("file:%s:%d", m.File, m.Line),
		}
		if m.Type == "FIXME" {
			td.Type = TypeBug
		}
		todos = append(todos, td)
	}
	return todos
}

// isSourceFile reports whether the path has a recognized source extension.
func isSourceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".rs",
		".java", ".rb", ".c", ".cc", ".cpp", ".h", ".hpp", ".sh":
		return true
	}
	return false
}

// parseMarkerLine checks a line for a TODO/FIXME/HACK/XXX marker inside a
// comment. Returns (markerType, text, true) when found.
func parseMarkerLine(line string) (string, string, bool) {
	for _, marker := range markerTypes {
		for _, prefix := range markerCommentPrefixes {
			combined := prefix + " " + marker
			idx := strings.Index(line, combined)
			if idx < 0 {
				combined = prefix + marker
				idx = strings.Index(line, combined)
			}
			if idx < 0 {
				continue
			}
			rest := line[idx+len(combined):]
			rest = strings.TrimLeft(rest, ":() ")
			rest = strings.TrimRight(rest, "*/ ")
			rest = strings.TrimSpace(rest)
			return marker, rest, true
		}
	}
	return "", "", false
}
