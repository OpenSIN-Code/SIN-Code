// SPDX-License-Identifier: MIT
// Purpose: `sin-code stack` — one-shot install/doctor for the 3-layer
// methodology stack (DOX context + Superpowers methodology + Vane
// research sidecar) on top of the SIN-Code tool layer.
// Also contains: `sin-code vane` — CLI binding for the Vane bridge
// (ItzCrazyKns/Vane, MIT, self-hosted AI answering engine with cited
// sources). HTTP bridge + stdio MCP server, graceful degradation →
// websearch fallback.
// Docs: stack.doc.md, vane.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/stack"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vane"
)

// NewStackCmd builds the `stack` cobra subcommand. Pattern matches
// NewVaneCmd: returns *cobra.Command with install + doctor attached.
func NewStackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stack",
		Short: "Manage the DOX + Superpowers + Vane methodology stack",
		Long: `sin-code stack is the one-shot manager for the methodology stack:

  Layer 1 (context):    DOX         — hierarchical AGENTS.md trees
  Layer 2 (methodology): Superpowers — mandatory workflows (TDD, debugging)
  Layer 3 (tools):      SIN-Code    — verified sin_* MCP tools
  Sidecar (research):   Vane        — citation-backed answering engine

'install' sets up all layers (idempotent). 'doctor' validates them
without modifying anything. Graceful degradation: a missing or down
optional layer is reported, never fatal.`,
	}

	cmd.AddCommand(newStackInstallCmd())
	cmd.AddCommand(newStackDoctorCmd())
	return cmd
}

func newStackInstallCmd() *cobra.Command {
	var (
		skipSuperpowers bool
		skipDox         bool
		skipVane        bool
		agentsMDPath    string
		vaneURL         string
		jsonOut         bool
	)
	c := &cobra.Command{
		Use:   "install",
		Short: "Install/refresh all stack layers (idempotent)",
		RunE: func(_ *cobra.Command, _ []string) error {
			opts := stack.InstallOptions{
				SkipSuperpowers: skipSuperpowers,
				SkipDox:         skipDox,
				SkipVane:        skipVane,
				AgentsMDPath:    agentsMDPath,
				VaneURL:         vaneURL,
			}
			report := stack.Install(opts)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			fmt.Print(stack.Format(report))
			if !report.AllOK {
				return fmt.Errorf("stack install completed with errors")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&skipSuperpowers, "skip-super-powers", false, "Do not install Superpowers")
	c.Flags().BoolVar(&skipDox, "skip-dox", false, "Do not inject the DOX protocol")
	c.Flags().BoolVar(&skipVane, "skip-vane", false, "Do not configure the Vane bridge")
	c.Flags().StringVar(&agentsMDPath, "agents-md", "", "Target AGENTS.md (default: ./AGENTS.md)")
	c.Flags().StringVar(&vaneURL, "vane-url", "", "Vane base URL (default: existing config)")
	c.Flags().BoolVar(&jsonOut, "json", false, "Machine-readable report")
	return c
}

func newStackDoctorCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "doctor [root]",
		Short: "Validate all stack layers without modifying anything",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			report := stack.Doctor(root)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			fmt.Print(stack.Format(report))
			if !report.AllOK {
				return fmt.Errorf("stack doctor found problems")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "Machine-readable report")
	return c
}

// ============================================================================
// vane — CLI binding for the Vane bridge
// ============================================================================

// NewVaneCmd builds the `vane` cobra subcommand. Pattern matches
// NewSuperpowersCmd / NewDoxCmd: returns *cobra.Command with the
// relevant subcommands attached.
func NewVaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vane",
		Short: "Bridge to a self-hosted Vane answering engine (citation-backed research)",
		Long: `sin-code vane bridges ItzCrazyKns/Vane (MIT, https://github.com/ItzCrazyKns/Vane),
a privacy-focused, self-hosted AI answering engine that returns synthesized
answers with cited source URLs. Vane is NEVER vendored (Bridged-External-
Contract, like GitNexus/RTK) — it runs as its own Docker container and is
consumed via its HTTP Search API with graceful degradation.

If the Vane instance is not running, vane_research returns a structured
fallback hint pointing at the websearch ecosystem skill — it never crashes
the agent loop.`,
	}

	cmd.AddCommand(newVaneSetupCmd())
	cmd.AddCommand(newVaneDoctorCmd())
	cmd.AddCommand(newVaneSearchCmd())
	cmd.AddCommand(newVaneConfigCmd())
	cmd.AddCommand(newVaneServeCmd())
	return cmd
}

func newVaneSetupCmd() *cobra.Command {
	var urlFlag string
	c := &cobra.Command{
		Use:   "setup",
		Short: "Register Vane MCP bridge and persist config; print Docker command if no instance found",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, _ := vane.LoadConfig()
			if urlFlag != "" {
				u, err := url.Parse(urlFlag)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
					return fmt.Errorf("--url must be a valid http(s) URL, got %q", urlFlag)
				}
				cfg.BaseURL = strings.TrimRight(urlFlag, "/")
			}
			if err := vane.SaveConfig(cfg); err != nil {
				return err
			}
			mcpPath := vane.MCPConfigPath()
			if _, err := vane.RegisterMCP(mcpPath); err != nil {
				return fmt.Errorf("register MCP: %w", err)
			}
			fmt.Println("vane MCP bridge registered in:", mcpPath)
			fmt.Println("config written to:", vane.ConfigPath())

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			if err := vane.NewClient(cfg).Healthy(ctx); err == nil {
				fmt.Println("Vane instance reachable at", cfg.BaseURL)
				return nil
			}
			fmt.Println("no running Vane instance at", cfg.BaseURL)
			fmt.Println("\nStart one with Docker (includes bundled SearxNG):")
			fmt.Println("  docker run -d -p 3000:3000 -v vane-data:/home/vane/data --name vane itzcrazykns1337/vane:latest")
			fmt.Println("\nThen open http://localhost:3000 once to configure models (e.g. Ollama),")
			fmt.Println("and verify with: sin-code vane doctor")
			if _, err := exec.LookPath("docker"); err != nil {
				fmt.Println("\nNote: docker not found in PATH — install Docker first.")
			}
			return nil
		},
	}
	c.Flags().StringVar(&urlFlag, "url", "", "Override Vane base URL (must be http(s))")
	return c
}

func newVaneDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check reachability of the configured Vane instance",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, present, err := vane.LoadConfig()
			if err != nil {
				return err
			}
			if !present {
				fmt.Println("vane bridge not configured — run: sin-code vane setup")
				return fmt.Errorf("vane not configured")
			}
			fmt.Println("base_url:", cfg.BaseURL)
			fmt.Println("config_path:", vane.ConfigPath())
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			if err := vane.NewClient(cfg).Healthy(ctx); err != nil {
				fmt.Println("unreachable:", err)
				return fmt.Errorf("vane doctor failed")
			}
			fmt.Println("Vane is reachable and ready")
			return nil
		},
	}
}

func newVaneSearchCmd() *cobra.Command {
	var focusMode, optimization string
	c := &cobra.Command{
		Use:   "search <query...>",
		Short: "Run a citation-backed search from the CLI (non-interactive)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := vane.LoadConfig()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
			defer cancel()
			answer, err := vane.NewClient(cfg).Search(ctx, strings.Join(args, " "), focusMode, optimization)
			if err != nil {
				return err
			}
			fmt.Println(vane.FormatAnswer(answer))
			return nil
		},
	}
	c.Flags().StringVar(&focusMode, "focus", "webSearch", "Focus mode: webSearch|academicSearch|writingAssistant|wolframAlphaSearch|youtubeSearch|redditSearch")
	c.Flags().StringVar(&optimization, "optimization", "balanced", "Optimization: speed|balanced")
	return c
}

func newVaneConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the active Vane bridge configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _, err := vane.LoadConfig()
			if err != nil {
				return err
			}
			orDef := func(s, def string) string {
				if s == "" {
					return def
				}
				return s
			}
			fmt.Printf("base_url:           %s\n", cfg.BaseURL)
			fmt.Printf("chat_provider:      %s\n", orDef(cfg.ChatProvider, "(instance default)"))
			fmt.Printf("chat_model:         %s\n", orDef(cfg.ChatModel, "(instance default)"))
			fmt.Printf("embedding_provider: %s\n", orDef(cfg.EmbedProvider, "(instance default)"))
			fmt.Printf("embedding_model:    %s\n", orDef(cfg.EmbedModel, "(instance default)"))
			fmt.Printf("timeout_seconds:    %d\n", cfg.TimeoutSeconds)
			fmt.Printf("config_path:        %s\n", vane.ConfigPath())
			return nil
		},
	}
}

func newVaneServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the vane stdio MCP bridge server (used by mcp.json)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return vane.Serve(cmd.Context())
		},
	}
}
