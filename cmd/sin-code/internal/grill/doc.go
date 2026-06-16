// SPDX-License-Identifier: MIT
// Purpose: grill — adversarial design-review interview session manager
// (issue #141 fusion). The native Go implementation of the external
// SIN-Code-Grill-Me-Skill Python MCP server.
//
// A grilling session is a multi-turn Q&A between the operator and
// the binary. The session keeps a tree of decisions: each answer
// can branch into sub-questions (e.g. "what about edge case X?")
// or resolve the parent decision.
//
// v0 storage: in-memory + JSON file via the Persister interface
// (same pattern as internal/rag). The issue body's "SQLite session/
// ledger DB" target is a follow-up: wire it to internal/session/store
// once the schema is stable. For now the JSON file is human-
// inspectable and ≤ 1 KB per session.
//
// Mandates (issue #141):
//   - M2: no new third-party deps, no CGO.
//   - M5: this package lives under cmd/sin-code/internal/grill/.
//   - M6: reuses the existing session/ledger package for storage
//     (deferred to v1).
//
// Docs: grill.doc.md
package grill
