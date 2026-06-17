// SPDX-License-Identifier: MIT
// Purpose: Source adapter for the external MCP server prefixes registered
// in mcpclient.DefaultServers(). These servers are not vendored; they are
// discovered at runtime via stdio. The catalog lists the server prefix so
// operators can discover the full tool surface.
package catalog

import "context"

// ExternalSource is a Source backed by the external MCP server registry.
type ExternalSource struct{}

// Name implements Source.
func (ExternalSource) Name() string { return "external" }

// List implements Source.
func (ExternalSource) List(_ context.Context, kind Kind) ([]*Asset, error) {
	if kind != "" && kind != KindExternal {
		return nil, nil
	}
	out := make([]*Asset, 0, len(externalServers))
	for _, e := range externalServers {
		out = append(out, &Asset{
			Kind:        KindExternal,
			Name:        e.name,
			Namespace:   e.namespace,
			Short:       e.short,
			Description: e.description,
			Example:     e.example,
			Source:      "external",
			Tags:        e.tags,
			ReadOnly:    false,
			Destructive: true, // external servers may be mutating; gated by permission engine
		})
	}
	return out, nil
}

// Get implements Source.
func (ExternalSource) Get(_ context.Context, kind Kind, name string) (*Asset, bool, error) {
	if kind != "" && kind != KindExternal {
		return nil, false, nil
	}
	for _, e := range externalServers {
		if e.name == name {
			return &Asset{
				Kind:        KindExternal,
				Name:        e.name,
				Namespace:   e.namespace,
				Short:         e.short,
				Description:   e.description,
				Example:       e.example,
				Source:        "external",
				Tags:          e.tags,
				ReadOnly:      false,
				Destructive:   true,
			}, true, nil
		}
	}
	return nil, false, nil
}

type externalServer struct {
	name        string
	namespace   string
	short       string
	description string
	example     string
	tags        []string
}

// externalServers mirrors the registry in
// cmd/sin-code/internal/mcpclient/registry.go.
var externalServers = []externalServer{
	{name: "autodev", namespace: "autodev__*", short: "Autodev bridge", description: "Bridged-External autodev MCP server (Python stdio).", example: "autodev__plan --repo owner/repo", tags: []string{"external", "methodology"}},
	{name: "browser", namespace: "browser__*", short: "Browser automation", description: "SIN-Browser-Tools MCP server (CDP/headless Chrome).", example: "browser__navigate https://example.com", tags: []string{"external", "browser"}},
	{name: "codocs", namespace: "codocs__*", short: "Doc coauthoring", description: "SIN-Code-Doc-Coauthoring-Skill MCP server.", example: "codocs__draft --section API", tags: []string{"external", "docs"}},
	{name: "contextbridge", namespace: "contextbridge__*", short: "Context bridge", description: "SIN-Code-Context-Bridge-Skill MCP server.", example: "contextbridge__query 'auth module'", tags: []string{"external", "context"}},
	{name: "frontend", namespace: "frontend__*", short: "Frontend design", description: "SIN-Code-Frontend-Design-Skill MCP server.", example: "frontend__component_create button", tags: []string{"external", "design"}},
	{name: "goalmode", namespace: "goalmode__*", short: "Goal mode", description: "SIN-Code-Goal-Mode-Skill MCP server.", example: "goalmode__add --title 'Add tests'", tags: []string{"external", "goals"}},
	{name: "grillme", namespace: "grillme__*", short: "Grill me", description: "SIN-Code-Grill-Me-Skill MCP server.", example: "grillme__start --topic 'API design'", tags: []string{"external", "review"}},
	{name: "honcho", namespace: "honcho__*", short: "Honcho memory", description: "SIN-Code-Honcho-Rollback-Skill MCP server.", example: "honcho__memory_add --insight 'user prefers terse'", tags: []string{"external", "memory"}},
	{name: "marketplace", namespace: "marketplace__*", short: "Skill marketplace", description: "SIN-Code-Marketplace-Skill MCP server.", example: "marketplace__search skill", tags: []string{"external", "skills"}},
	{name: "mcpbuilder", namespace: "mcpbuilder__*", short: "MCP server builder", description: "SIN-Code-MCP-Server-Builder-Skill MCP server.", example: "mcpbuilder__scaffold --name my-tool", tags: []string{"external", "mcp"}},
	{name: "scheduler", namespace: "scheduler__*", short: "Job scheduler", description: "SIN-Code-Scheduler-Skill MCP server.", example: "scheduler__job_add --command 'go test ./...' --schedule '0 9 * * *'", tags: []string{"external", "schedule"}},
	{name: "simone", namespace: "simone__*", short: "Simone code intelligence", description: "Simone-MCP server (AST/LSP code intelligence).", example: "simone__symbol_search 'Server.Start'", tags: []string{"external", "code"}},
	{name: "symfonylens", namespace: "symfonylens__*", short: "Symfony lens", description: "SIN-Code-Symfony-Lens MCP server.", example: "symfonylens__analyze_routes /project", tags: []string{"external", "php"}},
	{name: "websearch", namespace: "websearch__*", short: "Web search", description: "Go-native web_search_bundle MCP server (sin-websearch).", example: "websearch__search 'Go 1.24 release'", tags: []string{"external", "network"}},
}
