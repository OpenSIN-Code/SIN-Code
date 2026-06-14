package skills

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Debugger struct {
	runner      *Runner
	skill       *Skill
	breakpoints map[int]bool
	stepMode    bool
}

func NewDebugger(runner *Runner, skill *Skill) *Debugger {
	return &Debugger{
		runner:      runner,
		skill:       skill,
		breakpoints: make(map[int]bool),
		stepMode:    true,
	}
}

func (d *Debugger) Run(ctx context.Context, opts RunOptions) error {
	fmt.Printf("\n🔍 Debugging skill '%s' (%d steps)\n", d.skill.Name, len(d.skill.Steps))
	fmt.Print("Commands: s (step), c (continue), b <N> (breakpoint), p (print state), q (quit)\n\n")

	state := make(map[string]interface{})
	opts.AutoConfirm = true // In debug mode, we control manually.

	for i, step := range d.skill.Steps {
		// Check breakpoint
		if d.breakpoints[step.Number] {
			fmt.Printf("🔴 Breakpoint hit at step %d: %s\n", step.Number, step.Instruction)
			if !d.waitForContinue() {
				return nil
			}
		}
		if d.stepMode {
			fmt.Printf("\n[Step %d] %s\n", step.Number, step.Instruction)
			fmt.Print("> ")
			scanner := bufio.NewScanner(os.Stdin)
			if !scanner.Scan() {
				return nil
			}
			cmd := scanner.Text()
			switch cmd {
			case "s", "step":
				// execute and continue
			case "c", "continue":
				d.stepMode = false
			case "q", "quit":
				return nil
			default:
				if strings.HasPrefix(cmd, "b ") {
					num, _ := strconv.Atoi(strings.TrimSpace(cmd[2:]))
					d.breakpoints[num] = !d.breakpoints[num]
					fmt.Printf("Breakpoint %d set=%v\n", num, d.breakpoints[num])
					i-- // re-evaluate this step
					continue
				} else if cmd == "p" {
					fmt.Printf("State: %v\n", state)
					i-- // stay on same step
					continue
				}
			}
		}
		// Execute step
		result, err := d.runner.runStep(ctx, step, opts)
		if err != nil {
			fmt.Printf("❌ Step %d failed: %v\n", step.Number, err)
			return err
		}
		state[fmt.Sprintf("step_%d", step.Number)] = result
		fmt.Printf("✅ Step %d completed. Output: %s\n", step.Number, truncate(result, 100))
	}
	fmt.Println("🎉 Debugging finished.")
	return nil
}

func (d *Debugger) waitForContinue() bool {
	fmt.Print("(c to continue, q to quit) > ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	cmd := scanner.Text()
	return cmd == "c" || cmd == "continue"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
