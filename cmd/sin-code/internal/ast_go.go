// SPDX-License-Identifier: MIT
// Purpose: ast_go — exact Go outlines via go/parser + go/ast (stdlib, zero
// dependencies, always on). Replaces every Go regex heuristic: real start/end
// lines from the token.FileSet, methods named "Receiver.Name", struct fields
// and interface methods as children, full import list.
// Docs: cmd/sin-code/internal/ast.doc.md
package internal

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

type goASTProvider struct{}

func init() { registerProvider(goASTProvider{}, false) }

func (goASTProvider) languages() []string { return []string{"go"} }

func (goASTProvider) parse(path string, src []byte) (*FileOutline, error) {
	out := &FileOutline{Engine: "go/ast"}
	if len(src) == 0 {
		return out, nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(src), "\n")
	sigOf := func(start int) string {
		if start-1 >= 0 && start-1 < len(lines) {
			return strings.TrimSpace(lines[start-1])
		}
		return ""
	}
	lineRange := func(n ast.Node) (int, int) {
		return fset.Position(n.Pos()).Line, fset.Position(n.End()).Line
	}

	for _, imp := range file.Imports {
		out.Imports = append(out.Imports, strings.Trim(imp.Path.Value, `"`))
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			start, end := lineRange(d)
			sym := SymbolInfo{Kind: "func", Name: d.Name.Name, StartLine: start, EndLine: end, Signature: sigOf(start)}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				sym.Kind = "method"
				sym.Name = recvTypeName(d.Recv.List[0].Type) + "." + d.Name.Name
			}
			out.Symbols = append(out.Symbols, sym)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					start, end := lineRange(s)
					sym := SymbolInfo{Name: s.Name.Name, StartLine: start, EndLine: end, Signature: sigOf(start)}
					switch t := s.Type.(type) {
					case *ast.StructType:
						sym.Kind = "struct"
						if t.Fields != nil {
							for _, f := range t.Fields.List {
								fs, fe := lineRange(f)
								for _, n := range f.Names {
									sym.Children = append(sym.Children, SymbolInfo{
										Kind: "field", Name: n.Name, StartLine: fs, EndLine: fe, Signature: sigOf(fs),
									})
								}
							}
						}
					case *ast.InterfaceType:
						sym.Kind = "interface"
						if t.Methods != nil {
							for _, m := range t.Methods.List {
								ms, me := lineRange(m)
								for _, n := range m.Names {
									sym.Children = append(sym.Children, SymbolInfo{
										Kind: "method", Name: n.Name, StartLine: ms, EndLine: me, Signature: sigOf(ms),
									})
								}
							}
						}
					default:
						sym.Kind = "type"
					}
					out.Symbols = append(out.Symbols, sym)
				case *ast.ValueSpec:
					start, end := lineRange(s)
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, n := range s.Names {
						if n.Name == "_" {
							continue
						}
						out.Symbols = append(out.Symbols, SymbolInfo{
							Kind: kind, Name: n.Name, StartLine: start, EndLine: end, Signature: sigOf(start),
						})
					}
				}
			}
		}
	}
	return out, nil
}

func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	}
	return "?"
}
