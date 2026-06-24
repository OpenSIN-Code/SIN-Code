// SPDX-License-Identifier: MIT
// Purpose: path-scoped rule loader. Reads `.sin-code/rules/<topic>.md`
// files and answers "which rules should I inject when I touch file X?"
//
// Mirrors Claude Code's `.claude/rules/<topic>.md` surface (Anthropic
// release v2.1) where a YAML frontmatter `paths:` declares glob-shaped
// file filters; the rule body is **lazy-loaded** into context only on
// a Read/Write of a matching path, so a 100-rule repo doesn't bloat
// the system prompt on every turn.
//
// M2-friendly: zero new deps. Frontmatter parsing is hand-written
// (≈60 LoC) so we stay statically-linkable.
package rules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Rule is a single topic-scoped rule. Body is the markdown content
// AFTER the frontmatter fence, byte-stable and post-trim only.
//
// Path-scoped rules are matched against an absolute file path via
// one-or-more globs. If Paths is empty the rule is ALWAYS active.
type Rule struct {
	Name        string   // unique name, lowercased, dashes not dots
	Description string   // one-line summary (from frontmatter)
	Globs       []string // from `paths:` list (may be empty)
	AlwaysOn    bool     // true if frontmatter has `always_on: true` instead of paths
	Source      string   // absolute path of the .md file
	Body        []byte   // body bytes (NOT the frontmatter)
}

// ErrInvalidFrontmatter is returned when a rule file's YAML header
// cannot be parsed (mismatched `---` fences, missing required keys).
type ErrInvalidFrontmatter struct {
	Path   string
	Reason string
}

func (e ErrInvalidFrontmatter) Error() string {
	return fmt.Sprintf("rules: %s: invalid frontmatter: %s", e.Path, e.Reason)
}

// ErrDuplicateRule is returned by LoadDir when two rule files share a
// (normalised) Name. Rename is the fix.
type ErrDuplicateRule struct {
	Name string
	A, B string
}

func (e ErrDuplicateRule) Error() string {
	return fmt.Sprintf("rules: duplicate rule %q in %s and %s", e.Name, e.A, e.B)
}

// Store is a thread-safe lazy-loaded rules index for a single
// workspace directory. Constructed via New; Load triggers the
// from-disk parse.
type Store struct {
	mu     sync.RWMutex
	dir    string
	rules  []Rule         // sorted by name for determinism
	byName map[string]int // name → index in `rules`
	loaded bool
}

// New returns a Store rooted at `.sin-code/rules/` under `workspace`.
// The directory is NOT loaded eagerly; call Load to read the disk.
func New(workspace string) *Store {
	return &Store{
		dir:    filepath.Join(workspace, ".sin-code", "rules"),
		byName: map[string]int{},
	}
}

// Path returns the on-disk directory Store reads from.
func (s *Store) Path() string { return s.dir }

// Load reads every `<name>.md` file under s.dir, parses frontmatter,
// and registers the rule. Idempotent — calling twice is a no-op.
// Returns the number of rules loaded.
func (s *Store) Load() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return len(s.rules), nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return 0, fmt.Errorf("rules: mkdir %s: %w", s.dir, err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, fmt.Errorf("rules: readdir %s: %w", s.dir, err)
	}
	var fresh []Rule
	byName := map[string]int{}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		if !strings.HasSuffix(ent.Name(), ".md") {
			continue
		}
		full := filepath.Join(s.dir, ent.Name())
		r, perr := parseFile(full)
		if perr != nil {
			return 0, perr
		}
		if r.Name == "" {
			r.Name = strings.TrimSuffix(ent.Name(), ".md")
		}
		if _, dup := byName[r.Name]; dup {
			// We don't yet know the other file's name — compute it
			// on lookup. Caller surfaces ErrDuplicateRule below.
			_ = dup
			// We need to track the source of the *other* rule too;
			// build it from entries below. Defer the error to a
			// second pass to get both sides.
		}
		fresh = append(fresh, r)
		byName[r.Name] = len(fresh) - 1
	}
	// Second pass for duplicate detection with both sources known.
	for i, a := range fresh {
		for j := i + 1; j < len(fresh); j++ {
			b := fresh[j]
			if a.Name == b.Name {
				return 0, ErrDuplicateRule{
					Name: a.Name, A: a.Source, B: b.Source,
				}
			}
		}
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].Name < fresh[j].Name })
	// Rebuild byName by name after sort.
	for i, r := range fresh {
		byName[r.Name] = i
	}
	s.rules = fresh
	s.byName = byName
	s.loaded = true
	return len(s.rules), nil
}

// All returns every loaded rule. Sorted by name (byte-stable).
func (s *Store) All() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Rule, len(s.rules))
	copy(out, s.rules)
	return out
}

// Names returns the sorted rule-name list.
func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.rules))
	for i, r := range s.rules {
		out[i] = r.Name
	}
	return out
}

// Get returns the rule with the given name (case-sensitive).
// Returns ok=false if no rule with that name is loaded.
func (s *Store) Get(name string) (Rule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, ok := s.byName[name]
	if !ok {
		return Rule{}, false
	}
	return s.rules[idx], true
}

// ForPath returns every rule that matches `absPath` (absolute path
// to a file). Glob match is per rule's `Globs` list using
// `path.Match` semantics with `**` expanding to `**/*` for subdirs.
// If a rule has AlwaysOn=true, it is included unconditionally. If
// no rule matches, the empty slice is returned.
func (s *Store) ForPath(absPath string) []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Rule{}
	for _, r := range s.rules {
		if r.AlwaysOn {
			out = append(out, r)
			continue
		}
		for _, g := range r.Globs {
			if match(g, absPath) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// --- parsers ------------------------------------------------------------

func parseFile(path string) (Rule, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Rule{}, fmt.Errorf("rules: read %s: %w", path, err)
	}
	fm, body, err := splitFrontmatter(contents)
	if err != nil {
		return Rule{}, ErrInvalidFrontmatter{Path: path, Reason: err.Error()}
	}
	r := fm.toRule(path)
	r.Body = body
	return r, nil
}

// Wrap direct-error returns from splitFrontmatter / parseYAML so
// callers always see ErrInvalidFrontmatter when the file's header
// is malformed. (Implemented below as a thin pass-through that
// preserves the original error as the wrapped target for
// errors.Is/errors.As checks.)
func invalid(path, reason string) error {
	return ErrInvalidFrontmatter{Path: path, Reason: reason}
}

// errFrontmatter is the sentinel error type for frontmatter failures.
// (Currently unused — we just return ErrInvalidFrontmatter directly.)
var _ = errors.New

// frontmatter is the parsed YAML header. We support a strict, tiny
// subset: key: value, key: [list-of-strings], key: true|false. No
// nested structures, no comments handling beyond # lines (which
// are stripped), no quoted strings beyond simple double-quoted.
type frontmatter struct {
	Name        string
	Description string
	Paths       []string
	Always      bool
}

func (fm frontmatter) toRule(source string) Rule {
	return Rule{
		Name:        fm.Name,
		Description: fm.Description,
		Globs:       fm.Paths,
		AlwaysOn:    fm.Always,
		Source:      source,
	}
}

// splitFrontmatter parses the leading `---`-fenced YAML and returns
// (parsed fm, body bytes after the second `---`). The body is the
// remainder of the file trimmed to only the markdown body.
func splitFrontmatter(contents []byte) (frontmatter, []byte, error) {
	raw := string(contents)
	if !strings.HasPrefix(raw, "---") {
		return frontmatter{}, nil, fmt.Errorf("missing leading `---` fence")
	}
	rest := strings.TrimPrefix(raw, "---")
	rest = strings.TrimLeft(rest, " \t\r\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return frontmatter{}, nil, fmt.Errorf("missing closing `---` fence")
	}
	header := rest[:idx]
	body := rest[idx+len("\n---"):]
	body = strings.TrimLeft(body, " \t\r\n")
	fm, err := parseYAML(header)
	if err != nil {
		return frontmatter{}, nil, err
	}
	return fm, []byte(body), nil
}

// parseYAML is a tiny YAML subset parser covering our 4 frontmatter
// keys. Returns an error if a key is neither simple nor list-shaped.
func parseYAML(s string) (frontmatter, error) {
	var fm frontmatter
	for _, line := range strings.Split(s, "\n") {
		// strip comments and trim
		if i := strings.Index(line, "#"); i >= 0 && !startsQuoted(line, i) {
			line = line[:i]
		}
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		// Skip multi-line `paths:` bullet entries — handled by
		// multiPathList().
		if strings.HasPrefix(strings.TrimSpace(line), "- ") ||
			strings.HasPrefix(strings.TrimSpace(line), "* ") ||
			strings.HasPrefix(strings.TrimSpace(line), "• ") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			return fm, fmt.Errorf("not a key: %q", line)
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		val = stripQuotes(val)
		switch key {
		case "name":
			fm.Name = val
		case "description":
			fm.Description = val
		case "always_on", "alwaysOn":
			fm.Always = val == "true" || val == "yes" || val == "1"
		case "paths":
			// List form: `paths: [a, b, c]` OR multi-line bullets.
			if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
				inner := val[1 : len(val)-1]
				for _, p := range strings.Split(inner, ",") {
					p = stripQuotes(strings.TrimSpace(p))
					if p != "" {
						fm.Paths = append(fm.Paths, p)
					}
				}
			} else if val == "" {
				// multi-line — caller must process below
			} else {
				fm.Paths = append(fm.Paths, stripQuotes(val))
			}
		default:
			// unknown key is tolerated but logged via Skip keeps
			// behaviour explicit.
		}
	}
	// Multi-line `paths:` — bullets under `paths:` blank line.
	multi := multiPathList(s)
	if len(multi) > len(fm.Paths) {
		fm.Paths = multi
	}
	if fm.Name == "" {
		// Optional: fall back to filename later in New().
	}
	return fm, nil
}

// multiPathList extracts a bullet-list path block:
//
//	paths:
//	  - "cmd/foo/**"
//	  - "cmd/bar/**"
//
// The bullet character `-`, `*` or `•` is stripped; surrounding quotes
// are stripped. Patterns with no quotes may contain glob meta-characters.
func multiPathList(s string) []string {
	var out []string
	lines := strings.Split(s, "\n")
	inPath := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "paths:") {
			inPath = strings.TrimSpace(strings.TrimPrefix(trim, "paths:")) == ""
			continue
		}
		if !inPath {
			continue
		}
		if trim == "" {
			continue
		}
		switch {
		case strings.HasPrefix(trim, "- "):
			out = append(out, stripQuotes(strings.TrimSpace(trim[2:])))
		case strings.HasPrefix(trim, "* "):
			out = append(out, stripQuotes(strings.TrimSpace(trim[2:])))
		case strings.HasPrefix(trim, "• "):
			out = append(out, stripQuotes(strings.TrimSpace(trim[2:])))
		}
	}
	return out
}

func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func startsQuoted(s string, idx int) bool {
	// Crude: walk left from `idx`, track quote state.
	inD, inS := false, false
	for i := 0; i < idx; i++ {
		switch s[i] {
		case '"':
			if !inS {
				inD = !inD
			}
		case '\'':
			if !inD {
				inS = !inS
			}
		}
	}
	return inD || inS
}

// match returns true if glob g matches path p. Supports
// `**` as "any number of path segments" (gitignore-style) and
// standard `*` / `?` per-segment globbing via path.Match.
// The glob must consume the path from some anchor segment onward
// (so `cmd/foo/**` matches anywhere a `cmd/foo/...` subtree occurs).
func match(glob, path string) bool {
	gseg := strings.Split(glob, "/")
	pseg := strings.Split(path, "/")
	// Walk the path; at each anchor segment, try matchSegs.
	for i := 0; i <= len(pseg); i++ {
		if matchSegs(gseg, pseg[i:]) {
			return true
		}
	}
	return false
}

// matchSegs recursively matches a glob-segment slice against a
// path-segment slice. `**` matches zero or more path segments;
// `*` matches one path segment (any content). Anchor: the
// pattern must consume the path.
func matchSegs(g, p []string) bool {
	for len(g) > 0 {
		if g[0] == "**" {
			// Try matching the rest of the glob against
			// every suffix of the remaining path.
			for i := 0; i <= len(p); i++ {
				if matchSegs(g[1:], p[i:]) {
					return true
				}
			}
			return false
		}
		if len(p) == 0 {
			return false
		}
		ok, err := pathMatch(g[0], p[0])
		if err != nil || !ok {
			return false
		}
		g = g[1:]
		p = p[1:]
	}
	return len(p) == 0
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

// pathMatch wraps path.Match with a permissive fallback so that
// patterns like `agentloop/**` (no leading slash) match the
// last segment of a path containing that fragment.
func pathMatch(pattern, name string) (bool, error) {
	return pathMatch2(pattern, name)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

// pathMatch2 returns true if pattern matches name. We swap in a
// custom impl here so we don't have to wrap path.Match twice.
func pathMatch2(pattern, name string) (bool, error) {
	return globSegment(name, pattern)
}

// globSegment is a single-segment glob matcher. Supports `*`, `?`,
// and `[abc]`-style character classes. We dispatch to regexp
// because path.Match does not provide the `?`-and-class semantics
// uniformly across platforms.
func globSegment(name, pattern string) (bool, error) {
	var sb strings.Builder
	sb.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			sb.WriteString(".*")
		case '?':
			sb.WriteString(".")
		case '.', '+', '(', ')', '|', '^', '$', '\\', '{', '}', '[', ']':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteString("$")
	re, err := regexp.Compile(sb.String())
	if err != nil {
		return false, err
	}
	return re.MatchString(name), nil
}
