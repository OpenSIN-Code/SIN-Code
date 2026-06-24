// SPDX-License-Identifier: MIT
// Purpose: `sin-code review --complexity` — ponytail 5-tag complexity review.
// Also contains: `sin-code analyse` — passive bridge shell for the
// sin-analyse-suite Python MCP server (OpenSIN-Code/sin-analyse-suite).
// Docs: review_cmd.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/complexity"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vision"
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

` + "// sin-debt" + `:` + ` markers are respected and shown as "approved: sin-debt".
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

// ============================================================================
// analyse — passive bridge shell for the sin-analyse-suite Python MCP
// ============================================================================

// analyseBinary is the upstream Python MCP entry point.
const analyseBinary = "sin-analyse"

// analyseSuiteVersion is the expected upstream version. Override at
// runtime with SIN_ANALYSE_EXPECTED_VERSION.
const analyseSuiteVersion = "v0.1.0"

// analyseRegistry is the curated list of analyse__* tools shipped by
// the upstream suite. Kept here (not imported from Python) so the CLI
// shell is byte-stable and self-test-only.
var analyseRegistry = []struct {
	Name        string
	Description string
}{
	{"analyse__image_extract", "Extract text + metadata from images (OCR, EXIF, scene)."},
	{"analyse__pdf_parse", "Parse PDF documents to structured text + tables."},
	{"analyse__log_analyze", "Analyze log files: error clusters, tail histograms, anomaly hints."},
	{"analyse__data_detect", "Detect data-file schema (CSV/Parquet/JSON/Arrow) and infer types."},
	{"analyse__audio_transcribe", "Transcribe audio files via Whisper-1 / local whisper.cpp."},
	{"analyse__video_extract", "Extract keyframes + audio track from video files."},
}

// effectiveVersion returns the env-overridable expected version. Empty
// env means fall back to the compile-time constant.
func effectiveVersion() string {
	if v := os.Getenv("SIN_ANALYSE_EXPECTED_VERSION"); v != "" {
		return v
	}
	return analyseSuiteVersion
}

// NewAnalyseCmd returns the `sin-code analyse` subcommand shell.
// Three read-only subcommands; no mutating ops by design.
func NewAnalyseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyse",
		Short: "Passive bridge shell for the sin-analyse-suite Python MCP (read-only)",
		Long: `sin-code analyse is a passive bridge to the sin-analyse-suite Python
MCP server (OpenSIN-Code/sin-analyse-suite). It performs read-only
introspection only — no tool calls are dispatched, no files are
modified. Use ` + "`sin-code serve`" + ` to consume the upstream MCP
tools over JSON-RPC.

Subcommands:
  status   PATH lookup + version banner
  tools    curated catalogue of analyse__* tools (byte-stable)
  doctor   comprehensive readiness check (PATH, API, version)`,
		SilenceUsage: true,
	}
	cmd.AddCommand(newAnalyseStatusCmd())
	cmd.AddCommand(newAnalyseToolsCmd())
	cmd.AddCommand(newAnalyseDoctorCmd())
	return cmd
}

func newAnalyseStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check if `sin-analyse` is on PATH and print version banner",
		Long: `Resolves the upstream Python MCP entry point (sin-analyse) on the
host PATH. If found, prints the resolved path and the expected suite
version. If missing, exits non-zero with install guidance.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			path, err := exec.LookPath(analyseBinary)
			if err != nil {
				fmt.Fprintf(out, "✗ %s: not found on PATH\n", analyseBinary)
				fmt.Fprintf(out, "  expected suite version: %s\n", effectiveVersion())
				fmt.Fprintf(out, "  install:  pipx install sin-analyse-suite\n")
				return fmt.Errorf("%s not on PATH", analyseBinary)
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "FIELD\tVALUE")
			fmt.Fprintf(tw, "binary\t%s\n", analyseBinary)
			fmt.Fprintf(tw, "path\t%s\n", path)
			fmt.Fprintf(tw, "expected_version\t%s\n", effectiveVersion())
			fmt.Fprintf(tw, "invoke\t%s serve\n", path)
			fmt.Fprintf(tw, "shell_mode\tpassive (read-only introspection)\n")
			return tw.Flush()
		},
	}
	return cmd
}

func newAnalyseToolsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List the curated catalogue of analyse__* tools",
		Long: `Prints the byte-stable table of analyse__* tools exposed by the
upstream sin-analyse-suite MCP. This is the local registry view — the
authoritative live list comes from ` + "`sin-code serve`" + ` over
stdio JSON-RPC.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "TOOL\tDESCRIPTION")
			fmt.Fprintf(tw, "(count)\t%d\n", len(analyseRegistry))
			for _, t := range analyseRegistry {
				fmt.Fprintf(tw, "%s\t%s\n", t.Name, t.Description)
			}
			return tw.Flush()
		},
	}
	return cmd
}

func newAnalyseDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Comprehensive readiness check for sin-analyse-suite",
		Long: `Runs all readiness checks:
  - PATH lookup for sin-analyse
  - API reachability placeholder (skipped until upstream ships)
  - Version compatibility against expected suite version

The doctor is advisory only — it never modifies state. Each check
prints PASS / FAIL / SKIP; overall exit is non-zero only when a hard
FAIL (binary missing) is reported.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			var hardFail bool

			// Check 1 — binary on PATH.
			path, lookErr := exec.LookPath(analyseBinary)
			if lookErr != nil {
				fmt.Fprintf(out, "[FAIL] binary    %s not on PATH\n", analyseBinary)
				fmt.Fprintf(out, "        install:  pipx install sin-analyse-suite\n")
				hardFail = true
			} else {
				fmt.Fprintf(out, "[ OK ] binary    %s -> %s\n", analyseBinary, path)
			}

			// Check 2 — API reachability. Passive shell — no live probe.
			fmt.Fprintf(out, "[SKIP] api       reachability probe not wired (passive shell, waiting on upstream)\n")

			// Check 3 — version compatibility. Cannot probe without invoking
			// `sin-analyse --version`, which would exceed the passive contract.
			fmt.Fprintf(out, "[SKIP] version   expected %s; cannot probe without invoking %s --version\n",
				effectiveVersion(), analyseBinary)

			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw)
			fmt.Fprintln(tw, "FIELD\tVALUE")
			fmt.Fprintf(tw, "binary\t%s\n", analyseBinary)
			fmt.Fprintf(tw, "expected_version\t%s\n", effectiveVersion())
			fmt.Fprintf(tw, "tool_count\t%d\n", len(analyseRegistry))
			fmt.Fprintf(tw, "shell_mode\tpassive\n")
			fmt.Fprintf(tw, "override_env\tSIN_ANALYSE_EXPECTED_VERSION\n")
			if hardFail {
				fmt.Fprintf(tw, "verdict\tFAIL\n")
			} else {
				fmt.Fprintf(tw, "verdict\tOK\n")
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if hardFail {
				return fmt.Errorf("doctor: %s missing", analyseBinary)
			}
			return nil
		},
	}
	return cmd
}

// ============================================================================
// analyse-image — native image analysis with a vision-capable LLM (issue #423)
// ============================================================================

// analyseImageHook is a test seam. Production callers leave it nil; tests
// replace it to avoid real API calls.
var analyseImageHook func(context.Context, string, vision.Config) (*vision.AnalyzeResult, error)

// NewAnalyseImageCmd returns the `sin-code analyse-image` cobra command.
func NewAnalyseImageCmd() *cobra.Command {
	var (
		prompt     string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "analyse-image <path>",
		Short: "Analyze an image with a vision-capable LLM (no Tesseract)",
		Long: `sin-code analyse-image reads an image file and sends it to a vision-
capable LLM (default: minimax-m3 on Fireworks AI). It returns a structured
description including visible text, UI elements, and layout.

Configuration precedence (highest first):
  1. SIN_ANALYSE_IMAGE_MODEL / SIN_ANALYSE_IMAGE_API_KEY / SIN_ANALYSE_IMAGE_BASE_URL
  2. llm.model / llm.api_key / llm.base_url from sin-code config
  3. Built-in default model: accounts/fireworks/models/minimax-m3

The command is read-only and does not modify the image or workspace.

Examples:
  sin-code analyse-image screenshot.png
  sin-code analyse-image diagram.png --prompt "List every UI element."
  sin-code analyse-image assets/chart.png --json`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			out := cmd.OutOrStdout()

			cfg, err := internal.VisionConfigFromEnv()
			if err != nil {
				return fmt.Errorf("analyse-image: %w", err)
			}
			if prompt != "" {
				cfg.Prompt = prompt
			}

			var result *vision.AnalyzeResult
			if analyseImageHook != nil {
				result, err = analyseImageHook(cmd.Context(), path, cfg)
			} else {
				result, err = vision.AnalyzeImageWithConfig(cmd.Context(), path, cfg)
			}
			if err != nil {
				return fmt.Errorf("analyse-image: %w", err)
			}

			if jsonOutput {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			fmt.Fprintln(out, result.Description)
			return nil
		},
	}
	cmd.Flags().StringVar(&prompt, "prompt", "", "Custom prompt for the vision model")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output structured JSON (description, model, provider)")
	return cmd
}
