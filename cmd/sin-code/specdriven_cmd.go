// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/specdriven"
	"github.com/spf13/cobra"
)

func newSpecDrivenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec-driven",
		Short: "Spec-Driven Development: EARS → Architecture → Code (issue #480)",
	}
	cmd.AddCommand(newSpecDrivenParseCmd())
	cmd.AddCommand(newSpecDrivenArchCmd())
	return cmd
}

func newSpecDrivenParseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "parse <file>",
		Short: "Parse EARS-format requirements from a spec file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			reqs, err := specdriven.ParseEARS(string(data))
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Parsed %d requirements:\n\n", len(reqs))
			for _, r := range reqs {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s [%s] %s\n", r.ID, r.Type, r.Raw)
			}
			return nil
		},
	}
}

func newSpecDrivenArchCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "arch <file>",
		Short: "Generate architecture from an EARS spec",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := specdriven.LoadSpec(args[0])
			if err != nil {
				return err
			}
			arch := specdriven.GenerateArchitecture(spec)
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(arch)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Architecture: %s\n\n", spec.Title)
			for _, c := range arch.Components {
				fmt.Fprintf(cmd.OutOrStdout(), "Component: %s\n  %s\n", c.Name, c.Description)
				for _, r := range c.Responsibilities {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", r)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}
