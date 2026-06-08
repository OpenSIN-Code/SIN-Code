// SPDX-License-Identifier: MIT
// Purpose: agent-edit, agent-set, agent-reset CLI commands.
package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
)

var (
	agEditAgent string
	agEditSet   []string
)

var OrchestratorAgentEditCmd = &cobra.Command{
	Use:   "agent-edit",
	Short: "Edit a per-agent TOML config (interactive $EDITOR or programmatic with --set)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		agEditAgent = strings.ToLower(strings.TrimSpace(agEditAgent))
		if agEditAgent == "" {
			return fmt.Errorf("--agent <name> required")
		}
		if len(agEditSet) > 0 {
			return applyAgentEdits(agEditAgent, agEditSet)
		}
		return openAgentInEditor(agEditAgent)
	},
}

var OrchestratorAgentSetCmd = &cobra.Command{
	Use:   "agent-set <name> key=value [key=value ...]",
	Short: "Programmatically set fields on a user agent (no editor required)",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.ToLower(args[0])
		return applyAgentEdits(name, args[1:])
	},
}

var OrchestratorAgentResetCmd = &cobra.Command{
	Use:   "agent-reset <name>",
	Short: "Remove a user agent (falls back to defaults)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.ToLower(args[0])
		dir, err := agentDir(name)
		if err != nil {
			return err
		}
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Printf("Agent %s has no user config — nothing to reset.\n", name)
			return nil
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		fmt.Printf("Reset agent %s to defaults (removed %s)\n", name, dir)
		return nil
	},
}

func init() {
	OrchestratorAgentEditCmd.Flags().StringVar(&agEditAgent, "agent", "", "Agent name to edit")
	OrchestratorAgentEditCmd.Flags().StringSliceVar(&agEditSet, "set", nil, "Set field as key=value (repeatable)")

	OrchestratorRunCmd.AddCommand(OrchestratorAgentEditCmd)
	OrchestratorRunCmd.AddCommand(OrchestratorAgentSetCmd)
	OrchestratorRunCmd.AddCommand(OrchestratorAgentResetCmd)
}

func openAgentInEditor(name string) error {
	dir, err := agentDir(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "agent.toml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		seed := buildAgentSeed(name)
		if err := os.WriteFile(cfgPath, []byte(seed), 0o644); err != nil {
			return err
		}
		fmt.Printf("Seeded %s with default-template — edit and save.\n", cfgPath)
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	cmd := exec.Command(editor, cfgPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildAgentSeed(name string) string {
	defaults := map[string]orchestrator.AgentConfig{}
	for _, c := range orchestrator.DefaultAgents() {
		defaults[c.Name] = c
	}
	if base, ok := defaults[name]; ok {
		data, _ := toml.Marshal(base)
		return string(data)
	}
	return fmt.Sprintf(`name = %q
description = "Custom agent"
type = "code"
provider = ""
model = ""
max_tokens = 4096
temperature = 0.0
`, name)
}

func applyAgentEdits(name string, kvPairs []string) error {
	dir, err := agentDir(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "agent.toml")
	cfg := orchestrator.AgentConfig{Name: name}
	if _, err := os.Stat(cfgPath); err == nil {
		if _, derr := toml.DecodeFile(cfgPath, &cfg); derr != nil {
			return fmt.Errorf("decode %s: %w", cfgPath, derr)
		}
	}
	if cfg.Name == "" {
		cfg.Name = name
	}
	for _, kv := range kvPairs {
		key, val, ok := splitKV(kv)
		if !ok {
			return fmt.Errorf("invalid key=value: %q (expected key=value)", kv)
		}
		if err := setAgentField(&cfg, key, val); err != nil {
			return err
		}
	}
	f, err := os.Create(cfgPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(&cfg); err != nil {
		return err
	}
	fmt.Printf("Updated %s\n", cfgPath)
	return nil
}

func splitKV(s string) (string, string, bool) {
	idx := strings.Index(s, "=")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:]), true
}

func setAgentField(cfg *orchestrator.AgentConfig, key, val string) error {
	switch key {
	case "name":
		cfg.Name = val
	case "description":
		cfg.Description = val
	case "type":
		cfg.Type = orchestrator.TaskType(val)
	case "provider":
		cfg.Provider = val
	case "base_url":
		cfg.BaseURL = val
	case "model":
		cfg.Model = val
	case "max_tokens":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("max_tokens: %w", err)
		}
		cfg.MaxTokens = n
	case "temperature":
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("temperature: %w", err)
		}
		cfg.Temperature = f
	case "system_file":
		cfg.SystemFile = val
	case "max_context":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("max_context: %w", err)
		}
		cfg.MaxContext = n
	case "memory_namespace":
		cfg.MemoryNS = val
	case "retention_days":
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("retention_days: %w", err)
		}
		cfg.RetentionDays = n
	case "tools_allow":
		cfg.ToolsAllow = splitCSV(val)
	case "tools_deny":
		cfg.ToolsDeny = splitCSV(val)
	default:
		return fmt.Errorf("unknown field: %q", key)
	}
	return nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
