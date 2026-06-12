// SPDX-License-Identifier: MIT
// Purpose: agent-show and agent-doctor CLI commands.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

var (
	agDoctorOffline bool
)

var OrchestratorAgentShowCmd = &cobra.Command{
	Use:   "agent-show <name>",
	Short: "Show effective config for an agent (merged defaults + user overrides)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.ToLower(args[0])
		merged, source, err := loadEffectiveAgent(name)
		if err != nil {
			return err
		}
		if orch2Format == "json" {
			out := map[string]interface{}{
				"agent":  merged,
				"source": source,
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		}
		fmt.Printf("Agent %s (source: %s):\n", name, source)
		fmt.Printf("  description:  %s\n", merged.Description)
		fmt.Printf("  type:         %s\n", merged.Type)
		fmt.Printf("  provider:     %s\n", orDash(merged.Provider))
		fmt.Printf("  base_url:     %s\n", orDash(merged.BaseURL))
		fmt.Printf("  model:        %s\n", orDash(merged.Model))
		fmt.Printf("  max_tokens:   %d\n", merged.MaxTokens)
		fmt.Printf("  temperature:  %g\n", merged.Temperature)
		fmt.Printf("  system_file:  %s\n", orDash(merged.SystemFile))
		fmt.Printf("  max_context:  %d\n", merged.MaxContext)
		fmt.Printf("  memory_ns:    %s\n", orDash(merged.MemoryNS))
		fmt.Printf("  retention:    %d days\n", merged.RetentionDays)
		if len(merged.ToolsAllow) > 0 {
			fmt.Printf("  tools_allow:  %s\n", strings.Join(merged.ToolsAllow, ", "))
		}
		if len(merged.ToolsDeny) > 0 {
			fmt.Printf("  tools_deny:   %s\n", strings.Join(merged.ToolsDeny, ", "))
		}
		return nil
	},
}

type DoctorReport struct {
	Agent  string                 `json:"agent"`
	OK     bool                   `json:"ok"`
	Issues []string               `json:"issues,omitempty"`
	Info   map[string]interface{} `json:"info,omitempty"`
}

var OrchestratorAgentDoctorCmd = &cobra.Command{
	Use:   "agent-doctor [name]",
	Short: "Validate agents: model IDs exist, API keys present, base URLs reachable",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		offline := agDoctorOffline || os.Getenv("SIN_LLM_OFFLINE") == "1"
		var filterName string
		if len(args) == 1 {
			filterName = strings.ToLower(args[0])
		}
		agents, err := loadAllEffectiveAgents()
		if err != nil {
			return err
		}
		if filterName != "" {
			filtered := []orchestrator.AgentConfig{}
			for _, a := range agents {
				if a.Name == filterName {
					filtered = append(filtered, a)
				}
			}
			agents = filtered
		}
		report := runDoctor(agents, offline)
		if orch2Format == "json" {
			return json.NewEncoder(os.Stdout).Encode(report)
		}
		printDoctor(report)
		failed := 0
		for _, r := range report {
			if !r.OK {
				failed++
			}
		}
		if failed > 0 {
			return fmt.Errorf("%d/%d agents have issues", failed, len(report))
		}
		return nil
	},
}

func init() {
	OrchestratorAgentDoctorCmd.Flags().BoolVar(&agDoctorOffline, "offline", false, "Skip /v1/models network check")
	OrchestratorRunCmd.AddCommand(OrchestratorAgentShowCmd)
	OrchestratorRunCmd.AddCommand(OrchestratorAgentDoctorCmd)
}

func runDoctor(agents []orchestrator.AgentConfig, offline bool) []DoctorReport {
	out := make([]DoctorReport, 0, len(agents))
	for _, a := range agents {
		rep := DoctorReport{Agent: a.Name, OK: true, Info: map[string]interface{}{}}
		providerName := a.Provider
		if providerName == "" {
			providerName = "nim"
		}
		prov, perr := llm.LookupProvider(providerName)
		if perr != nil {
			rep.OK = false
			rep.Issues = append(rep.Issues, "unknown provider: "+providerName)
			out = append(out, rep)
			continue
		}
		rep.Info["provider"] = providerName
		rep.Info["provider_description"] = prov.Description

		baseURL := a.BaseURL
		if baseURL == "" {
			baseURL = prov.BaseURL
		}
		if baseURL == "" {
			rep.OK = false
			rep.Issues = append(rep.Issues, "no base_url configured")
		} else {
			rep.Info["base_url"] = baseURL
		}

		if prov.APIKeyEnv != "" {
			if os.Getenv(prov.APIKeyEnv) == "" && os.Getenv("SIN_LLM_API_KEY") == "" {
				rep.OK = false
				rep.Issues = append(rep.Issues, fmt.Sprintf("missing API key: set %s or SIN_LLM_API_KEY", prov.APIKeyEnv))
			} else {
				rep.Info["api_key_env"] = prov.APIKeyEnv
			}
		}

		model := a.Model
		if model == "" {
			model = prov.DefaultModel
		}
		rep.Info["model"] = model

		if !offline && baseURL != "" {
			models, merr := fetchModels(baseURL, prov.APIKeyEnv)
			if merr != nil {
				rep.Issues = append(rep.Issues, "could not fetch /v1/models: "+merr.Error())
			} else {
				if !stringInList(models, model) {
					rep.OK = false
					rep.Issues = append(rep.Issues, fmt.Sprintf("model %q not in provider's model list", model))
				}
				rep.Info["models_available"] = len(models)
			}
		}
		out = append(out, rep)
	}
	return out
}

func fetchModels(baseURL, apiKeyEnv string) ([]string, error) {
	apiKey := ""
	if apiKeyEnv != "" {
		apiKey = os.Getenv(apiKeyEnv)
	}
	if apiKey == "" {
		apiKey = os.Getenv("SIN_LLM_API_KEY")
	}
	req, err := http.NewRequestWithContext(context.Background(), "GET", strings.TrimRight(baseURL, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

func stringInList(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func printDoctor(reports []DoctorReport) {
	fmt.Printf("Doctor — %d agent(s):\n\n", len(reports))
	for _, r := range reports {
		icon := "✓"
		if !r.OK {
			icon = "✗"
		}
		fmt.Printf("%s %s\n", icon, r.Agent)
		if prov, ok := r.Info["provider"].(string); ok {
			fmt.Printf("  provider: %s\n", prov)
		}
		if base, ok := r.Info["base_url"].(string); ok {
			fmt.Printf("  base_url: %s\n", base)
		}
		if model, ok := r.Info["model"].(string); ok {
			fmt.Printf("  model:    %s\n", model)
		}
		if cnt, ok := r.Info["models_available"].(int); ok {
			fmt.Printf("  available: %d models on provider\n", cnt)
		}
		for _, issue := range r.Issues {
			fmt.Printf("  ! %s\n", issue)
		}
		fmt.Println()
	}
}
