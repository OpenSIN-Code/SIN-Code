// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` helper functions — result printing, terminal
// ask, verify runner, hook loading, and small string/terminal utilities.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sandbox"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func printResult(res *agentloop.Result, jsonOut bool) error {
	if jsonOut {
		// Feature 3: compact JSON — the stable headless API contract
		// (AGENTS.md §7). No indentation, single line, so piping works.
		data, err := json.Marshal(res)
		if err != nil {
			return err
		}
		fmt.Fprintln(chatStdout, string(data))
		return nil
	}
	fmt.Fprintln(chatStdout, res.Summary)
	fmt.Fprintf(chatStdout, "[session=%s verified=%v turns=%d]\n", res.SessionID, res.Verified, res.Turns)
	return nil
}

func terminalAsk(tc agentloop.ToolCall) bool {
	fmt.Fprintf(chatStdout, "Permission required: tool %q with args %v — allow? [y/N] ", tc.Name, tc.Args)
	reader := bufio.NewReader(chatStdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

func commandRunner(command string) verify.Runner {
	if command == "" {
		return nil
	}
	return func(ctx context.Context, workspace string) (bool, string, error) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		policy := sandbox.DefaultPolicy(workspace, os.TempDir())
		policy.Timeout = 0 // preserve the 10-minute cctx deadline; DefaultPolicy's 2 min would clamp it
		cmd, sandboxResult, err := sandbox.Command(cctx, policy, "sh", "-c", command)
		if err != nil {
			return false, "", err
		}
		if !sandboxResult.Enforced && sandboxResult.Warning != "" {
			fmt.Fprintf(chatStderr, "warn: verify-cmd sandbox: %s\n", sandboxResult.Warning)
		}
		cmd.Dir = workspace
		out, err := cmd.CombinedOutput()
		report := strings.TrimSpace(string(out))
		if err != nil {
			return false, report, nil
		}
		return true, report, nil
	}
}

func loadHooks(workspace string) []hooks.Hook {
	var all []hooks.Hook
	paths := []string{}
	if cfg, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.json"))
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.yaml"))
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.yml"))
	}
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.json"))
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.yaml"))
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.yml"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var hs []hooks.Hook
		if strings.HasSuffix(p, ".json") {
			err = json.Unmarshal(data, &hs)
		} else {
			// YAML may be a top-level list or wrapped under `hooks:`.
			var list []hooks.Hook
			if yerr := yaml.Unmarshal(data, &list); yerr == nil {
				hs = list
			} else {
				var wrapped struct {
					Hooks []hooks.Hook `yaml:"hooks"`
				}
				err = yaml.Unmarshal(data, &wrapped)
				hs = wrapped.Hooks
			}
		}
		if err != nil {
			fmt.Fprintf(chatStderr, "warn: skipping invalid hooks file %s: %v\n", p, err)
			continue
		}
		all = append(all, hs...)
	}
	return all
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
