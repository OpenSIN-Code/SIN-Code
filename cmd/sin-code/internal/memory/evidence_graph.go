// SPDX-License-Identifier: MIT
// Purpose: Evidence graph — bitemporal links between memory, code, and
// verification verdicts (issue #352). Thread-safe (M7). No external deps (M2).
package memory

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type EvidenceNodeType string

const (
	EvidenceMemory EvidenceNodeType = "memory"
	EvidenceCode   EvidenceNodeType = "code"
	EvidenceVerify EvidenceNodeType = "verify"
)

type EvidenceNode struct {
	ID        string            `json:"id"`
	Type      EvidenceNodeType  `json:"type"`
	Ref       string            `json:"ref"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type EvidenceLink struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Relation  string    `json:"relation"`
	CreatedAt time.Time `json:"created_at"`
}

type EvidenceGraph struct {
	mu    sync.RWMutex
	nodes map[string]EvidenceNode
	out   map[string][]EvidenceLink
	in    map[string][]EvidenceLink
}

func NewEvidenceGraph() *EvidenceGraph {
	return &EvidenceGraph{
		nodes: make(map[string]EvidenceNode),
		out:   make(map[string][]EvidenceLink),
		in:    make(map[string][]EvidenceLink),
	}
}

func (g *EvidenceGraph) AddNode(n EvidenceNode) {
	if g == nil || n.ID == "" {
		return
	}
	if n.Timestamp.IsZero() {
		n.Timestamp = time.Now().UTC()
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[n.ID] = n
}

func (g *EvidenceGraph) AddLink(from, to, relation string) {
	if g == nil || from == "" || to == "" || from == to {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, l := range g.out[from] {
		if l.To == to && l.Relation == relation {
			return
		}
	}
	link := EvidenceLink{From: from, To: to, Relation: relation, CreatedAt: time.Now().UTC()}
	g.out[from] = append(g.out[from], link)
	g.in[to] = append(g.in[to], link)
}

func (g *EvidenceGraph) RemoveLink(from, to, relation string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	removed := false
	out := g.out[from]
	for i, l := range out {
		if l.To == to && l.Relation == relation {
			g.out[from] = append(out[:i], out[i+1:]...)
			removed = true
			break
		}
	}
	if !removed {
		return false
	}
	in := g.in[to]
	for i, l := range in {
		if l.From == from && l.Relation == relation {
			g.in[to] = append(in[:i], in[i+1:]...)
			break
		}
	}
	if len(g.out[from]) == 0 {
		delete(g.out, from)
	}
	if len(g.in[to]) == 0 {
		delete(g.in, to)
	}
	return true
}

func (g *EvidenceGraph) GetNode(id string) (EvidenceNode, bool) {
	if g == nil {
		return EvidenceNode{}, false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	return n, ok
}

func (g *EvidenceGraph) Neighbors(id string) []EvidenceNode {
	if g == nil {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	seen := map[string]bool{}
	var out []EvidenceNode
	for _, l := range g.out[id] {
		if n, ok := g.nodes[l.To]; ok && !seen[l.To] {
			seen[l.To] = true
			out = append(out, n)
		}
	}
	for _, l := range g.in[id] {
		if n, ok := g.nodes[l.From]; ok && !seen[l.From] {
			seen[l.From] = true
			out = append(out, n)
		}
	}
	sortNodes(out)
	return out
}

func (g *EvidenceGraph) LinksFrom(id string) []EvidenceLink {
	if g == nil {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	cp := make([]EvidenceLink, len(g.out[id]))
	copy(cp, g.out[id])
	return cp
}

func (g *EvidenceGraph) LinksTo(id string) []EvidenceLink {
	if g == nil {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	cp := make([]EvidenceLink, len(g.in[id]))
	copy(cp, g.in[id])
	return cp
}

func (g *EvidenceGraph) Trace(id string, depth int) []EvidenceNode {
	if g == nil {
		return nil
	}
	if depth <= 0 {
		depth = 3
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	visited := map[string]bool{id: true}
	type frame struct {
		id    string
		depth int
	}
	queue := []frame{{id: id, depth: 0}}
	var out []EvidenceNode
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= depth {
			continue
		}
		adj := make(map[string]bool)
		for _, l := range g.out[cur.id] {
			adj[l.To] = true
		}
		for _, l := range g.in[cur.id] {
			adj[l.From] = true
		}
		keys := make([]string, 0, len(adj))
		for k := range adj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, nid := range keys {
			if visited[nid] {
				continue
			}
			visited[nid] = true
			if n, ok := g.nodes[nid]; ok {
				out = append(out, n)
			}
			queue = append(queue, frame{id: nid, depth: cur.depth + 1})
		}
	}
	sortNodes(out)
	return out
}

func (g *EvidenceGraph) NodeCount() int {
	if g == nil {
		return 0
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

func (g *EvidenceGraph) LinkCount() int {
	if g == nil {
		return 0
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	n := 0
	for _, list := range g.out {
		n += len(list)
	}
	return n
}

func (g *EvidenceGraph) RenderDOT() string {
	if g == nil {
		return "digraph evidence {\n}\n"
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	var b strings.Builder
	b.WriteString("digraph evidence {\n")
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := g.nodes[id]
		fmt.Fprintf(&b, "  %q [label=\"%s\\n%s\", shape=%s];\n",
			id, n.Type, n.Ref, dotShape(n.Type))
	}
	type edgeKey struct{ from, to, rel string }
	edges := make([]edgeKey, 0)
	for from, list := range g.out {
		for _, l := range list {
			edges = append(edges, edgeKey{from: from, to: l.To, rel: l.Relation})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		if edges[i].to != edges[j].to {
			return edges[i].to < edges[j].to
		}
		return edges[i].rel < edges[j].rel
	})
	for _, e := range edges {
		fmt.Fprintf(&b, "  %q -> %q [label=%q];\n", e.from, e.to, e.rel)
	}
	b.WriteString("}\n")
	return b.String()
}

func dotShape(t EvidenceNodeType) string {
	switch t {
	case EvidenceMemory:
		return "ellipse"
	case EvidenceCode:
		return "box"
	case EvidenceVerify:
		return "diamond"
	default:
		return "ellipse"
	}
}

func sortNodes(nodes []EvidenceNode) {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
}
