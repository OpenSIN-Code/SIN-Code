// SPDX-License-Identifier: MIT
// Package autoactivate adds a per-session rule-injection layer to the
// hooklife subsystem. It intercpts SessionStart and UserPrompt events
// and, when activated, prepends a deterministic byte-stable rule body
// to the agent's system prompt for the duration of the session.
//
// Inspired by JuliusBrussee/caveman's "always-on" ruleset:
//
//	SessionStart -> write flag + emit body to stdout (hidden context)
//	UserPrompt   -> read flag + re-inject per-turn anchor
//
// SIN-Code has the same primitives as Phase hooks in the hooklife
// registry; this package adds a thin per-session state layer so the body
// comes from a RuleSet, not a global flag file.
//
// Privacy: off by default. Auto-activation requires either:
//   - `sin-code chat --activate <rule>` (one-shot, this session)
//   - `.sin-code/autoactivate.toml` at the project root (always-on)
//   - A `trigger:` phrase in a skill's frontmatter that matches the
//     user's prompt (semantic-only, never exfiltrated).
package autoactivate
