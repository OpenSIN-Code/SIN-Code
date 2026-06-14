// SPDX-License-Identifier: MIT
// Purpose: Direct unit tests for AST helpers in ast_go.go and ast_structural.go. (st-cov1)
package internal

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func parseFuncDecl(src string) *ast.FuncDecl {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", "package x\n"+src, 0)
	if err != nil {
		panic(err)
	}
	return f.Decls[0].(*ast.FuncDecl)
}

func TestRecvTypeName(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"func (a A) M() {}", "A"},
		{"func (a *A) M() {}", "A"},
		{"func (a A[B]) M() {}", "A"},
		{"func (a A[B, C]) M() {}", "A"},
		{"func (a map[string]int) M() {}", "?"},
	}
	for _, c := range cases {
		fd := parseFuncDecl(c.src)
		got := recvTypeName(fd.Recv.List[0].Type)
		if got != c.want {
			t.Errorf("recvTypeName(%s) = %q, want %q", c.src, got, c.want)
		}
	}
}
