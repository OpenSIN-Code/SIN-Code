// SPDX-License-Identifier: MIT
// Purpose: Source adapter for the built-in chat tools registered in
// chat_tools.go and chat_tools_extra.go. These tools are only available
// inside the `sin-code chat` REPL agent loop.
package catalog

import "context"

// ChatSource is a Source backed by the static built-in chat tool list.
type ChatSource struct{}

// Name implements Source.
func (ChatSource) Name() string { return "chat" }

// List implements Source.
func (ChatSource) List(_ context.Context, kind Kind) ([]*Asset, error) {
	if kind != "" && kind != KindChat {
		return nil, nil
	}
	out := make([]*Asset, 0, len(chatTools))
	for _, t := range chatTools {
		out = append(out, &Asset{
			Kind:        KindChat,
			Name:        t.name,
			Namespace:   t.name,
			Short:       t.short,
			Description: t.description,
			Example:     t.example,
			Source:      "chat",
			Tags:        t.tags,
			ReadOnly:    t.readOnly,
			Destructive: t.destructive,
		})
	}
	return out, nil
}

// Get implements Source.
func (ChatSource) Get(_ context.Context, kind Kind, name string) (*Asset, bool, error) {
	if kind != "" && kind != KindChat {
		return nil, false, nil
	}
	for _, t := range chatTools {
		if t.name == name {
			return &Asset{
				Kind:        KindChat,
				Name:        t.name,
				Namespace:   t.name,
				Short:       t.short,
				Description: t.description,
				Example:     t.example,
				Source:      "chat",
				Tags:        t.tags,
				ReadOnly:    t.readOnly,
				Destructive: t.destructive,
			}, true, nil
		}
	}
	return nil, false, nil
}

type chatTool struct {
	name        string
	short       string
	description string
	example     string
	tags        []string
	readOnly    bool
	destructive bool
}

// chatTools mirrors the tools defined in chat_tools.go and chat_tools_extra.go.
var chatTools = []chatTool{
	{name: "sin_bash", short: "Run shell command", description: "Run a shell command in the workspace (120s timeout).", example: `{\"command\": \"go test ./...\"}`, tags: []string{"destructive", "shell"}, destructive: true},
	{name: "sin_bootstrap_skill", short: "Scaffold MCP skill", description: "Scaffold a new MCP skill server (Python stdio) in .sin-code/skills/<name>/ and register it in mcp.json. Requires the workspace to allow bootstrap.", example: `{\"name\": \"my_skill\", \"spec\": \"description\"}`, tags: []string{"destructive", "meta", "skills"}, destructive: true},
	{name: "sin_browser_diff", short: "Compare browser sessions", description: "Compare two browser sessions — the stored baseline with the current session — and return a Diff.", example: `{\"window\": \"25\"}`, tags: []string{"read-only", "browser"}, readOnly: true},
	{name: "sin_browser_findings", short: "Browser findings", description: "Return a full structured Report from the last sin_browser_navigate session.", example: `{}`, tags: []string{"read-only", "browser"}, readOnly: true},
	{name: "sin_browser_navigate", short: "Navigate headless Chrome", description: "Navigate headless Chrome to a URL and record the full CDP event stream.", example: `{\"url\": \"https://example.com\", \"wait_sec\": \"3\"}`, tags: []string{"destructive", "browser"}, destructive: true},
	{name: "sin_browser_snapshot", short: "Browser snapshot", description: "Return a compact JSON summary of the last sin_browser_navigate session.", example: `{}`, tags: []string{"read-only", "browser"}, readOnly: true},
	{name: "sin_browser_vitals_flush", short: "Flush Web Vitals", description: "Force a final Web Vitals metric flush in the current browser tab.", example: `{}`, tags: []string{"destructive", "browser"}, destructive: true},
	{name: "sin_edit", short: "Edit file", description: "Replace the first exact occurrence of old with new in a file.", example: `{\"path\": \"main.go\", \"old\": \"foo\", \"new\": \"bar\"}`, tags: []string{"destructive", "filesystem"}, destructive: true},
	{name: "sin_git_commit", short: "Commit changes", description: "Stage all changes and commit with a message (mutating — gated).", example: `{\"message\": \"feat: add auth\"}`, tags: []string{"destructive", "git"}, destructive: true},
	{name: "sin_git_diff", short: "Show diff", description: "Show working tree diff or diff vs a ref (read-only).", example: `{\"ref\": \"main\"}`, tags: []string{"read-only", "git"}, readOnly: true},
	{name: "sin_git_log", short: "Show commit history", description: "Show recent commit history (read-only).", example: `{\"limit\": \"10\"}`, tags: []string{"read-only", "git"}, readOnly: true},
	{name: "sin_http_get", short: "Fetch URL", description: "Fetch a URL (GET only, 256KB cap, 30s timeout). For docs/APIs.", example: `{\"url\": \"https://api.example.com/data\"}`, tags: []string{"read-only", "network"}, readOnly: true},
	{name: "sin_web_search", short: "Web search", description: "Search the web using multiple providers (DuckDuckGo free, Tavily AI, SerpAPI, Brave). Returns ranked results.", example: `{\"query\": \"Go 1.26 release date\"}`, tags: []string{"read-only", "network", "search"}, readOnly: true},
	{name: "sin_read", short: "Read file", description: "Read a file (UTF-8, capped at 64KB).", example: `{\"path\": \"main.go\"}`, tags: []string{"read-only", "filesystem"}, readOnly: true},
	{name: "sin_search", short: "Search files", description: "Search files for a substring; returns file:line matches.", example: `{\"pattern\": \"TODO\", \"dir\": \".\"}`, tags: []string{"read-only", "search"}, readOnly: true},
	{name: "sin_test", short: "Run tests", description: "Run the workspace test suite with race detection and coverage, returning structured pass/fail output.", example: `{\"target\": \"./...\", \"race\": \"true\"}`, tags: []string{"destructive", "test"}, destructive: true},
	{name: "sin_test_generate", short: "Generate tests", description: "Generate table-driven Go tests for a file or package.", example: `{\"file\": \"foo.go\"}`, tags: []string{"destructive", "test"}, destructive: true},
	{name: "sin_write", short: "Write file", description: "Atomically write content to a file, creating parent dirs.", example: `{\"path\": \"out.txt\", \"content\": \"hello\"}`, tags: []string{"destructive", "filesystem"}, destructive: true},
}
