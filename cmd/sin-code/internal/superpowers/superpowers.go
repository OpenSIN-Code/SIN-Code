// SPDX-License-Identifier: MIT
// Purpose: superpowers integration — clone obra/superpowers, pin the commit
// hash for supply-chain integrity, parse SKILL.md frontmatter, append a
// SIN-Code overlay (idempotent), and register the stdio MCP server.
// Docs: superpowers.doc.md
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
	if h, err := os.UserHomeDir(); err == nil {
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err == nil {
		if err := runGit(ctx, dst, "fetch", "--depth", "1", "origin", branch); err != nil {
			return nil, err
		}
		if err := runGit(ctx, dst, "reset", "--hard", "FETCH_HEAD"); err != nil {
			return nil, err
		}
	} else {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir dst: %w", err)
		}
		if err := runGit(ctx, ".", "clone", "--depth", "1", "--branch", branch, repoURL, dst); err != nil {
			return nil, err
		}
	}
	sha, err := currentSHA(ctx, dst)
	if err != nil {
		return nil, err
	}
	// Apply overlay + generate PROMPT.md (best-effort: count but don't fail
	// the whole install if PROMPT.md can't be rendered).
	infos, _ := List(SkillsDir())
	for i := range infos {
		_ = AppendOverlay(infos[i].Path)
	}
	if _, err := WritePrompt(infos); err != nil {
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
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		return nil, fmt.Errorf("pin: not installed (%s missing .git): %w", dst, err)
	}
	if err := runGit(ctx, dst, "reset", "--hard", sha); err != nil {
		return nil, err
	}
	branch, _ := currentBranch(ctx, dst)
	state := PinState{SHA: sha, Branch: branch, UpdatedAt: time.Now().UTC()}
	if err := WriteJSON(PinFile(), state); err != nil {
		return nil, err
	}
	// Re-apply overlay (cheap, idempotent) and regenerate PROMPT.md.
	infos, _ := List(dst)
	for i := range infos {
		_ = AppendOverlay(infos[i].Path)
	}
	if _, err := WritePrompt(infos); err != nil {
		return nil, err
	}
	return &state, nil
}

// CurrentPin reads the .sin-code-pin file. Returns (nil, nil) if the
// caller has not run Install yet — that is NOT an error, just "not pinned".
func CurrentPin() (*PinState, error) {
	b, err := os.ReadFile(PinFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var p PinState
	if err := json.Unmarshal(b, &p); err != nil {
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
	if _, err := os.Stat(root); err != nil {
		return nil, nil // not installed → empty result, not an error
	}
	var out []SkillInfo
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries; do not abort the walk
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		body, rerr := os.ReadFile(p)
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
	existing, err := os.ReadFile(agentsPath)
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
	return os.WriteFile(agentsPath, []byte(body), 0o644)
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
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".superpowers-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, strings.NewReader(string(data))); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write([]byte("\n")); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
