// SPDX-License-Identifier: MIT
package orchestrator

import (
	"context"
	"strings"
	"testing"
)

func TestPredictComputesTransitiveClosure(t *testing.T) {
	g := &ImpactGraph{
		nodes: map[string]*PkgNode{
			"repo/core":  {ImportPath: "repo/core", TestFiles: []string{"core_test.go"}},
			"repo/api":   {ImportPath: "repo/api", Imports: []string{"repo/core"}, TestFiles: []string{"api_test.go"}},
			"repo/tui":   {ImportPath: "repo/tui", Imports: []string{"repo/api"}},
			"repo/other": {ImportPath: "repo/other"},
		},
		reverse: map[string][]string{
			"repo/core": {"repo/api"},
			"repo/api":  {"repo/tui"},
		},
		fileToPkg: map[string]string{
			"core/engine.go": "repo/core",
		},
	}
	imp := g.Predict([]string{"core/engine.go"})
	if len(imp.AffectedPkgs) != 3 {
		t.Fatalf("expected 3 affected pkgs, got %v", imp.AffectedPkgs)
	}
	if len(imp.AffectedTestPkgs) != 2 {
		t.Fatalf("expected 2 test pkgs, got %v", imp.AffectedTestPkgs)
	}
	if imp.Radius != 0.75 {
		t.Fatalf("expected radius 0.75, got %.2f", imp.Radius)
	}
	for _, p := range imp.AffectedPkgs {
		if p == "repo/other" {
			t.Fatal("unrelated package must not be in blast radius")
		}
	}
}

func TestPredictUnknownFileYieldsEmpty(t *testing.T) {
	g := &ImpactGraph{
		nodes:     map[string]*PkgNode{"repo/a": {ImportPath: "repo/a"}},
		reverse:   map[string][]string{},
		fileToPkg: map[string]string{},
	}
	imp := g.Predict([]string{"README.md"})
	if len(imp.AffectedPkgs) != 0 {
		t.Fatalf("doc change must predict empty impact, got %v", imp.AffectedPkgs)
	}
}

func TestRiskBriefWarningOnHighRadius(t *testing.T) {
	imp := &Impact{Radius: 0.8, AffectedPkgs: []string{"a", "b", "c", "d", "e"}}
	brief := imp.RiskBrief()
	if !strings.Contains(brief, "WARNING") {
		t.Fatalf("high radius should emit WARNING, got:\n%s", brief)
	}
}

func TestRiskBriefEmptyImpact(t *testing.T) {
	if got := (*Impact)(nil).RiskBrief(); got != "" {
		t.Fatalf("nil impact should produce empty brief, got %q", got)
	}
}

func TestParseAddedLinesExtractsPositions(t *testing.T) {
	diff := `--- a/internal/calc.go
+++ b/internal/calc.go
@@ -10,4 +10,5 @@
 func Add(a, b int) int {
-	return a + b
+	if a == 0 && b == 0 {
+		return 0
+	}
 	return a + b
 }`
	lines := ParseAddedLines(diff)
	if len(lines) != 3 {
		t.Fatalf("expected 3 substantive added lines (if-body, return, brace), got %d: %+v", len(lines), lines)
	}
	if lines[0].File != "internal/calc.go" {
		t.Fatalf("wrong file: %s", lines[0].File)
	}
	if lines[0].Line != 11 {
		t.Fatalf("expected first added line at 11, got %d", lines[0].Line)
	}
}

func TestBuildImpactGraphEmptyRepo(t *testing.T) {
	g, err := BuildImpactGraph(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.nodes) != 0 {
		t.Fatalf("empty repoRoot should produce empty graph, got %d nodes", len(g.nodes))
	}
}
