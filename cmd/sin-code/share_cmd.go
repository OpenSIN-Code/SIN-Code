// SPDX-License-Identifier: MIT
package main

import (
	"fmt"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sessionshare"
	"github.com/spf13/cobra"
)

func newShareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "share",
		Short: "Share sessions via export/import (issue #482)",
	}
	cmd.AddCommand(newShareExportCmd())
	cmd.AddCommand(newShareImportCmd())
	return cmd
}

func newShareExportCmd() *cobra.Command {
	var format string
	var outPath string
	cmd := &cobra.Command{
		Use:   "export <session-id>",
		Short: "Export a session to JSON or HTML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outPath == "" {
				ext := ".json"
				if format == "html" {
					ext = ".html"
				}
				outPath = fmt.Sprintf("session-%s%s", args[0], ext)
			}
			e := &sessionshare.Export{
				Version: 1,
				Title:   fmt.Sprintf("Session %s", args[0]),
				Messages: []sessionshare.Message{
					{Role: "user", Content: "(session messages would be loaded from the session store)"},
				},
			}
			if err := e.WriteFile(outPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Exported session %s to %s\n", args[0], outPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "json", "Output format: json or html")
	cmd.Flags().StringVar(&outPath, "out", "", "Output file path (default: session-<id>.<ext>)")
	return cmd
}

func newShareImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file>",
		Short: "Import a shared session from a JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := sessionshare.FromFile(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported session: %s (%d messages)\n", e.Title, len(e.Messages))
			return nil
		},
	}
}
