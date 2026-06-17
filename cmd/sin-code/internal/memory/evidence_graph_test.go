// SPDX-License-Identifier: MIT
// Purpose: tests for the evidence graph (issue #352).
package memory

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func addTestNodes(t *testing.T, g *EvidenceGraph) {
	t.Helper()
	g.AddNode(EvidenceNode{ID: "m1", Type: EvidenceMemory, Ref: "mem-abc", Timestamp: time.Now()})
	g.AddNode(EvidenceNode{ID: "c1", Type: EvidenceCode, Ref: "auth.go:42", Timestamp: time.Now()})
	g.AddNode(EvidenceNode{ID: "v1", Type: EvidenceVerify, Ref: "poc-pass-1", Timestamp: time.Now()})
}

func TestEvidenceGraphAddNodeAndGet(t *testing.T) {
	g := NewEvidenceGraph()
	g.AddNode(EvidenceNode{ID: "m1", Type: EvidenceMemory, Ref: "mem-abc"})
	n, ok := g.GetNode("m1")
	if !ok {
		t.Fatal("node should exist")
	}
	if n.Type != EvidenceMemory || n.Ref != "mem-abc" {
		t.Errorf("node: %+v", n)
	}
	if n.Timestamp.IsZero() {
		t.Error("timestamp should default to now")
	}
	if _, ok := g.GetNode("missing"); ok {
		t.Error("missing node should not exist")
	}
}

func TestEvidenceGraphAddNodeEmptyIDIgnored(t *testing.T) {
	g := NewEvidenceGraph()
	g.AddNode(EvidenceNode{ID: "", Type: EvidenceMemory})
	if g.NodeCount() != 0 {
		t.Error("empty-id node should be ignored")
	}
}

func TestEvidenceGraphAddLinkAndNeighbors(t *testing.T) {
	g := NewEvidenceGraph()
	addTestNodes(t, g)
	g.AddLink("m1", "c1", "references")
	g.AddLink("c1", "v1", "verified-by")
	neighbors := g.Neighbors("m1")
	if len(neighbors) != 1 || neighbors[0].ID != "c1" {
		t.Fatalf("m1 neighbors: %+v", neighbors)
	}
	c1n := g.Neighbors("c1")
	if len(c1n) != 2 {
		t.Fatalf("c1 should have 2 neighbors, got %d", len(c1n))
	}
	ids := map[string]bool{}
	for _, n := range c1n {
		ids[n.ID] = true
	}
	if !ids["m1"] || !ids["v1"] {
		t.Errorf("c1 neighbors missing m1 or v1")
	}
}

func TestEvidenceGraphAddLinkSelfIgnored(t *testing.T) {
	g := NewEvidenceGraph()
	g.AddNode(EvidenceNode{ID: "x", Type: EvidenceMemory})
	g.AddLink("x", "x", "self")
	if g.LinkCount() != 0 {
		t.Error("self-link should be ignored")
	}
}

func TestEvidenceGraphAddLinkIdempotent(t *testing.T) {
	g := NewEvidenceGraph()
	addTestNodes(t, g)
	g.AddLink("m1", "c1", "references")
	g.AddLink("m1", "c1", "references")
	if g.LinkCount() != 1 {
		t.Errorf("duplicate link should be ignored, count=%d", g.LinkCount())
	}
	g.AddLink("m1", "c1", "extends")
	if g.LinkCount() != 2 {
		t.Errorf("different relation should add link, count=%d", g.LinkCount())
	}
}

func TestEvidenceGraphRemoveLink(t *testing.T) {
	g := NewEvidenceGraph()
	addTestNodes(t, g)
	g.AddLink("m1", "c1", "references")
	if !g.RemoveLink("m1", "c1", "references") {
		t.Error("RemoveLink should return true for existing link")
	}
	if g.LinkCount() != 0 {
		t.Error("link should be removed")
	}
	if g.RemoveLink("m1", "c1", "references") {
		t.Error("RemoveLink should return false for missing link")
	}
}

func TestEvidenceGraphTraceBFS(t *testing.T) {
	g := NewEvidenceGraph()
	g.AddNode(EvidenceNode{ID: "m1", Type: EvidenceMemory, Ref: "mem-1"})
	g.AddNode(EvidenceNode{ID: "c1", Type: EvidenceCode, Ref: "code-1"})
	g.AddNode(EvidenceNode{ID: "v1", Type: EvidenceVerify, Ref: "verdict-1"})
	g.AddNode(EvidenceNode{ID: "m2", Type: EvidenceMemory, Ref: "mem-2"})
	g.AddNode(EvidenceNode{ID: "far", Type: EvidenceMemory, Ref: "mem-far"})
	g.AddLink("m1", "c1", "references")
	g.AddLink("c1", "v1", "verified-by")
	g.AddLink("v1", "m2", "proves")
	g.AddLink("m2", "far", "references")
	d1 := g.Trace("m1", 1)
	if len(d1) != 1 || d1[0].ID != "c1" {
		t.Errorf("depth-1 trace: %+v", d1)
	}
	d2 := g.Trace("m1", 2)
	if len(d2) != 2 {
		t.Errorf("depth-2 trace: got %d", len(d2))
	}
	d3 := g.Trace("m1", 3)
	if len(d3) != 3 {
		t.Errorf("depth-3 trace: got %d", len(d3))
	}
	d4 := g.Trace("m1", 4)
	if len(d4) != 4 {
		t.Errorf("depth-4 trace: got %d", len(d4))
	}
	d0 := g.Trace("m1", 0)
	if len(d0) != 3 {
		t.Errorf("depth-0 (default 3): got %d", len(d0))
	}
}

func TestEvidenceGraphRenderDOT(t *testing.T) {
	g := NewEvidenceGraph()
	addTestNodes(t, g)
	g.AddLink("m1", "c1", "references")
	g.AddLink("c1", "v1", "verified-by")
	dot := g.RenderDOT()
	if !strings.HasPrefix(dot, "digraph evidence {") {
		t.Errorf("DOT prefix wrong: %q", dot[:30])
	}
	if !strings.Contains(dot, `"m1"`) || !strings.Contains(dot, `->`) {
		t.Errorf("DOT missing nodes or edges: %s", dot)
	}
}

func TestEvidenceGraphRenderDOTEmpty(t *testing.T) {
	g := NewEvidenceGraph()
	dot := g.RenderDOT()
	if !strings.Contains(dot, "digraph evidence {") {
		t.Errorf("empty DOT: %q", dot)
	}
}

func TestEvidenceGraphRenderDOTIsDeterministic(t *testing.T) {
	g1 := NewEvidenceGraph()
	g2 := NewEvidenceGraph()
	for _, g := range []*EvidenceGraph{g1, g2} {
		g.AddNode(EvidenceNode{ID: "b", Type: EvidenceCode, Ref: "b.go"})
		g.AddNode(EvidenceNode{ID: "a", Type: EvidenceMemory, Ref: "a-mem"})
		g.AddLink("a", "b", "references")
		g.AddLink("b", "a", "verified-by")
	}
	if g1.RenderDOT() != g2.RenderDOT() {
		t.Error("RenderDOT should be byte-stable")
	}
}

func TestEvidenceGraphConcurrentAddLink(t *testing.T) {
	g := NewEvidenceGraph()
	for i := 0; i < 20; i++ {
		g.AddNode(EvidenceNode{ID: string(rune('a' + i)), Type: EvidenceMemory, Ref: "r"})
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		for j := 0; j < 20; j++ {
			if i == j {
				continue
			}
			wg.Add(1)
			go func(from, to int) {
				defer wg.Done()
				g.AddLink(string(rune('a'+from)), string(rune('a'+to)), "r")
			}(i, j)
		}
	}
	wg.Wait()
	if g.LinkCount() != 380 {
		t.Errorf("link count: got %d, want 380", g.LinkCount())
	}
}

func TestEvidenceGraphLinksFromAndTo(t *testing.T) {
	g := NewEvidenceGraph()
	addTestNodes(t, g)
	g.AddLink("m1", "c1", "references")
	g.AddLink("c1", "v1", "verified-by")
	from := g.LinksFrom("c1")
	if len(from) != 1 || from[0].To != "v1" {
		t.Errorf("LinksFrom(c1): %+v", from)
	}
	to := g.LinksTo("v1")
	if len(to) != 1 || to[0].From != "c1" {
		t.Errorf("LinksTo(v1): %+v", to)
	}
	from[0].Relation = "tampered"
	again := g.LinksFrom("c1")
	if again[0].Relation == "tampered" {
		t.Error("LinksFrom should return a defensive copy")
	}
}
