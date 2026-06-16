// SPDX-License-Identifier: MIT
// Purpose: audit command — repo-wide complexity audit (ponytail-audit analog).
// Docs: docs/complexity-audit.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/audit"
)

var (
	auditPath   string
	auditFormat string
	auditTags   string
	auditRank   string
	auditSince  string
	auditMaxNet int
	auditStrict bool
	auditNoLLM  bool
)

// NewAuditCmd creates `sin-code audit` with `complexity` subcommand.
func NewAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Repo-wide audits (complexity, ...)",
		Long: `Run repo-wide audits. The complexity subcommand is a ponytail-audit analog:

  sin-code audit complexity
  sin-code audit complexity --path ./cmd/sin-code
  sin-code audit complexity --format json
  sin-code audit complexity --tags yagni,delete --rank lines
  sin-code audit complexity --strict --max-net-lines 100`,
	}

	complexityCmd := &cobra.Command{
		Use:   "complexity [path]",
		Short: "Repo-wide complexity audit — ponytail-audit analog",
		Long: `Scan the whole tree for structural complexity and emit one-line findings:

  <tag> <what to cut>. <replacement>. [path]

Tags: delete, stdlib, native, yagni, shrink.
End with: net: -<N> lines, -<M> deps possible.  or  Lean already. Ship.

// sin-debt: markers approve a finding and exclude it from the net total.`,
		Args:    cobra.MaximumNArgs(1),
		Version: Version,
		RunE:    runComplexityAudit,
	}
	complexityCmd.Flags().StringVar(&auditPath, "path", "", "Sub-tree to audit (default: current directory)")
	complexityCmd.Flags().StringVarP(&auditFormat, "format", "f", "text", "Output format: text, json, markdown")
	complexityCmd.Flags().StringVar(&auditTags, "tags", "", "Comma-separated tags (default: all)")
	complexityCmd.Flags().StringVar(&auditRank, "rank", "lines", "Rank by: lines, deps")
	complexityCmd.Flags().StringVar(&auditSince, "since", "", "Audit only files changed since git ref (not implemented in static pass)")
	complexityCmd.Flags().IntVar(&auditMaxNet, "max-net-lines", 0, "Fail if removable net-lines exceed this threshold")
	complexityCmd.Flags().BoolVarP(&auditStrict, "strict", "s", false, "Exit with error if threshold exceeded")
	complexityCmd.Flags().BoolVar(&auditNoLLM, "no-llm", false, "Skip LLM second pass")

	cmd.AddCommand(complexityCmd)
	return cmd
}

func runComplexityAudit(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	if auditPath != "" {
		root = auditPath
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", abs)
	}

	var tags []string
	if auditTags != "" {
		for _, t := range strings.Split(auditTags, ",") {
			tags = append(tags, strings.TrimSpace(t))
		}
		if err := audit.ValidateTags(tags); err != nil {
			return err
		}
	}
	if auditRank != "lines" && auditRank != "deps" {
		return fmt.Errorf("rank must be 'lines' or 'deps'")
	}
	if auditFormat != "text" && auditFormat != "json" && auditFormat != "markdown" {
		return fmt.Errorf("format must be text, json, or markdown")
	}

	opts := audit.Options{
		Tags:     tags,
		Rank:     auditRank,
		SinceRef: auditSince,
		MaxNet:   auditMaxNet,
		Strict:   auditStrict,
		NoLLM:    auditNoLLM,
	}
	if auditMaxNet > 0 {
		opts.Strict = true
	}

	res, err := audit.NewAuditor(nil).Audit(context.Background(), abs, opts)
	if err != nil {
		return err
	}

	switch auditFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	case "markdown":
		fmt.Print(formatMarkdown(res))
	default:
		fmt.Print(audit.FormatResult(res, "text"))
	}
	return nil
}

func formatMarkdown(res *audit.Result) string {
	var sb strings.Builder
	sb.WriteString("# Complexity Audit\n\n")
	if len(res.Findings) == 0 {
		sb.WriteString("**" + res.Status + "**\n")
		return sb.String()
	}
	sb.WriteString("| Tag | What to cut | Replacement | Path | Lines |\n")
	sb.WriteString("|-----|-------------|-------------|------|------|\n")
	for _, f := range res.Findings {
		approved := ""
		if f.Approved {
			approved = " (approved: " + f.Approver + ")"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s:%d | %d |%s\n", f.Tag, f.Problem, f.Replacement, f.Path, f.Line, f.LineCount, approved))
	}
	sb.WriteString("\n**" + res.Status + "**\n")
	return sb.String()
}
