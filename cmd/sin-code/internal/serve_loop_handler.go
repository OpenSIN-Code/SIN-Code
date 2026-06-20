// SPDX-License-Identifier: MIT
// Purpose: sin_run_loop MCP tool handler — exposes the full SIN-Code
// agent loop (PLAN→ACT→VERIFY→DONE) as a single synchronous MCP call.
// In-process: delegates to a factory registered by package main to
// avoid an import cycle (internal ← loopbuilder ← internal).
// Docs: serve_loop_handler.doc.md
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

type RunLoopOptions struct {
	Workspace  string
	Headless   bool
	VerifyMode string
	VerifyCmd  string
	MaxTurns   int
	Yolo       bool
	Model      string
	Style      string
	AgentName  string
	SkipMCP    bool
	Contract   *goalcontract.GoalContract
}

type RunLoopFactory func(ctx context.Context, opts RunLoopOptions, memStore *lessons.Store) (*agentloop.Loop, func() error, error)

var runLoopFactory RunLoopFactory

func RegisterRunLoopFactory(f RunLoopFactory) error {
	if f == nil {
		return fmt.Errorf("RegisterRunLoopFactory: nil factory")
	}
	runLoopFactory = f
	return nil
}

var (
	runLoopLessonsOpenFn = lessons.Open
	runLoopSessionOpenFn = session.Open
)

func buildLoopOptions(args map[string]any) (RunLoopOptions, error) {
	prompt := stringArg(args, "prompt", "")
	if prompt == "" {
		return RunLoopOptions{}, fmt.Errorf("prompt is required")
	}

	workspace := stringArg(args, "workspace", ".")
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return RunLoopOptions{}, fmt.Errorf("resolve workspace: %w", err)
	}

	maxTurns := intArg(args, "max_turns", 80)
	verifyCmd := stringArg(args, "verify_cmd", "")
	yolo := boolArg(args, "yolo")
	model := stringArg(args, "model", "")
	agent := stringArg(args, "agent", "")
	style := stringArg(args, "style", "default")

	var criteria []string
	if raw, ok := args["criteria"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok && s != "" {
				criteria = append(criteria, s)
			}
		}
	}

	verifyMode := "poc"
	if verifyCmd == "" && len(criteria) == 0 {
		verifyMode = "off"
	}

	opts := RunLoopOptions{
		Workspace:  absWorkspace,
		Headless:   true,
		VerifyMode: verifyMode,
		VerifyCmd:  verifyCmd,
		MaxTurns:   maxTurns,
		Yolo:       yolo,
		Model:      model,
		Style:      style,
		AgentName:  agent,
		SkipMCP:    true,
	}

	if len(criteria) > 0 {
		opts.Contract = &goalcontract.GoalContract{SemanticCriteria: criteria}
	}

	return opts, nil
}

func handleRunLoop(ctx context.Context, args map[string]any) (string, error) {
	prompt := stringArg(args, "prompt", "")
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	opts, err := buildLoopOptions(args)
	if err != nil {
		return "", err
	}

	if runLoopFactory == nil {
		return "", fmt.Errorf("sin_run_loop: factory not registered — run sin-code serve from the main binary")
	}

	memStore, err := runLoopLessonsOpenFn("")
	if err != nil {
		return "", fmt.Errorf("open lessons store: %w", err)
	}
	defer memStore.Close()

	loop, cleanup, err := runLoopFactory(ctx, opts, memStore)
	if err != nil {
		return "", fmt.Errorf("build agent loop: %w", err)
	}
	defer cleanup()

	store, err := runLoopSessionOpenFn(session.DefaultPath())
	if err != nil {
		return "", fmt.Errorf("open sessions store: %w", err)
	}
	defer store.Close()

	sess, err := store.StartOrResume("")
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	result, err := loop.Run(ctx, sess, prompt)
	if err != nil {
		return "", fmt.Errorf("agent loop failed: %w", err)
	}

	out := map[string]any{
		"session_id": result.SessionID,
		"summary":    result.Summary,
		"verified":   result.Verified,
		"turns":      result.Turns,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b), nil
}
