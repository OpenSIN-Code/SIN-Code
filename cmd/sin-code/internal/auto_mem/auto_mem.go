// SPDX-License-Identifier: MIT
// Purpose: byte-stable MEMORY.md-equivalent for SIN-Code. Mirrors the
// Claude Code Auto-Memory surface (memory/MEMORY.md + topic files)
// without the silent-LLM-write hazard (mandate M3: the verification
// gate is sacred, so writes are always deterministic and visible).
//
// In contrast to Claude Code's "model decides what to remember", every
// write here is **triggered by a hook** (verify-fail, tool-error, lesson
// derived from `internal/lessons`) — never silent. Operators can
// always read, edit, or delete MEMORY.md in their $SIN_CODE_HOME tree.
package auto_mem

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IndexBytesCap is the byte-stable byte cap on the index block we
// inject into the session system prompt. Mirrors Claude Code v2.1.59
// (`MEMORY.md` truncated at 25 KB / 200 lines, first reached).
const IndexBytesCap = 25 * 1024

// IndexLinesCap is the line cap that mirrors Claude Code's 200-line
// rule. Whichever limit is reached first defines the effective cap.
const IndexLinesCap = 200

// Store is the byte-stable MEMORY.md surface for a single project.
// All methods are safe for concurrent use (each call opens, edits
// under a tmp+rename, fsync, close — no in-memory shared state).
type Store struct {
	dir string // ~/.local/share/sin-code/memory/<project-hash>/memory/
	md  string // $dir/MEMORY.md
}

// Open creates or opens the auto-memory store for the project
// identified by `projectKey` (a stable, opaque string — typically the
// git remote URL or the abs-path of the repo root). The key is hashed
// to a 12-char hex digest so the on-disk directory is byte-stable and
// contains no path separators.
func Open(homeDir, projectKey string) (*Store, error) {
	if projectKey == "" {
		return nil, errors.New("auto_mem: empty project key")
	}
	hashed := projHash(projectKey)
	dir := filepath.Join(homeDir, "memory", hashed, "memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("auto_mem: mkdir: %w", err)
	}
	return &Store{
		dir: dir,
		md:  filepath.Join(dir, "MEMORY.md"),
	}, nil
}

// Path returns the absolute path to MEMORY.md for diagnostics.
func (s *Store) Path() string { return s.md }

// projHash returns a short stable hash from any string key so callers
// can pass either a pre-hashed key or a human-readable key. Uses
// sha256 prefix-12 for byte-stable, collision-resistant directory names.
func projHash(key string) string {
	if len(key) != 12 || strings.ContainsAny(key, "/\\. ") {
		sum := sha256.Sum256([]byte(key))
		return hex.EncodeToString(sum[:6])
	}
	return key
}

// Entry is one topic block in MEMORY.md. The Heading is normalised
// (lowercase, dash-separated, no leading "## ") so two writes of the
// same conceptual topic converge to one block — appending rather than
// duplicating. Body is the markdown content under the heading.
type Entry struct {
	Heading   string
	Body      string
	SourceTag string // "verify-fail" / "tool-error" / "manual" — provenance
	AddedAt   time.Time
}

// Append atomically adds or updates an entry. The on-disk format is:
//   <!-- SIN-CODE-AUTO-MEMORY-START -->
//   ## <heading>
//   _added: <RFC3339> · source: <source-tag>_
//
//   <body>
//
//   ## <heading2>
//   ...
//   <!-- SIN-CODE-AUTO-MEMORY-END -->
//
// Headings are sorted lexicographically so the file is byte-stable
// regardless of insertion order.
func (s *Store) Append(e Entry) error {
	if e.Heading == "" {
		return errors.New("auto_mem: empty heading")
	}
	if e.AddedAt.IsZero() {
		e.AddedAt = time.Now().UTC()
	}
	if e.SourceTag == "" {
		e.SourceTag = "manual"
	}
	heading := normaliseHeading(e.Heading)
	entries, err := s.readAll()
	if err != nil {
		return err
	}
	replaced := false
	for i := range entries {
		if normaliseHeading(entries[i].Heading) == heading {
			entries[i] = Entry{
				Heading:   heading,
				Body:      e.Body,
				SourceTag: e.SourceTag,
				AddedAt:   e.AddedAt,
			}
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, Entry{
			Heading:   heading,
			Body:      e.Body,
			SourceTag: e.SourceTag,
			AddedAt:   e.AddedAt,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Heading < entries[j].Heading
	})
	return s.writeAll(entries)
}

// Index returns the heading list in the deterministically-sorted
// order they'll appear inside IndexBytes.
func (s *Store) Index() ([]string, error) {
	entries, err := s.readAll()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Heading
	}
	return out, nil
}

// IndexBytes returns a byte-stable fragment of MEMORY.md suitable for
// system-prompt injection. Cap = first 200 lines OR 25 KB, whichever
// is reached first (mirrors Claude Code v2.1.59).
func (s *Store) IndexBytes() ([]byte, error) {
	contents, err := os.ReadFile(s.md)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(contents) > IndexBytesCap {
		contents = truncateAtLine(contents, IndexLinesCap)
		contents = contents[:IndexBytesCap]
	}
	return contents, nil
}

// ReadTopic returns the body of a single topic by heading. Returns
// ErrNoSuchTopic if the heading is not present.
var ErrNoSuchTopic = errors.New("auto_mem: no such topic")

func (s *Store) ReadTopic(heading string) ([]byte, error) {
	target := normaliseHeading(heading)
	entries, err := s.readAll()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Heading == target {
			return []byte(e.Body), nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrNoSuchTopic, heading)
}

// Remove deletes a topic from MEMORY.md by heading.
func (s *Store) Remove(heading string) error {
	target := normaliseHeading(heading)
	entries, err := s.readAll()
	if err != nil {
		return err
	}
	out := entries[:0]
	found := false
	for _, e := range entries {
		if e.Heading == target {
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrNoSuchTopic, heading)
	}
	return s.writeAll(out)
}

// Rotate trims MEMORY.md to the most recent N entries (by AddedAt).
// Returns the number of entries retained.
func (s *Store) Rotate(max int) (int, error) {
	entries, err := s.readAll()
	if err != nil {
		return 0, err
	}
	if len(entries) <= max {
		return len(entries), nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].AddedAt.After(entries[j].AddedAt)
	})
	entries = entries[:max]
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Heading < entries[j].Heading
	})
	if err := s.writeAll(entries); err != nil {
		return 0, err
	}
	return max, nil
}

// --- private helpers -----------------------------------------------------

func normaliseHeading(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.Trim(s, "-")
	return s
}

func (s *Store) readAll() ([]Entry, error) {
	contents, err := os.ReadFile(s.md)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return parse(contents), nil
}

// parse extracts entries between the start/end markers. Deterministic
// — same input bytes yield same parsed entries.
func parse(contents []byte) []Entry {
	var out []Entry
	md := string(contents)
	startIdx := strings.Index(md, "<!-- SIN-CODE-AUTO-MEMORY-START -->")
	endIdx := strings.Index(md, "<!-- SIN-CODE-AUTO-MEMORY-END -->")
	if startIdx < 0 || endIdx <= startIdx {
		return nil
	}
	md = md[startIdx:endIdx]
	// Drop the preamble by anchoring on the first "\n## " marker
	// before splitting. Future writes may include a different
	// preamble so this is intentional.
	if i := strings.Index(md, "\n## "); i < 0 {
		return nil
	} else {
		md = md[i+1:]
	}
	for _, blk := range strings.Split(md, "\n## ") {
		blk = strings.TrimSpace(blk)
		if blk == "" {
			continue
		}
		// Strip the literal "## " prefix that the very first block
		// carries after the preamble drop above; subsequent blocks
		// already have no prefix.
		blk = strings.TrimPrefix(blk, "## ")
		// `blk` is "<heading>\n<rest>" or just "<heading>" if body empty.
		parts := strings.SplitN(blk, "\n", 2)
		if len(parts) < 2 {
			out = append(out, Entry{Heading: normaliseHeading(parts[0])})
			continue
		}
		heading := normaliseHeading(parts[0])
		rest := parts[1]
		// Find the annotation line "_added: ... · source: ..._"
		// and the body that comes AFTER it (separated by a blank line).
		var added time.Time
		var source = "manual"
		annotationEnd := 0
		for i, ln := range strings.Split(rest, "\n") {
			if strings.HasPrefix(ln, "_added: ") {
				// parse "_added: <RFC3339> · source: <tag>_"
				end := strings.LastIndex(ln, "_")
				if end > 0 {
					body := ln[:end]
					// crude split on "·" (U+00B7, kept verbatim)
					if idx := strings.Index(body, " · source: "); idx >= 0 {
						dateStr := strings.TrimPrefix(body[:idx], "_added: ")
						source = strings.TrimSpace(body[idx+len(" · source: "):])
						added, _ = time.Parse(time.RFC3339, dateStr)
					}
				}
				annotationEnd = i + 1
				break
			}
		}
		body := rest
		if annotationEnd > 0 {
			lines := strings.SplitN(rest, "\n", annotationEnd+1)
			if len(lines) >= annotationEnd+1 {
				body = lines[annotationEnd]
				body = strings.TrimLeft(body, "\n")
			}
		}
		out = append(out, Entry{
			Heading:   heading,
			Body:      body,
			SourceTag: source,
			AddedAt:   added,
		})
	}
	return out
}

func (s *Store) writeAll(entries []Entry) error {
	var sb strings.Builder
	sb.WriteString("<!-- SIN-CODE-AUTO-MEMORY-START -->\n")
	sb.WriteString("# Nested Sin-Code Memory Index\n\n")
	sb.WriteString("Auto-generated by `internal/auto_mem`. Edit via `sin-code memory`.\n")
	sb.WriteString("Each `## <heading>` block is its own topic. Sorted lexicographically.\n\n")
	for _, e := range entries {
		fmt.Fprintf(&sb, "## %s\n\n", e.Heading)
		fmt.Fprintf(&sb, "_added: %s · source: %s_\n\n",
			e.AddedAt.UTC().Format(time.RFC3339), e.SourceTag)
		sb.WriteString(e.Body)
		if !strings.HasSuffix(e.Body, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("<!-- SIN-CODE-AUTO-MEMORY-END -->\n")
	return atomicWrite(s.md, []byte(sb.String()))
}

func truncateAtLine(contents []byte, maxLines int) []byte {
	nl := 0
	for i := 0; i < len(contents); i++ {
		if contents[i] == '\n' {
			nl++
			if nl >= maxLines {
				return contents[:i+1]
			}
		}
	}
	return contents
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".mem-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmp != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	tmp = nil
	return os.Rename(tmpName, path)
}

// DefaultHome returns the default sin-code home directory, following
// mandate M2 (no runtime deps, pure os.UserConfigDir).
func DefaultHome() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sin-code"), nil
}
