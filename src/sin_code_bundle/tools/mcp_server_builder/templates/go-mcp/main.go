// Purpose: {{ name }} MCP server entry point.
// Docs: main.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

{% for tool in tools %}
// {{ tool }} is a generated MCP tool.
func {{ tool | capitalize }}(ctx context.Context, args map[string]any) (map[string]any, error) {
	return map[string]any{
		"tool": "{{ tool }}",
		"ok":   true,
	}, nil
}
{% endfor %}

func main() {
	server := mcp.NewServer(&mcp.ServerOptions{
		Name:    "sin-{{ slug }}",
		Version: "{{ version }}",
	})

	{% for tool in tools %}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "{{ tool }}",
		Description: "{{ tool }} — generated tool.",
	}, {{ tool | capitalize }})
	{% endfor %}

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(json.NewEncoder(nil).Buffer, "%v\n", err)
	}
}
