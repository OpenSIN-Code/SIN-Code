// SPDX-License-Identifier: MIT
// Per-format writers for the profile renderer.
//
// Render is the public entry point: it takes a Target and the source
// markdown body, returns the rendered bytes the writer will write to
// disk (for dir: a single-file body, for rule: fenced markdown, for
// marker: a marker envelope around the body).
//
// Render is **pure** — no IO, no I/O side effects, deterministic. The
// callers (RenderAll / Install) are responsible for reading the source
// from disk and writing the result. Keeping Render pure means the
// verify-gate can compute the expected SHA from the source alone.
package profile

import (
	"errors"
	"fmt"
	"strings"
)

// StripFrontmatter removes the leading YAML frontmatter (`---` … `---`)
// from `raw`. Mirrors skilldist.StripFrontmatter byte-for-byte so a
// frontmatter pinned in one renders identically in the other.
//
// Both LF and CRLF line endings are tolerated. If the file starts with
// no frontmatter, the body is returned verbatim.
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
	// Unterminated frontmatter — treat as no frontmatter rather than
	// emit a half block. Operator should fix the file.
	return raw
}

// Render returns the bytes that should be written to disk for one
// (target, source-body) pair. The rules:
//
//	FormatDir    — returns `body` verbatim. The dir writer then writes
//	               that body to `<repo>/<InstallPath>` which is itself
//	               the SKILL.md file. We strip leading frontmatter so
//	               the host-agent Skills-FS picks up only the rules.
//	FormatRule   — returns body inside a SIN-CODE-SKILL marker fence.
//	FormatMarker — alias of FormatRule: the file is shared but the
//	               block is bracketed the same way.
func Render(tgt Target, body string) (string, error) {
	if _, ok := Targets[tgt.Name]; !ok {
		return "", fmt.Errorf("profile: unknown target %q", tgt.Name)
	}
	if tgt.Format != FormatDir && tgt.Format != FormatRule && tgt.Format != FormatMarker {
		return "", fmt.Errorf("profile: target %q has invalid Format %q", tgt.Name, tgt.Format)
	}
	if strings.TrimSpace(body) == "" {
		return "", errors.New("profile: empty render body")
	}

	body = StripFrontmatter(body)
	body = strings.TrimRight(body, "\n") + "\n"

	switch tgt.Format {
	case FormatDir:
		return body, nil
	case FormatRule, FormatMarker:
		return RenderBlock(ProfileSkill, body), nil
	}
	return "", fmt.Errorf("profile: unreachable format %q", tgt.Format)
}

// Resolve returns the absolute repo-root path the writer should write
// for `tgt`. The base parameter is the repository root (the writer's
// cwd by default). Skill placeholder is substituted with ProfileSkill.
//
// Resolve refuses unsafe skill names — defense in depth, even though
// the only caller passes ProfileSkill.
func Resolve(tgt Target, base string) (string, error) {
	if tgt.InstallPath == "" {
		return "", fmt.Errorf("profile: target %q has no InstallPath", tgt.Name)
	}
	resolved := strings.ReplaceAll(tgt.InstallPath, "<skill>", ProfileSkill)
	placeholderCount := strings.Count(tgt.InstallPath, "<skill>")
	if placeholderCount > 0 && !strings.Contains(resolved, ProfileSkill) {
		return "", fmt.Errorf("profile: target %q InstallPath has unresolved placeholders", tgt.Name)
	}
	if strings.Contains(base, "..") {
		return "", fmt.Errorf("profile: unsafe base %q", base)
	}
	return joinPath(base, resolved), nil
}

// joinPath joins a base directory with a relative path using forward
// slashes on POSIX. Kept private to avoid pulling in path/filepath at
// the global scope — pure-stdlib test pins must be stable.
func joinPath(base, rel string) string {
	if base == "" {
		return rel
	}
	if rel == "" {
		return base
	}
	if strings.HasSuffix(base, "/") {
		return base + rel
	}
	return base + "/" + rel
}
