// SPDX-License-Identifier: MIT
// Purpose: ibd — Intent-Based Diffing. Delegates to the Python `ibd` module
// in SIN-Code-IBD-Tool (source of truth). Provides Go CLI surface + MCP
// registration.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	ibdBefore   string
	ibdAfter    string
	ibdIntent   string
	ibdFrom     string
	ibdTo       string
	ibdOutput   string
	ibdFormat   string
)

var IbdCmd = &cobra.Command{
	Use:   "ibd",
	Short: "Intent-Based Diffing — compare code changes against stated intent",
	Long: `Compare two versions of code and determine if the changes match the
stated intent. Delegates to the Python ` + "`ibd`" + ` module.

Examples:
  sin-code ibd --before v1.0 --after HEAD --intent "add retry logic"
  sin-code ibd path/to/file.py --from main --to feature-branch
  sin-code ibd --before old.py --after new.py --intent "refactor"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pythonModule := "ibd.cli"
		pythonArgs := []string{"-m", pythonModule, "diff"}

		if ibdBefore != "" && ibdAfter != "" {
			pythonArgs = append(pythonArgs, ibdBefore, "--before", ibdBefore, "--after", ibdAfter)
		} else if len(args) > 0 {
			pythonArgs = append(pythonArgs, args[0])
			if ibdFrom != "" {
				pythonArgs = append(pythonArgs, "--from", ibdFrom)
			}
			if ibdTo != "" {
				pythonArgs = append(pythonArgs, "--to", ibdTo)
			}
		} else {
			return fmt.Errorf("either --before/--after or a target path with --from/--to is required")
		}

		if ibdIntent != "" {
			pythonArgs = append(pythonArgs, "--intent", ibdIntent)
		}
		if ibdOutput != "" {
			pythonArgs = append(pythonArgs, "--output", ibdOutput)
		}

		c := exec.Command("python3", pythonArgs...)
		c.Env = append(os.Environ(), "PYTHONPATH=pythonPath")
		c.Stderr = os.Stderr
		out, err := c.Output()
		if err != nil {
			return fmt.Errorf("ibd execution failed: %w", err)
		}

		if ibdFormat == "json" {
			var pretty map[string]any
			if err := json.Unmarshal(out, &pretty); err == nil {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(pretty)
			}
		}
		fmt.Print(string(out))
		return nil
	},
}

func init() {
	IbdCmd.Flags().StringVarP(&ibdBefore, "before", "b", "", "Before version (ref, file, or commit)")
	IbdCmd.Flags().StringVarP(&ibdAfter, "after", "a", "", "After version (ref, file, or commit)")
	IbdCmd.Flags().StringVarP(&ibdIntent, "intent", "i", "", "Stated intent of the change")
	IbdCmd.Flags().StringVarP(&ibdFrom, "from", "f", "", "Git commit (old) for path target")
	IbdCmd.Flags().StringVarP(&ibdTo, "to", "t", "", "Git commit (new) for path target")
	IbdCmd.Flags().StringVarP(&ibdOutput, "output", "o", "", "Output JSON file")
	IbdCmd.Flags().StringVarP(&ibdFormat, "format", "", "text", "Output format: text|json")
	_ = filepath.Separator
}
