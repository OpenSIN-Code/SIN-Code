// Package hooklife provides a native Go lifecycle-hook system (a port of ECC's
// event hooks) with no Node dependency.
//
// Hooks fire at lifecycle Phases (PreToolUse, PostToolUse, Stop, SessionStart,
// SessionEnd, PreCompact, UserPrompt). A PreToolUse hook may return Block to
// veto a tool invocation (the ECC exit-code-2 equivalent); other phases may
// only Warn. The Runner enforces per-hook timeouts and recovers from panics so
// a misbehaving hook never breaks a session.
//
// Built-in hooks (builtin.go) are thin wrappers around SIN-Code's existing
// subsystems via small interfaces: TypeChecker (internal/lsp), Verifier
// (internal/verify), Ledger (internal/ledger). Wire concrete implementations
// through internal/adapters.
//
// SPDX-License-Identifier: MIT
package hooklife
