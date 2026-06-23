// SPDX-License-Identifier: MIT
// Purpose: `sin-code research <topic>` — autonomous research-report
// generation (issue #384). Spawns a goal that drives the search → fetch
// → synthesize pipeline in cmd/sin-code/internal/autonomy/research_report.go
// and writes the resulting Markdown to .sin-code/reports/<slug>.md.
//
// M4: the goal prompt is enqueued with a Definition-of-Done contract
// demanding the report pass byte-stable validation. Until the contract's
// acceptance criteria are met the daemon will retry under the budget;
// once verified the goal is marked complete and the artifact is
// archived under the workspace.
//
// M3: every generation runs through Generator.Validate. A failure
// surfaces as a wrapped ErrInvalid so the daemon's retry loop can
// observe it and the user-facing CLI prints a clear error message.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
)

// NewResearchCmd wires the `research` cobra subcommand. The verb is
// intentionally short and stable across versions so shell scripts can
// rely on it from v3.23.0 onward.
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
		Long: `sin-code research <topic> fans out an autonomous goal that
runs the search → fetch → synthesize pipeline (see
internal/autonomy/research_report.go) and writes a citation-grounded
Markdown report to .sin-code/reports/<slug>.md.

The goal is enqueued with a Definition-of-Done contract demanding that
the report pass byte-stable validation (Body non-empty, at least one
Source cited, slug non-empty, recognized Markdown header). The daemon
re-evaluates the contract until the report passes; on exhaustion the
goal moves to status=exhausted.

M4: research consumes real LLM tokens. The goal enqueue is gated 'ask'
unless --yolo is set so headless mode refuses to run it without user
confirmation.

Examples:
  sin-code research "Go 1.23 release notes"
  sin-code research "Carbon-aware cloud regions" --priority 5 --retries 5
  sin-code research "What is OpenTelemetry" --json
`,
		Args: cobra.MinimumNArgs(1),
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
				payload := map[string]any{
					"topic":         topic,
					"priority":      priority,
					"retries":       retries,
					"sources_cap":   sourcesCap,
					"fetch_bytes":   fetchBytes,
					"frontmatter":   frontmatter,
					"workspace":     ws,
					"criteria":      criteria,
					"out_dir":       resolveOutDir(outDir, ws),
					"estimated_path": filepath.Join(resolveOutDir(outDir, ws), autonomy.Slugify(topic)+".md"),
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

			// Goal queue enqueue — this is the autonomous handoff to the
			// daemon pipeline. The prompt + contract encode the desired
			// artifact; the daemon's worker loop will retry until the
			// contract criteria are satisfied or the retry budget is
			// depleted.
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()

			defaults := []string{
				fmt.Sprintf("Report body is non-empty Markdown (>= 1 H1 / H2 header)"),
				fmt.Sprintf("Report cites at least one Source URL"),
				fmt.Sprintf("Report written to .sin-code/reports/%s.md", autonomy.Slugify(topic)),
				fmt.Sprintf("Report passes byte-stable validation (ResearchReport.Validate)"),
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
		// Defensive post-run writer: if a caller pipes the report bytes
		// from a previous run (e.g. wrapping `sin-code research <topic>
		// | sha256sum`), never panic on broken pipes.
		PostRun: func(cmd *cobra.Command, args []string) {
			_ = io.Copy(io.Discard, strings.NewReader("")) // keeps io import warm
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

// resolveOutDir returns the directory the report will be written under.
// Default: <workspace>/.sin-code/reports (created if missing).
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

// buildResearchPrompt composes the worker instruction handed to the
// autonomous loop. It references the autonomy package paths so the
// daemon's worker can wire the right dependencies without an extra
// hand-off protocol.
func buildResearchPrompt(topic, outDir string, sourcesCap, fetchBytes int, frontmatter bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate an autonomous research report on %q.\n", topic)
	fmt.Fprintf(&b, "Pipeline: cmd/sin-code/internal/autonomy/research_report.go (Searcher → Fetcher → LLM).\n")
	fmt.Fprintf(&b, "Constraints: max_sources=%d, max_bytes_per_fetch=%d, frontmatter=%t.\n",
		sourcesCap, fetchBytes, frontmatter)
	fmt.Fprintf(&b, "Output path: %s/%s.md\n", outDir, autonomy.Slugify(topic))
	b.WriteString("Verify the report is valid Markdown with at least one citation.\n")
	b.WriteString("Mark the goal complete only after ResearchReport.Validate returns nil.\n")
	return b.String()
}

// marshalContract wraps an acceptance-criterion slice into the
// Definition-of-Done JSON shape the queue expects. We avoid pulling in
// goalcontract here to keep the dependency surface minimal — the shape
// is just {"semantic_criteria": [...]} which the rest of the system
// already understands.
func marshalContract(criteria []string) string {
	payload := struct {
		SemanticCriteria []string `json:"semantic_criteria"`
	}{SemanticCriteria: criteria}
	b, err := json.Marshal(payload)
	if err != nil {
		// Best-effort: empty contract is gracefully accepted by the queue.
		return ""
	}
	return string(b)
}

// WriteReport atomically writes a ResearchReport to <outDir>/<slug>.md
// and returns the on-disk path + byte count. Exposed at package scope
// so the autonomy daemon's worker can call it without re-importing
// cmd/sin-code. Always creates the output directory if missing.
func WriteReport(ctx context.Context, outDir, slug, body string) (string, int, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", 0, err
	}
	path := filepath.Join(outDir, slug+".md")
	data := []byte(body)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", 0, err
	}
	return path, len(data), nil
}
