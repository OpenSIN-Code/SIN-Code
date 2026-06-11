//go:build !treesitter

// SPDX-License-Identifier: MIT
// Purpose: Build-tag stub: without -tags treesitter the structural engine
// (ast_structural.go) handles all non-Go languages and the bundle stays
// zero-dependency. This file only documents the seam.
// Docs: cmd/sin-code/internal/ast.doc.md
package internal
