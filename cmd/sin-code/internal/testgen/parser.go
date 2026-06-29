// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when testgen is refactored
package testgen

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// generateFallback produces a minimal table-driven test from a Go source file
// without requiring gotests. It targets exported functions with simple
// signatures; LLM-supplied cases (in cases) are spliced into the table
// when a matching entry exists. Functions missing an entry fall back to
// the zero-value scaffold.
func generateFallback(ctx context.Context, file string, llm func(context.Context, string) (string, error), cases map[string][]TestCase) (string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}

	pkgName := f.Name.Name

	var funcs []FuncInfo
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !ast.IsExported(fn.Name.Name) {
			continue
		}
		funcs = append(funcs, describeFunc(fn))
	}

	if len(funcs) == 0 {
		return "", fmt.Errorf("no exported functions found in %s", file)
	}
	if cases == nil {
		cases = map[string][]TestCase{}
	}

	data := struct {
		Package string
		Marker  string
		Funcs   []FuncInfo
		Cases   map[string][]TestCase
		HasLLM  bool
	}{
		Package: pkgName,
		Marker:  GeneratedMarker,
		Funcs:   funcs,
		Cases:   cases,
		HasLLM:  llm != nil,
	}

	var buf bytes.Buffer
	if err := fallbackTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FuncInfo is a minimal description of a Go function for template rendering.
type FuncInfo struct {
	Name     string
	Args     []Param
	Returns  []Param
	IsMethod bool
	Receiver string
	HasError bool
}

type Param struct {
	Name string
	Type string
}

func describeFunc(fn *ast.FuncDecl) FuncInfo {
	info := FuncInfo{Name: fn.Name.Name}
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		info.IsMethod = true
		info.Receiver = exprToString(fn.Recv.List[0].Type)
	}
	for _, p := range fn.Type.Params.List {
		typ := exprToString(p.Type)
		for _, n := range p.Names {
			info.Args = append(info.Args, Param{Name: n.Name, Type: typ})
		}
		if len(p.Names) == 0 {
			info.Args = append(info.Args, Param{Name: "arg", Type: typ})
		}
	}
	if fn.Type.Results != nil {
		for _, r := range fn.Type.Results.List {
			typ := exprToString(r.Type)
			if typ == "error" {
				info.HasError = true
			}
			for _, n := range r.Names {
				info.Returns = append(info.Returns, Param{Name: n.Name, Type: typ})
			}
			if len(r.Names) == 0 {
				info.Returns = append(info.Returns, Param{Name: "got", Type: typ})
			}
		}
	}
	return info
}

func exprToString(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprToString(v.X)
	case *ast.ArrayType:
		return "[]" + exprToString(v.Elt)
	case *ast.MapType:
		return "map[" + exprToString(v.Key) + "]" + exprToString(v.Value)
	case *ast.SelectorExpr:
		return exprToString(v.X) + "." + v.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.ChanType:
		return "chan"
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// FunctionsFromSource is a public parser entry point so the chat tool
// can ask the LLM for cases for every exported function in a source
// file without re-implementing describeFunc in caller code.
func FunctionsFromSource(file string) ([]FuncInfo, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var out []FuncInfo
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !ast.IsExported(fn.Name.Name) {
			continue
		}
		out = append(out, describeFunc(fn))
	}
	return out, nil
}
