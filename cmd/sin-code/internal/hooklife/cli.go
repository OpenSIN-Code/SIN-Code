// SPDX-License-Identifier: MIT
// Purpose: `sin hooks ...` subcommand tree — `list` and `test`.
// Docs: cli.doc.md
package hooklife

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

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
