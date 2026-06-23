// SPDX-License-Identifier: MIT
// Package chat contains chat-loop helpers that are shared between the
// embedded chat subcommand and any future adapter that wants the same
// behaviour (e.g. the WebUI-v2 stdio bridge or a CI smoke driver).
//
// At present the package owns the typed JSON-Schema generator
// (schema_gen.go) used by issue #370 to replace hand-written
// map[string]any schema literals with reflect-derived ones. The
// generator is deterministic per (input struct, package config) pair —
// a prerequisite for the system-prompt hash metric (issue #2) and the
// 4-arm eval harness (issue #171).
package chat
