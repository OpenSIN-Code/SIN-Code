// SPDX-License-Identifier: MIT
// Purpose: resolve a slash command asset into a final prompt with
// placeholders substituted. The frontmatter's `allowed-tools` is
// surfaced so the agent loop can restrict the toolset.
// Docs: command.doc.md
package dispatch

import (
	"fmt"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/assets"
)

// ResolvedCommand is a command asset with its placeholders filled in.
type ResolvedCommand struct {
	Name         string
	Prompt       string   // command body with arguments substituted
	AllowedTools []string // tool whitelist from frontmatter (empty = inherit all)
	Args         Args
}

// ResolveCommand looks up a command by name and substitutes the given
// raw args.
func ResolveCommand(reg *assets.Registry, name, rawArgs string) (ResolvedCommand, error) {
	a, ok := reg.Get(assets.KindCommand, strings.TrimPrefix(name, "/"))
	if !ok {
		return ResolvedCommand{}, fmt.Errorf("unknown command: %s", name)
	}
	args := ParseArgs(rawArgs)
	return ResolvedCommand{
		Name:         a.Name,
		Prompt:       args.Substitute(a.Body),
		AllowedTools: a.AllowedTools,
		Args:         args,
	}, nil
}

// ParseSlash splits a "/name rest of the args" line into name + raw
// args.
func ParseSlash(line string) (name, rawArgs string, isSlash bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return "", "", false
	}
	rest := strings.TrimPrefix(line, "/")
	if sp := strings.IndexByte(rest, ' '); sp >= 0 {
		return rest[:sp], strings.TrimSpace(rest[sp+1:]), true
	}
	return rest, "", true
}
