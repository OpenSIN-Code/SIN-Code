// SPDX-License-Identifier: MIT
// Purpose: ast_structural — pure-Go structural outlines for non-Go languages
// (always on; upgraded transparently by tree-sitter when built with
// -tags treesitter). Unlike the legacy single-line regex heuristics this
// engine computes REAL end lines: brace-depth tracking (string/comment aware)
// for C-family languages, indentation tracking for Python. That is what makes
// AST-anchored symbol edits safe without CGO.
// Docs: cmd/sin-code/internal/ast.doc.md
package internal

import (
	"regexp"
	"strings"
)

type structuralProvider struct{}

func init() { registerProvider(structuralProvider{}, false) }

func (structuralProvider) languages() []string {
	return []string{"python", "javascript", "typescript", "rust", "java", "c", "cpp", "csharp", "php", "ruby"}
}

var structuralDefRe = regexp.MustCompile(
	`^(\s*)(?:export\s+)?(?:default\s+)?(?:pub(?:\s*\(\s*crate\s*\)\s*)?\s+)?(?:public\s+|private\s+|protected\s+|static\s+|abstract\s+|final\s+|async\s+)*` +
		`(func|fn|def|function|class|struct|interface|trait|enum|impl|type|module)\s+([A-Za-z_][A-Za-z0-9_]*)`)

var structuralImportRe = regexp.MustCompile(
	`^\s*(?:import|from|use|require|include|using)\b\s*(.{0,120})`)

func (structuralProvider) parse(path string, src []byte) (*FileOutline, error) {
	out := &FileOutline{Engine: "structural"}
	if len(src) == 0 {
		return out, nil
	}
	lang := detectLanguage(path)
	lines := strings.Split(string(src), "\n")

	for i, line := range lines {
		if m := structuralImportRe.FindStringSubmatch(line); m != nil {
			out.Imports = append(out.Imports, strings.TrimSpace(m[0]))
		}
		m := structuralDefRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent, kindWord, name := m[1], m[2], m[3]
		kind := normalizeKind(kindWord)
		end := i + 1
		if lang == "python" {
			end = pythonBlockEnd(lines, i, len(expandTabs(indent)))
		} else {
			end = braceBlockEnd(lines, i)
		}
		out.Symbols = append(out.Symbols, SymbolInfo{
			Kind: kind, Name: name,
			StartLine: i + 1, EndLine: end,
			Signature: strings.TrimSpace(line),
		})
	}
	return out, nil
}

func normalizeKind(w string) string {
	switch w {
	case "fn", "function", "def":
		return "func"
	}
	return w
}

func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }

func pythonBlockEnd(lines []string, defIdx, defIndent int) int {
	end := defIdx + 1
	for i := defIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(expandTabs(lines[i])) - len(strings.TrimLeft(expandTabs(lines[i]), " "))
		if indent <= defIndent {
			break
		}
		end = i + 1
	}
	return end
}

func braceBlockEnd(lines []string, defIdx int) int {
	depth := 0
	opened := false
	inBlockComment := false
	for i := defIdx; i < len(lines); i++ {
		line := lines[i]
		inString := rune(0)
		escaped := false
		for j := 0; j < len(line); j++ {
			c := line[j]
			if inBlockComment {
				if c == '*' && j+1 < len(line) && line[j+1] == '/' {
					inBlockComment = false
					j++
				}
				continue
			}
			if inString != 0 {
				if escaped {
					escaped = false
				} else if c == '\\' {
					escaped = true
				} else if rune(c) == inString {
					inString = 0
				}
				continue
			}
			switch c {
			case '"', '\'', '`':
				inString = rune(c)
			case '/':
				if j+1 < len(line) {
					if line[j+1] == '/' {
						j = len(line)
					} else if line[j+1] == '*' {
						inBlockComment = true
						j++
					}
				}
			case '{':
				depth++
				opened = true
			case '}':
				depth--
				if opened && depth <= 0 {
					return i + 1
				}
			case ';':
				if !opened {
					return i + 1
				}
			}
		}
		if !opened && i > defIdx {
			return defIdx + 1
		}
	}
	return len(lines)
}
