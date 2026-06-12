// SPDX-License-Identifier: MIT
// Purpose: agent0ai/dox (MIT) self-maintaining AGENTS.md hierarchy
// protocol. Marker-based injection (coexistent with the existing
// `<!-- SIN-Code superpowers:begin/end -->` block), tree validation,
// child scaffold with parent INDEX registration, and a human-readable
// tree renderer. CGo-free, stdlib-only.
// Docs: dox.doc.md
package dox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ── Markers ────────────────────────────────────────────────────────────
//
// The dox block lives between BeginMarker / EndMarker HTML-comment
// sentinels, the same idiom used by the superpowers overlay so two
// independent tools can own distinct blocks in the same AGENTS.md
// without colliding. BeginMarker / EndMarker are exported so the CLI
// layer (and the test suite) can assert on them.

const (
	// BeginMarker opens the dox-managed region inside AGENTS.md.
	BeginMarker = "<!-- SIN-Code dox:begin -->"
	// EndMarker closes the dox-managed region inside AGENTS.md.
	EndMarker = "<!-- SIN-Code dox:end -->"

	// AgentsFileName is the canonical AGENTS.md file name.
	AgentsFileName = "AGENTS.md"
	// IndexFileName is the canonical child INDEX file.
	IndexFileName = "INDEX.md"
	// DefaultFileMode is the mode used when writing new files.
	DefaultFileMode fs.FileMode = 0o644
	// DefaultDirMode is the mode used when creating new directories.
	DefaultDirMode fs.FileMode = 0o755
)

// ── Errors ─────────────────────────────────────────────────────────────

var (
	// ErrEmptyPath is returned when an input path is empty.
	ErrEmptyPath = errors.New("dox: empty path")
	// ErrNotADirectory is returned when a path that should be a dir is not.
	ErrNotADirectory = errors.New("dox: not a directory")
	// ErrAlreadyExists is returned by Scaffold when the target already
	// has a child node registered for the requested name.
	ErrAlreadyExists = errors.New("dox: child already exists")
)

// ── Tree model ─────────────────────────────────────────────────────────

// Node represents one directory entry in the dox tree. A node is
// either a leaf (no children, IsLeaf=true) or an internal node with
// at least one child.
type Node struct {
	Name     string  // basename of the directory or "."
	Path     string  // absolute path
	Title    string  // human title (taken from frontmatter or directory name)
	IsLeaf   bool    // true ⇒ no children allowed
	Children []*Node // empty for leaves
	Parent   *Node   // nil for the root node
}

// Depth returns the depth of this node in the tree (root = 0).
func (n *Node) Depth() int {
	d := 0
	for p := n.Parent; p != nil; p = p.Parent {
		d++
	}
	return d
}

// Frontmatter is a tiny, dependency-free frontmatter reader. It
// supports the subset we need: `key: value` pairs on separate lines,
// optional `---` fences. It does not try to be a full YAML parser.
type Frontmatter map[string]string

// ParseFrontmatter extracts `key: value` pairs from the top of `body`.
// If the body starts with `---`, parsing stops at the next `---`. The
// returned map is empty (never nil) when no frontmatter is present.
func ParseFrontmatter(body string) Frontmatter {
	out := Frontmatter{}
	lines := strings.Split(body, "\n")
	start := 0
	end := len(lines)
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		// fenced frontmatter
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				end = i
				break
			}
		}
		start = 1
	}
	for i := start; i < end; i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, "\"'")
		if key != "" {
			out[key] = val
		}
	}
	return out
}

// TitleOf returns the human title for `body`: frontmatter `title:` if
// present, otherwise empty string. The caller is expected to fall back
// to the directory name when the returned string is empty.
func TitleOf(body string) string {
	return ParseFrontmatter(body)["title"]
}

// ── Build / inspect ────────────────────────────────────────────────────

// Build walks `root` recursively and returns the tree of dox nodes. A
// directory is considered a leaf iff it contains a `LEAF.md` sentinel
// (matches the upstream dox convention). Returns an error if `root`
// does not exist or is not a directory.
func Build(root string) (*Node, error) {
	if root == "" {
		return nil, ErrEmptyPath
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrNotADirectory, abs)
	}
	return buildNode(abs, nil)
}

func buildNode(dir string, parent *Node) (*Node, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	n := &Node{
		Name:   filepath.Base(dir),
		Path:   dir,
		Parent: parent,
	}
	// Title: prefer frontmatter title from AGENTS.md / INDEX.md.
	for _, candidate := range []string{AgentsFileName, IndexFileName, "README.md"} {
		body, err := os.ReadFile(filepath.Join(dir, candidate))
		if err != nil {
			continue
		}
		if t := TitleOf(string(body)); t != "" {
			n.Title = t
			break
		}
	}
	// A directory is a leaf if it contains a LEAF.md sentinel.
	isLeaf := false
	for _, e := range entries {
		if e.Name() == "LEAF.md" {
			isLeaf = true
			break
		}
	}
	if isLeaf {
		n.IsLeaf = true
		return n, nil
	}
	// Otherwise recurse into subdirectories (skip dot-dirs and the
	// `.sin-code` scratch dir used by the CLI itself).
	names := []string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		child, err := buildNode(filepath.Join(dir, name), n)
		if err != nil {
			return nil, err
		}
		n.Children = append(n.Children, child)
	}
	return n, nil
}

// ── Validation ─────────────────────────────────────────────────────────

// Finding is one issue discovered by Check. Severity is "error"
// (broken tree) or "warn" (cosmetic / TODO).
type Finding struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Check walks the tree starting at `root` and returns any structural
// problems. It is safe to call on a partial / broken tree — the goal
// is to surface every issue at once, not stop at the first failure.
//
// Detected issues:
//
//	orphan         — non-root directory with no AGENTS.md / INDEX.md
//	broken-link    — markdown link to a sibling that does not exist
//	todo           — TODO sentinel left in title/description
//	missing-index  — internal node with no INDEX.md
//	missing-agents  — root with no AGENTS.md
func Check(root string) ([]Finding, error) {
	tree, err := Build(root)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	checkNode(tree, true, &findings)
	return findings, nil
}

func checkNode(n *Node, isRoot bool, out *[]Finding) {
	// Root must have an AGENTS.md.
	if isRoot {
		if _, err := os.Stat(filepath.Join(n.Path, AgentsFileName)); err != nil {
			*out = append(*out, Finding{
				Path:     n.Path,
				Kind:     "missing-agents",
				Severity: "error",
				Message:  "root directory has no AGENTS.md",
			})
		}
	} else {
		// Non-root nodes should have an INDEX.md.
		hasIndex := false
		hasAgents := false
		for _, name := range []string{IndexFileName, AgentsFileName} {
			if _, err := os.Stat(filepath.Join(n.Path, name)); err == nil {
				if name == IndexFileName {
					hasIndex = true
				}
				if name == AgentsFileName {
					hasAgents = true
				}
			}
		}
		if !hasIndex && !hasAgents {
			*out = append(*out, Finding{
				Path:     n.Path,
				Kind:     "orphan",
				Severity: "error",
				Message:  "directory is not registered in parent INDEX and has no own AGENTS.md",
			})
		}
	}
	// Scan the body of AGENTS.md / INDEX.md for TODO sentinels and
	// broken markdown links.
	for _, candidate := range []string{AgentsFileName, IndexFileName} {
		body, err := os.ReadFile(filepath.Join(n.Path, candidate))
		if err != nil {
			continue
		}
		s := string(body)
		// TODO sentinels: anywhere in the body, case-insensitive.
		lower := strings.ToLower(s)
		if strings.Contains(lower, "todo") {
			// Cheap heuristic: only flag the word "TODO" when it is a
			// standalone token (preceded by start-of-line or whitespace,
			// followed by whitespace, `:`, `,`, `!`, or EOL).
			for _, line := range strings.Split(s, "\n") {
				if hasStandaloneTODO(line) {
					*out = append(*out, Finding{
						Path:     filepath.Join(n.Path, candidate),
						Kind:     "todo",
						Severity: "warn",
						Message:  "TODO sentinel found: " + strings.TrimSpace(line),
					})
				}
			}
		}
		// Broken links: every [text](path.md) where path.md is a
		// relative path that does not resolve on disk.
		for _, link := range extractMarkdownLinks(s) {
			target := link
			if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
				continue
			}
			// Strip anchor + query.
			if i := strings.IndexAny(target, "#?"); i >= 0 {
				target = target[:i]
			}
			if target == "" {
				continue
			}
			full := filepath.Join(n.Path, target)
			if _, err := os.Stat(full); err != nil {
				*out = append(*out, Finding{
					Path:     filepath.Join(n.Path, candidate),
					Kind:     "broken-link",
					Severity: "error",
					Message:  "link target does not exist: " + link,
				})
			}
		}
	}
	for _, c := range n.Children {
		checkNode(c, false, out)
	}
}

func hasStandaloneTODO(line string) bool {
	idx := strings.Index(line, "TODO")
	if idx < 0 {
		return false
	}
	// Word boundary on the left: start-of-line or non-letter.
	if idx > 0 {
		prev := line[idx-1]
		if isLetter(byte(prev)) || isDigit(byte(prev)) {
			return false
		}
	}
	// Word boundary on the right: end-of-line or non-letter.
	if idx+4 < len(line) {
		next := line[idx+4]
		if isLetter(byte(next)) || isDigit(byte(next)) {
			return false
		}
	}
	return true
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// extractMarkdownLinks returns every link target found in `body`. It
// handles the standard `[text](target)` form. Inline code spans are
// ignored so that example links inside fenced code blocks do not get
// flagged — we use a simple "drop lines starting with three or more
// backticks" heuristic.
func extractMarkdownLinks(body string) []string {
	var out []string
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		// Find every [..](..) on the line.
		i := 0
		for {
			open := strings.Index(line[i:], "[")
			if open < 0 {
				break
			}
			open += i
			close := strings.Index(line[open:], "](")
			if close < 0 {
				break
			}
			close += open
			end := strings.Index(line[close+2:], ")")
			if end < 0 {
				break
			}
			end += close + 2
			target := line[close+2 : end]
			out = append(out, target)
			i = end + 1
		}
	}
	return out
}

// ── Injection ──────────────────────────────────────────────────────────

// InjectRoot appends (or replaces) the dox block at the end of
// `agentsPath`. The block is delimited by BeginMarker / EndMarker so
// it can coexist with other managed blocks in the same AGENTS.md
// (e.g. the `<!-- SIN-Code superpowers:begin/end -->` block owned by
// the superpowers tool). Idempotent: subsequent calls with the same
// `body` produce the same file content byte-for-byte.
//
// If `agentsPath` does not exist, it is created. Parent directories
// are NOT created — callers should ensure the path is valid.
func InjectRoot(agentsPath, body string) error {
	if agentsPath == "" {
		return ErrEmptyPath
	}
	// block is the canonical dox block, always terminated by exactly
	// one trailing newline. The leading and trailing "\n" around the
	// body ensure the block is visually separated from surrounding
	// text in the rendered markdown.
	block := BeginMarker + "\n" + body + "\n" + EndMarker + "\n"
	existing, err := os.ReadFile(agentsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content := string(existing)
	// Normalize: strip the previous dox block (if any) so the
	// replace path produces byte-identical output to the
	// initial-append path. We trim ALL trailing newlines first,
	// then drop a single optional "\n" separator that we own
	// (the blank line between the previous content and the
	// dox-managed region), then re-append exactly one "\n".
	trimmed := strings.TrimRight(content, "\n")
	if i := strings.Index(trimmed, BeginMarker); i >= 0 {
		head := trimmed[:i]
		// Drop a single trailing "\n" we previously inserted as
		// a separator, but only if the original content ended with
		// newlines. Otherwise we would erase the user's final
		// newline of prose.
		head = strings.TrimRight(head, "\n")
		trimmed = head
	}
	trimmed += "\n"
	content = trimmed + block
	return os.WriteFile(agentsPath, []byte(content), DefaultFileMode)
}

// RemoveBlock strips the dox block from `agentsPath` (no-op if the
// block is not present). Returns the number of bytes removed.
func RemoveBlock(agentsPath string) (int, error) {
	if agentsPath == "" {
		return 0, ErrEmptyPath
	}
	existing, err := os.ReadFile(agentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	content := string(existing)
	i := strings.Index(content, BeginMarker)
	if i < 0 {
		return 0, nil
	}
	j := strings.Index(content, EndMarker)
	if j < 0 || j < i {
		return 0, nil
	}
	end := j + len(EndMarker)
	// Trim a single trailing newline that was part of the block.
	if end < len(content) && content[end] == '\n' {
		end++
	}
	removed := end - i
	newContent := content[:i] + content[end:]
	if err := os.WriteFile(agentsPath, []byte(newContent), DefaultFileMode); err != nil {
		return 0, err
	}
	return removed, nil
}

// ── Scaffold ───────────────────────────────────────────────────────────

// Scaffold creates a new child node named `name` under `parent`,
// writes a minimal AGENTS.md (or INDEX.md) into it, and registers the
// child in the parent's INDEX.md (or AGENTS.md) so the tree stays
// self-maintaining. Returns the absolute path of the new directory.
//
// `title` is the human title embedded in the new file's frontmatter
// (falls back to `name` if empty). If the directory already exists,
// Scaffold returns ErrAlreadyExists.
func Scaffold(parent, name, title string) (string, error) {
	if parent == "" || name == "" {
		return "", ErrEmptyPath
	}
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(absParent); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrNotADirectory, absParent)
	}
	child := filepath.Join(absParent, name)
	if info, err := os.Stat(child); err == nil && info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrAlreadyExists, child)
	}
	if err := os.MkdirAll(child, DefaultDirMode); err != nil {
		return "", err
	}
	if title == "" {
		title = name
	}
	body := "---\n" +
		"title: " + title + "\n" +
		"---\n\n" +
		"# " + title + "\n\n" +
		"Describe this node. Sub-trees are auto-discovered by `sin-code dox check`.\n"
	// Pick INDEX.md for non-root children, AGENTS.md for the root.
	target := IndexFileName
	if isRootSibling(absParent) {
		target = AgentsFileName
	}
	if err := os.WriteFile(filepath.Join(child, target), []byte(body), DefaultFileMode); err != nil {
		return "", err
	}
	// Register the child in the parent's index. We append (not
	// replace) — re-running scaffold is a no-op once the entry exists.
	if err := registerInParent(absParent, name, title); err != nil {
		return child, err
	}
	return child, nil
}

// isRootSibling reports whether `parent` is the root of a dox tree
// (i.e. contains an AGENTS.md). We only auto-promote the child to
// AGENTS.md when the parent itself is a root, so nested trees stay
// pure.
func isRootSibling(parent string) bool {
	_, err := os.Stat(filepath.Join(parent, AgentsFileName))
	return err == nil
}

// registerInParent appends a `- [name](name/<file>) — title` bullet
// to the parent's INDEX.md (or AGENTS.md at the root). Idempotent:
// re-registration is a no-op when the link is already present. The
// child file name matches what Scaffold wrote — AGENTS.md for root
// children (parent is a root sibling), INDEX.md otherwise.
func registerInParent(parent, name, title string) error {
	isRoot := isRootSibling(parent)
	target := IndexFileName
	if isRoot {
		target = AgentsFileName
	}
	childFile := IndexFileName
	if isRoot {
		childFile = AgentsFileName
	}
	indexPath := filepath.Join(parent, target)
	linkNeedle := "[" + name + "](" + name + "/" + childFile + ")"
	existing, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No index yet — create a minimal one with this child.
			seed := "---\ntitle: " + filepath.Base(parent) + "\n---\n\n" +
				"# " + filepath.Base(parent) + "\n\n" +
				"## Children\n\n" +
				"- " + linkNeedle + " — " + title + "\n"
			return os.WriteFile(indexPath, []byte(seed), DefaultFileMode)
		}
		return err
	}
	body := string(existing)
	if strings.Contains(body, linkNeedle) {
		return nil // already registered
	}
	var b strings.Builder
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("- ")
	b.WriteString(linkNeedle)
	b.WriteString(" — ")
	b.WriteString(title)
	b.WriteByte('\n')
	return os.WriteFile(indexPath, []byte(b.String()), DefaultFileMode)
}

// ── Renderer ───────────────────────────────────────────────────────────

// RenderTree returns a human-readable ASCII tree for `root`. The
// output uses box-drawing characters. Returns the tree as a single
// string with a trailing newline.
func RenderTree(root string) (string, error) {
	tree, err := Build(root)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	renderNode(&b, tree, "", true)
	return b.String(), nil
}

func renderNode(b *strings.Builder, n *Node, prefix string, isLast bool) {
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix == "" {
		// Root: no connector, just the name.
		title := n.Title
		if title == "" {
			title = n.Name
		}
		fmt.Fprintf(b, "%s/\n", title)
	} else {
		title := n.Title
		if title == "" {
			title = n.Name
		}
		leafTag := ""
		if n.IsLeaf {
			leafTag = "  (leaf)"
		}
		fmt.Fprintf(b, "%s%s%s%s\n", prefix, connector, title, leafTag)
	}
	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	} else {
		childPrefix = "    "
	}
	for i, c := range n.Children {
		renderNode(b, c, childPrefix, i == len(n.Children)-1)
	}
}
