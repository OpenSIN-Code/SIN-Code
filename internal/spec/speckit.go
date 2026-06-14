// SPDX-License-Identifier: MIT
// Purpose: SpecKit — Spec Layer chat integration with slash-commands.
// Provides interactive spec commands within the agent loop: /spec, /goal, /verify, etc. (Phase 5).
// Docs: internal/spec/speckit.go.doc.md
package spec

import (
	"fmt"
	"strings"
)

// SlashCommand represents a spec-related slash command in the chat interface.
type SlashCommand struct {
	Name        string            // Command name (e.g., "spec", "goal", "verify")
	Aliases     []string          // Short aliases (e.g., "s", "g", "v")
	Description string            // Human-readable description
	Args        string            // Argument template (e.g., "<spec-id>")
	Handler     SlashCommandHandler // Execution handler
	Hidden      bool              // Hide from help
}

// SlashCommandHandler is the function signature for command execution.
type SlashCommandHandler func(ctx *CommandContext) (string, error)

// CommandContext holds context for command execution within the chat.
type CommandContext struct {
	Command    string           // Full command line
	Args       []string         // Parsed arguments
	Collection *SpecCollection  // Current spec collection
	MetaSpec   *MetaSpec        // Indexed specs (if available)
	Session    map[string]interface{} // Session state
	User       string           // Current user ID
}

// SpecKit holds the spec-related command registry for chat integration.
type SpecKit struct {
	Commands  map[string]*SlashCommand
	Aliases   map[string]string // Alias -> command name
	Compiler  *Compiler
	Indexer   *SpecIndexer
	Registry  *GateRegistry
	Budgeter  *TokenBudgeter
}

// NewSpecKit creates a new SpecKit for a collection.
func NewSpecKit(collection *SpecCollection) *SpecKit {
	kit := &SpecKit{
		Commands:  make(map[string]*SlashCommand),
		Aliases:   make(map[string]string),
		Compiler:  NewCompiler(collection),
		Indexer:   NewSpecIndexer(collection, 100000),
		Registry:  NewGateRegistry(),
		Budgeter:  NewTokenBudgeter(100000, len(collection.Specs), 20),
	}

	// Register default commands
	kit.registerDefaultCommands()

	return kit
}

// registerDefaultCommands registers all built-in spec commands.
func (sk *SpecKit) registerDefaultCommands() {
	sk.Register(&SlashCommand{
		Name:        "spec",
		Aliases:     []string{"s"},
		Description: "List, show, or search specs",
		Args:        "[list|show|search] [spec-id|query]",
		Handler:     sk.handleSpec,
	})

	sk.Register(&SlashCommand{
		Name:        "goal",
		Aliases:     []string{"g"},
		Description: "Interact with goal specs",
		Args:        "[list|show|create] [goal-id]",
		Handler:     sk.handleGoal,
	})

	sk.Register(&SlashCommand{
		Name:        "verify",
		Aliases:     []string{"v"},
		Description: "Run quality gates on a spec",
		Args:        "<spec-id> [--gates gate1,gate2]",
		Handler:     sk.handleVerify,
	})

	sk.Register(&SlashCommand{
		Name:        "compile",
		Aliases:     []string{"c"},
		Description: "Compile specs and build dependency graph",
		Args:        "[--check-cycles] [--stats]",
		Handler:     sk.handleCompile,
	})

	sk.Register(&SlashCommand{
		Name:        "budget",
		Aliases:     []string{"b"},
		Description: "Show token budget allocation",
		Args:        "[--suggest-selection]",
		Handler:     sk.handleBudget,
	})

	sk.Register(&SlashCommand{
		Name:        "search",
		Aliases:     []string{"find", "search"},
		Description: "Full-text search specs",
		Args:        "<query>",
		Handler:     sk.handleSearch,
	})

	sk.Register(&SlashCommand{
		Name:        "deps",
		Aliases:     []string{"d"},
		Description: "Show spec dependencies",
		Args:        "<spec-id>",
		Handler:     sk.handleDeps,
	})

	sk.Register(&SlashCommand{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Show spec command help",
		Args:        "[command]",
		Handler:     sk.handleHelp,
	})
}

// Register adds a command to the registry.
func (sk *SpecKit) Register(cmd *SlashCommand) {
	sk.Commands[cmd.Name] = cmd

	// Register aliases
	for _, alias := range cmd.Aliases {
		sk.Aliases[alias] = cmd.Name
	}
}

// Execute processes a slash command from the chat.
func (sk *SpecKit) Execute(ctx *CommandContext) (string, error) {
	if len(ctx.Args) == 0 {
		return "", fmt.Errorf("no command provided")
	}

	// Get command name (first arg)
	cmdName := ctx.Args[0]

	// Resolve alias if necessary
	if realName, ok := sk.Aliases[cmdName]; ok {
		cmdName = realName
	}

	// Find command
	cmd, ok := sk.Commands[cmdName]
	if !ok {
		return "", fmt.Errorf("unknown command: %s", cmdName)
	}

	// Remove command name from args for handler
	ctx.Args = ctx.Args[1:]

	// Execute handler
	return cmd.Handler(ctx)
}

// ─────────────────────────────────────────────────────────────────────
// Command Handlers
// ─────────────────────────────────────────────────────────────────────

func (sk *SpecKit) handleSpec(ctx *CommandContext) (string, error) {
	if len(ctx.Args) == 0 {
		return sk.handleSpecList(ctx)
	}

	subcommand := ctx.Args[0]
	switch subcommand {
	case "list":
		return sk.handleSpecList(ctx)
	case "show":
		if len(ctx.Args) < 2 {
			return "", fmt.Errorf("spec show requires a spec-id")
		}
		return sk.handleSpecShow(ctx, ctx.Args[1])
	case "search":
		if len(ctx.Args) < 2 {
			return "", fmt.Errorf("spec search requires a query")
		}
		return sk.handleSpecSearch(ctx, ctx.Args[1])
	default:
		return "", fmt.Errorf("unknown spec subcommand: %s", subcommand)
	}
}

func (sk *SpecKit) handleSpecList(ctx *CommandContext) (string, error) {
	specs := ctx.Collection.Specs
	if len(specs) == 0 {
		return "No specs in collection", nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("**%d Specs** in collection:\n\n", len(specs)))

	for id, spec := range specs {
		result.WriteString(fmt.Sprintf("• `%s` — %s (%s, %s)\n", id, spec.Title, spec.Kind, spec.Status))
	}

	return result.String(), nil
}

func (sk *SpecKit) handleSpecShow(ctx *CommandContext, specID string) (string, error) {
	spec, ok := ctx.Collection.Specs[specID]
	if !ok {
		return "", fmt.Errorf("spec not found: %s", specID)
	}

	return spec.MarkdownFormat(), nil
}

func (sk *SpecKit) handleSpecSearch(ctx *CommandContext, query string) (string, error) {
	if ctx.MetaSpec == nil {
		return "", fmt.Errorf("search index not available (run /compile first)")
	}

	results := ctx.MetaSpec.SearchByKeyword(query)
	if len(results) == 0 {
		return fmt.Sprintf("No results for query: %s", query), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("**%d Results** for `%s`:\n\n", len(results), query))

	for i, index := range results {
		if i >= 10 { // Limit to top 10
			result.WriteString(fmt.Sprintf("... and %d more\n", len(results)-i))
			break
		}
		result.WriteString(fmt.Sprintf("%d. `%s` — %s (relevance: %.0f%%)\n",
			i+1, index.SpecID, index.Title, index.Score*100))
	}

	return result.String(), nil
}

func (sk *SpecKit) handleGoal(ctx *CommandContext) (string, error) {
	goals := make([]*Spec, 0)
	for _, spec := range ctx.Collection.Specs {
		if spec.Kind == SpecKindGoal && spec.Status == SpecStatusActive {
			goals = append(goals, spec)
		}
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("**%d Active Goals**:\n\n", len(goals)))

	for _, goal := range goals {
		result.WriteString(fmt.Sprintf("• `%s` — %s\n", goal.ID, goal.Title))
		if goal.Goals != "" {
			// Show first line
			lines := strings.Split(goal.Goals, "\n")
			result.WriteString(fmt.Sprintf("  %s\n", lines[0]))
		}
	}

	return result.String(), nil
}

func (sk *SpecKit) handleVerify(ctx *CommandContext) (string, error) {
	if len(ctx.Args) == 0 {
		return "", fmt.Errorf("verify requires a spec-id")
	}

	specID := ctx.Args[0]
	spec, ok := ctx.Collection.Specs[specID]
	if !ok {
		return "", fmt.Errorf("spec not found: %s", specID)
	}

	// Run all gates
	verifyCtx := &VerificationContext{
		Collection:  ctx.Collection,
		TokenBudget: 100000,
		AllowWarnings: true,
	}

	results := sk.Registry.Run(spec, verifyCtx)

	var result strings.Builder
	result.WriteString(fmt.Sprintf("**Verification Results** for `%s`:\n\n", specID))
	result.WriteString(results.Details())

	// Store in spec
	spec.GateResults = make(map[string]GateResult)
	for name, gateResult := range results.Results {
		spec.GateResults[name] = *gateResult
	}

	return result.String(), nil
}

func (sk *SpecKit) handleCompile(ctx *CommandContext) (string, error) {
	result := sk.Compiler.Compile()

	var output strings.Builder
	output.WriteString(fmt.Sprintf("**Compilation Result**:\n\n%s\n", result.String()))

	if result.Stats != nil {
		output.WriteString(fmt.Sprintf("\n**Statistics**:\n"))
		output.WriteString(fmt.Sprintf("• Specs compiled: %d\n", result.Stats.SpecsCompiled))
		output.WriteString(fmt.Sprintf("• Dependencies: %d\n", result.Stats.TotalDependencies))
		output.WriteString(fmt.Sprintf("• Max depth: %d\n", result.Stats.MaxDepth))
		output.WriteString(fmt.Sprintf("• Time: %dms\n", result.Stats.CompilationTimeMs))
	}

	if len(result.Errors) > 0 {
		output.WriteString(fmt.Sprintf("\n**Errors**:\n"))
		for _, err := range result.Errors {
			output.WriteString(fmt.Sprintf("• [%s] %s\n", err.Phase, err.Message))
		}
	}

	// Build index
	_ = sk.Indexer.BuildIndex()
	ctx.MetaSpec = sk.Indexer.MetaSpec

	return output.String(), nil
}

func (sk *SpecKit) handleBudget(ctx *CommandContext) (string, error) {
	var output strings.Builder
	output.WriteString(fmt.Sprintf("**Token Budget**:\n\n%s\n", sk.Budgeter.Summary()))

	// Suggest selection if requested
	if len(ctx.Args) > 0 && ctx.Args[0] == "--suggest-selection" {
		if ctx.MetaSpec == nil {
			return output.String() + "\n(Build index with `/compile` to get suggestions)\n", nil
		}

		selected := ctx.MetaSpec.SelectByBudget(sk.Budgeter.TotalBudget-sk.Budgeter.ReserveBudget, 20)
		output.WriteString(fmt.Sprintf("\n**Suggested Specs** (top %d by priority):\n", len(selected)))

		totalTokens := 0
		for i, index := range selected {
			output.WriteString(fmt.Sprintf("%d. `%s` — %d tokens (priority: %d)\n",
				i+1, index.SpecID, index.TokenEstimate, index.Priority))
			totalTokens += index.TokenEstimate
		}

		output.WriteString(fmt.Sprintf("\nTotal: %d tokens\n", totalTokens))
	}

	return output.String(), nil
}

func (sk *SpecKit) handleSearch(ctx *CommandContext) (string, error) {
	if len(ctx.Args) == 0 {
		return "", fmt.Errorf("search requires a query")
	}

	query := ctx.Args[0]

	if ctx.MetaSpec == nil {
		return "", fmt.Errorf("search index not available (run `/compile` first)")
	}

	results := ctx.MetaSpec.SearchByKeyword(query)
	if len(results) == 0 {
		return fmt.Sprintf("No results for: %s", query), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("**Search Results** for `%s`:\n\n", query))

	for i, index := range results {
		output.WriteString(fmt.Sprintf("%d. **%s** (`%s`)\n", i+1, index.Title, index.SpecID))
		if index.Summary != "" {
			output.WriteString(fmt.Sprintf("   %s\n", index.Summary))
		}
		output.WriteString("\n")
	}

	return output.String(), nil
}

func (sk *SpecKit) handleDeps(ctx *CommandContext) (string, error) {
	if len(ctx.Args) == 0 {
		return "", fmt.Errorf("deps requires a spec-id")
	}

	specID := ctx.Args[0]
	spec, ok := ctx.Collection.Specs[specID]
	if !ok {
		return "", fmt.Errorf("spec not found: %s", specID)
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("**Dependencies** for `%s`:\n\n", specID))

	if len(spec.Dependencies) == 0 {
		output.WriteString("No dependencies\n")
	} else {
		for i, depID := range spec.Dependencies {
			if depSpec, ok := ctx.Collection.Specs[depID]; ok {
				output.WriteString(fmt.Sprintf("%d. `%s` — %s\n", i+1, depID, depSpec.Title))
			} else {
				output.WriteString(fmt.Sprintf("%d. `%s` — (missing)\n", i+1, depID))
			}
		}
	}

	if len(spec.Dependents) > 0 {
		output.WriteString(fmt.Sprintf("\n**Dependents** (specs that depend on this):\n\n"))
		for i, depID := range spec.Dependents {
			if depSpec, ok := ctx.Collection.Specs[depID]; ok {
				output.WriteString(fmt.Sprintf("%d. `%s` — %s\n", i+1, depID, depSpec.Title))
			}
		}
	}

	return output.String(), nil
}

func (sk *SpecKit) handleHelp(ctx *CommandContext) (string, error) {
	var output strings.Builder
	output.WriteString("**Spec Commands**:\n\n")

	for _, cmd := range sk.Commands {
		if cmd.Hidden {
			continue
		}

		aliases := ""
		if len(cmd.Aliases) > 0 {
			aliases = fmt.Sprintf(" (%s)", strings.Join(cmd.Aliases, ", "))
		}

		output.WriteString(fmt.Sprintf("• `/%s`%s — %s\n", cmd.Name, aliases, cmd.Description))
		if cmd.Args != "" {
			output.WriteString(fmt.Sprintf("  Usage: `/%s %s`\n", cmd.Name, cmd.Args))
		}
	}

	return output.String(), nil
}
