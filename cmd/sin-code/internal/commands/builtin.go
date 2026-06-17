// SPDX-License-Identifier: MIT
// Purpose: registry for built-in Go-implemented slash commands (issue #276
// /btw, issue #274 /undercover). Complements the markdown-template Command
// loader (commands.go) which serves user-defined .md commands. Built-in
// commands implement BuiltinCommand and are dispatched by name; their
// Execute return value is rendered to the user WITHOUT being injected into
// the main conversation history (mandate C8, AGENTS.md §8).
package commands

import (
	"context"
	"fmt"
	"sort"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// BuiltinCommand is a slash command implemented in Go (not a markdown
// template). Execute returns a string that is displayed to the user but
// NOT appended to the session history — the main conversation context is
// preserved. This is the contract that /btw (issue #276) relies on.
type BuiltinCommand interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string, sess *session.Session) (string, error)
}

// SideLLM is the minimal LLM surface a built-in command needs to fire a
// one-shot completion. It is intentionally decoupled from internal/llm so
// the commands package has no upstream dependency and is trivially testable
// with a fake. The chat layer adapts *llm.Client to this interface.
type SideLLM interface {
	Complete(ctx context.Context, systemPrompt, userMessage string) (string, error)
}

// Registry holds built-in slash commands keyed by Name(). It is safe for
// concurrent registration and lookup (mandate M7). A nil Registry is
// valid and all methods are no-ops / return not-found.
type Registry struct {
	cmds map[string]BuiltinCommand
}

// NewRegistry returns an empty Registry ready for Register calls.
func NewRegistry() *Registry {
	return &Registry{cmds: make(map[string]BuiltinCommand)}
}

// Register adds (or replaces) a built-in command. Panics if c is nil.
func (r *Registry) Register(c BuiltinCommand) {
	if c == nil {
		panic("commands: Register called with nil command")
	}
	r.cmds[c.Name()] = c
}

// Get looks up a command by name. Returns (cmd, true) on hit.
func (r *Registry) Get(name string) (BuiltinCommand, bool) {
	if r == nil {
		return nil, false
	}
	c, ok := r.cmds[name]
	return c, ok
}

// Names returns the sorted list of registered command names.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.cmds))
	for name := range r.cmds {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Dispatch resolves name (with or without leading '/') and executes the
// matching command. Returns (handled, output, err). handled is false when
// the line is not a slash command or the name is not a registered built-in.
func (r *Registry) Dispatch(ctx context.Context, line string, sess *session.Session) (handled bool, output string, err error) {
	if r == nil {
		return false, "", nil
	}
	name, rawArgs, ok := parseSlashLine(line)
	if !ok {
		return false, "", nil
	}
	c, found := r.Get(name)
	if !found {
		return false, "", nil
	}
	out, e := c.Execute(ctx, rawArgs, sess)
	return true, out, e
}

// parseSlashLine splits a "/name rest" line into name (without slash) and
// the remainder. Returns ok=false when line does not start with '/'.
func parseSlashLine(line string) (name, args string, ok bool) {
	if len(line) == 0 || line[0] != '/' {
		return "", "", false
	}
	rest := line[1:]
	for i, ch := range rest {
		if ch == ' ' || ch == '\t' {
			return rest[:i], trimLeftSpaces(rest[i+1:]), true
		}
	}
	return rest, "", true
}

func trimLeftSpaces(s string) string {
	for i, ch := range s {
		if ch != ' ' && ch != '\t' && ch != '\n' {
			return s[i:]
		}
	}
	return ""
}

// ErrNoLLM is returned by built-in commands that require an LLM client when
// none was wired at construction time.
type ErrNoLLM struct{ CommandName string }

func (e ErrNoLLM) Error() string {
	return fmt.Sprintf("command %q: no LLM client available", e.CommandName)
}
