package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// ChainStep represents one skill invocation within a chain.
type ChainStep struct {
	SkillName string            `json:"skill"`
	OnFailure string            `json:"on_failure"` // "abort", "retry", "skip", "fallback"
	MaxRetries int              `json:"max_retries"`
	Variables  map[string]string `json:"variables"`
}

// Chain defines a sequential workflow of skills.
type Chain struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Steps       []ChainStep  `json:"steps"`
	OnSuccess   string       `json:"on_success"` // "stop", "next", "restart"
	OnFailure   string       `json:"on_failure"`
	MaxLoops    int          `json:"max_loops"` // prevent infinite loops
}

// ChainExecutor executes chains with loop detection and retry logic.
type ChainExecutor struct {
	runner   *Runner
	registry *Registry
	state    map[string]interface{} // shared state between steps
}

func NewChainExecutor(runner *Runner, registry *Registry) *ChainExecutor {
	return &ChainExecutor{
		runner:   runner,
		registry: registry,
		state:    make(map[string]interface{}),
	}
}

// ExecuteChain runs a chain, handling retries and fallbacks.
func (c *ChainExecutor) ExecuteChain(ctx context.Context, chain *Chain, opts RunOptions) error {
	loopCount := 0
	stepIndex := 0

	for loopCount < chain.MaxLoops && stepIndex < len(chain.Steps) {
		step := chain.Steps[stepIndex]
		log.Printf("[Chain %s] Executing step %d: %s", chain.Name, stepIndex+1, step.SkillName)

		var result *RunResult
		var err error
		retries := 0
		for retries <= step.MaxRetries {
			result, err = c.runner.Run(ctx, step.SkillName, opts)
			if err == nil && result.Success {
				break
			}
			retries++
			log.Printf("Retry %d/%d for skill %s", retries, step.MaxRetries, step.SkillName)
			time.Sleep(1 * time.Second)
		}

		if err != nil || !result.Success {
			// Failure handling
			switch step.OnFailure {
			case "abort":
				return fmt.Errorf("chain aborted at step %s: %v", step.SkillName, err)
			case "skip":
				log.Printf("Skipping failed skill %s", step.SkillName)
				stepIndex++
				continue
			case "fallback":
				// You could define a fallback skill name in step.Variables["fallback_skill"]
				if fb, ok := step.Variables["fallback_skill"]; ok {
					log.Printf("Running fallback skill: %s", fb)
					result, err = c.runner.Run(ctx, fb, opts)
					if err != nil || !result.Success {
						return fmt.Errorf("fallback %s also failed: %v", fb, err)
					}
				} else {
					return fmt.Errorf("no fallback defined for %s", step.SkillName)
				}
			default: // "retry" already handled inside loop, but if max retries exceeded:
				return fmt.Errorf("max retries exceeded for skill %s", step.SkillName)
			}
		}

		// Store outputs in shared state (for variable substitution in next steps)
		c.state[step.SkillName] = result.Outputs
		stepIndex++
	}

	if chain.OnSuccess == "restart" {
		log.Printf("Restarting chain %s", chain.Name)
		return c.ExecuteChain(ctx, chain, opts)
	}
	return nil
}

// LoadChainFromFile reads a chain definition from JSON or YAML.
func LoadChainFromFile(path string) (*Chain, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var chain Chain
	if err := json.Unmarshal(data, &chain); err != nil {
		return nil, err
	}
	if chain.MaxLoops == 0 {
		chain.MaxLoops = 3 // default safety
	}
	return &chain, nil
}

// SaveChain writes a chain definition.
func SaveChain(chain *Chain, path string) error {
	data, err := json.MarshalIndent(chain, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
