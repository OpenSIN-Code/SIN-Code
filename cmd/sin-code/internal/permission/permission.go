// SPDX-License-Identifier: MIT
// Purpose: allow/ask/deny engine that gates every tool call (mandate M4,
// AGENTS.md §8). Yolo bypasses Ask (never Deny). Headless: Ask -> Deny
// unless Yolo.
package permission

import (
	"path"
	"strings"
)

type Policy int

const (
	Deny Policy = iota
	Ask
	Allow
)

func (p Policy) String() string {
	switch p {
	case Allow:
		return "allow"
	case Ask:
		return "ask"
	default:
		return "deny"
	}
}

type Rule struct {
	Tool   string `json:"tool"`
	Policy string `json:"policy"`
}

type Engine struct {
	rules []Rule
	// Yolo bypasses every Ask (headless --yolo). Deny is NEVER bypassed.
	Yolo bool
	// Headless: Ask resolves to Deny unless Yolo is set.
	Headless bool
}

func New(rules []Rule) *Engine {
	return &Engine{rules: rules}
}

// Check returns the effective policy for a tool name.
// First matching rule wins; no match defaults to Ask.
func (e *Engine) Check(tool string) Policy {
	p := Ask
	for _, r := range e.rules {
		if matched, _ := path.Match(strings.ToLower(r.Tool), strings.ToLower(tool)); matched {
			p = parse(r.Policy)
			break
		}
	}
	if p == Ask {
		if e.Yolo {
			return Allow
		}
		if e.Headless {
			return Deny
		}
	}
	return p
}

func parse(s string) Policy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow":
		return Allow
	case "ask":
		return Ask
	default:
		return Deny
	}
}
