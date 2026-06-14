package skills

import (
	"context"
	"fmt"
	"log"
	"strings"
)

type Runner struct {
	registry   *Registry
	mcpClient  interface{} // MCP-Client zum Aufruf von Tools
	agentSys   interface{} // Multi-Agent-System (Governor, Critic, Adversary)
}

type RunOptions struct {
	Verbose      bool
	AutoConfirm  bool
	MaxSteps     int
	BudgetTokens int
}

type RunResult struct {
	SkillName    string
	StepsExecuted int
	Success      bool
	Outputs      map[string]string
	Error        error
}

func NewRunner(reg *Registry, mcpClient interface{}, agentSys interface{}) *Runner {
	return &Runner{
		registry:  reg,
		mcpClient: mcpClient,
		agentSys:  agentSys,
	}
}

// Run executes a skill by name with given context and options.
func (r *Runner) Run(ctx context.Context, skillName string, opts RunOptions) (*RunResult, error) {
	skill, err := r.registry.Get(skillName)
	if err != nil {
		return nil, err
	}

	result := &RunResult{
		SkillName: skill.Name,
		Outputs:   make(map[string]string),
	}

	for i, step := range skill.Steps {
		if opts.MaxSteps > 0 && i >= opts.MaxSteps {
			break
		}
		if opts.Verbose {
			log.Printf("Executing step %d: %s\n", step.Number, step.Instruction)
		}

		// Map instruction to tools
		tools := r.mapInstructionToTools(step.Instruction)
		stepOutput := ""
		for _, toolName := range tools {
			// In a real implementation, invoke MCP tool
			// For now, just log
			stepOutput += fmt.Sprintf("Tool: %s, ", toolName)
		}
		result.Outputs[fmt.Sprintf("step_%d", step.Number)] = stepOutput
		result.StepsExecuted++
	}

	result.Success = true
	return result, nil
}

// mapInstructionToTools is a heuristic mapper; extend with ML or config.
func (r *Runner) mapInstructionToTools(instruction string) []string {
	lower := strings.ToLower(instruction)
	switch {
	case strings.Contains(lower, "write") || strings.Contains(lower, "implement"):
		return []string{"sin-code code", "sin-code write_file"}
	case strings.Contains(lower, "test") || strings.Contains(lower, "verify"):
		return []string{"sin-code test", "sin-code verify"}
	case strings.Contains(lower, "plan") || strings.Contains(lower, "design"):
		return []string{"sin-code analyze", "sin-code plan"}
	case strings.Contains(lower, "review") || strings.Contains(lower, "audit"):
		return []string{"sin-code review", "sin-code security"}
	default:
		return []string{"sin-code exec"}
	}
}

// runStep executes a single step
func (r *Runner) runStep(ctx context.Context, step SkillStep, opts RunOptions) (string, error) {
	tools := r.mapInstructionToTools(step.Instruction)
	var output string
	for _, toolName := range tools {
		output += fmt.Sprintf("Tool %s executed. ", toolName)
	}
	return output, nil
}
