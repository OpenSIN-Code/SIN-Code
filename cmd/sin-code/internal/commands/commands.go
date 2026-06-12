// SPDX-License-Identifier: MIT
// Purpose: Custom slash commands loaded from .sin/commands/*.md (project)
// and ~/.config/sin-code/commands/*.md (user). YAML frontmatter supports
// description / agent profile override / verify_mode override
// (mandate C8, AGENTS.md §8). $ARGUMENTS is replaced with everything
// after the command name.
package commands

import (
	"os"
	"path/filepath"
	"strings"
)

type Command struct {
	Name        string
	Description string
	Agent       string
	VerifyMode  string
	Template    string
}

// Load merges user-level and project-level commands; project wins on conflict.
func Load(projectDir string) (map[string]Command, error) {
	out := map[string]Command{}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{
		filepath.Join(home, ".config", "sin-code", "commands"),
		filepath.Join(projectDir, ".sin", "commands"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			cmd := parse(strings.TrimSuffix(e.Name(), ".md"), string(raw))
			out[cmd.Name] = cmd
		}
	}
	return out, nil
}

// Expand renders the template with the user's arguments.
func (c Command) Expand(args string) string {
	return strings.ReplaceAll(c.Template, "$ARGUMENTS", strings.TrimSpace(args))
}

func parse(name, raw string) Command {
	cmd := Command{Name: name, Template: raw}
	if !strings.HasPrefix(raw, "---\n") {
		return cmd
	}
	rest := raw[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return cmd
	}
	front, body := rest[:end], rest[end+4:]
	cmd.Template = strings.TrimLeft(body, "\n")
	for _, line := range strings.Split(front, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "description":
			cmd.Description = v
		case "agent":
			cmd.Agent = v
		case "verify_mode":
			cmd.VerifyMode = v
		}
	}
	return cmd
}
