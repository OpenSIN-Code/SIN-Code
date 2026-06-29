// SPDX-License-Identifier: MIT
// Purpose: oracle symbol extraction — language-specific parsers for the
// Verification Oracle. Extracts functions, classes, and types from source.
// sin-debt: shrink, upgrade: when a second symbols-related function is needed, merge into a shared file
package internal

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

func extractSymbols(path, content, lang string) []symbolInfo {
	switch lang {
	case "go":
		return extractGoSymbols(path, content)
	case "python":
		return extractPythonSymbols(content)
	case "javascript", "typescript", "tsx", "jsx":
		return extractJSSymbols(content)
	case "rust":
		return extractRustSymbols(content)
	case "java":
		return extractJavaSymbols(content)
	default:
		return extractGenericSymbols(content)
	}
}

func extractGoSymbols(path, content string) []symbolInfo {
	var symbols []symbolInfo
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.AllErrors)
	if err != nil {
		return nil
	}
	for _, decl := range f.Decls {
		pos := fset.Position(decl.Pos())
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				if recv, ok := d.Recv.List[0].Type.(*ast.StarExpr); ok {
					if ident, ok := recv.X.(*ast.Ident); ok {
						name = fmt.Sprintf("(%s).%s", ident.Name, name)
					}
				} else if ident, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
					name = fmt.Sprintf("(%s).%s", ident.Name, name)
				}
			}
			symbols = append(symbols, symbolInfo{Name: name, Type: "function", Line: pos.Line})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					symbols = append(symbols, symbolInfo{Name: ts.Name.Name, Type: "type", Line: pos.Line})
				}
			}
		}
	}
	return symbols
}

func extractPythonSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`^(\s*)(def|class)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 3 {
			typ := "function"
			if matches[2] == "class" {
				typ = "class"
			}
			symbols = append(symbols, symbolInfo{Name: matches[3], Type: typ, Line: i + 1})
		}
	}
	return symbols
}

func extractJSSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`(?:export\s+)?(?:async\s+)?(?:function|class|const|let|var|interface|type)\s+([a-zA-Z_$][a-zA-Z0-9_$]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				typ := "function"
				if strings.Contains(line, "class") {
					typ = "class"
				} else if strings.Contains(line, "interface") {
					typ = "interface"
				} else if strings.Contains(line, "type") {
					typ = "type"
				} else if strings.Contains(line, "const") || strings.Contains(line, "let") || strings.Contains(line, "var") {
					typ = "variable"
				}
				symbols = append(symbols, symbolInfo{Name: m[1], Type: typ, Line: i + 1})
			}
		}
	}
	return symbols
}

func extractRustSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`(?:pub\s+)?(?:fn|struct|enum|trait|impl)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				typ := "function"
				if strings.Contains(line, "struct") {
					typ = "struct"
				} else if strings.Contains(line, "enum") {
					typ = "enum"
				} else if strings.Contains(line, "trait") {
					typ = "trait"
				}
				symbols = append(symbols, symbolInfo{Name: m[1], Type: typ, Line: i + 1})
			}
		}
	}
	return symbols
}

func extractJavaSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`(?:public\s+|private\s+|protected\s+|static\s+)*(?:class|interface|enum|void|int|String|boolean|double|float|long|short|byte|char)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				typ := "function"
				if strings.Contains(line, "class") {
					typ = "class"
				} else if strings.Contains(line, "interface") {
					typ = "interface"
				}
				symbols = append(symbols, symbolInfo{Name: m[1], Type: typ, Line: i + 1})
			}
		}
	}
	return symbols
}

func extractGenericSymbols(content string) []symbolInfo {
	var symbols []symbolInfo
	re := regexp.MustCompile(`(?:function|def|fn|func|method|class|struct|interface|trait|enum|record|sub|procedure)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				symbols = append(symbols, symbolInfo{Name: m[1], Type: "symbol", Line: i + 1})
			}
		}
	}
	return symbols
}
