// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` activator, string-list utilities, and sandbox
// policy resolution for the chat session.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife/autoactivate"
)

// chatActivator bundles the autoactivate.Activator with the CLI flags
// that should be applied when a real session id is known. A single
// instance is created per chat invocation; it is GC'd on exit.
type chatActivator struct {
	Act      *autoactivate.Activator
	Defaults autoactivate.RuleSet
	Def      autoactivate.Default
	Rules    []string // CLI --activate list (names only — bodies come from TOML)
}

// newChatActivator constructs a chatActivator from workspace +
// the optional `--activate <list>` and `--no-trigger` flags. Reads
// `.sin-code/autoactivate.toml` silently when present (privacy-first).
func newChatActivator(workspace string, opts *chatOptions) *chatActivator {
	defaults, def, _ := autoactivate.LoadFile(filepath.Join(workspace, ".sin-code", "autoactivate.toml"))
	return &chatActivator{
		Act:      autoactivate.NewActivator(defaults),
		Defaults: defaults,
		Def:      def,
		Rules:    parseActivateFlag(opts.activate),
	}
}

// parseActivateFlag splits a comma-separated rule list into trimmed
// non-empty names. Empty input returns nil.
func parseActivateFlag(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, raw := range strings.Split(s, ",") {
		n := strings.TrimSpace(raw)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// splitList splits a comma-separated list into trimmed, non-empty tokens.
// Empty input returns nil.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, raw := range strings.Split(s, ",") {
		n := strings.TrimSpace(raw)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// applyChatSandboxPolicy resolves the sin_bash sandbox posture for
// `sin-code chat` (issue #420) and writes it to the package-level
// sandboxConfig. The decision is:
//
//	--no-sandbox            → warn + force "none"
//	--sandbox <backend>     → use that backend (or "none" to disable)
//	headless (-p / --json) → announce sandbox=ON to stderr (M3/M4:
//	                          no silent security-posture change)
//	otherwise               → platform default (already enabled by
//	                          setSandboxConfig when --sandbox is empty)
//
// The headless mode (-p / --json) defaults to sandbox=ON per the M3
// (verification gate) + M4 (headless ask→deny) mandate: every
// destructive tool that the LLM can drive when the user is not at a
// terminal must be confined. `opts.noSandbox` is the single explicit
// escape hatch and prints a WARN to stderr so the operator can spot
// the relaxed posture in CI logs.
func applyChatSandboxPolicy(opts *chatOptions, headless bool, workspace string) {
	backend := opts.sandbox
	if opts.noSandbox {
		backend = "none"
		fmt.Fprintf(chatStderr,
			"WARN: --no-sandbox disables OS-level isolation for sin_bash. "+
				"Headless mode (M3/M4, issue #420) defaults to ON; use this only for debugging.\n")
	}
	setSandboxConfig(backend, workspace)
	if !headless {
		return
	}
	if sandboxConfig.enabled {
		name := sandboxBackendDisplay(backend)
		fmt.Fprintf(chatStderr,
			"sin-code chat: headless mode — sandbox enabled (backend=%s workspace=%s, issue #420)\n",
			name, workspace)
	} else {
		fmt.Fprintf(chatStderr,
			"sin-code chat: headless mode — sandbox DISABLED (--no-sandbox / --sandbox none)\n")
	}
}

// sandboxBackendDisplay returns a human-friendly rendering of the
// backend string for the headless-mode stderr announcement. Empty
// input falls back to the platform default name via the sandbox
// package, but we keep the resolver dependency-free here so a missing
// backend selection stays visible.
func sandboxBackendDisplay(backend string) string {
	switch backend {
	case "":
		return "platform-default"
	case "none":
		return "none"
	default:
		return backend
	}
}
