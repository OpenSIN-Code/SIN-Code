// SPDX-License-Identifier: MIT
// Purpose: emit the three derived JSON artifacts the SIN-Code
// engines need. Each emitter takes a parsed Config and returns
// a []byte ready to be written to disk.
//
// The emitted JSON is a *contract* with the three engines; this
// PR defines the contract. v1.1 wires the engines to read it.
package compiler

import (
	"encoding/json"
	"fmt"
)

// HooksOutput is the contract for .sin/hooks.json.
type HooksOutput struct {
	Version  int         `json:"version"`
	PreTool  []HookEntry `json:"pre_tool"`
	PostTool []HookEntry `json:"post_tool"`
}

// HookEntry is one hook in the .sin/hooks.json contract.
type HookEntry struct {
	Name    string `json:"name"`
	When    string `json:"when"`
	Block   bool   `json:"block,omitempty"`
	Run     string `json:"run,omitempty"`
	Message string `json:"message,omitempty"`
}

// VerifyOutput is the contract for internal/verify/config.json.
type VerifyOutput struct {
	Version    int               `json:"version"`
	Mode       string            `json:"mode"`
	Predicates []VerifyPredicate `json:"predicates"`
}

// VerifyPredicate is one predicate in the verify contract.
type VerifyPredicate struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Required bool   `json:"required"`
}

// PermissionOutput is the contract for
// internal/permission/policies.json.
type PermissionOutput struct {
	Version int      `json:"version"`
	Allow   []string `json:"allow"`
	Ask     []string `json:"ask"`
	Deny    []string `json:"deny"`
}

// LoopOutput is the v1.1 contract for the loop-engineering
// parameters from issue #155. v0 emits it (so the migration
// path is ready) but no engine reads it yet.
type LoopOutput struct {
	Version        int      `json:"version"`
	MaxTurns       int      `json:"max_turns,omitempty"`
	MaxStopRejects int      `json:"max_stop_rejects,omitempty"`
	StallThreshold int      `json:"stall_threshold,omitempty"`
	MaxTokens      int      `json:"max_tokens,omitempty"`
	VerifyMode     string   `json:"verify_mode,omitempty"`
	DisableChecks  []string `json:"disable_checks,omitempty"`
}

// EmitHooks produces .sin/hooks.json.
func EmitHooks(c *Config) ([]byte, error) {
	out := HooksOutput{
		Version:  SchemaVersion,
		PreTool:  convertHooks(c.Hooks.PreTool),
		PostTool: convertHooks(c.Hooks.PostTool),
	}
	return json.MarshalIndent(out, "", "  ")
}

// EmitVerify produces internal/verify/config.json.
func EmitVerify(c *Config) ([]byte, error) {
	out := VerifyOutput{
		Version:    SchemaVersion,
		Mode:       c.Verify.Mode,
		Predicates: make([]VerifyPredicate, 0, len(c.Verify.Predicates)),
	}
	for _, p := range c.Verify.Predicates {
		out.Predicates = append(out.Predicates, VerifyPredicate{
			Name:     p.Name,
			Command:  p.Command,
			Required: p.Required,
		})
	}
	return json.MarshalIndent(out, "", "  ")
}

// EmitPermissions produces internal/permission/policies.json.
func EmitPermissions(c *Config) ([]byte, error) {
	out := PermissionOutput{
		Version: SchemaVersion,
		Allow:   orEmpty(c.Permissions.Allow),
		Ask:     orEmpty(c.Permissions.Ask),
		Deny:    orEmpty(c.Permissions.Deny),
	}
	return json.MarshalIndent(out, "", "  ")
}

// EmitLoop produces the v1.1 .sin/loop.json. v0 emits it for
// the migration path; no engine reads it yet.
func EmitLoop(c *Config) ([]byte, error) {
	out := LoopOutput{
		Version:        SchemaVersion,
		MaxTurns:       c.Loop.MaxTurns,
		MaxStopRejects: c.Loop.MaxStopRejects,
		StallThreshold: c.Loop.StallThreshold,
		MaxTokens:      c.Loop.MaxTokens,
		VerifyMode:     c.Loop.VerifyMode,
		DisableChecks:  orEmpty(c.Loop.DisableChecks),
	}
	return json.MarshalIndent(out, "", "  ")
}

// emitAll runs the four emitters and returns the four
// (filename, bytes) pairs. Used by the CLI and by the round-trip
// test.
func emitAll(c *Config) ([]OutputFile, error) {
	hooks, err := EmitHooks(c)
	if err != nil {
		return nil, fmt.Errorf("emit hooks: %w", err)
	}
	verify, err := EmitVerify(c)
	if err != nil {
		return nil, fmt.Errorf("emit verify: %w", err)
	}
	perms, err := EmitPermissions(c)
	if err != nil {
		return nil, fmt.Errorf("emit permissions: %w", err)
	}
	loop, err := EmitLoop(c)
	if err != nil {
		return nil, fmt.Errorf("emit loop: %w", err)
	}
	return []OutputFile{
		{Path: ".sin/hooks.json", Data: hooks},
		{Path: "internal/verify/config.json", Data: verify},
		{Path: "internal/permission/policies.json", Data: perms},
		{Path: ".sin/loop.json", Data: loop},
	}, nil
}

// OutputFile is one (path, data) pair from emitAll.
type OutputFile struct {
	Path string
	Data []byte
}

func convertHooks(in []Hook) []HookEntry {
	out := make([]HookEntry, 0, len(in))
	for _, h := range in {
		out = append(out, HookEntry{
			Name:    h.Name,
			When:    h.When,
			Block:   h.Block,
			Run:     h.Run,
			Message: h.Message,
		})
	}
	return out
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
