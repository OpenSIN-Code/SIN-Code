package rtk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// MCPToolDefinition represents an MCP tool definition
type MCPToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// MCPToolHandler handles MCP tool calls
type MCPToolHandler struct {
	executor RTKExecutor
	tools    *RTKToolRegistry
}

// NewMCPToolHandler creates a new MCP tool handler
func NewMCPToolHandler(executor RTKExecutor) *MCPToolHandler {
	if executor == nil {
		return nil
	}

	return &MCPToolHandler{
		executor: executor,
		tools:    NewRTKToolRegistry(),
	}
}

// RegisterTool adds a tool to the MCP handler
func (h *MCPToolHandler) RegisterTool(tool *RTKTool) error {
	if tool == nil {
		return ErrInvalidTool
	}
	return h.tools.Register(tool)
}

// GetToolDefinitions returns all available tool definitions for MCP
func (h *MCPToolHandler) GetToolDefinitions() []MCPToolDefinition {
	definitions := []MCPToolDefinition{}
	
	for _, tool := range h.tools.List() {
		def := MCPToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"args": map[string]interface{}{
						"type":  "array",
						"items": map[string]interface{}{"type": "string"},
						"description": "Command arguments",
					},
					"timeout": map[string]interface{}{
						"type":        "string",
						"description": "Execution timeout (e.g., '30s')",
					},
				},
				"required": []string{"args"},
			},
		}
		definitions = append(definitions, def)
	}

	return definitions
}

// CallTool executes a tool through MCP interface
func (h *MCPToolHandler) CallTool(ctx context.Context, name string, input map[string]interface{}) (string, error) {
	// Get tool
	tool, ok := h.tools.Get(name)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	// Prepare tool for execution
	execTool := &RTKTool{
		Name:        tool.Name,
		Kind:        tool.Kind,
		Description: tool.Description,
		Args:        tool.Args,
		Timeout:     tool.Timeout,
		CacheKey:    tool.CacheKey,
		RetryPolicy: tool.RetryPolicy,
		Tags:        tool.Tags,
		Enabled:     tool.Enabled,
	}

	// Override args if provided
	if args, ok := input["args"].([]interface{}); ok {
		execTool.Args = []string{}
		for _, arg := range args {
			if str, ok := arg.(string); ok {
				execTool.Args = append(execTool.Args, str)
			}
		}
	}

	// Execute
	result, err := h.executor.Execute(ctx, execTool)
	if err != nil {
		return "", err
	}

	// Format output
	output := map[string]interface{}{
		"name":       result.Name,
		"status":     result.Status,
		"exitCode":   result.ExitCode,
		"duration":   result.Duration.String(),
		"cached":     result.Cached,
		"tokenCount": result.TokenCount,
	}

	// Include cleaned output for better token efficiency
	if h.executor.GetConfig().StripANSI {
		output["stdout"] = result.StdoutClean
		output["stderr"] = result.StderrClean
	} else {
		output["stdout"] = result.Stdout
		output["stderr"] = result.Stderr
	}

	// Add metadata if available
	if result.Metadata != nil {
		output["metadata"] = result.Metadata
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// ValidateTool checks if a tool is valid and enabled
func (h *MCPToolHandler) ValidateTool(name string) error {
	tool, ok := h.tools.Get(name)
	if !ok {
		return fmt.Errorf("tool not found: %s", name)
	}

	if !tool.Enabled {
		return fmt.Errorf("tool disabled: %s", name)
	}

	return nil
}

// GetToolStats returns statistics for a tool
func (h *MCPToolHandler) GetToolStats(name string) map[string]interface{} {
	tool, ok := h.tools.Get(name)
	if !ok {
		return nil
	}

	return map[string]interface{}{
		"name":        tool.Name,
		"kind":        tool.Kind,
		"enabled":     tool.Enabled,
		"timeout":     tool.Timeout.String(),
		"description": tool.Description,
		"tags":        tool.Tags,
	}
}

// MCPToolOptions defines standard MCP tool configurations
type MCPToolOptions struct {
	CommandTools   bool // Include standard commands (ls, cat, grep, etc.)
	AnalyzerTools  bool // Include analyzer tools
	ValidatorTools bool // Include validator tools
	FormatterTools bool // Include formatter tools
}

// RegisterDefaultTools registers standard RTK tools
func (h *MCPToolHandler) RegisterDefaultTools(opts MCPToolOptions) {
	if opts.CommandTools {
		h.RegisterTool(&RTKTool{
			Name:        "rtk_lint",
			Kind:        RTKToolKindValidator,
			Description: "Run RTK linter",
			Args:        []string{"lint"},
			Timeout:     30 * time.Second,
			Enabled:     true,
		})

		h.RegisterTool(&RTKTool{
			Name:        "rtk_format",
			Kind:        RTKToolKindFormatter,
			Description: "Format code with RTK",
			Args:        []string{"format"},
			Timeout:     30 * time.Second,
			Enabled:     true,
		})

		h.RegisterTool(&RTKTool{
			Name:        "rtk_test",
			Kind:        RTKToolKindValidator,
			Description: "Run RTK tests",
			Args:        []string{"test"},
			Timeout:     60 * time.Second,
			Enabled:     true,
		})

		h.RegisterTool(&RTKTool{
			Name:        "rtk_analyze",
			Kind:        RTKToolKindAnalyzer,
			Description: "Analyze code with RTK",
			Args:        []string{"analyze"},
			Timeout:     45 * time.Second,
			Enabled:     true,
		})
	}
}

// ComposeToolResult combines multiple tool results
func ComposeToolResult(results map[string]*RTKResult) map[string]interface{} {
	summary := map[string]interface{}{
		"toolCount":    len(results),
		"successCount": 0,
		"failureCount": 0,
		"totalTokens":  0,
		"totalDuration": 0,
		"results":      map[string]interface{}{},
	}

	var successCount, failureCount int
	var totalTokens int
	var totalDuration int64

	for name, result := range results {
		resultData := map[string]interface{}{
			"status":     result.Status,
			"exitCode":   result.ExitCode,
			"tokenCount": result.TokenCount,
			"duration":   result.Duration.String(),
		}

		if result.Status == RTKStatusSuccess {
			successCount++
		} else {
			failureCount++
		}

		totalTokens += result.TokenCount
		totalDuration += int64(result.Duration)

		summary["results"].(map[string]interface{})[name] = resultData
	}

	summary["successCount"] = successCount
	summary["failureCount"] = failureCount
	summary["totalTokens"] = totalTokens
	summary["totalDuration"] = fmt.Sprintf("%dms", totalDuration/1000000)

	return summary
}
