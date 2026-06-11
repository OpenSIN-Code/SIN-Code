// SPDX-License-Identifier: MIT
// Purpose: MCP tool handlers for the v2.0+ subcommands (todo, memory,
// notifications, orchestrator-*, agent-*, lsp). Each handler dispatches
// to the corresponding cobra subcommand and returns stdout.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/todo"
)

func runSinCodeCLI(args ...string) (string, error) {
	bin := os.Getenv("SIN_CODE_BIN")
	if bin != "" {
		// explicit override (tests, custom install layout) — trust it
	} else if _, err := exec.LookPath("sin-code"); err == nil {
		bin = "sin-code"
	} else {
		bin = os.Args[0]
	}
	// 5-minute upper bound — sub-commands are all internal and should
	// finish in seconds.  Guards against a buggy/fake script that reads
	// stdin forever in test contexts.
	ctx, cancel := context.WithTimeout(context.Background(), 5*60*1_000_000_000)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, string(out))
	}
	return string(out), nil
}

func handleTodoAdd(ctx context.Context, args map[string]any) (string, error) {
	cliArgs := []string{"todo", "add"}
	title, _ := args["title"].(string)
	if title == "" {
		return "", fmt.Errorf("title is required")
	}
	cliArgs = append(cliArgs, "--title", title)
	if v, ok := args["description"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--desc", v)
	}
	if v, ok := args["priority"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--priority", v)
	}
	if v, ok := args["type"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--type", v)
	}
	if v, ok := args["tags"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--tags", v)
	}
	if v, ok := args["project"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--project", v)
	}
	if v, ok := args["assignee"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--assignee", v)
	}
	cliArgs = append(cliArgs, "--format", "json")
	return runSinCodeCLI(cliArgs...)
}

func handleTodoList(ctx context.Context, args map[string]any) (string, error) {
	cliArgs := []string{"todo", "list", "--format", "json"}
	if v, ok := args["status"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--status", v)
	}
	if v, ok := args["priority"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--priority", v)
	}
	if v, ok := args["project"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--project", v)
	}
	if v, ok := args["tag"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--tag", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		cliArgs = append(cliArgs, "--limit", fmt.Sprintf("%d", int(v)))
	}
	return runSinCodeCLI(cliArgs...)
}

func handleTodoShow(ctx context.Context, args map[string]any) (string, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	return runSinCodeCLI("todo", "show", id, "--format", "json")
}

func handleTodoComplete(ctx context.Context, args map[string]any) (string, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	return runSinCodeCLI("todo", "complete", id)
}

func handleTodoClaim(ctx context.Context, args map[string]any) (string, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	as, _ := args["as"].(string)
	cliArgs := []string{"todo", "claim", id}
	if as != "" {
		cliArgs = append(cliArgs, "--as", as)
	}
	return runSinCodeCLI(cliArgs...)
}

func handleTodoReady(ctx context.Context, args map[string]any) (string, error) {
	return runSinCodeCLI("todo", "ready", "--format", "json")
}

func handleTodoBlocked(ctx context.Context, args map[string]any) (string, error) {
	return runSinCodeCLI("todo", "blocked", "--format", "json")
}

func handleTodoSearch(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	return runSinCodeCLI("todo", "list", "--format", "json", "--all", "--search", query)
}

func handleTodoPrime(ctx context.Context, args map[string]any) (string, error) {
	return runSinCodeCLI("todo", "prime")
}

func handleTodoStats(ctx context.Context, args map[string]any) (string, error) {
	return runSinCodeCLI("todo", "stats", "--format", "json")
}

func handleTodoDepAdd(ctx context.Context, args map[string]any) (string, error) {
	child, _ := args["child"].(string)
	parent, _ := args["parent"].(string)
	rel, _ := args["rel"].(string)
	if child == "" || parent == "" {
		return "", fmt.Errorf("child and parent are required")
	}
	if rel == "" {
		rel = "blocks"
	}
	return runSinCodeCLI("todo", "dep", "add", child, parent, "--type", rel)
}

func handleMemoryAdd(ctx context.Context, args map[string]any) (string, error) {
	insight, _ := args["insight"].(string)
	if insight == "" {
		return "", fmt.Errorf("insight is required")
	}
	cliArgs := []string{"memory", "add", insight, "--format", "json"}
	if v, ok := args["project"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--project", v)
	}
	if v, ok := args["tags"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--tags", v)
	}
	return runSinCodeCLI(cliArgs...)
}

func handleMemoryList(ctx context.Context, args map[string]any) (string, error) {
	cliArgs := []string{"memory", "list", "--format", "json"}
	if v, ok := args["project"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--project", v)
	}
	if v, ok := args["tag"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--tags", v)
	}
	return runSinCodeCLI(cliArgs...)
}

func handleMemorySearch(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	cliArgs := []string{"memory", "search", query, "--format", "json"}
	if v, ok := args["project"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--project", v)
	}
	if v, ok := args["top"].(float64); ok && v > 0 {
		cliArgs = append(cliArgs, "--top", fmt.Sprintf("%d", int(v)))
	}
	return runSinCodeCLI(cliArgs...)
}

func handleMemoryPrime(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	cliArgs := []string{"memory", "prime", query}
	if v, ok := args["project"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--project", v)
	}
	if v, ok := args["top"].(float64); ok && v > 0 {
		cliArgs = append(cliArgs, "--top", fmt.Sprintf("%d", int(v)))
	}
	return runSinCodeCLI(cliArgs...)
}

func handleMemoryStats(ctx context.Context, args map[string]any) (string, error) {
	store, err := memory.Open("")
	if err != nil {
		return "", err
	}
	defer store.Close()
	stats, err := store.Stats()
	if err != nil {
		return "", err
	}
	enabled, dim := store.EmbeddingStatus()
	out := map[string]any{
		"stats":     stats,
		"embedder":  enabled,
		"embed_dim": dim,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b), nil
}

func handleNotificationsList(ctx context.Context, args map[string]any) (string, error) {
	store, err := notifications.Open("")
	if err != nil {
		return "", err
	}
	defer store.Close()
	limit := 50
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	ns, err := store.List(notifications.ListFilter{NotDismissed: true}, limit)
	if err != nil {
		return "", err
	}
	b, _ := json.MarshalIndent(ns, "", "  ")
	return string(b), nil
}

func handleNotificationsStats(ctx context.Context, args map[string]any) (string, error) {
	store, err := notifications.Open("")
	if err != nil {
		return "", err
	}
	defer store.Close()
	st, err := store.ComputeStats()
	if err != nil {
		return "", err
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return string(b), nil
}

func handleNotificationsMarkRead(ctx context.Context, args map[string]any) (string, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	store, err := notifications.Open("")
	if err != nil {
		return "", err
	}
	defer store.Close()
	if err := store.MarkRead(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Read %s", id), nil
}

func handleOrchestratorRun(ctx context.Context, args map[string]any) (string, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	cliArgs := []string{"orchestrator-run", prompt, "--format", "json"}
	if v, ok := args["timeout"].(string); ok && v != "" {
		cliArgs = append(cliArgs, "--timeout", v)
	}
	if v, ok := args["max_parallel"].(float64); ok && v > 0 {
		cliArgs = append(cliArgs, "--max-parallel", fmt.Sprintf("%d", int(v)))
	}
	return runSinCodeCLI(cliArgs...)
}

func handleOrchestratorPlan(ctx context.Context, args map[string]any) (string, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	return runSinCodeCLI("orchestrator-plan", prompt, "--format", "json")
}

func handleOrchestratorAgents(ctx context.Context, args map[string]any) (string, error) {
	return runSinCodeCLI("orchestrator-agents", "--format", "json")
}

func handleAgentShow(ctx context.Context, args map[string]any) (string, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	return runSinCodeCLI("orchestrator-run", "agent-show", name, "--format", "json")
}

func handleAgentSet(ctx context.Context, args map[string]any) (string, error) {
	name, _ := args["name"].(string)
	kvs, _ := args["kvs"].([]any)
	if name == "" || len(kvs) == 0 {
		return "", fmt.Errorf("name and kvs required")
	}
	strs := make([]string, 0, len(kvs))
	for _, v := range kvs {
		if s, ok := v.(string); ok {
			strs = append(strs, s)
		}
	}
	cliArgs := []string{"orchestrator-run", "agent-set", name}
	cliArgs = append(cliArgs, strs...)
	return runSinCodeCLI(cliArgs...)
}

func handleAgentDoctor(ctx context.Context, args map[string]any) (string, error) {
	cliArgs := []string{"orchestrator-run", "agent-doctor", "--format", "json"}
	if v, ok := args["offline"].(bool); ok && v {
		cliArgs = append(cliArgs, "--offline")
	}
	if name, ok := args["name"].(string); ok && name != "" {
		cliArgs = append(cliArgs, name)
	}
	return runSinCodeCLI(cliArgs...)
}

func handleLspServers(ctx context.Context, args map[string]any) (string, error) {
	return runSinCodeCLI("lsp", "servers", "--format", "json")
}

func handleTodoDep(ctx context.Context, args map[string]any) (string, error) {
	child, _ := args["child"].(string)
	parent, _ := args["parent"].(string)
	if child == "" || parent == "" {
		return "", fmt.Errorf("child and parent are required")
	}
	return runSinCodeCLI("todo", "deps", child)
}

func stringJoin(parts []string, sep string) string {
	return strings.Join(parts, sep)
}

var _ = orchestrator.New
var _ = todo.GenerateID
