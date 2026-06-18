// SPDX-License-Identifier: MIT
// Purpose: `sin-code analyse` — passive bridge shell for the
// sin-analyse-suite Python MCP server (OpenSIN-Code/sin-analyse-suite).
// The actual tool surface lives in the upstream Python MCP; this CLI is
// read-only introspection only (status / tools / doctor). No tool calls
// are dispatched, no files are modified from this subcommand.
//
// The upstream binary is `sin-analyse` (invoked as `sin-analyse serve`
// per the registry entry in cmd/sin-code/internal/<bridge>/registry.go).
//
// Subcommands:
//
//	analyse status   # is sin-analyse on PATH + version banner
//	analyse tools    # curated catalogue of analyse__* tools
//	analyse doctor   # comprehensive readiness check (PATH, API, version)
//
// This file is deliberately NOT registered in cmd/sin-code/main.go.
// The Python upstream is still landing; the shell stays self-test-only
// until the MCP tool surface stabilises.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

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
