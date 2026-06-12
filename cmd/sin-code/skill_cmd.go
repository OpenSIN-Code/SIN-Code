// SPDX-License-Identifier: MIT
// Purpose: `sin-code skill` — manage ecosystem skill installations.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/spf13/cobra"
)

func NewSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Install and manage ecosystem MCP skills",
	}

	var jsonOut bool
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show install + runnable state of all known skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			sts := skillmgr.Status(cmd.Context())
			sort.Slice(sts, func(i, j int) bool { return sts[i].Name < sts[j].Name })
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(sts)
			}
			fmt.Printf("%-15s %-10s %-9s %s\n", "SKILL", "INSTALLED", "RUNNABLE", "DETAIL")
			for _, s := range sts {
				fmt.Printf("%-15s %-10v %-9v %s\n", s.Name, s.Installed, s.Runnable, s.Detail)
			}
			return nil
		},
	}
	statusCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	installCmd := &cobra.Command{
		Use:   "install <name>... | all",
		Short: "Clone/update skill repos and verify their MCP entrypoints",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			names := args
			if len(args) == 1 && args[0] == "all" {
				names = names[:0]
				for n := range skillmgr.KnownSkills() {
					names = append(names, n)
				}
				sort.Strings(names)
			}
			failed := 0
			for _, n := range names {
				st, err := skillmgr.Install(cmd.Context(), n)
				if err != nil {
					fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", n, err)
					failed++
					continue
				}
				fmt.Printf("OK   %s (runnable=%v, %s)\n", st.Name, st.Runnable, st.Detail)
			}
			if failed > 0 {
				return fmt.Errorf("%d skill(s) failed to install", failed)
			}
			return nil
		},
	}

	cmd.AddCommand(statusCmd, installCmd)
	return cmd
}
