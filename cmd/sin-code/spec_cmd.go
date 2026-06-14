// SPDX-License-Identifier: MIT
// Purpose: `sin-code spec` — Cobra command group for Spec Layer management.
// Supports: spec init, spec validate, spec create, spec archive, spec list, spec show.
// All operations are deterministic and non-breaking to existing Agent Loop.
// Docs: cmd/sin-code/spec_cmd.go.doc.md
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/internal/spec"
)

// NewSpecCmd builds the `spec` cobra subcommand group.
func NewSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Manage SIN-Code Specs (specifications layer)",
		Long: `sin-code spec provides comprehensive spec management for the
SIN-Code Spec Layer. All operations are deterministic and LLM-free.

Subcommands:
  init      Initialize a new spec collection in .sin/specs/
  validate  Validate all specs in a collection
  create    Create a new spec interactively or via stdin
  archive   Archive an existing spec
  list      List all specs in collection
  show      Display a single spec's details
  merge     Three-way merge two specs

Examples:
  sin-code spec init
  sin-code spec create --kind goal --title "Auth System"
  sin-code spec validate --check-cycles
  sin-code spec show spec_auth_001
`,
	}

	cmd.AddCommand(newSpecInitCmd())
	cmd.AddCommand(newSpecValidateCmd())
	cmd.AddCommand(newSpecCreateCmd())
	cmd.AddCommand(newSpecArchiveCmd())
	cmd.AddCommand(newSpecListCmd())
	cmd.AddCommand(newSpecShowCmd())
	cmd.AddCommand(newSpecMergeCmd())

	return cmd
}

// ── init ──────────────────────────────────────────────────────────────

func newSpecInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new spec collection",
		Long: `Initialize a new spec collection in .sin/specs/.
Creates default directories and metadata files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			specDir := filepath.Join(".sin", "specs")

			// Create directory structure
			dirs := []string{
				specDir,
				filepath.Join(specDir, "active"),
				filepath.Join(specDir, "drafts"),
				filepath.Join(specDir, "archive"),
			}

			for _, dir := range dirs {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
			}

			// Create collection metadata
			collection := spec.NewCollection("root", "SIN-Code Spec Collection")
			data, err := json.MarshalIndent(collection, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal collection: %w", err)
			}

			metaPath := filepath.Join(specDir, "collection.json")
			if err := os.WriteFile(metaPath, data, 0644); err != nil {
				return fmt.Errorf("failed to write collection metadata: %w", err)
			}

			fmt.Printf("✓ Initialized spec collection in %s\n", specDir)
			fmt.Printf("  Active:  %s\n", filepath.Join(specDir, "active"))
			fmt.Printf("  Drafts:  %s\n", filepath.Join(specDir, "drafts"))
			fmt.Printf("  Archive: %s\n", filepath.Join(specDir, "archive"))

			return nil
		},
	}
}

// ── validate ──────────────────────────────────────────────────────────

func newSpecValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate all specs in the collection",
		Long: `Validate all specs in the collection for:
  - Required fields (title, description, goals)
  - Markdown syntax
  - Dependency graph (cycles, missing refs)
  - Token budgets`,
		RunE: func(cmd *cobra.Command, args []string) error {
			checkCycles, _ := cmd.Flags().GetBool("check-cycles")
			checkTokens, _ := cmd.Flags().GetBool("check-tokens")
			maxTokens, _ := cmd.Flags().GetInt("max-tokens")

			specDir := filepath.Join(".sin", "specs")
			collection, err := loadCollection(specDir)
			if err != nil {
				return err
			}

			// Load all specs from files
			if err := loadSpecsFromDir(collection, specDir); err != nil {
				return err
			}

			validationResults := make(map[string]spec.ValidationResult)
			var errorCount int

			// Validate each spec
			for id, s := range collection.Specs {
				result := spec.ValidateSpec(s)
				validationResults[id] = result
				if !result.Valid {
					errorCount += len(result.Errors)
					fmt.Printf("✗ %s: %s\n", id, result.Summary())
					for _, err := range result.Errors {
						fmt.Printf("    • %s\n", err.Message)
					}
				} else {
					fmt.Printf("✓ %s validated\n", id)
				}
			}

			// Check cycles if requested
			if checkCycles {
				depResult := spec.ValidateDependencies(collection)
				if !depResult.Valid {
					errorCount += len(depResult.Errors)
					fmt.Println("\n✗ Dependency graph errors:")
					for _, err := range depResult.Errors {
						fmt.Printf("    • %s: %s\n", err.SpecID, err.Message)
					}
				} else {
					fmt.Println("\n✓ Dependency graph valid (no cycles)")
				}
			}

			// Check token budget if requested
			if checkTokens {
				tokenResult := spec.ValidateTokenBudget(collection, maxTokens)
				if !tokenResult.Valid {
					errorCount += len(tokenResult.Errors)
					fmt.Println("\n✗ Token budget exceeded:")
					for _, err := range tokenResult.Errors {
						fmt.Printf("    • %s\n", err.Message)
					}
				} else {
					fmt.Printf("\n✓ Token budget OK: %d / %d\n", 
						collection.Statistics.TotalTokenEstimate, maxTokens)
				}
			}

			if errorCount > 0 {
				return fmt.Errorf("%d validation error(s)", errorCount)
			}

			fmt.Printf("\n✓ All %d spec(s) validated successfully\n", len(collection.Specs))
			return nil
		},
	}

	cmd.Flags().Bool("check-cycles", false, "Check dependency graph for cycles")
	cmd.Flags().Bool("check-tokens", false, "Check token budget")
	cmd.Flags().Int("max-tokens", 100000, "Maximum total token budget")

	return cmd
}

// ── create ────────────────────────────────────────────────────────────

func newSpecCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new spec",
		Long: `Create a new spec with interactive prompts or via flags.
Saves to .sin/specs/drafts/ by default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			title, _ := cmd.Flags().GetString("title")
			kind, _ := cmd.Flags().GetString("kind")
			namespace, _ := cmd.Flags().GetString("namespace")

			if title == "" {
				return fmt.Errorf("--title is required")
			}
			if kind == "" {
				return fmt.Errorf("--kind is required (goal, process, constraint, component, integration)")
			}

			// Generate ID
			id := fmt.Sprintf("spec_%s_%d", 
				kind[:3], time.Now().UnixNano()%1000000)

			// Create spec
			s := spec.NewSpec(id, title, spec.SpecKind(kind))
			s.Namespace = namespace

			// Validate
			result := spec.ValidateSpec(s)
			if !result.Valid {
				return fmt.Errorf("validation failed: %s", result.Summary())
			}

			// Save to file
			specDir := filepath.Join(".sin", "specs", "drafts")
			os.MkdirAll(specDir, 0755)

			filename := filepath.Join(specDir, id+".json")
			data, _ := json.MarshalIndent(s, "", "  ")
			if err := os.WriteFile(filename, data, 0644); err != nil {
				return fmt.Errorf("failed to save spec: %w", err)
			}

			fmt.Printf("✓ Created spec: %s\n", id)
			fmt.Printf("  Title:     %s\n", title)
			fmt.Printf("  Kind:      %s\n", kind)
			fmt.Printf("  Saved to:  %s\n", filename)

			return nil
		},
	}

	cmd.Flags().String("title", "", "Spec title (required)")
	cmd.Flags().String("kind", "", "Spec kind: goal|process|constraint|component|integration (required)")
	cmd.Flags().String("namespace", "", "Spec namespace (optional)")
	cmd.MarkFlagRequired("title")
	cmd.MarkFlagRequired("kind")

	return cmd
}

// ── archive ───────────────────────────────────────────────────────────

func newSpecArchiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "archive [spec-id]",
		Short: "Archive a spec (move to inactive)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]
			reason, _ := cmd.Flags().GetString("reason")

			specDir := filepath.Join(".sin", "specs")
			collection, err := loadCollection(specDir)
			if err != nil {
				return err
			}

			if err := loadSpecsFromDir(collection, specDir); err != nil {
				return err
			}

			s, ok := collection.Specs[specID]
			if !ok {
				return fmt.Errorf("spec not found: %s", specID)
			}

			// Archive the spec
			archived := s.Archive(reason)
			s.Status = spec.SpecStatusArchived
			s.UpdatedAt = time.Now()

			// Save archive
			archiveDir := filepath.Join(specDir, "archive")
			os.MkdirAll(archiveDir, 0755)

			archiveFile := filepath.Join(archiveDir, fmt.Sprintf("%s_v%d.json", specID, s.Version))
			data, _ := json.MarshalIndent(archived, "", "  ")
			os.WriteFile(archiveFile, data, 0644)

			fmt.Printf("✓ Archived spec: %s\n", specID)
			fmt.Printf("  Reason: %s\n", reason)
			fmt.Printf("  Saved to: %s\n", archiveFile)

			return nil
		},
	}
}

// ── list ──────────────────────────────────────────────────────────────

func newSpecListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all specs",
		RunE: func(cmd *cobra.Command, args []string) error {
			specDir := filepath.Join(".sin", "specs")
			collection, err := loadCollection(specDir)
			if err != nil {
				return err
			}

			if err := loadSpecsFromDir(collection, specDir); err != nil {
				return err
			}

			if len(collection.Specs) == 0 {
				fmt.Println("No specs found")
				return nil
			}

			fmt.Println("Specs:")
			for id, s := range collection.Specs {
				fmt.Printf("  [%s] %s (%s, %s)\n", id, s.Title, s.Kind, s.Status)
			}

			return nil
		},
	}
}

// ── show ──────────────────────────────────────────────────────────────

func newSpecShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [spec-id]",
		Short: "Display a spec's details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]

			specDir := filepath.Join(".sin", "specs")
			collection, err := loadCollection(specDir)
			if err != nil {
				return err
			}

			if err := loadSpecsFromDir(collection, specDir); err != nil {
				return err
			}

			s, ok := collection.Specs[specID]
			if !ok {
				return fmt.Errorf("spec not found: %s", specID)
			}

			fmt.Println(s.MarkdownFormat())
			return nil
		},
	}
}

// ── merge ─────────────────────────────────────────────────────────────

func newSpecMergeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "merge [base] [ours] [theirs]",
		Short: "Three-way merge two specs",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			specDir := filepath.Join(".sin", "specs")
			collection, err := loadCollection(specDir)
			if err != nil {
				return err
			}

			if err := loadSpecsFromDir(collection, specDir); err != nil {
				return err
			}

			base, ok1 := collection.Specs[args[0]]
			ours, ok2 := collection.Specs[args[1]]
			theirs, ok3 := collection.Specs[args[2]]

			if !ok1 || !ok2 || !ok3 {
				return fmt.Errorf("one or more specs not found")
			}

			strategy, _ := cmd.Flags().GetString("strategy")
			result := spec.MergeSpecs(base, ours, theirs, spec.MergeStrategy(strategy))

			fmt.Println(result.String())

			if result.Successful {
				fmt.Printf("\n✓ Merge successful\n")
				data, _ := json.MarshalIndent(result.Merged, "", "  ")
				fmt.Printf("\nMerged Spec:\n%s\n", string(data))
			}

			return nil
		},
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

// loadCollection loads the collection metadata.
func loadCollection(specDir string) (*spec.SpecCollection, error) {
	collPath := filepath.Join(specDir, "collection.json")
	data, err := os.ReadFile(collPath)
	if err != nil {
		// Return empty collection if not found
		return spec.NewCollection("root", "SIN-Code Spec Collection"), nil
	}

	var collection spec.SpecCollection
	if err := json.Unmarshal(data, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse collection: %w", err)
	}

	return &collection, nil
}

// loadSpecsFromDir loads all spec files from the directory tree.
func loadSpecsFromDir(collection *spec.SpecCollection, specDir string) error {
	dirs := []string{"active", "drafts", "archive"}

	for _, dir := range dirs {
		path := filepath.Join(specDir, dir)
		entries, err := os.ReadDir(path)
		if err != nil {
			continue // Skip missing dirs
		}

		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
				specPath := filepath.Join(path, entry.Name())
				data, err := os.ReadFile(specPath)
				if err != nil {
					continue
				}

				var s spec.Spec
				if err := json.Unmarshal(data, &s); err != nil {
					continue
				}

				collection.AddSpec(&s)
			}
		}
	}

	return nil
}
