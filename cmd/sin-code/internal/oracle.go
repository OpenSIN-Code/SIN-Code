// SPDX-License-Identifier: MIT
// Purpose: oracle — Verification Oracle. Delegates to the Python `oracle`
// module in SIN-Code-Oracle-Tool (source of truth). The Python oracle
// actually checks test coverage of a source file against an existing test
// file (subcommand: check).
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	oracleClaim    string
	oracleEvidence string
	oracleFormat   string
)

var OracleCmd = &cobra.Command{
	Use:   "oracle",
	Short: "Verification Oracle — test coverage check (delegates to oracle.cli check)",
	Long: `The Python oracle module checks test coverage of a source file against
an existing test file. Use --claim to specify the source file and
--evidence to specify the test file.

Examples:
  sin-code oracle --claim src/main.py --evidence tests/test_main.py
  sin-code oracle --claim src/auth.py --evidence tests/test_auth.py`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if oracleClaim == "" {
			return fmt.Errorf("--claim (source file) is required")
		}
		if oracleEvidence == "" {
			return fmt.Errorf("--evidence (test file) is required")
		}

		pythonArgs := []string{"-m", "oracle.cli", "check", oracleClaim, "--against", oracleEvidence}

		c := exec.Command("python3", pythonArgs...)
		c.Stderr = os.Stderr
		out, err := c.Output()
		if err != nil {
			return fmt.Errorf("oracle execution failed: %w", err)
		}

		if oracleFormat == "json" {
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
	OracleCmd.Flags().StringVarP(&oracleClaim, "claim", "c", "", "Source file to check coverage for")
	OracleCmd.Flags().StringVarP(&oracleEvidence, "evidence", "e", "", "Existing test file to compare against")
	OracleCmd.Flags().StringVarP(&oracleFormat, "format", "f", "text", "Output format: text|json")
}
