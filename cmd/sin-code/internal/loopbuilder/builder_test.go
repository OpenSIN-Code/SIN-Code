// SPDX-License-Identifier: MIT
// Purpose: regression test for the shared loop factory.
package loopbuilder

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
)

func TestBuildRequiresValidConfig(t *testing.T) {
	_, _, err := Build(context.Background(), Config{
		Workspace: t.TempDir(),
		MaxTurns:  5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildLoadsAgentProfile(t *testing.T) {
	// If a non-existent agent is requested, Build should error out.
	_, _, err := Build(context.Background(), Config{
		Workspace: t.TempDir(),
		AgentName: "definitely-does-not-exist",
		MaxTurns:  5,
	}, nil)
	if err == nil {
		t.Fatal("expected error for non-existent agent profile")
	}
}

func TestBuildWithMemoryStore(t *testing.T) {
	mem, err := lessons.Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()
	loop, cleanup, err := Build(context.Background(), Config{
		Workspace: t.TempDir(),
		MaxTurns:  5,
	}, mem)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup function")
	}
	if loop == nil {
		t.Fatal("expected loop")
	}
	if loop.Lessons != mem {
		t.Fatal("expected Loop.Lessons to be the provided store")
	}
}

func TestBuildSetsSystemPrompt(t *testing.T) {
	loop, cleanup, err := Build(context.Background(), Config{
		Workspace: t.TempDir(),
		MaxTurns:  5,
		Style:     "terse",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if loop.SystemPrompt == "" {
		t.Fatal("expected Loop.SystemPrompt to be set")
	}
	if !strings.Contains(loop.SystemPrompt, "Tool preference (mandate M6)") {
		t.Fatalf("system prompt missing M6 block: %q", loop.SystemPrompt)
	}
	if !strings.Contains(loop.SystemPrompt, "# Output style") {
		t.Fatalf("system prompt missing style block for terse mode: %q", loop.SystemPrompt)
	}
}

func TestBuildSetsSystemPromptDefaultStyle(t *testing.T) {
	loop, cleanup, err := Build(context.Background(), Config{
		Workspace: t.TempDir(),
		MaxTurns:  5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if loop.SystemPrompt == "" {
		t.Fatal("expected Loop.SystemPrompt to be set")
	}
	if !strings.Contains(loop.SystemPrompt, "Tool preference (mandate M6)") {
		t.Fatalf("system prompt missing M6 block: %q", loop.SystemPrompt)
	}
	if strings.Contains(loop.SystemPrompt, "# Output style") {
		t.Fatalf("system prompt should not contain style block for default mode: %q", loop.SystemPrompt)
	}
}
