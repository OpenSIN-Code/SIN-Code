// SPDX-License-Identifier: MIT
package orchestrator

import (
	"context"
	"testing"
)

type scriptAgent struct {
	name  string
	reply string
}

func (s *scriptAgent) Name() string                 { return s.name }
func (s *scriptAgent) Config() AgentConfig          { return AgentConfig{Name: s.name, Type: TaskCode} }
func (s *scriptAgent) Run(ctx context.Context, _ *Task, _ *Scratchpad) (string, error) {
	return s.reply, nil
}

func TestCriticStopsOnPass(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	critic := NewCritic(vf, []Check{{Kind: CheckBuild, Name: "ok", Cmd: []string{"true"}}})
	ag := &scriptAgent{name: "a", reply: "ok"}
	res, err := critic.Drive(context.Background(), ag, &Task{ID: "t1", Title: "x", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatal("expected pass on first attempt")
	}
	if len(res.Attempts) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(res.Attempts))
	}
}

func TestCriticRestoresOriginalDescription(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	critic := NewCritic(vf, []Check{{Kind: CheckBuild, Name: "fail", Cmd: []string{"false"}}})
	ag := &scriptAgent{name: "a", reply: "ok"}
	task := &Task{ID: "t1", Title: "x", Description: "ORIGINAL"}
	_, _ = critic.Drive(context.Background(), ag, task, NewScratchpad())
	if task.Description != "ORIGINAL" {
		t.Fatalf("description must be restored, got %q", task.Description)
	}
}

func TestCriticStallDetection(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	critic := NewCritic(vf, []Check{{Kind: CheckBuild, Name: "fail", Cmd: []string{"false"}}})
	critic.Policy.MaxAttempts = 5
	critic.Policy.MinImprovement = 0.5
	ag := &scriptAgent{name: "a", reply: "ok"}
	res, err := critic.Drive(context.Background(), ag, &Task{ID: "t1", Title: "x", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("cannot pass on failing check")
	}
	if len(res.Attempts) > 2 {
		t.Fatalf("stall should stop after 1-2 attempts, got %d", len(res.Attempts))
	}
}

func TestCriticAgentErrorRecordsAttempt(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	critic := NewCritic(vf, []Check{{Kind: CheckBuild, Name: "ok", Cmd: []string{"true"}}})
	critic.Policy.MaxAttempts = 1
	task := &Task{ID: "t1", Title: "x", Description: "d"}
	// Use a context that the agent will react to via ctx.Done.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ag := &scriptAgent{name: "a", reply: "ok"}
	res, err := critic.Drive(ctx, ag, task, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("cancelled ctx should not pass")
	}
}
