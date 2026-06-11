// SPDX-License-Identifier: MIT
// Purpose: ast_provider — language-aware structure extraction behind a single
// interface. Three tiers, best available wins: exact go/ast for Go (stdlib,
// always on), tree-sitter for Python/JS/TS/Rust/Java when built with
// -tags treesitter (CGO, opt-in — default build stays zero-dependency), and
// a pure-Go structural engine (brace/indent tracking with real end lines)
// as the universal fallback. Consumers: read --mode outline, edit --symbol,
// grasp, and (later) map/SCKG.
// Docs: cmd/sin-code/internal/ast.doc.md
package internal

import (
	"path/filepath"
	"strings"
)

type SymbolInfo struct {
	Name      string       `json:"name"`
	Kind      string       `json:"kind"`
	StartLine int          `json:"start_line"`
	EndLine   int          `json:"end_line"`
	Signature string       `json:"signature"`
	Children  []SymbolInfo `json:"children,omitempty"`
}

type FileOutline struct {
	Language string       `json:"language"`
	Engine   string       `json:"engine"`
	Symbols  []SymbolInfo `json:"symbols"`
	Imports  []string     `json:"imports"`
}

type astProvider interface {
	languages() []string
	parse(path string, src []byte) (*FileOutline, error)
}

var providerRegistry = map[string]astProvider{}

func registerProvider(p astProvider, override bool) {
	for _, lang := range p.languages() {
		if _, exists := providerRegistry[lang]; exists && !override {
			continue
		}
		providerRegistry[lang] = p
	}
}

func parseOutline(path string, src []byte) *FileOutline {
	lang := detectLanguage(path)
	if p, ok := providerRegistry[lang]; ok {
		if out, err := p.parse(path, src); err == nil && out != nil {
			out.Language = lang
			return out
		}
	}
	return &FileOutline{Language: lang, Engine: "none"}
}

func findSymbol(out *FileOutline, name string) []SymbolInfo {
	var hits []SymbolInfo
	var walk func(syms []SymbolInfo)
	walk = func(syms []SymbolInfo) {
		for _, s := range syms {
			if s.Name == name || strings.HasSuffix(s.Name, "."+name) {
				hits = append(hits, s)
			}
			walk(s.Children)
		}
	}
	walk(out.Symbols)
	return hits
}

func outlineEngineFor(path string) string {
	lang := detectLanguage(path)
	if p, ok := providerRegistry[lang]; ok {
		if out, err := p.parse(path, []byte{}); err == nil && out != nil {
			return out.Engine
		}
		_ = filepath.Ext(path)
	}
	return "none"
}
