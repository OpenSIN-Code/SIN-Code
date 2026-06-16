// SPDX-License-Identifier: MIT
// Purpose: `sin-code review --complexity` — ponytail 5-tag complexity review.
// Docs: review_cmd.doc.md
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/complexity"
)

var (
	reviewComplexity bool
	reviewPath       string
	reviewSince      string
	reviewTags       string
	reviewFormat     string
)

// NewReviewCmd builds the `review` cobra subcommand.
func NewReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review code for complexity and other quality dimensions",
		Long: `sin-code review runs static, deterministic review passes.

The first mode is the ponytail complexity review:

  sin-code review --complexity
  sin-code review --complexity --path ./pkg --since HEAD~1 --tags yagni,shrink
  sin-code review --complexity --format json

It reports one line per cut in the ponytail format:
  <tag>: <what to cut>. <replacement>. [path:line]

Tags: delete | stdlib | native | yagni | shrink

` + "// sin-debt:" + ` markers are respected and shown as "approved: sin-debt".
If nothing can be cut, it prints "Lean already. Ship."`,
		RunE: runReview,
	}
	cmd.Flags().BoolVar(&reviewComplexity, "complexity", false, "Run a ponytail-style complexity review")
	cmd.Flags().StringVar(&reviewPath, "path", ".", "Path to review")
	cmd.Flags().StringVar(&reviewSince, "since", "", "Git ref to diff against (e.g. HEAD~1)")
	cmd.Flags().StringVar(&reviewTags, "tags", "", "Comma-separated tags (delete,stdlib,native,yagni,shrink)")
	cmd.Flags().StringVarP(&reviewFormat, "format", "f", "text", "Output format: text|json|markdown")
	internal.RegisterVersionCmd(cmd)
	return cmd
}

func runReview(_ *cobra.Command, _ []string) error {
	if !reviewComplexity {
		return fmt.Errorf("review mode required; use --complexity")
	}
	root, err := filepath.Abs(reviewPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		if err != nil {
			return fmt.Errorf("path not found: %w", err)
		}
		return fmt.Errorf("path is not a directory: %s", root)
	}

	var tags []string
	if reviewTags != "" {
		for _, t := range strings.Split(reviewTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	markers, err := complexity.ParseMarkers(root)
	if err != nil {
		return fmt.Errorf("parse sin-debt markers: %w", err)
	}
	findings, err := complexity.Find(complexity.Options{
		Root:      root,
		SinceRef:  reviewSince,
		Tags:      tags,
		MarkerMap: markers,
	})
	if err != nil {
		return fmt.Errorf("complexity review: %w", err)
	}
	ranked := complexity.Rank(findings)
	out, err := complexity.Report(ranked, reviewFormat)
	if err != nil {
		return fmt.Errorf("render report: %w", err)
	}
	fmt.Print(out)
	return nil
}
