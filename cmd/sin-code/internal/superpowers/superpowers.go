// SPDX-License-Identifier: MIT
// Purpose: superpowers integration — clone obra/superpowers, pin the commit
// hash for supply-chain integrity, parse SKILL.md frontmatter, append a
// SIN-Code overlay (idempotent), and register the stdio MCP server.
// Also contains a minimal YAML frontmatter parser for SKILL.md files
// (plain scalars, quoted strings, folded/literal block scalars).
// Docs: superpowers.doc.md, frontmatter.doc.md
package superpowers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// ── Public configuration constants ─────────────────────────────────────

// DefaultRepoURL is the upstream obra/superpowers repository. The variable
// is package-level (not a const) so tests can override it to a local fixture
// path or a hermetic mirror.
var DefaultRepoURL = "https://github.com/obra/superpowers.git"

// DefaultBranch is the upstream branch tracked when the user has not
// requested a specific tag/ref. obra/superpowers keeps the stable skill
// corpus on `main`.
const DefaultBranch = "main"

// testHook variables expose hard-to-reach error paths to the test suite
// without heavy refactoring. They are restored per-test via t.Cleanup.
var (
	osUserHomeDir     = os.UserHomeDir
	osMkdirAll        = os.MkdirAll
	osStat            = os.Stat
	osReadFile        = os.ReadFile
	osWriteFile       = os.WriteFile
	osCreateTemp      = os.CreateTemp
	osRenameHook      = os.Rename
	jsonMarshalIndent = json.MarshalIndent
	jsonUnmarshal     = json.Unmarshal
	ioCopyHook        = io.Copy
	fileWriteHook     = func(f *os.File, p []byte) (int, error) { return f.Write(p) }
	fileCloseHook     = func(f *os.File) error { return f.Close() }
	runGitHook        = runGit
	currentShaHook    = currentSHA
	currentBranchHook = currentBranch
	writePromptHook   = WritePrompt
	walkDirHook       = filepath.WalkDir
)

// OverlayMarker is the sentinel HTML comment that delimiters the
// automatically-appended overlay block. Idempotency: if the marker is
// already present in a SKILL.md, AppendOverlay is a no-op for that file.
const OverlayMarker = "<!-- SIN-Code overlay:begin -->"

// OverlayMarkerEnd closes the block.
const OverlayMarkerEnd = "<!-- SIN-Code overlay:end -->"

// Home resolves $SIN_CODE_HOME (preferred) or falls back to the legacy
// ~/.local/share/sin-code path. The Install/Update pipeline creates
// $Home/skills/superpowers/ on first run.
func Home() string {
	if v := os.Getenv("SIN_CODE_HOME"); v != "" {
		return v
	}
	// Use os.UserHomeDir for cross-platform safety (macOS/Linux/Windows).
	if h, err := osUserHomeDir(); err == nil {
		return filepath.Join(h, ".local", "share", "sin-code")
	}
	// Last resort: cwd-relative fallback so the function is total.
	return filepath.Join(".", ".sin-code-home")
}

// SkillsDir is the per-skill checkout root, e.g. $Home/skills/superpowers.
func SkillsDir() string {
	return filepath.Join(Home(), "skills", "superpowers")
}

// PinFile records the exact commit SHA currently in use. Supply-chain
// pinning means "what we shipped is reproducible from this hash".
func PinFile() string {
	return filepath.Join(Home(), "skills", "superpowers", ".sin-code-pin")
}

// MCPConfigPath is where the superpowers MCP server is registered so the
// SIN-Code runtime (`sin-code serve` / mcpclient) can launch it on demand.
// Per spec: $SIN_CODE_HOME/mcp.json (the home root), NOT a workspace-local
// .sin-code/mcp.json.
func MCPConfigPath() string {
	return filepath.Join(Home(), "mcp.json")
}

// PROMPTFile is the file the agent (and human) reads to see the current
// system-prompt block that lists all installed superpowers skills.
func PROMPTFile() string {
	return filepath.Join(Home(), "skills", "superpowers", "PROMPT.md")
}

// SkillInfo is a lightweight summary of one skill discovered on disk.
type SkillInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
	// SHA256 of the SKILL.md body (post-overlay) — stable identity for cache
	// invalidation and for embedding into AGENTS.md as a "what changed" tag.
	Hash string `json:"hash"`
}

// PinState is the JSON shape of the .sin-code-pin file.
type PinState struct {
	SHA       string    `json:"sha"`
	Branch    string    `json:"branch"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ── Install / Update / Pin ─────────────────────────────────────────────

// InstallResult is what Install returns to the CLI / MCP layer.
type InstallResult struct {
	Repo     string    `json:"repo"`
	SHA      string    `json:"sha"`
	Branch   string    `json:"branch"`
	PinFile  string    `json:"pin_file"`
	Skills   int       `json:"skills"`
	Synced   time.Time `json:"synced_at"`
	Duration string    `json:"duration"`
}

// Install clones (or pulls) the configured repo, resolves the pinned
// commit SHA, applies the overlay to every SKILL.md, and writes PROMPT.md.
// network: the function shells out to `git`; tests swap DefaultRepoURL to
// a local file:// URL to stay fully hermetic.
func Install(ctx context.Context, repoURL, branch string) (*InstallResult, error) {
	if repoURL == "" {
		repoURL = DefaultRepoURL
	}
	if branch == "" {
		branch = DefaultBranch
	}
	start := time.Now()
	dst := SkillsDir()
	if err := osMkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	if _, err := osStat(filepath.Join(dst, ".git")); err == nil {
		if err := runGitHook(ctx, dst, "fetch", "--depth", "1", "origin", branch); err != nil {
			return nil, err
		}
		if err := runGitHook(ctx, dst, "reset", "--hard", "FETCH_HEAD"); err != nil {
			return nil, err
		}
	} else {
		if err := osMkdirAll(dst, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir dst: %w", err)
		}
		if err := runGitHook(ctx, ".", "clone", "--depth", "1", "--branch", branch, repoURL, dst); err != nil {
			return nil, err
		}
	}
	sha, err := currentShaHook(ctx, dst)
	if err != nil {
		return nil, err
	}
	// Apply overlay + generate PROMPT.md (best-effort: count but don't fail
	// the whole install if PROMPT.md can't be rendered).
	infos, _ := List(SkillsDir())
	for i := range infos {
		_ = AppendOverlay(infos[i].Path)
	}
	if _, err := writePromptHook(infos); err != nil {
		return nil, err
	}
	// Write pin file.
	pin := PinState{SHA: sha, Branch: branch, UpdatedAt: time.Now().UTC()}
	if err := WriteJSON(PinFile(), pin); err != nil {
		return nil, err
	}
	return &InstallResult{
		Repo:     repoURL,
		SHA:      sha,
		Branch:   branch,
		PinFile:  PinFile(),
		Skills:   len(infos),
		Synced:   pin.UpdatedAt,
		Duration: time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

// Pin records a specific commit SHA as the active superpowers version
// without performing a network fetch. It does a local `git reset --hard` to
// the requested SHA, which requires that the SHA is already present in the
// object database (typically satisfied by a prior Install).
func Pin(ctx context.Context, sha string) (*PinState, error) {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return nil, errors.New("pin: empty sha")
	}
	dst := SkillsDir()
	if _, err := osStat(filepath.Join(dst, ".git")); err != nil {
		return nil, fmt.Errorf("pin: not installed (%s missing .git): %w", dst, err)
	}
	if err := runGitHook(ctx, dst, "reset", "--hard", sha); err != nil {
		return nil, err
	}
	branch, _ := currentBranchHook(ctx, dst)
	state := PinState{SHA: sha, Branch: branch, UpdatedAt: time.Now().UTC()}
	if err := WriteJSON(PinFile(), state); err != nil {
		return nil, err
	}
	// Re-apply overlay (cheap, idempotent) and regenerate PROMPT.md.
	infos, _ := List(dst)
	for i := range infos {
		_ = AppendOverlay(infos[i].Path)
	}
	if _, err := writePromptHook(infos); err != nil {
		return nil, err
	}
	return &state, nil
}

// CurrentPin reads the .sin-code-pin file. Returns (nil, nil) if the
// caller has not run Install yet — that is NOT an error, just "not pinned".
func CurrentPin() (*PinState, error) {
	b, err := osReadFile(PinFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var p PinState
	if err := jsonUnmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ── Discovery ─────────────────────────────────────────────────────────

// List walks the skills root and returns every skill that contains a
// SKILL.md. Sorted by name for stable output. If root is empty,
// SkillsDir() is used.
func List(root string) ([]SkillInfo, error) {
	if root == "" {
		root = SkillsDir()
	}
	if _, err := osStat(root); err != nil {
		return nil, nil // not installed → empty result, not an error
	}
	var out []SkillInfo
	err := walkDirHook(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries; do not abort the walk
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		body, rerr := osReadFile(p)
		if rerr != nil {
			return nil
		}
		fm, _ := ParseFrontmatter(string(body))
		name := fm["name"]
		if name == "" {
			// Fall back to parent directory name (obra layout: <skill>/SKILL.md).
			name = filepath.Base(filepath.Dir(p))
		}
		out = append(out, SkillInfo{
			Name:        name,
			Path:        p,
			Description: fm["description"],
			Hash:        sha256Hex(body),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get resolves a single skill by exact name match.
func Get(name string) (*SkillInfo, error) {
	all, err := List("")
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("superpowers: skill %q not found", name)
}

// Find performs a case-insensitive substring search across name +
// description. Returns up to maxResults (0 means "all").
func Find(query string, maxResults int) ([]SkillInfo, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	all, err := List("")
	if err != nil {
		return nil, err
	}
	var hits []SkillInfo
	for _, s := range all {
		if strings.Contains(strings.ToLower(s.Name), q) ||
			strings.Contains(strings.ToLower(s.Description), q) {
			hits = append(hits, s)
			if maxResults > 0 && len(hits) >= maxResults {
				break
			}
		}
	}
	return hits, nil
}

// ── AGENTS.md injection ───────────────────────────────────────────────

// InjectAGENTS appends (or replaces) the superpowers block at the end of
// the given AGENTS.md path. Idempotent: if the marker is present, the
// existing block is replaced in place.
func InjectAGENTS(agentsPath string, prompt string) error {
	if agentsPath == "" {
		return errors.New("InjectAGENTS: empty path")
	}
	const start = "<!-- SIN-Code superpowers:begin -->"
	const end = "<!-- SIN-Code superpowers:end -->"
	block := start + "\n" + prompt + "\n" + end + "\n"
	existing, err := osReadFile(agentsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body := string(existing)
	if i := strings.Index(body, start); i >= 0 {
		j := strings.Index(body, end)
		if j >= 0 && j > i {
			body = body[:i] + block + body[j+len(end):]
		} else {
			body = body[:i] + block
		}
	} else {
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += "\n" + block
	}
	return osWriteFile(agentsPath, []byte(body), filemode.Default())
}

// ── Internal helpers ──────────────────────────────────────────────────

func runGit(ctx context.Context, dir string, args ...string) error {
	c, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(c, "git", args...)
	if dir != "" && dir != "." {
		cmd.Dir = dir
	}
	// Suppress interactive prompts; never block on credentials.
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=true",
		"LC_ALL=C",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

func currentSHA(ctx context.Context, dir string) (string, error) {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, "git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func currentBranch(ctx context.Context, dir string) (string, error) {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// WriteJSON marshals v with 2-space indent and writes atomically.
// The parent directory of path is created on demand — callers do not
// need to MkdirAll beforehand.
func WriteJSON(path string, v any) error {
	data, err := jsonMarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := osMkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := osCreateTemp(filepath.Dir(path), ".superpowers-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := ioCopyHook(tmp, strings.NewReader(string(data))); err != nil {
		fileCloseHook(tmp)
		return err
	}
	if _, err := fileWriteHook(tmp, []byte("\n")); err != nil {
		fileCloseHook(tmp)
		return err
	}
	if err := fileCloseHook(tmp); err != nil {
		return err
	}
	return osRenameHook(tmpName, path)
}

// ParseFrontmatter extracts the leading `--- ... ---` block from a
// SKILL.md body and returns its key/value map. Both keys and values are
// returned with surrounding whitespace trimmed. If the body has no
// frontmatter, an empty map is returned and ok=false.
//
// Block-scalar semantics (matters for description):
//
//	>- / >    folded     — newlines become single spaces
//	|- / |    literal    — newlines are preserved verbatim
//
// All other newlines inside a block scalar are treated as content.
func ParseFrontmatter(body string) (map[string]string, bool) {
	out := make(map[string]string)
	// Frontmatter MUST start at byte 0 (with optional leading whitespace).
	trimmed := strings.TrimLeft(body, " \t")
	if !strings.HasPrefix(trimmed, "---") {
		return out, false
	}
	// Skip the opening "---" line, then scan until the closing "---".
	rest := trimmed[3:]
	// Strip a single newline right after the dashes.
	rest = strings.TrimPrefix(rest, "\r\n")
	rest = strings.TrimPrefix(rest, "\n")
	// Find the closing fence. It must appear at column 0 (ignoring leading
	// whitespace on the body), but obra/superpowers always uses column 0.
	end := indexClosingFence(rest)
	if end < 0 {
		return out, false
	}
	block := rest[:end]
	parseBlock(block, out)
	return out, true
}

// indexClosingFence returns the byte index of the next "---" line in body,
// or -1 if none. The fence must be at column 0 (possibly preceded by
// \r\n).
func indexClosingFence(body string) int {
	for i := 0; i < len(body); i++ {
		if body[i] != '-' {
			continue
		}
		// Must be at column 0 or right after a newline.
		if i != 0 && body[i-1] != '\n' {
			continue
		}
		// Now look for "---" followed by EOL or EOF.
		if i+3 > len(body) {
			return -1
		}
		if body[i] == '-' && body[i+1] == '-' && body[i+2] == '-' {
			after := i + 3
			if after == len(body) {
				return i
			}
			c := body[after]
			if c == '\n' || c == '\r' {
				return i
			}
		}
	}
	return -1
}

// parseBlock walks a frontmatter block and fills out. Lines look like:
//
//	name: value
//	name: "quoted value"
//	name: 'quoted value'
//	name: >-
//	  folded value
//	  continues here
//	name: |-
//	  literal value
//	  preserved
func parseBlock(block string, out map[string]string) {
	lines := strings.Split(block, "\n")
	i := 0
	for i < len(lines) {
		raw := lines[i]
		// Skip blank lines and comment-only lines (#).
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			i++
			continue
		}
		// A "key: value" line. The colon must NOT be inside quotes.
		key, val, hasVal, _ := splitKeyValue(raw)
		if key == "" {
			i++
			continue
		}
		if !hasVal {
			// "key:" with empty value.
			out[strings.TrimSpace(key)] = ""
			i++
			continue
		}
		// Detect block scalars.
		head := strings.TrimRight(val, " \t")
		if strings.HasSuffix(head, ">") || strings.HasSuffix(head, ">-") ||
			strings.HasSuffix(head, "+") || strings.HasSuffix(head, "+-") {
			// Folded scalar.
			chomp := head[len(head)-1]
			stripTrailing := strings.HasSuffix(head, "-") || strings.HasSuffix(head, "+-")
			_ = chomp
			indent := leadingSpaces(raw)
			// Block may start on next line.
			startIdx := i + 1
			collected, next := collectBlockLines(lines, startIdx, indent+1, stripTrailing)
			joined := strings.Join(collected, "\n")
			folded := foldScalar(joined, stripTrailing)
			out[strings.TrimSpace(key)] = folded
			i = next
			continue
		}
		if strings.HasSuffix(head, "|") || strings.HasSuffix(head, "|-") ||
			strings.HasSuffix(head, "+") || strings.HasSuffix(head, "+-") {
			indent := leadingSpaces(raw)
			startIdx := i + 1
			stripTrailing := strings.HasSuffix(head, "-") || strings.HasSuffix(head, "+-")
			collected, next := collectBlockLines(lines, startIdx, indent+1, stripTrailing)
			joined := strings.Join(collected, "\n")
			out[strings.TrimSpace(key)] = joined
			i = next
			continue
		}
		// Plain scalar: unquote, trim.
		out[strings.TrimSpace(key)] = unquote(strings.TrimSpace(val))
		i++
	}
}

// splitKeyValue parses a single frontmatter line. It returns:
//   - key:    the trimmed key (empty if no colon found)
//   - value:  the raw value (right of the colon, untouched — caller
//     decides how to interpret block scalars)
//   - hasVal: true if a colon was present
//   - colonIdx: the byte index of the colon in the original line
func splitKeyValue(line string) (key, value string, hasVal bool, colonIdx int) {
	// Skip leading whitespace for the key itself; we still report colonIdx
	// relative to the original line so indent detection works.
	trimmed := line
	leading := 0
	for leading < len(trimmed) && (trimmed[leading] == ' ' || trimmed[leading] == '\t') {
		leading++
	}
	trimmed = trimmed[leading:]
	colon := -1
	inS, inD := false, false
	for i, r := range trimmed {
		switch r {
		case '\'':
			if !inD {
				inS = !inS
			}
		case '"':
			if !inS {
				inD = !inD
			}
		case ':':
			if inS || inD {
				continue
			}
			// Colon must be followed by space, EOL, or end-of-line.
			next := byte(' ')
			if i+1 < len(trimmed) {
				next = trimmed[i+1]
			}
			if next == ' ' || next == '\t' || next == '\r' || next == '\n' || i == len(trimmed)-1 {
				colon = i
			}
		}
		if colon >= 0 {
			break
		}
	}
	if colon < 0 {
		return "", "", false, 0
	}
	keyPart := trimmed[:colon]
	valPart := trimmed[colon+1:]
	return keyPart, valPart, valPart != "", leading + colon
}

// leadingSpaces returns the count of leading space/tab runes in line.
func leadingSpaces(line string) int {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return len(line)
}

// unquote strips a single layer of matched ' or " quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// collectBlockLines returns the run of consecutive lines whose indent is
// strictly greater than baseIndent, plus the index of the first line that
// did NOT match (i.e. the next thing the outer loop should process).
// If stripTrailing is true (YAML "strip" chomping indicator: -), the
// trailing empty/blank lines are removed.
//
// YAML semantics: the block scalar's "indentation indicator" is the
// difference between the parent key's column and the first non-blank line
// of the block. We detect that indicator on the fly and strip it from
// every subsequent line.
func collectBlockLines(lines []string, start, baseIndent int, stripTrailing bool) ([]string, int) {
	var collected []string
	j := start
	blockIndent := -1
	for j < len(lines) {
		l := lines[j]
		// Blank line: include as content, keep scanning.
		if strings.TrimSpace(l) == "" {
			collected = append(collected, "")
			j++
			continue
		}
		ind := leadingSpaces(l)
		if ind <= baseIndent {
			break
		}
		if blockIndent < 0 {
			blockIndent = ind
		}
		stripped := l
		if len(stripped) >= blockIndent {
			stripped = stripped[blockIndent:]
		}
		collected = append(collected, stripped)
		j++
	}
	// Eat trailing blank lines if stripTrailing.
	if stripTrailing {
		for len(collected) > 0 && strings.TrimSpace(collected[len(collected)-1]) == "" {
			collected = collected[:len(collected)-1]
		}
	}
	return collected, j
}

// foldScalar converts a folded block scalar body to its string form:
//   - newlines become single spaces
//   - runs of multiple newlines ("\n\n") collapse to a single space too
//     (matches the YAML folded-scalar rule: blank line → space)
//   - leading/trailing whitespace is trimmed from the final result
func foldScalar(body string, stripTrailing bool) string {
	if stripTrailing {
		body = strings.TrimRight(body, " \t\n")
	}
	var b strings.Builder
	prevSpace := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\n' || c == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			// Eat a paired \r\n.
			if c == '\r' && i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
			continue
		}
		// Collapse multiple internal spaces/tabs into one.
		if c == ' ' || c == '\t' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteByte(c)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

// trimUnicode is a small helper used by tests / callers that want to
// collapse internal whitespace runs in a fold result.
func trimUnicode(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
}
