// SPDX-License-Identifier: MIT
// Purpose: verification gate tests (mandate M3, AGENTS.md §8).
package verify

import (
	"context"
	"testing"
)

func TestModeOffAlwaysPasses(t *testing.T) {
	g := NewGate("off", nil, nil)
	res := g.Run(context.Background(), "/tmp")
	if !res.Passed {
		t.Fatal("off must always pass")
	}
}

func TestDefaultModeIsPoC(t *testing.T) {
	g := NewGate("garbage", nil, nil)
	if g.Mode() != ModePoC {
		t.Fatalf("default must be poc, got %q", g.Mode())
	}
}

func TestMissingRunnerFails(t *testing.T) {
	g := NewGate("poc", nil, nil)
	res := g.Run(context.Background(), "/tmp")
	if res.Passed {
		t.Fatal("missing runner must fail")
	}
	if res.Report == "" {
		t.Fatal("expected report")
	}
}

func TestPocRunner(t *testing.T) {
	g := NewGate("poc", func(ctx context.Context, ws string) (bool, string, error) {
		return true, "poc-ok", nil
	}, nil)
	res := g.Run(context.Background(), "/tmp")
	if !res.Passed || res.Report != "poc-ok" {
		t.Fatalf("poc runner failed: %+v", res)
	}
}

func TestOracleRunner(t *testing.T) {
	g := NewGate("oracle", nil, func(ctx context.Context, ws string) (bool, string, error) {
		return false, "tests-fail", nil
	})
	res := g.Run(context.Background(), "/tmp")
	if res.Passed {
		t.Fatal("oracle should fail")
	}
	if res.Report != "tests-fail" {
		t.Fatalf("report wrong: %q", res.Report)
	}
}

func TestSetModeOverride(t *testing.T) {
	g := NewGate("poc", nil, nil)
	g.SetMode(ModeOff)
	if g.Mode() != ModeOff {
		t.Fatal("setmode failed")
	}
	g.SetMode("garbage")
	if g.Mode() != ModeOff {
		t.Fatal("setmode must ignore invalid values")
	}
}
