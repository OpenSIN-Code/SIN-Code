// SPDX-License-Identifier: MIT
// Purpose: declarative per-repository config compiler (issue #164).
// Reads a single .sin-code.yml and produces the three derived
// artifacts the SIN-Code binary needs:
//
//	.sin/hooks.json               for internal/hooks/
//	internal/verify/config.json   for internal/verify/
//	internal/permission/policies.json for internal/permission/
//
// v0 scope (this PR):
//   - schema + parser + validator + 3 output emitters
//   - sin-code compile-spec CLI
//   - round-trip test (idempotent: re-run = no diff)
//
// v1.1 scope (follow-up, NOT in this PR):
//   - the three engines must learn to read their derived JSON
//     files (they currently read code, not config — the wire-up
//     is a 2-week refactor of internal/{hooks,verify,permission})
//   - Loop-Engineering parameters from issue #155
//     (max_turns, max_tokens, disable_checks) as a top-level
//     `loop:` block
//   - extends: <base> for remote spec inheritance (issue #164 v2)
//
// Docs: docs/SPEC-COMPILER.md
package compiler
