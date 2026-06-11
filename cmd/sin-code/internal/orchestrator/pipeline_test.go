// SPDX-License-Identifier: MIT
// Purpose: tests for the typed pipeline mechanism.

package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewChain_RejectsEmpty(t *testing.T) {
	if _, err := NewChain("empty"); err == nil {
		t.Fatal("NewChain with no stages must error")
	}
}

func TestNewChain_TypeMismatch(t *testing.T) {
	a := &FuncStage{StageName: "a", In: "x", Out: "y",
		Fn: func(context.Context, Artifact) (Artifact, error) { return Artifact{Type: "y"}, nil }}
	b := &FuncStage{StageName: "b", In: "z", Out: "z",
		Fn: func(context.Context, Artifact) (Artifact, error) { return Artifact{Type: "z"}, nil }}
	if _, err := NewChain("bad", a, b); err == nil {
		t.Fatal("mismatched OutType / InType must error")
	}
}

func TestNewChain_AnyInIsAccepted(t *testing.T) {
	a := &FuncStage{StageName: "a", In: "x", Out: "y",
		Fn: func(context.Context, Artifact) (Artifact, error) { return Artifact{Type: "y"}, nil }}
	b := &FuncStage{StageName: "b", In: "any", Out: "y",
		Fn: func(context.Context, Artifact) (Artifact, error) { return Artifact{Type: "y"}, nil }}
	if _, err := NewChain("flex", a, b); err != nil {
		t.Fatalf("InType=any must be accepted as next In: %v", err)
	}
}

func TestChainRun_HappyPath(t *testing.T) {
	a := &FuncStage{StageName: "a", In: "x", Out: "y",
		Fn: func(_ context.Context, in Artifact) (Artifact, error) {
			return Artifact{Type: "y", Data: in.Data.(string) + "+a"}, nil
		}}
	b := &FuncStage{StageName: "b", In: "y", Out: "z",
		Fn: func(_ context.Context, in Artifact) (Artifact, error) {
			return Artifact{Type: "z", Data: in.Data.(string) + "+b"}, nil
		}}
	c, err := NewChain("seq", a, b)
	if err != nil {
		t.Fatal(err)
	}
	out, trace := c.Run(context.Background(), Artifact{Type: "x", Data: "start"})
	if out.Type != "z" || out.Data.(string) != "start+a+b" {
		t.Fatalf("bad final output: %+v", out)
	}
	if trace.Halted {
		t.Fatal("happy-path chain must not halt")
	}
	if len(trace.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(trace.Results))
	}
	diag := trace.Diagnosis()
	if !strings.Contains(diag, "PIPELINE seq: 2/2 stages completed") {
		t.Fatalf("Diagnosis missing header: %q", diag)
	}
}

func TestChainRun_HaltsOnError(t *testing.T) {
	a := &FuncStage{StageName: "a", In: "x", Out: "y",
		Fn: func(_ context.Context, in Artifact) (Artifact, error) {
			return Artifact{Type: "y", Data: in.Data}, nil
		}}
	boom := &FuncStage{StageName: "boom", In: "y", Out: "y",
		Fn: func(context.Context, Artifact) (Artifact, error) {
			return Artifact{}, errors.New("kaboom")
		}}
	c, _ := NewChain("explodes", a, boom)
	_, trace := c.Run(context.Background(), Artifact{Type: "x", Data: 1})
	if !trace.Halted {
		t.Fatal("chain must halt on error")
	}
	if trace.HaltedAt != "boom" {
		t.Fatalf("expected halt at boom, got %q", trace.HaltedAt)
	}
	if trace.completed() != 1 {
		t.Fatalf("expected 1 completed, got %d", trace.completed())
	}
	diag := trace.Diagnosis()
	if !strings.Contains(diag, "FAILED: kaboom") {
		t.Fatalf("Diagnosis must include error: %q", diag)
	}
}

func TestParallelStage_MergesResults(t *testing.T) {
	stage := Parallel("para", "merged", func(arts []Artifact) (Artifact, error) {
		parts := make([]string, len(arts))
		for i, a := range arts {
			parts[i] = a.Data.(string)
		}
		return Artifact{Type: "merged", Data: strings.Join(parts, "|")}, nil
	},
		&FuncStage{StageName: "b1", In: "x", Out: "x",
			Fn: func(context.Context, Artifact) (Artifact, error) {
				return Artifact{Type: "x", Data: "B1"}, nil
			}},
		&FuncStage{StageName: "b2", In: "x", Out: "x",
			Fn: func(context.Context, Artifact) (Artifact, error) {
				return Artifact{Type: "x", Data: "B2"}, nil
			}},
		&FuncStage{StageName: "b3", In: "x", Out: "x",
			Fn: func(context.Context, Artifact) (Artifact, error) {
				return Artifact{Type: "x", Data: "B3"}, nil
			}},
	)
	if stage.InType() != "x" || stage.OutType() != "merged" {
		t.Fatalf("type signatures wrong: in=%q out=%q",
			stage.InType(), stage.OutType())
	}
	if !strings.HasPrefix(stage.Name(), "para") {
		t.Fatalf("name should be 'para...', got %q", stage.Name())
	}
	out, err := stage.Run(context.Background(), Artifact{Type: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data.(string) != "B1|B2|B3" {
		t.Fatalf("merge wrong: %q", out.Data)
	}
}

func TestParallelStage_BranchErrorPropagates(t *testing.T) {
	stage := Parallel("para", "merged", func([]Artifact) (Artifact, error) {
		return Artifact{}, errors.New("merge never reached")
	},
		&FuncStage{StageName: "ok", In: "x", Out: "x",
			Fn: func(context.Context, Artifact) (Artifact, error) {
				return Artifact{Type: "x", Data: "ok"}, nil
			}},
		&FuncStage{StageName: "boom", In: "x", Out: "x",
			Fn: func(context.Context, Artifact) (Artifact, error) {
				return Artifact{}, errors.New("branch fail")
			}},
	)
	_, err := stage.Run(context.Background(), Artifact{Type: "x"})
	if err == nil || !strings.Contains(err.Error(), "branch fail") {
		t.Fatalf("branch error must propagate: %v", err)
	}
}

func TestGuard_RejectsAndPasses(t *testing.T) {
	inner := &FuncStage{StageName: "inner", In: "x", Out: "x",
		Fn: func(context.Context, Artifact) (Artifact, error) {
			return Artifact{Type: "x", Data: "ok"}, nil
		}}
	g := Guard(inner, func(a Artifact) error {
		if a.Data.(int) < 0 {
			return errors.New("negative rejected")
		}
		return nil
	})
	if !strings.HasSuffix(g.Name(), "+guard") {
		t.Fatalf("guard name should suffix +guard, got %q", g.Name())
	}
	if g.InType() != "x" || g.OutType() != "x" {
		t.Fatalf("guard preserves types: %q -> %q", g.InType(), g.OutType())
	}
	if _, err := g.Run(context.Background(), Artifact{Type: "x", Data: -1}); err == nil {
		t.Fatal("guard should reject negative")
	}
	out, err := g.Run(context.Background(), Artifact{Type: "x", Data: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data.(string) != "ok" {
		t.Fatalf("positive input must pass: %+v", out)
	}
}

func TestFuncStage_Accessors(t *testing.T) {
	f := &FuncStage{StageName: "name", In: "A", Out: "B",
		Fn: func(_ context.Context, in Artifact) (Artifact, error) {
			return Artifact{Type: "B", Data: in.Data}, nil
		}}
	if f.Name() != "name" || f.InType() != "A" || f.OutType() != "B" {
		t.Fatalf("accessors wrong: %s/%s/%s", f.Name(), f.InType(), f.OutType())
	}
	out, err := f.Run(context.Background(), Artifact{Data: 7})
	if err != nil || out.Data.(int) != 7 {
		t.Fatalf("FuncStage passthrough broken: %+v err=%v", out, err)
	}
}

func TestChain_OneStage(t *testing.T) {
	c, err := NewChain("one", &FuncStage{StageName: "single", In: "x", Out: "x",
		Fn: func(context.Context, Artifact) (Artifact, error) {
			return Artifact{Type: "x", Data: 42}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	out, trace := c.Run(context.Background(), Artifact{Type: "x", Data: 0})
	if out.Data.(int) != 42 || len(trace.Results) != 1 {
		t.Fatalf("single-stage chain wrong: %+v", out)
	}
}
