// SPDX-License-Identifier: MIT
// Purpose: the chain engine (issue #108) links tools into closed execution
// loops. When a step fails or a validator rejects the result, the error is
// fed back as input to the next self-repair loop until the result is valid
// or the loop budget is exhausted. Corrected Go implementation of the design
// sketched in issue #108 (fixed module path, slice types, error checks).
package chains

import (
	"errors"
	"fmt"
	"io"

	"github.com/OpenSIN-Code/SIN-Code/pkg/tools"
)

// Step is one tool invocation in a chain. InputMapper builds the next tool's
// args from the previous output and the current loop input.
type Step struct {
	ToolName    string
	InputMapper func(prevOutput interface{}, input map[string]interface{}) map[string]interface{}
}

// Validator reports whether a chain's final output satisfies the goal.
type Validator func(finalOutput interface{}) bool

// Engine runs chains against a tool registry.
type Engine struct {
	registry *tools.Registry
	log      io.Writer // optional; nil = silent
}

// NewEngine builds an Engine bound to the global registry.
func NewEngine() *Engine {
	return &Engine{registry: tools.GetRegistry()}
}

// NewEngineWith builds an Engine bound to a specific registry and log sink
// (used by tests).
func NewEngineWith(r *tools.Registry, log io.Writer) *Engine {
	return &Engine{registry: r, log: log}
}

func (e *Engine) logf(format string, a ...interface{}) {
	if e.log != nil {
		fmt.Fprintf(e.log, format, a...)
	}
}

// ErrChainExhausted is returned when maxLoops passes without the validator
// accepting the output.
var ErrChainExhausted = errors.New("chains: loop budget exhausted without passing validation")

// ExecuteLoopChain runs steps sequentially, retrying the whole chain up to
// maxLoops times. On any step error or validator rejection it feeds the
// failure context into the next loop's input for self-repair.
func (e *Engine) ExecuteLoopChain(
	chainName string,
	steps []Step,
	initialInput map[string]interface{},
	validator Validator,
	maxLoops int,
) (interface{}, error) {
	if len(steps) == 0 {
		return nil, errors.New("chains: chain has no steps")
	}
	if validator == nil {
		validator = func(interface{}) bool { return true }
	}
	if maxLoops < 1 {
		maxLoops = 1
	}

	currentInput := initialInput
	var lastOutput interface{}

	e.logf("[CHAIN] starting autopilot chain: %s\n", chainName)

	for loop := 1; loop <= maxLoops; loop++ {
		e.logf("[CHAIN] loop %d/%d\n", loop, maxLoops)

		var stepErr error
		for _, step := range steps {
			tool, ok := e.registry.GetTool(step.ToolName)
			if !ok {
				return nil, fmt.Errorf("chains: tool %q in chain %q does not exist", step.ToolName, chainName)
			}
			mapped := currentInput
			if step.InputMapper != nil {
				mapped = step.InputMapper(lastOutput, currentInput)
			}
			lastOutput, stepErr = tool.Execute(mapped)
			if stepErr != nil {
				e.logf("[CHAIN] tool error at %s: %v — attempting self-repair\n", step.ToolName, stepErr)
				break
			}
		}

		if stepErr == nil && validator(lastOutput) {
			e.logf("[CHAIN] chain %s succeeded after %d loop(s)\n", chainName, loop)
			return lastOutput, nil
		}

		// Feed the failure forward as the next loop's input.
		currentInput = map[string]interface{}{
			"error_context":  stepErr,
			"failed_output":  lastOutput,
			"original_input": initialInput,
		}
	}

	return nil, fmt.Errorf("%w: chain %q after %d loops", ErrChainExhausted, chainName, maxLoops)
}
