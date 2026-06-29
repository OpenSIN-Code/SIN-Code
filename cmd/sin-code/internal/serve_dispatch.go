// SPDX-License-Identifier: MIT
// Purpose: serve — subcommand dispatch helpers shared by MCP tool handlers.
// sin-debt: shrink, upgrade: when a second dispatch-related function is needed, merge into a shared file
package internal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func runSubcommand(ctx context.Context, name string, args map[string]any) (string, error) {
	cmdArgs := []string{name}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val == "" {
				continue
			}
			cmdArgs = append(cmdArgs, "--"+k, val)
		case bool:
			if val {
				cmdArgs = append(cmdArgs, "--"+k)
			}
		case float64:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%v", int(val)))
		case int:
			cmdArgs = append(cmdArgs, "--"+k, fmt.Sprintf("%d", val))
		}
	}
	return runSubcommandRaw(ctx, cmdArgs)
}

// osExecutable is a test hook for the fallback path in resolveBinary.
var osExecutable = os.Executable

// resolveBinary picks the sin-code binary to use for subcommand dispatch.
// Order: SIN_CODE_BIN env, sin-code on PATH, os.Executable().
func resolveBinary() (string, error) {
	if bin := os.Getenv("SIN_CODE_BIN"); bin != "" {
		return bin, nil
	}
	if bin, err := exec.LookPath("sin-code"); err == nil {
		return bin, nil
	}
	return osExecutable()
}

func runSubcommandRaw(ctx context.Context, cmdArgs []string) (string, error) {
	selfPath, err := resolveBinary()
	if err != nil {
		return "", fmt.Errorf("cannot find self: %w", err)
	}

	c := exec.CommandContext(ctx, selfPath, cmdArgs...)
	c.Env = os.Environ()
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s\nERROR: %v", string(out), err), nil
	}
	return string(out), nil
}
