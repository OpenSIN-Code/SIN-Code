// SPDX-License-Identifier: MIT
// Purpose: unit tests for the quality gate runner.
package testgate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunAllPass(t *testing.T) {
	calls := []string{}
	cfg := Config{
		Workdir: ".",
		Timeout: 30 * time.Second,
		Steps:   []StepKind{StepBuild, StepVet},
		CommandRunner: func(ctx context.Context, name string, args []string, dir string, timeout time.Duration) (string, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return "ok", nil
		},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
	}
	report := Run(context.Background(), cfg)
	if report.Status != "PASS" {
		t.Fatalf("expected PASS, got %s", report.Status)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
}

func TestRunRequiredFail(t *testing.T) {
	cfg := Config{
		Workdir: ".",
		Timeout: 30 * time.Second,
		Steps:   []StepKind{StepBuild, StepVet},
		CommandRunner: func(ctx context.Context, name string, args []string, dir string, timeout time.Duration) (string, error) {
			if name == "go" && len(args) > 0 && args[0] == "build" {
				return "build error", errors.New("exit status 1")
			}
			return "ok", nil
		},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
	}
	report := Run(context.Background(), cfg)
	if report.Status != "FAIL" {
		t.Fatalf("expected FAIL, got %s", report.Status)
	}
	if len(report.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(report.Steps))
	}
	if report.Steps[0].Status != "FAIL" {
		t.Fatalf("expected build step FAIL, got %s", report.Steps[0].Status)
	}
	if report.Steps[1].Status != "SKIP" {
		t.Fatalf("expected vet step SKIP, got %s", report.Steps[1].Status)
	}
}

func TestRunOptionalMissing(t *testing.T) {
	cfg := Config{
		Workdir: ".",
		Timeout: 30 * time.Second,
		Steps:   []StepKind{StepStaticcheck},
		CommandRunner: func(ctx context.Context, name string, args []string, dir string, timeout time.Duration) (string, error) {
			return "ok", nil
		},
		LookPath: func(name string) (string, error) { return "", errors.New("not found") },
	}
	report := Run(context.Background(), cfg)
	if report.Status != "PASS" {
		t.Fatalf("expected PASS when optional step missing, got %s", report.Status)
	}
	if len(report.Steps) != 1 || !report.Steps[0].Skipped {
		t.Fatalf("expected staticcheck to be skipped, got %+v", report.Steps)
	}
}

func TestCoverageThreshold(t *testing.T) {
	cfg := Config{
		Workdir:           ".",
		Timeout:           30 * time.Second,
		Steps:             []StepKind{StepTest},
		CoverageThreshold: 90.0,
		CommandRunner: func(ctx context.Context, name string, args []string, dir string, timeout time.Duration) (string, error) {
			return "ok\ncoverage: 82.4% of statements\n", nil
		},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
	}
	report := Run(context.Background(), cfg)
	if report.Status != "FAIL" {
		t.Fatalf("expected FAIL below threshold, got %s", report.Status)
	}
	if report.CoveragePercent() != 82.4 {
		t.Fatalf("expected coverage 82.4, got %f", report.CoveragePercent())
	}
}

func TestParseCoveragePercent(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"coverage: 82.4% of statements", 82.4},
		{"", 0},
		{"no coverage", 0},
		{"coverage: 100.0% of statements", 100.0},
	}
	for _, c := range cases {
		if got := parseCoveragePercent(c.in); got != c.want {
			t.Errorf("parseCoveragePercent(%q) = %f, want %f", c.in, got, c.want)
		}
	}
}
