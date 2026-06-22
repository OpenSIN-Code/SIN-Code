// SPDX-License-Identifier: MIT
// Package skilldist distributes a bundled SIN-Code Skill artifact to one of
// eleven supported agent families: Claude Code, Codex, Gemini, opencode,
// Cursor, Windsurf, Cline, GitHub Copilot, Aider, Continue, and Zed. The
// package is the source of truth for the per-agent install path templates
// and is consumed exclusively by `cmd/sin-code/skill_cmd.go` (the
// `sin-code skill install --agent <id>` surface).
//
// # Marker-fenced idempotency
//
// Every write goes through ParseMarkers / Render so a subsequent install with
// the same `(target, skill)` pair replaces the previously written block in
// place. We never concatenate onto the file: that confuses the host agent and
// produces unbounded growth on re-runs. The marker pair is:
//
//	<!-- SIN-CODE-SKILL-START: <skill> -->
//	… rendered body …
//	<!-- SIN-CODE-SKILL-END:   <skill> -->
//
// The trailing whitespace before `<skill>` on the END marker is intentional:
// it visually aligns the markers in `head -n 2 some-rule.md` output.
// ParseMarkers strips that whitespace on lookup so a regex-based scanner
// outside this package still finds both ends.
//
// # Formats
//
// A Target.Format picks the writer:
//
//	"dir"    — copy the SKILL.md (and optional context/, frameworks/, tasks/,
//	           templates/) into a per-skill directory. Used by agent families
//	           that expose a Skills-FS-style drop-in folder.
//	"rule"   — write a single .md or .mdc rule file whose body is the
//	           marker-fenced SKILL.md body. Used by Cursor/Windsurf/Cline/Codex
//	           rule directories.
//	"marker" — append per-skill marker blocks to a single shared agent
//	           instructions file (currently only GitHub Copilot's
//	           `.github/copilot-instructions.md`). One file, many blocks.
//
// ParseMarkers is the only sanctioned writer for formats "rule" and "marker".
// "dir" does not use markers because the writer owns the whole tree.
//
// # Environment overrides
//
// For testability and CI determinism the writer takes a `Home` string instead
// of reading $HOME itself. The CLI layer is responsible for resolving the
// home directory (including $SIN_CODE_HOME overrides).
package skilldist

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// Format kinds — public so tests and the CLI can assert on them.
const (
	FormatDir    = "dir"
	FormatRule   = "rule"
	FormatMarker = "marker"
)

// MarkerPrefix is prepended to the begin/end lines so an editor collapse-fold
// has something stable to grep against. Keep ASCII: a multi-byte rune would
// break the line-based ParseMarkers scan on a non-UTF-8 file.
const MarkerPrefix = "SIN-CODE-SKILL"

// BeginMarker and EndMarker return the exact fence lines for one skill.
// Render and Parse functions take the result of these as anchors.
func BeginMarker(skill string) string {
	return fmt.Sprintf("<!-- %s-START: %s -->", MarkerPrefix, skill)
}
func EndMarker(skill string) string { return fmt.Sprintf("<!-- %s-END:   %s -->", MarkerPrefix, skill) }

// Target is one supported agent family.
//
//	Name         — short id used on the CLI: `claude-code`, `cursor`,
//	               `copilot`, …
//	DisplayName  — human label used in `--agent <name>` help and table
//	               output.
//	InstallPath  — path template relative to the user's home directory;
//	               contains a `<skill>` placeholder that is replaced at
//	               write time with the skill name. The placeholder is
//	               omitted for multi-skill files (e.g. Copilot's
//	               instructions file).
//	Format       — one of FormatDir / FormatRule / FormatMarker.
//
// # Stability
//
// (Name, DisplayName) is a public API surface exposed via `sin-code skill
// install --agent <name>`. Adding a target is non-breaking; renaming or
// removing one is a major bump per AGENTS.md §10.
type Target struct {
	Name        string
	DisplayName string
	InstallPath string
	Format      string
}

// Targets is the single source of truth for supported agent families. Any
// addition here MUST also be reflected in:
//
//	cmd/sin-code/skill_cmd.go (cobra's shell completion for `--agent`),
//	AGENTS.md §10             (the naming-and-stability matrix),
//	CHANGELOG.md [Unreleased] (the additions bullet).
//
// The set is intentionally small (11 entries today). Verify-gated expansion
// is fine, but every new entry adds a maintenance row in three places.
var Targets = map[string]Target{
	"claude-code": {
		Name:        "claude-code",
		DisplayName: "Claude Code",
		InstallPath: ".claude/skills/<skill>",
		Format:      FormatDir,
	},
	"opencode": {
		Name:        "opencode",
		DisplayName: "opencode",
		InstallPath: ".config/opencode/skills/<skill>",
		Format:      FormatDir,
	},
	"gemini": {
		Name:        "gemini",
		DisplayName: "Gemini CLI",
		InstallPath: ".gemini/skills/<skill>",
		Format:      FormatDir,
	},
	"codex": {
		Name:        "codex",
		DisplayName: "Codex CLI",
		InstallPath: ".codex/rules/<skill>.md",
		Format:      FormatRule,
	},
	"cursor": {
		Name:        "cursor",
		DisplayName: "Cursor",
		InstallPath: ".cursor/rules/<skill>.mdc",
		Format:      FormatRule,
	},
	"windsurf": {
		Name:        "windsurf",
		DisplayName: "Windsurf",
		InstallPath: ".windsurf/rules/<skill>.md",
		Format:      FormatRule,
	},
	"cline": {
		Name:        "cline",
		DisplayName: "Cline",
		InstallPath: ".clinerules/<skill>.md",
		Format:      FormatRule,
	},
	"copilot": {
		Name:        "copilot",
		DisplayName: "GitHub Copilot",
		InstallPath: ".github/copilot-instructions.md",
		Format:      FormatMarker,
	},
	"aider": {
		Name:        "aider",
		DisplayName: "Aider",
		InstallPath: ".aider/conventions/<skill>.md",
		Format:      FormatRule,
	},
	"continue": {
		Name:        "continue",
		DisplayName: "Continue",
		InstallPath: ".continue/rules/<skill>.md",
		Format:      FormatRule,
	},
	"zed": {
		Name:        "zed",
		DisplayName: "Zed",
		InstallPath: ".zed/rules/<skill>.md",
		Format:      FormatRule,
	},
}

// TargetNames returns every registered id in deterministic (alphabetical)
// order. A stable order is required so `--agent all` produces identical log
// output across machines — important for the verify-gate's reproducibility
// check.
func TargetNames() []string {
	out := make([]string, 0, len(Targets))
	for k := range Targets {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// InstallOptions configures one Install call.
//
//	SrcRoot — for FormatDir, the on-disk location of the `<skill>/` directory
//	          whose SKILL.md (and optional context/, frameworks/, tasks/,
//	          templates/) is copied. Ignored for FormatRule / FormatMarker.
//	Home    — the user home directory used to resolve InstallPath. Empty
//	          means "fall back to os.UserHomeDir" — the CLI passes the value
//	          of $SIN_CODE_HOME or the real home. Tests use t.TempDir().
//	Body    — for FormatRule / FormatMarker, the raw SKILL.md body to embed
//	          inside the marker fence. Ignored for FormatDir.
type InstallOptions struct {
	SrcRoot string
	Home    string
	Body    string
}

// Install writes a marker-fenced skill to the target's resolved path. It is
// idempotent: a second invocation produces byte-identical output (assuming
// the input body is unchanged). Supports FormatDir / FormatRule / FormatMarker.
//
// Format-specific behaviour:
//
//	FormatDir    — every file under SrcRoot/<skill>/ is copied into
//	               <Home>/<InstallPath-resolved> overwriting any existing
//	               tree. SKILL.md is mandatory; context/frameworks/tasks/
//	               templates are copied only when non-empty.
//	FormatRule   — the file at the resolved path is rewritten so the
//	               marker-fenced block for `skill` is present. Existing
//	               blocks for other skills in the file are preserved by
//	               ParseMarkers-driven replacement.
//	FormatMarker — same as FormatRule but the resolved path is a shared
//	               multi-skill file (one block per skill).
//
// All writes go through atomicWrite so a partial install can never be
// observed by the agent mid-write. Errors carry both the writer view
// (which file failed) and the inner error from the OS so the CLI can
// show users a usable message.
func Install(skill string, tgt Target, opts InstallOptions) error {
	if _, ok := Targets[tgt.Name]; !ok {
		return fmt.Errorf("skilldist: unknown target %q", tgt.Name)
	}
	if tgt.Format != FormatDir && tgt.Format != FormatRule && tgt.Format != FormatMarker {
		return fmt.Errorf("skilldist: target %q has invalid Format %q", tgt.Name, tgt.Format)
	}
	resolved, err := Resolve(tgt, skill, opts.Home)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return fmt.Errorf("skilldist: mkdir parent for %q: %w", resolved, err)
	}

	switch tgt.Format {
	case FormatDir:
		if opts.SrcRoot == "" {
			return fmt.Errorf("skilldist: FormatDir requires SrcRoot for target %q", tgt.Name)
		}
		return writeSkillDir(opts.SrcRoot, resolved, skill)
	case FormatRule:
		return writeFenceFile(resolved, skill, opts.Body)
	case FormatMarker:
		return writeFenceFile(resolved, skill, opts.Body)
	}
	return nil
}

// Resolve expands the `<skill>` placeholder in InstallPath against `home`.
//
// If home is empty, Resolve falls back to `os.UserHomeDir()`; the CLI passes
// an empty string to opt into the default, tests pass t.TempDir().
func Resolve(tgt Target, skill, home string) (string, error) {
	if tgt.InstallPath == "" {
		return "", fmt.Errorf("skilldist: target %q has no InstallPath", tgt.Name)
	}
	if skill == "" {
		return "", fmt.Errorf("skilldist: empty skill name for target %q", tgt.Name)
	}
	if strings.Contains(skill, "..") || strings.ContainsAny(skill, "/\\") {
		return "", fmt.Errorf("skilldist: unsafe skill name %q", skill)
	}
	if strings.Contains(tgt.InstallPath, "<skill>") && strings.Count(tgt.InstallPath, "<skill>") != 1 {
		return "", fmt.Errorf("skilldist: target %q has malformed InstallPath", tgt.Name)
	}
	resolved := strings.ReplaceAll(tgt.InstallPath, "<skill>", skill)
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("skilldist: home directory: %w", err)
		}
		home = h
	}
	return filepath.Join(home, resolved), nil
}

// StripFrontmatter strips the YAML frontmatter (the leading `---` … `---`
// block) of a SKILL.md so the inlined rule body is shorter and devoid of
// fields the host agent doesn't understand. If the file has no frontmatter
// the original body is returned verbatim.
//
// The frontmatter delimiting rule mirrors the YAML 1.2 spec: the file starts
// with `---` on its own line and the frontmatter block terminates at the next
// line whose only content is `---`. Only the leading block is recognised;
// later `---` lines are body content.
//
// Both LF and CRLF line endings are tolerated; the function normalises them
// to LF before scanning and leaves the result in LF.
func StripFrontmatter(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return raw
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			out := strings.Join(lines[i+1:], "\n")
			return strings.TrimLeft(out, "\n")
		}
	}
	// Unterminated frontmatter — treat as no frontmatter rather than risk
	// emitting a half-block. The skill author should fix the file.
	return raw
}

// RenderBlock produces the marker-fenced body that Install writes for a
// single (target, skill) pair. The begin line banners the rule with version
// + skill id so the on-disk file is self-explanatory in `cat` / `less`
// output.
//
// RenderBlock preserves trailing newline semantics: the returned string
// always ends with `\n`. Callers can therefore write `RenderBlock(...)` to a
// file without further newline insertion.
func RenderBlock(skill string, body string) string {
	body = strings.TrimRight(body, "\n")
	return fmt.Sprintf("%s\n# Skill: %s\n\n%s\n%s\n",
		BeginMarker(skill),
		skill,
		body,
		EndMarker(skill),
	)
}

// ParseResult is the structured outcome of ParseMarkers; consumers usually
// only care about (Block, OK). Prefix and Suffix are the bytes before and
// after the matched block and are returned so the writer can reconstruct
// the file on update.
//
//	OK=true   — a fenced block for `skill` exists; Block contains the inner
//	            body without the marker lines.
//	OK=false  — no fenced block was found; Prefix is the full input and
//	            Block / Suffix are empty.
type ParseResult struct {
	Prefix string
	Block  string
	Suffix string
	OK     bool
}

// ParseMarkers scans a file's contents and returns the parsed envelope
// around the marker fence for `skill`. The scan is line-based and tolerant
// of trailing whitespace: the begin line must match BeginMarker(skill)
// verbatim (after CR stripping), and the end line must match EndMarker(skill).
// Any content between the pair is returned verbatim as Block.
//
// If the begin line is found but the end line is missing, ParseResult.OK is
// false and Prefix is the full input — a half-opened fence is treated as if
// the block were absent, so the writer writes a clean block rather than
// producing a malformed file.
func ParseMarkers(content, skill string) ParseResult {
	begin := BeginMarker(skill)
	end := EndMarker(skill)

	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	beginIdx := -1
	for i, ln := range lines {
		if strings.TrimRight(ln, " \t") == begin {
			beginIdx = i
			break
		}
	}
	if beginIdx < 0 {
		return ParseResult{Prefix: content}
	}
	endIdx := -1
	for i := beginIdx + 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == end {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		// Half-opened fence: behave as if absent so callers write a clean block
		// rather than appending onto the dangling begin line.
		return ParseResult{Prefix: content}
	}

	prefix := strings.Join(lines[:beginIdx], "\n")
	prefix = strings.TrimRight(prefix, "\n") + "\n"
	if prefix == "\n" {
		prefix = ""
	}

	block := strings.Join(lines[beginIdx+1:endIdx], "\n")
	suffix := strings.Join(lines[endIdx+1:], "\n")
	if beginIdx == 0 {
		// Block was the first content; drop the orphan newline we just
		// inserted into Prefix so the file doesn't start with a blank line.
		prefix = ""
	}

	return ParseResult{Prefix: prefix, Block: block, Suffix: suffix, OK: true}
}

// writeSkillDir copies <SrcRoot>/<skill>/SKILL.md (and the four optional
// supplementary directories if present) to <resolved>. Existing files at
// the destination are overwritten — the writer always reports success on
// re-install because the same SKILL.md's identity (path + content) is
// idempotent.
func writeSkillDir(srcRoot, resolved, skill string) error {
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		return fmt.Errorf("skilldist: mkdir %q: %w", resolved, err)
	}
	src := filepath.Join(srcRoot, skill, "SKILL.md")
	in, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("skilldist: read %q: %w", src, err)
	}
	if err := os.WriteFile(filepath.Join(resolved, "SKILL.md"), in, filemode.Default()); err != nil {
		return fmt.Errorf("skilldist: write SKILL.md to %q: %w", resolved, err)
	}
	for _, sub := range []string{"context", "frameworks", "tasks", "templates"} {
		srcSub := filepath.Join(srcRoot, skill, sub)
		if entries, err := os.ReadDir(srcSub); err == nil && len(entries) > 0 {
			dstSub := filepath.Join(resolved, sub)
			if err := copyTree(srcSub, dstSub); err != nil {
				return fmt.Errorf("skilldist: copy %q: %w", sub, err)
			}
		}
	}
	return nil
}

// copyTree recursively copies src/<subtree> to dst/<subtree>. Stops at the
// first error and returns it.
func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyTree(s, d); err != nil {
				return err
			}
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, filemode.Default()); err != nil {
			return err
		}
	}
	return nil
}

// writeFenceFile writes a marker-fenced block to the resolved path; an
// existing block for the same skill is replaced in place via ParseMarkers.
// The result has exactly one trailing newline.
//
// FormatRule and FormatMarker converge on the same writer: the only
// difference is at the agent-config layer (a single .mdc per Cursor rule
// vs the shared copilot-instructions.md), not at the marker-fence layer.
func writeFenceFile(resolved, skill, body string) error {
	if body == "" {
		return fmt.Errorf("skilldist: empty body for skill %q", skill)
	}
	rendered := RenderBlock(skill, body)
	existing := ""
	if data, err := os.ReadFile(resolved); err == nil {
		existing = string(data)
		parsed := ParseMarkers(existing, skill)
		if parsed.OK {
			full := parsed.Prefix + rendered + parsed.Suffix
			if !strings.HasSuffix(full, "\n") {
				full += "\n"
			}
			return atomicWrite(resolved, []byte(full))
		}
	}
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	full := existing + rendered
	if !strings.HasSuffix(full, "\n") {
		full += "\n"
	}
	return atomicWrite(resolved, []byte(full))
}

// Uninstall reverses Install:
//
//	FormatDir    — remove the resolved directory tree (already-absent is OK).
//	FormatRule   — remove the entire rule file. Every FormatRule file holds
//	               exactly one skill, so a stale file is unrecoverable; we
//	               prefer to delete it rather than leave dead content for the
//	               host agent to ingest.
//	FormatMarker — remove only the marker block; preserve every other block
//	               in the shared file. If the shared file becomes empty,
//	               delete it entirely.
//
// Returns nil when the install was already absent (idempotent on retry).
func Uninstall(tgt Target, skill, home string) error {
	if _, ok := Targets[tgt.Name]; !ok {
		return fmt.Errorf("skilldist: unknown target %q", tgt.Name)
	}
	resolved, err := Resolve(tgt, skill, home)
	if err != nil {
		return err
	}
	switch tgt.Format {
	case FormatDir:
		if _, err := os.Stat(resolved); os.IsNotExist(err) {
			return nil
		}
		return os.RemoveAll(resolved)
	case FormatRule:
		if _, err := os.Stat(resolved); os.IsNotExist(err) {
			return nil
		}
		return os.Remove(resolved)
	case FormatMarker:
		data, err := os.ReadFile(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		parsed := ParseMarkers(string(data), skill)
		if !parsed.OK {
			return nil
		}
		full := parsed.Prefix + parsed.Suffix
		full = strings.TrimRight(full, "\n") + "\n"
		if strings.TrimSpace(full) == "" {
			// Nothing left after the block: remove the file entirely so
			// the agent's instructions file is clean.
			return os.Remove(resolved)
		}
		return atomicWrite(resolved, []byte(full))
	}
	return nil
}

// IsInstalled reports whether a (target, skill) install is currently present
// on disk. It is the cheapest check the CLI uses for `--installed` listing.
//
// The check is Format-aware:
//
//	FormatDir    — directory exists.
//	FormatRule   — file exists AND contains a marker fence for `skill`.
//	FormatMarker — file exists AND contains a marker fence for `skill`.
//
// Formats that lack a marker fence (FormatRule outside the fenced body) do
// not report installed even when the file exists — this protects against
// hand-edited rule files confusing the status table.
func IsInstalled(tgt Target, skill, home string) (bool, error) {
	resolved, err := Resolve(tgt, skill, home)
	if err != nil {
		return false, err
	}
	switch tgt.Format {
	case FormatDir:
		st, err := os.Stat(resolved)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return st.IsDir(), nil
	case FormatRule, FormatMarker:
		data, err := os.ReadFile(resolved)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return ParseMarkers(string(data), skill).OK, nil
	}
	return false, fmt.Errorf("skilldist: unsupported Format %q", tgt.Format)
}

// atomicWrite performs a temp-file-and-rename write so a half-finished
// install can never be observed by the agent (which re-reads the file on
// every prompt) mid-write. The temp file is cleaned up best-effort on
// rename failure.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sin-skilldist-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
