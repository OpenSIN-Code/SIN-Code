// SPDX-License-Identifier: MIT
// Purpose: `sin-code analyse-image` — native image analysis using a vision-
// capable LLM instead of Tesseract OCR. Issue #423.
//
// The command is read-only: it reads the image file and calls the configured
// LLM provider. No workspace files are modified.
//
// Examples:
//
//	sin-code analyse-image screenshot.png
//	sin-code analyse-image diagram.png --prompt "List every UI element."
//	sin-code analyse-image assets/chart.png --json
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vision"
)

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
