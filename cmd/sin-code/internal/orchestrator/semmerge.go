// SPDX-License-Identifier: MIT
// Purpose: Semantic Merge for Go — declaration-level three-way merge.
// Line-based merge fails on adjacent but independent changes. Semantic
// merge diffs at function/type/var/import granularity and merges cleanly
// whenever sides touch DIFFERENT declarations, even when textual
// neighbors. Only genuine same-decl conflicts escalate.
package orchestrator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

type Decl struct {
	Key string
	Src string
	Pos int
}

type SemConflict struct {
	Key       string
	Base, A, B string
}

type MergeResult struct {
	Merged     []byte
	Conflicts  []SemConflict
	AutoMerged int
}

func SemanticMergeGo(base, a, b []byte) (*MergeResult, error) {
	baseD, err := extractDecls(base)
	if err != nil {
		return nil, fmt.Errorf("semmerge parse base: %w", err)
	}
	aD, err := extractDecls(a)
	if err != nil {
		return nil, fmt.Errorf("semmerge parse A: %w", err)
	}
	bD, err := extractDecls(b)
	if err != nil {
		return nil, fmt.Errorf("semmerge parse B: %w", err)
	}

	res := &MergeResult{}
	merged := map[string]string{}
	order := map[string]int{}

	keys := unionKeys(baseD, aD, bD)
	for _, key := range keys {
		bv, inBase := baseD[key]
		av, inA := aD[key]
		cv, inB := bD[key]

		switch {
		case inA && inB && av.Src == cv.Src:
			merged[key] = av.Src
		case inA && (!inB || (inBase && cv.Src == bv.Src)):
			if inB && inBase && av.Src != bv.Src {
				res.AutoMerged++
			}
			merged[key] = av.Src
		case inB && (!inA || (inBase && av.Src == bv.Src)):
			if inA && inBase && cv.Src != bv.Src {
				res.AutoMerged++
			}
			merged[key] = cv.Src
		case !inA && inBase && inB && cv.Src == bv.Src:
			continue
		case !inB && inBase && inA && av.Src == bv.Src:
			continue
		default:
			c := SemConflict{Key: key}
			if inBase {
				c.Base = bv.Src
			}
			if inA {
				c.A = av.Src
			}
			if inB {
				c.B = cv.Src
			}
			res.Conflicts = append(res.Conflicts, c)
			continue
		}

		switch {
		case inBase:
			order[key] = bv.Pos
		case inA:
			order[key] = 10000 + av.Pos
		default:
			order[key] = 20000 + cv.Pos
		}
	}

	if len(res.Conflicts) > 0 {
		return res, nil
	}

	var buf bytes.Buffer
	buf.WriteString(packageClause(a))
	type kv struct {
		key string
		pos int
	}
	ordered := make([]struct {
		key string
		pos int
	}, 0, len(merged))
	for k := range merged {
		ordered = append(ordered, struct {
			key string
			pos int
		}{k, order[k]})
	}
	sortKVs(ordered)
	for _, e := range ordered {
		buf.WriteString(merged[e.key])
		buf.WriteString("\n\n")
	}

	out, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("semmerge reassembly invalid: %w", err)
	}
	res.Merged = out
	return res, nil
}

func (r *MergeResult) ConflictBrief() string {
	var b strings.Builder
	fmt.Fprintf(&b, "semantic merge: %d declarations auto-merged, %d genuine conflicts:\n",
		r.AutoMerged, len(r.Conflicts))
	for _, c := range r.Conflicts {
		fmt.Fprintf(&b, "\n=== CONFLICT %s ===\n--- version A ---\n%s\n--- version B ---\n%s\n",
			c.Key, c.A, c.B)
	}
	b.WriteString("\nResolve by writing ONE merged declaration per conflict that preserves both intents.\n")
	return b.String()
}

func extractDecls(src []byte) (map[string]Decl, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	out := map[string]Decl{}
	for i, d := range f.Decls {
		key, ok := declKey(d)
		if !ok {
			continue
		}
		start := fset.Position(d.Pos()).Offset
		end := fset.Position(d.End()).Offset
		if fd, isFn := d.(*ast.FuncDecl); isFn && fd.Doc != nil {
			start = fset.Position(fd.Doc.Pos()).Offset
		} else if gd, isGen := d.(*ast.GenDecl); isGen && gd.Doc != nil {
			start = fset.Position(gd.Doc.Pos()).Offset
		}
		out[key] = Decl{Key: key, Src: string(src[start:end]), Pos: i}
	}
	return out, nil
}

func declKey(d ast.Decl) (string, bool) {
	switch v := d.(type) {
	case *ast.FuncDecl:
		if v.Recv != nil && len(v.Recv.List) > 0 {
			return "method:" + recvTypeName(v.Recv.List[0].Type) + "." + v.Name.Name, true
		}
		return "func:" + v.Name.Name, true
	case *ast.GenDecl:
		if len(v.Specs) == 1 {
			switch s := v.Specs[0].(type) {
			case *ast.TypeSpec:
				return "type:" + s.Name.Name, true
			case *ast.ValueSpec:
				if len(s.Names) > 0 {
					return fmt.Sprintf("%s:%s", v.Tok, s.Names[0].Name), true
				}
			case *ast.ImportSpec:
				return "import:" + s.Path.Value, true
			}
		}
		return fmt.Sprintf("group:%s:%d", v.Tok, len(v.Specs)), true
	}
	return "", false
}

func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	}
	return "?"
}

func packageClause(src []byte) string {
	for _, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			return line + "\n\n"
		}
	}
	return "package main\n\n"
}

func unionKeys(ms ...map[string]Decl) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range ms {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

func sortKVs(kvs []struct {
	key string
	pos int
}) {
	for i := 1; i < len(kvs); i++ {
		for j := i; j > 0 && kvs[j].pos < kvs[j-1].pos; j-- {
			kvs[j], kvs[j-1] = kvs[j-1], kvs[j]
		}
	}
}
