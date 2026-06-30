// SPDX-License-Identifier: MIT
// Purpose: plugin type taxonomy and auto-detection. The unified plugin
// system (issue #489) lets users install MCP servers, skills, tools, and
// hooks from a single interface. DetectType infers the kind from the
// plugin name or source URL so the user rarely needs --type.
package plugins

import "strings"

// PluginType represents the kind of plugin.
type PluginType string

const (
	TypeMCP   PluginType = "mcp"
	TypeSkill PluginType = "skill"
	TypeTool  PluginType = "tool"
	TypeHook  PluginType = "hook"
)

func (t PluginType) String() string { return string(t) }

// DetectType tries to determine the plugin type from its name or source.
// It checks for "mcp", "skill", "hook" substrings (case-insensitive) and
// defaults to TypeTool when none match.
func DetectType(name, source string) PluginType {
	lowName := strings.ToLower(name)
	lowSrc := strings.ToLower(source)
	switch {
	case strings.Contains(lowName, "mcp") || strings.Contains(lowSrc, "mcp"):
		return TypeMCP
	case strings.Contains(lowName, "skill") || strings.Contains(lowSrc, "skill"):
		return TypeSkill
	case strings.Contains(lowName, "hook") || strings.Contains(lowSrc, "hook"):
		return TypeHook
	default:
		return TypeTool
	}
}
