// SPDX-License-Identifier: MIT
// Purpose: in-memory hook registry, grouped by phase, with stable
// iteration order (sorted by ID) for deterministic runs.
// Docs: registry.doc.md
package hooklife

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// Registry holds hooks grouped by phase.
type Registry struct {
	byPhase map[Phase][]Hook
}

func NewRegistry() *Registry {
	return &Registry{byPhase: map[Phase][]Hook{}}
}

// NewCommand returns `sin hooks ...`. Pass the registry you assembled
// at startup.
func NewCommand(reg *Registry) *cobra.Command {
	root := &cobra.Command{Use: "hooks", Short: "Inspect and test lifecycle hooks"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List registered hooks by phase",
		RunE: func(c *cobra.Command, _ []string) error {
			for _, h := range reg.All() {
				fmt.Printf("  %-22s %v\n", h.ID(), h.Phases())
			}
			return nil
		},
	}

	var tool, command, path string
	test := &cobra.Command{
		Use:   "test [phase]",
		Short: "Dispatch a synthetic event through the hooks",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ev := Event{
				Phase: Phase(args[0]),
				Tool:  tool,
				Args:  map[string]string{"command": command, "path": path},
				Meta:  map[string]string{},
			}
			d := NewRunner(reg).Dispatch(context.Background(), ev)
			fmt.Printf("verdict=%s\n%s\n", d.Verdict, d.Message)
			return nil
		},
	}
	test.Flags().StringVar(&tool, "tool", "Bash", "tool name")
	test.Flags().StringVar(&command, "command", "", "command arg")
	test.Flags().StringVar(&path, "path", "", "path arg")

	root.AddCommand(list, test)
	return root
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
