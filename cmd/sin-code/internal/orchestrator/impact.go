// SPDX-License-Identifier: MIT
// Purpose: Impact Oracle — predict blast-radius BEFORE editing.
// Builds a reverse-dependency graph from `go list -json ./...` and answers
// "if these files change, which packages and which tests are affected?".
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type PkgNode struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	TestFiles  []string
	Imports    []string
}

type ImpactGraph struct {
	mu        sync.RWMutex
	nodes     map[string]*PkgNode
	reverse   map[string][]string
	fileToPkg map[string]string
	builtAt   time.Time
	repoRoot  string
}

func BuildImpactGraph(ctx context.Context, repoRoot string) (*ImpactGraph, error) {
	g := &ImpactGraph{
		nodes:     map[string]*PkgNode{},
		reverse:   map[string][]string{},
		fileToPkg: map[string]string{},
		builtAt:   timeNow(),
		repoRoot:  repoRoot,
	}

	if repoRoot == "" {
		return g, nil
	}
	cmd := exec.CommandContext(ctx, "go", "list", "-json", "./...")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("impact: go list: %w", err)
	}

	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p struct {
			ImportPath   string
			Dir          string
			GoFiles      []string
			TestGoFiles  []string
			XTestGoFiles []string
			Imports      []string
		}
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("impact: decode: %w", err)
		}
		node := &PkgNode{
			ImportPath: p.ImportPath,
			Dir:        p.Dir,
			GoFiles:    p.GoFiles,
			TestFiles:  append(p.TestGoFiles, p.XTestGoFiles...),
			Imports:    p.Imports,
		}
		g.nodes[p.ImportPath] = node

		rel, relErr := filepath.Rel(repoRoot, p.Dir)
		if relErr != nil {
			rel = p.Dir
		}
		for _, f := range p.GoFiles {
			g.fileToPkg[filepath.Join(rel, f)] = p.ImportPath
		}
		for _, f := range node.TestFiles {
			g.fileToPkg[filepath.Join(rel, f)] = p.ImportPath
		}
	}

	for ip, node := range g.nodes {
		for _, dep := range node.Imports {
			if _, inRepo := g.nodes[dep]; inRepo {
				g.reverse[dep] = append(g.reverse[dep], ip)
			}
		}
	}
	return g, nil
}

type Impact struct {
	ChangedPkgs       []string
	AffectedPkgs      []string
	AffectedTestPkgs  []string
	Radius            float64
}

func (g *ImpactGraph) Predict(changedFiles []string) *Impact {
	g.mu.RLock()
	defer g.mu.RUnlock()

	changed := map[string]bool{}
	for _, f := range changedFiles {
		if pkg, ok := g.fileToPkg[filepath.ToSlash(f)]; ok {
			changed[pkg] = true
		} else if pkg, ok := g.fileToPkg[f]; ok {
			changed[pkg] = true
		}
	}

	affected := map[string]bool{}
	queue := make([]string, 0, len(changed))
	for p := range changed {
		affected[p] = true
		queue = append(queue, p)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, dep := range g.reverse[cur] {
			if !affected[dep] {
				affected[dep] = true
				queue = append(queue, dep)
			}
		}
	}

	imp := &Impact{}
	for p := range changed {
		imp.ChangedPkgs = append(imp.ChangedPkgs, p)
	}
	for p := range affected {
		imp.AffectedPkgs = append(imp.AffectedPkgs, p)
		if n := g.nodes[p]; n != nil && len(n.TestFiles) > 0 {
			imp.AffectedTestPkgs = append(imp.AffectedTestPkgs, p)
		}
	}
	sort.Strings(imp.ChangedPkgs)
	sort.Strings(imp.AffectedPkgs)
	sort.Strings(imp.AffectedTestPkgs)

	if len(g.nodes) > 0 {
		imp.Radius = float64(len(imp.AffectedPkgs)) / float64(len(g.nodes))
	}
	return imp
}

func (i *Impact) RiskBrief() string {
	if i == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Predicted blast radius (before editing)\n")
	fmt.Fprintf(&b, "- Directly changed packages: %d\n", len(i.ChangedPkgs))
	fmt.Fprintf(&b, "- Transitively affected packages: %d (radius %.0f%%)\n",
		len(i.AffectedPkgs), i.Radius*100)
	fmt.Fprintf(&b, "- Test packages to re-verify: %d\n", len(i.AffectedTestPkgs))
	if i.Radius > 0.5 {
		b.WriteString("- WARNING: change affects more than half the repo. " +
			"Prefer an interface-preserving approach; do not change exported signatures.\n")
	}
	return b.String()
}
