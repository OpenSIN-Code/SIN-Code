// SPDX-License-Identifier: MIT
// Purpose: `sin-code research <topic>` — autonomous research-report
// generation (issue #384).
// Also hosts `sin-code codegraph` — operator + agent entry point for the
// CodeGraph bridge (issue #126), an external multi-language static-analysis
// engine exposed as an MCP tool. Follows the Bridged-External-Contract:
// CodeGraph is never vendored; we shell out to the user's installed binary.
// Docs: docs/codegraph-integration.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/codegraph"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

func NewResearchCmd() *cobra.Command {
	var (
		priority    int
		retries     int
		sourcesCap  int
		fetchBytes  int
		outDir      string
		dryRun      bool
		jsonOut     bool
		frontmatter bool
		criteria    []string
	)

	cmd := &cobra.Command{
		Use:   "research <topic>",
		Short: "Autonomous research-report generation (issue #384)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			topic := strings.TrimSpace(strings.Join(args, " "))
			if topic == "" {
				return fmt.Errorf("research: empty topic")
			}
			ws, err := os.Getwd()
			if err != nil {
				return err
			}

			if dryRun {
				resolvedDir := resolveOutDir(outDir, ws)
				payload := map[string]any{
					"topic":          topic,
					"priority":       priority,
					"retries":        retries,
					"sources_cap":    sourcesCap,
					"fetch_bytes":    fetchBytes,
					"frontmatter":    frontmatter,
					"workspace":      ws,
					"criteria":       criteria,
					"out_dir":        resolvedDir,
					"estimated_path": filepath.Join(resolvedDir, autonomy.Slugify(topic)+".md"),
				}
				if jsonOut {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(payload)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "DRY-RUN: topic=%q priority=%d retries=%d out=%s\n",
					topic, priority, retries, payload["estimated_path"])
				return nil
			}

			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()

			defaults := []string{
				"Report body is non-empty Markdown (>= 1 H1 / H2 header)",
				"Report cites at least one Source URL",
				fmt.Sprintf("Report written to .sin-code/reports/%s.md", autonomy.Slugify(topic)),
				"Report passes byte-stable validation (ResearchReport.Validate)",
			}
			contractCriteria := append(defaults, criteria...)
			prompt := buildResearchPrompt(topic, resolveOutDir(outDir, ws), sourcesCap, fetchBytes, frontmatter)

			id, err := q.AddWithContract(cmd.Context(), prompt, ws, priority, retries, marshalContract(contractCriteria))
			if err != nil {
				return fmt.Errorf("research: enqueue: %w", err)
			}

			payload := struct {
				GoalID    int64  `json:"goal_id"`
				Topic     string `json:"topic"`
				Slug      string `json:"slug"`
				OutPath   string `json:"out_path"`
				Workspace string `json:"workspace"`
				Priority  int    `json:"priority"`
				Retries   int    `json:"retries"`
				Criteria  int    `json:"criteria"`
			}{
				GoalID:    id,
				Topic:     topic,
				Slug:      autonomy.Slugify(topic),
				OutPath:   filepath.Join(resolveOutDir(outDir, ws), autonomy.Slugify(topic)+".md"),
				Workspace: ws,
				Priority:  priority,
				Retries:   retries,
				Criteria:  len(contractCriteria),
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"research: goal %d enqueued topic=%q slug=%s out=%s retries=%d\n",
				id, topic, payload.Slug, payload.OutPath, retries)
			return nil
		},
	}

	cmd.Flags().IntVar(&priority, "priority", 0, "higher runs sooner")
	cmd.Flags().IntVar(&retries, "retries", 3, "retry budget when contract criteria unmet")
	cmd.Flags().IntVar(&sourcesCap, "sources", 5, "max sources to fetch before synthesis")
	cmd.Flags().IntVar(&fetchBytes, "fetch-bytes", 64*1024, "max bytes fetched per source")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "override .sin-code/reports/ output directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print plan without enqueueing")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON envelope")
	cmd.Flags().BoolVar(&frontmatter, "frontmatter", false, "wrap report body in topic + timestamp header")
	cmd.Flags().StringArrayVar(&criteria, "criteria", nil, "additional acceptance criterion (repeatable)")
	return cmd
}

func resolveOutDir(override, workspace string) string {
	base := workspace
	if base == "" {
		base = "."
	}
	if strings.TrimSpace(override) != "" {
		return override
	}
	return filepath.Join(base, ".sin-code", "reports")
}

func buildResearchPrompt(topic, outDir string, sourcesCap, fetchBytes int, frontmatter bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate an autonomous research report on %q.\n", topic)
	b.WriteString("Pipeline: cmd/sin-code/internal/autonomy/research_report.go (Searcher -> Fetcher -> LLM).\n")
	fmt.Fprintf(&b, "Constraints: max_sources=%d, max_bytes_per_fetch=%d, frontmatter=%t.\n",
		sourcesCap, fetchBytes, frontmatter)
	fmt.Fprintf(&b, "Output path: %s/%s.md\n", outDir, autonomy.Slugify(topic))
	b.WriteString("Verify the report is valid Markdown with at least one citation.\n")
	b.WriteString("Mark the goal complete only after ResearchReport.Validate returns nil.\n")
	return b.String()
}

func marshalContract(criteria []string) string {
	payload := struct {
		SemanticCriteria []string `json:"semantic_criteria"`
	}{SemanticCriteria: criteria}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

func WriteReport(outDir, slug, body string) (string, int, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", 0, err
	}
	path := filepath.Join(outDir, slug+".md")
	data := []byte(body)
	if err := os.WriteFile(path, data, filemode.Default()); err != nil {
		return "", 0, err
	}
	return path, len(data), nil
}

// codegraphBridge is the minimal interface used by the codegraph subcommand so
// tests can inject a fake bridge without a real CodeGraph binary.
type codegraphBridge interface {
	Analyze(ctx context.Context, path string) (*codegraph.Graph, error)
	Find() (string, error)
	Version(ctx context.Context) (string, error)
}

// codegraphHookVars holds injectable dependencies for the codegraph
// subcommand. Coverage tests replace these fields to avoid requiring a real
// CodeGraph binary on PATH.
var codegraphHookVars = struct {
	newBridge func() codegraphBridge
}{
	newBridge: func() codegraphBridge { return codegraph.New() },
}

// NewCodeGraphCmd builds the `codegraph` cobra subcommand (analyze + doctor),
// matching the gh/vane/dox/rtk external-bridge pattern.
func NewCodeGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codegraph",
		Short: "Bridge to CodeGraph for multi-language code analysis",
		Long: `sin-code codegraph bridges CodeGraph (https://github.com/codegraph-ai/codegraph,
never vendored), a static-analysis engine that builds a symbol/edge graph
across many languages. It powers code-aware navigation for the agent.

  sin-code codegraph analyze .            # graph summary for the repo
  sin-code codegraph analyze --json .     # raw JSON graph for tooling/MCP
  sin-code codegraph doctor               # check CodeGraph is installed

When CodeGraph is not installed, commands fail with a clear install hint.`,
	}
	cmd.AddCommand(newCodeGraphAnalyzeCmd())
	cmd.AddCommand(newCodeGraphDoctorCmd())
	return cmd
}

// newCodeGraphAnalyzeCmd runs an analysis and prints either a human summary
// or the raw JSON graph (for MCP / downstream tooling).
func newCodeGraphAnalyzeCmd() *cobra.Command {
	var asJSON bool
	var timeout time.Duration
	c := &cobra.Command{
		Use:   "analyze [path]",
		Short: "Analyze a path and print the code graph (summary or --json)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			g, err := codegraphHookVars.newBridge().Analyze(ctx, path)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(g)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "CodeGraph: %s\n  nodes: %d\n  edges: %d\n",
				g.Root, len(g.Nodes), len(g.Edges))
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit the raw JSON graph instead of a summary")
	c.Flags().DurationVar(&timeout, "timeout", 120*time.Second, "max time to wait for analysis (0 = no timeout)")
	return c
}

// newCodeGraphDoctorCmd verifies CodeGraph is installed and prints its version.
func newCodeGraphDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that CodeGraph is installed and reachable",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			b := codegraphHookVars.newBridge()
			path, err := b.Find()
			if err != nil {
				fmt.Fprintln(os.Stderr, "codegraph: NOT installed")
				return err
			}
			ver, verr := b.Version(ctx)
			fmt.Fprintf(cmd.OutOrStdout(), "codegraph: OK\n  path:    %s\n", path)
			if verr == nil && ver != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  version: %s\n", ver)
			}
			return nil
		},
	}
}
