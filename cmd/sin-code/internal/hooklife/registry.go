// SPDX-License-Identifier: MIT
// Purpose: in-memory hook registry, grouped by phase, with stable
// iteration order (sorted by ID) for deterministic runs.
// Docs: registry.doc.md
package hooklife

import "sort"

// Registry holds hooks grouped by phase.
type Registry struct {
	byPhase map[Phase][]Hook
}

func NewRegistry() *Registry {
	return &Registry{byPhase: map[Phase][]Hook{}}
}

// Register adds a hook to all phases it declares.
func (r *Registry) Register(h Hook) {
	for _, p := range h.Phases() {
		r.byPhase[p] = append(r.byPhase[p], h)
	}
}

// Hooks returns the hooks registered for a phase (stable order by ID).
func (r *Registry) Hooks(p Phase) []Hook {
	hs := r.byPhase[p]
	sort.SliceStable(hs, func(a, b int) bool { return hs[a].ID() < hs[b].ID() })
	return hs
}

// All returns every distinct registered hook.
func (r *Registry) All() []Hook {
	seen := map[string]bool{}
	var out []Hook
	for _, hs := range r.byPhase {
		for _, h := range hs {
			if !seen[h.ID()] {
				seen[h.ID()] = true
				out = append(out, h)
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].ID() < out[b].ID() })
	return out
}
