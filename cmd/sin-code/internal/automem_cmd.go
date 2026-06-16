// SPDX-License-Identifier: MIT
// Purpose: `sin-code memory auto-*` subcommands exposing the byte-stable
// MEMORY.md auto-memory surface (issue #192 followup, parity with Claude
// Code v2.1.59 release). Persists to ~/.local/share/sin-code/memory/
// <project-hash>/MEMORY.md and survives across sessions.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/auto_mem"
)

var (
	autoMemProject string
	autoMemSource  string
	autoMemMax     int
	autoMemFormat  string
)

func openAutoMem() (*auto_mem.Store, string, error) {
	home, err := auto_mem.DefaultHome()
	if err != nil {
		return nil, "", err
	}
	proj := autoMemProject
	if proj == "" {
		proj = "global"
	}
	s, err := auto_mem.Open(home, proj)
	if err != nil {
		return nil, "", err
	}
	return s, proj, nil
}

var memAutoListCmd = &cobra.Command{
	Use:   "auto-list",
	Short: "List topics in MEMORY.md for the active project (issue #192 parity).",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, proj, err := openAutoMem()
		if err != nil {
			return err
		}
		idx, err := s.Index()
		if err != nil {
			return err
		}
		if autoMemFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(struct {
				Project  string   `json:"project"`
				Path     string   `json:"path"`
				Headings []string `json:"headings"`
			}{proj, s.Path(), idx})
		}
		if len(idx) == 0 {
			fmt.Printf("(no entries) — %s\n", s.Path())
			return nil
		}
		fmt.Printf("MEMORY.md for %s (%d topics, %s):\n", proj, len(idx), s.Path())
		for _, h := range idx {
			fmt.Printf("  - %s\n", h)
		}
		return nil
	},
}

var memAutoShowCmd = &cobra.Command{
	Use:   "auto-show <heading>",
	Short: "Show the body of a single MEMORY.md topic.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		body, err := s.ReadTopic(args[0])
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	},
}

var memAutoAppendCmd = &cobra.Command{
	Use:   "auto-append <heading> <body>",
	Short: "Append or replace a MEMORY.md topic. Re-issues replace the prior body.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		src := autoMemSource
		if src == "" {
			src = "manual"
		}
		if err := s.Append(auto_mem.Entry{
			Heading:   args[0],
			Body:      args[1],
			SourceTag: src,
			AddedAt:   time.Now().UTC(),
		}); err != nil {
			return err
		}
		fmt.Printf("updated MEMORY.md topic %q (%s)\n", args[0], s.Path())
		return nil
	},
}

var memAutoRmCmd = &cobra.Command{
	Use:   "auto-rm <heading>",
	Short: "Remove a MEMORY.md topic by heading.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		if err := s.Remove(args[0]); err != nil {
			return err
		}
		fmt.Printf("removed topic %q from %s\n", args[0], s.Path())
		return nil
	},
}

var memAutoGcCmd = &cobra.Command{
	Use:   "auto-gc",
	Short: "Rotate MEMORY.md down to the most recent N topics (default 32).",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		n := autoMemMax
		if n <= 0 {
			n = 32
		}
		kept, err := s.Rotate(n)
		if err != nil {
			return err
		}
		fmt.Printf("rotated MEMORY.md to %d most-recent topics (%s)\n", kept, s.Path())
		return nil
	},
}

func init() {
	MemoryCmd.AddCommand(memAutoListCmd, memAutoShowCmd, memAutoAppendCmd, memAutoRmCmd, memAutoGcCmd)

	// Reuse --project and --format where possible; the auto_mem layer
	// does not use --as (persistence is per-project, not per-actor).
	memAutoListCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key (default 'global')")
	memAutoListCmd.Flags().StringVar(&autoMemFormat, "format", "text", "Output: text|json")

	memAutoShowCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")

	memAutoAppendCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")
	memAutoAppendCmd.Flags().StringVar(&autoMemSource, "source", "manual", "Provenance tag for this entry")

	memAutoRmCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")

	memAutoGcCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")
	memAutoGcCmd.Flags().IntVar(&autoMemMax, "max", 32, "Max topics to keep (rotates down to most-recent N)")
}
