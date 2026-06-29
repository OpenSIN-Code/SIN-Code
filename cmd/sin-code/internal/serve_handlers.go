// SPDX-License-Identifier: MIT
// Purpose: serve — core analysis MCP tool handlers (discover, execute, map,
// grasp, scout, harvest, orchestrate, ibd, poc, sckg, adw, oracle, efm).
// sin-debt: shrink, upgrade: when a second handler-related function is needed, merge into a shared file
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// pathAbs is a test seam for filepath.Abs, overridable in tests.
var pathAbs = filepath.Abs

func handleDiscover(ctx context.Context, args map[string]any) (string, error) {
	// discover takes path as positional argument, not --path
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"discover", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
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

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func handleExecute(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "execute", args)
}

func handleMap(ctx context.Context, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"map", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
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

func handleGrasp(ctx context.Context, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"grasp", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
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

func handleScout(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
	}
	searchType := "regex"
	if st, ok := args["search_type"].(string); ok && st != "" {
		searchType = st
	}
	maxResults := intArg(args, "max_results", 50)
	root, err := pathAbs(path)
	if err != nil {
		return "", err
	}
	results, err := scoutSearchAuto(root, query, searchType, maxResults, false)
	if err != nil {
		return "", err
	}
	b, _ := json.MarshalIndent(results, "", "  ")
	return string(b), nil
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func handleHarvest(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "harvest", args)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func handleOrchestrate(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "orchestrate", args)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func handleIbd(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "ibd", args)
}

func handlePoc(ctx context.Context, args map[string]any) (string, error) {
	code := "."
	if c, ok := args["code"].(string); ok && c != "" {
		code = c
		delete(args, "code")
	} else if s, ok := args["spec"].(string); ok && s != "" {
		code = s
		delete(args, "spec")
	}
	cmdArgs := []string{"poc", code}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
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

func handleSckg(ctx context.Context, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"sckg", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
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

func handleAdw(ctx context.Context, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		delete(args, "path")
	}
	cmdArgs := []string{"adw", path}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if val != "" {
				cmdArgs = append(cmdArgs, "--"+k, val)
			}
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

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func handleOracle(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "oracle", args)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func handleEfm(ctx context.Context, args map[string]any) (string, error) {
	return runSubcommand(ctx, "efm", args)
}
