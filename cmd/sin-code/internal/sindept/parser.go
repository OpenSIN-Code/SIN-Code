// SPDX-License-Identifier: MIT
// Purpose: sindept — first-party scanner for the `// sin-debt: <reason>,
// upgrade: <trigger>` marker convention (issue #177). Adopts ponytail's
// `ponytail:` marker convention as a typed, auditable source-code contract.
// Docs: sindept.doc.md
package sindept

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Marker is one parsed `sin-debt:` comment in a source file.
//
// The struct's JSON tag set is the public contract — downstream automation
// (issue #179 complexity auditor / issue #180 audit-engine) reads the marker
// through this shape. Renaming a field is a breaking change (M10).
type Marker struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column,omitempty"`
	Reason   string `json:"reason"`
	Upgrade  string `json:"upgrade,omitempty"`
	HasUpg   bool   `json:"has_upgrade"`
	Raw      string `json:"raw"`
	Language string `json:"language,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

// Options controls ParseDir.
type Options struct {
	// Skip is the set of directory base names to skip (matched as exact
	// filepath.Base). Empty entries and entries starting with "." are
	// always skipped (e.g. `.git`, `.sin-code`).
	Skip []string
	// SkipSuffixes is the set of file base-name suffixes to skip (case
	// sensitive). DefaultOptions seeds this with `_test.go` and
	// `.doc.md` — see the rationale on DefaultOptions.
	SkipSuffixes []string
	// IncludeExt restricts the walk to files whose extension (lowercased,
	// without the leading dot) appears in the list. Empty = include all.
	IncludeExt []string
	// MaxFileBytes is the per-file read cap; files above this are skipped
	// with no error. 0 = unlimited.
	MaxFileBytes int64
}

// DefaultOptions is the canonical walk configuration for the agent loop.
// It mirrors the conventional build / vcs exclusions and keeps the walk
// deterministic (the slice is a fixed length and Go map iteration is
// sequential within a single goroutine — we sort after).
//
// Two non-obvious suffixes are also skipped:
//
//   - `_test.go`  : Go test files are NOT production source. A debt
//     marker in a test file almost always describes the
//     test fixture itself, not the system's debt. The
//     sindept package tests bypass this with a Sentinel
//     Option override.
//   - `.doc.md`   : the repo's convention for sibling design docs.
//     They explain the convention, not declare real debt.
func DefaultOptions() Options {
	return Options{
		Skip: []string{
			"node_modules",
			"vendor",
			"dist",
			"build",
			"target",
			"out",
			".venv",
			"venv",
			"__pycache__",
			".pytest_cache",
			".mypy_cache",
		},
		SkipSuffixes: []string{
			"_test.go",
			".doc.md",
		},
		IncludeExt:   nil,     // everything that is not in Skip
		MaxFileBytes: 2 << 20, // 2MiB — markers in larger files are noise
	}
}

// markerRe matches the canonical sin-debt: marker across our five comment
// families (//, #, --, /*, <!--).
//
//	(?P<prefix>(?://|#|--|/\*|<!--))               comment opener
//	\s*sin-debt:\s*                                 literal marker token
//	(?P<reason>[^,\n\r]+?)                          ceiling/cliff (lazy)
//	(?:,\s*upgrade:\s*(?P<upgrade>.+?))?            optional upgrade clause
//	\s*$                                            EOL anchor (?m)
//
// Go's RE2 syntax does not support \Q...\E, so the literal "sin-debt:" is
// spelled out directly. The (?m:...) block enables multi-line mode without
// affecting surrounding patterns. The captured reason/upgrade are
// post-processed by trimAction and stripCloser below to remove trailing
// comment closers ("*/", "-->") that bleed into lazy matches.
var markerRe = regexp.MustCompile(
	`(?m:(?://|#|--|/\*|<!--)\s*sin-debt:\s*(?P<reason>[^,\n\r]+?)(?:\s*,\s*upgrade:\s*(?P<upgrade>.+?))?\s*$)`,
)

// stripCloser removes a trailing block-comment closer ("*/" or "-->")
// from the captured reason/upgrade text. It does NOT touch block-comment
// openers in the body — those are written by humans and mean something.
func stripCloser(s string) string {
	for _, suf := range []string{"*/", "-->", "*/ ", "/*"} {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSuffix(s, suf)
			s = strings.TrimRight(s, " \t")
		}
	}
	return s
}

// extractCodeLang returns the conventional language tag for an extension.
// The map is small on purpose — the marker simply tags `Language` for the
// downstream auditor (#179/#180), it does not try to be a polyglot compiler.
var extLang = map[string]string{
	"go":     "go",
	"py":     "python",
	"js":     "javascript",
	"jsx":    "javascript",
	"ts":     "typescript",
	"tsx":    "typescript",
	"rs":     "rust",
	"java":   "java",
	"kt":     "kotlin",
	"swift":  "swift",
	"c":      "c",
	"h":      "c",
	"cpp":    "cpp",
	"hpp":    "cpp",
	"cc":     "cpp",
	"cs":     "csharp",
	"rb":     "ruby",
	"sh":     "shell",
	"bash":   "shell",
	"zsh":    "shell",
	"sql":    "sql",
	"yaml":   "yaml",
	"yml":    "yaml",
	"toml":   "toml",
	"json":   "json",
	"md":     "markdown",
	"html":   "html",
	"css":    "css",
	"scss":   "scss",
	"vue":    "vue",
	"svelte": "svelte",
	"php":    "php",
	"lua":    "lua",
}

// trimAction normalizes a single ceiling/upgrade value for byte-stable
// output: trims surrounding whitespace, strips a single trailing dot or
// comma (artifacts of natural-language authoring), and collapses internal
// runs of whitespace.
func trimAction(s string) string {
	s = strings.TrimSpace(s)
	for len(s) > 0 {
		c := s[len(s)-1]
		if c == '.' || c == ',' || c == ';' || c == ':' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return strings.Join(strings.Fields(s), " ")
}

// findColumn computes the 1-based column position of `pattern` in `line`,
// or 0 if not found. The marker scanner uses this purely for diagnostics —
// downstream audit passes can show a precise cursor.
func findColumn(line, pattern string) int {
	return strings.Index(line, pattern) + 1
}

// nextSymbol looks up at most 40 lines above AND below the marker for a
// Go-style `func Receiver(...)` or a python `def name(` definition. It is
// a best-effort guess — the marker itself does not need to know its
// symbol to be useful, only to be ergonomic when read in a report. The
// `markerLineIdx` argument is a 0-based line index, NOT a byte offset.
// Callers must convert offsets to indices with `byteOffsetOfLine` first
// or — preferred — pass the already-known `Line-1`.
func nextSymbol(content string, markerLineIdx int) string {
	if markerLineIdx < 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return ""
	}
	startUp := markerLineIdx - 40
	if startUp < 0 {
		startUp = 0
	}
	startDown := markerLineIdx + 1
	endDown := markerLineIdx + 40
	if endDown > len(lines) {
		endDown = len(lines)
	}
	tryOrder := func(slice []string) string {
		for _, line := range slice {
			line = strings.TrimSpace(line)
			for _, re := range []*regexp.Regexp{
				regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?([A-Za-z_][A-Za-z0-9_]*)`),
				regexp.MustCompile(`^def\s+([A-Za-z_][A-Za-z0-9_]*)`),
				regexp.MustCompile(`^function\s+([A-Za-z_][A-Za-z0-9_]*)`),
				regexp.MustCompile(`^class\s+([A-Za-z_][A-Za-z0-9_]*)`),
				regexp.MustCompile(`^(?:export\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)`),
				regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)`),
			} {
				if m := re.FindStringSubmatch(line); len(m) == 2 {
					return m[1]
				}
			}
		}
		return ""
	}
	// Prefer the closest declaration ABOVE — it represents what the
	// marker is documenting. Fall back to the closest declaration
	// BELOW when no above-the-line declaration exists (e.g. a marker
	// sitting right after a `package` line).
	for i := markerLineIdx - 1; i >= startUp; i-- {
		if s := tryOrder([]string{lines[i]}); s != "" {
			return s
		}
	}
	for i := startDown; i < endDown; i++ {
		if s := tryOrder([]string{lines[i]}); s != "" {
			return s
		}
	}
	return ""
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

// ParseFile reads a single file and returns its sorted-by-line markers.
// Returns an empty slice and no error for an unreadable, hidden, or empty
// file; the caller can verify with `len(result) == 0`.
func ParseFile(path string) ([]Marker, error) {
	return ParseFileWithCap(path, 0)
}

// ParseFileWithCap is ParseFile with an explicit per-file byte limit.
// Pass 0 to disable the cap.
func ParseFileWithCap(path string, max int64) ([]Marker, error) {
	f, err := os.Open(path) // #nosec G304 — input is a CLI path
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return nil, nil // ergonomic: missing / unreadable files are empty
		}
		return nil, fmt.Errorf("sindept: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if max > 0 {
		st, err := f.Stat()
		if err == nil && st.Size() > max {
			return nil, nil
		}
	}

	// Read fully so the regex can scan multiline content. We bound by
	// the caller-supplied `max` above, so this is not unbounded.
	var buf bytes.Buffer
	if _, err := bufio.NewReader(f).WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("sindept: read %s: %w", path, err)
	}
	content := buf.String()
	rawLines := strings.Split(content, "\n")

	// For Markdown files outside testdata, skip entirely — sin-debt markers
	// in .md files are documentation examples, not real debt markers.
	// testdata/*.md files are scanner fixtures and must be parsed.
	isMarkdown := strings.HasSuffix(strings.ToLower(path), ".md")
	isTestdata := strings.Contains(filepath.ToSlash(path), "/testdata/") || strings.HasPrefix(filepath.ToSlash(path), "testdata/")
	if isMarkdown && !isTestdata {
		return nil, nil
	}

	idx := markerRe.FindAllStringSubmatchIndex(content, -1)
	if len(idx) == 0 {
		return nil, nil
	}

	out := make([]Marker, 0, len(idx))
	for _, m := range idx {
		// [0:1] = full match, [2:3] = reason, [4:5] = upgrade (or -1)
		fullStart, fullEnd := m[0], m[1]
		reasonS, reasonE := m[2], m[3]
		upgS, upgE := m[4], m[5]

		raw := content[fullStart:fullEnd]
		reason := trimAction(stripCloser(content[reasonS:reasonE]))
		upgrade := ""
		hasUpg := false
		if upgS >= 0 && upgE >= 0 {
			upgrade = trimAction(stripCloser(content[upgS:upgE]))
			hasUpg = upgrade != ""
		}

		line, col := lineColumn(rawLines, fullStart)

		mark := Marker{
			File:     path,
			Line:     line,
			Column:   col,
			Reason:   reason,
			Upgrade:  upgrade,
			HasUpg:   hasUpg,
			Raw:      raw,
			Language: extLang[strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")],
			Symbol:   nextSymbol(content, line-1),
		}
		out = append(out, mark)
	}
	return out, nil
}

// lineColumn maps a byte offset into a line number and 1-based column.
func lineColumn(lines []string, off int) (int, int) {
	consumed := 0
	for i, l := range lines {
		consumed += len(l) + 1 // +1 for newline
		if consumed > off {
			col := off - (consumed - len(l) - 1) + 1
			return i + 1, col
		}
	}
	return len(lines), 1
}

// byteOffsetOfLine returns the starting byte offset of `idx`-th line in
// `lines` (0-indexed). It is used by ParseFile to look up `Symbol` for
// the marker we just saw.
func byteOffsetOfLine(lines []string, idx int) int {
	if idx < 0 {
		return 0
	}
	if idx >= len(lines) {
		return len(strings.Join(lines, "\n"))
	}
	off := 0
	for i := 0; i < idx; i++ {
		off += len(lines[i]) + 1
	}
	return off
}

// ParseDir walks `root` recursively and returns every marker found.
// It is byte-deterministic: file order is lex-sorted, then marker order
// is the order they appear in each file. The result is sorted by File
// then Line then Column.
func ParseDir(root string, opts Options) ([]Marker, error) {
	if root == "" {
		return nil, fmt.Errorf("sindept: empty root")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("sindept: stat %s: %w", root, err)
	}
	if !info.IsDir() {
		mk, err := ParseFileWithCap(root, opts.MaxFileBytes)
		if err != nil {
			return nil, err
		}
		return mk, nil
	}

	skipSet := map[string]bool{}
	for _, s := range opts.Skip {
		skipSet[s] = true
	}
	suffixSet := map[string]bool{}
	for _, s := range opts.SkipSuffixes {
		suffixSet[s] = true
	}
	extSet := map[string]bool{}
	for _, e := range opts.IncludeExt {
		extSet[strings.ToLower(strings.TrimPrefix(e, "."))] = true
	}

	// The walk is single-threaded so the output is stable regardless of
	// goroutine count. sin-debt is a deterministic scan, not a streaming
	// search.
	var out []Marker
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		base := d.Name()
		if d.IsDir() {
			if shouldSkipDir(base, skipSet) {
				return filepath.SkipDir
			}
			return nil
		}
		if base == "" || strings.HasPrefix(base, ".") && base != ".env" {
			// silently drop hidden files except a small allowance
			if base != ".env" {
				return nil
			}
		}
		if shouldSkipSuffix(base, suffixSet) {
			return nil
		}
		if strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".min.css") {
			return nil
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(base)), ".")
		if len(extSet) > 0 && !extSet[ext] {
			return nil
		}
		mks, err := ParseFileWithCap(path, opts.MaxFileBytes)
		if err != nil {
			return err
		}
		out = append(out, mks...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Column < out[j].Column
	})
	return out, nil
}

// shouldSkipDir returns true when `base` (a single path component) is in
// `skipSet` or starts with ".". Hidden dirs are skipped unconditionally —
// they should never contain markers we want to scan.
func shouldSkipDir(base string, skipSet map[string]bool) bool {
	if base == "" {
		return true
	}
	if strings.HasPrefix(base, ".") {
		return true
	}
	return skipSet[base]
}

// shouldSkipSuffix returns true when `base` ends with any of the suffixes
// in `suffixSet`. Used to drop test files (`_test.go`) and design docs
// (`.doc.md`) by default — see the rationale on DefaultOptions.
func shouldSkipSuffix(base string, suffixSet map[string]bool) bool {
	if len(suffixSet) == 0 {
		return false
	}
	for suf := range suffixSet {
		if suf != "" && strings.HasSuffix(base, suf) {
			return true
		}
	}
	return false
}
