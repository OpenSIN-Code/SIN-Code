// SPDX-License-Identifier: MIT
// Purpose: bridge between the Spec-Layer (internal/spec) and the
// SIN-Code runtime. Adapts llm.Client to the spec.Completer
// interface so `sin spec author` can drive the Planner +
// Implementer loop end-to-end. Without a model client, the
// author loop falls back to the dry-run path (stub spec).
// Docs: docs/SPEC-LAYER.md §"Self-authoring"
package wiring

import (
	"context"
	"fmt"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec"
)

// llmCompleter adapts the existing llm.Client to spec.Completer.
// It uses the cheap background-model pattern (a single model
// alias) for both Planner and Implementer. In production the
// model is configured via the spec.AuthorOptions.Model field.
//
// The Completer interface (defined in internal/spec/author.go) is
// the only thing the spec package depends on. Keeping the
// interface there means the spec package stays free of llm
// dependencies and can be unit-tested with a stub (see
// internal/spec/author_test.go).
type llmCompleter struct {
	client *llm.Client
	model  string
}

// Complete implements spec.Completer.
func (c llmCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	if c.client == nil {
		return "", nil // dry-run path; author.go handles nil
	}
	model := c.model
	if model == "" {
		model = "anthropic/claude-haiku-4-5"
	}
	resp, err := c.client.Chat(ctx, llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ExtractText(), nil
}

// NewSpecCompleter returns a spec.Completer that talks to the
// given llm.Client. If client is nil, returns a nil Completer —
// the spec package's author loop treats a nil Completer as
// "dry-run" (returns a stub spec for end-to-end testing).
func NewSpecCompleter(client *llm.Client, model string) spec.Completer {
	if client == nil {
		return nil
	}
	return llmCompleter{client: client, model: model}
}

// SpecAuthorOptions carries the wiring the spec CLI needs to run
// `sin spec author`. The Completer is mandatory for the LLM
// loop; with nil, the CLI runs in dry-run mode (stubs out the
// spec).
type SpecAuthorOptions struct {
	Completer spec.Completer
	Model     string
	Timeout   time.Duration
	MaxRetries int
	Workdir   string
}

// AuthorSpec is a thin wrapper that runs spec.Author with the
// wiring's options. Kept here so the spec CLI doesn't need to
// know about llm.Client.
func AuthorSpec(ctx context.Context, desc string, opts SpecAuthorOptions) (*spec.AuthorResult, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 3
	}
	res, err := spec.Author(ctx, desc, spec.AuthorOptions{
		Completer:  opts.Completer,
		Timeout:    opts.Timeout,
		MaxRetries: opts.MaxRetries,
		Workdir:    opts.Workdir,
	})
	if err != nil {
		return nil, fmt.Errorf("wiring: spec author: %w", err)
	}
	return res, nil
}
