// SPDX-License-Identifier: MIT
//
// # Package profile
//
// Profile is the single-source-of-truth rewriter for SIN-Code's
// per-agent project rules (issue #175). The source is the in-repo
// markdown at docs/agent-profiles/sin-profile.md; output is one
// artifact per supported host-agent family:
//
//	Claude Code   → <repo>/.claude/skills/sin-code/SKILL.md
//	opencode      → <repo>/.config/opencode/skills/sin-code/SKILL.md
//	Gemini CLI    → <repo>/.gemini/skills/sin-code/SKILL.md
//	Codex         → <repo>/.codex/rules/sin-code.md
//	Cursor        → <repo>/.cursor/rules/sin-code.mdc
//	Windsurf      → <repo>/.windsurf/rules/sin-code.md
//	Cline         → <repo>/.clinerules/sin-code.md
//	GitHub Copilot → <repo>/.github/copilot-instructions.md
//
// (Updates AGENTS.md §10 — same table.)
//
// # Why byte-stable
//
// Render(tgt, body) is a pure function over (tgt, body). The verify
// gate (Verify + HashSource) computes the expected SHA-256 of every
// rendered output and refuses to merge if any on-disk mirror drifts.
// This is the same shape as the verify-gate in `internal/verify`
// (M3) — the renderer is a deterministic preprocessor.
//
// # Marker-fence covenant
//
// RenderBlock / BeginMarker / EndMarker are byte-identical to
// internal/skilldist's outputs for the same `<skill>` value. See
// parser.go for the exact ASCII bytes.
package profile
