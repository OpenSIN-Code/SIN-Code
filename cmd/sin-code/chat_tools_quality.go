// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when tools are MCP-externalized
// Purpose: quality-gate tool implementations — sin_quality_gate,
// sin_mutation, sin_fuzz, sin_property. Specs and dispatch remain in
// chat_tools_extra.go.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/testgate"
)

func toolQualityGate(ctx context.Context, args map[string]any) (string, error) {
	covStr := argStr(args, "coverage")
	threshold := testConfig().TestCoverageThreshold
	if covStr != "" {
		v, err := strconv.ParseFloat(covStr, 64)
		if err != nil {
			return "", fmt.Errorf("sin_quality_gate: invalid coverage %q", covStr)
		}
		threshold = v
	}

	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "5m"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_quality_gate: invalid timeout %q", timeout)
	}

	var steps []testgate.StepKind
	if raw := argStr(args, "steps"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			steps = append(steps, testgate.StepKind(p))
		}
	}

	report := testgate.Run(ctx, testgate.Config{
		Workdir:           ".",
		Timeout:           dur,
		CoverageThreshold: threshold,
		Race:              argBool(args, "race", true),
		Steps:             steps,
	})

	jsonOut := argBool(args, "json", false)
	if jsonOut {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "QUALITY GATE %s (coverage=%s, threshold=%.1f%%)\n", report.Status, report.Coverage, report.Threshold)
	for _, s := range report.Steps {
		fmt.Fprintf(&sb, "\n[%s] %s (%s)\n", s.Status, s.Name, s.Duration)
		if s.Error != "" {
			fmt.Fprintf(&sb, "ERROR: %s\n", s.Error)
		}
		if s.Output != "" {
			out := s.Output
			if len(out) > maxToolOutput {
				out = out[:maxToolOutput] + "\n[... truncated]"
			}
			fmt.Fprint(&sb, out)
		}
	}
	return sb.String(), nil
}

func toolMutation(ctx context.Context, args map[string]any) (string, error) {
	pkg := argStr(args, "package")
	if pkg == "" {
		pkg = "./..."
	}
	thresholdStr := argStr(args, "threshold")
	threshold := testConfig().TestMutationThreshold
	if thresholdStr != "" {
		v, err := strconv.ParseFloat(thresholdStr, 64)
		if err != nil {
			return "", fmt.Errorf("sin_mutation: invalid threshold %q", thresholdStr)
		}
		threshold = v
	}
	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "10m"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_mutation: invalid timeout %q", timeout)
	}

	if _, err := exec.LookPath("gremlins"); err != nil {
		return "", fmt.Errorf("sin_mutation: gremlins not found on PATH; install from https://github.com/go-gremlins/gremlins")
	}

	cctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	cmdArgs := []string{"unleash", "--test-cpu=1", pkg}
	if threshold > 0 {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--threshold=%.2f", threshold))
	}
	cmd := exec.CommandContext(cctx, "gremlins", cmdArgs...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	passed := err == nil
	score := extractMutationScore(text)

	jsonOut := argBool(args, "json", false)
	if jsonOut {
		report := map[string]any{
			"status":    "PASS",
			"package":   pkg,
			"threshold": threshold,
			"score":     score,
			"output":    text,
		}
		if !passed {
			report["status"] = "FAIL"
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		return string(b), nil
	}

	status := "PASS"
	if !passed {
		status = "FAIL"
	}
	return fmt.Sprintf("MUTATION %s (score=%.2f%% threshold=%.2f%%)\n%s", status, score, threshold, text), nil
}

func extractMutationScore(out string) float64 {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Mutation Score") || strings.Contains(line, "score") {
			for _, field := range strings.Fields(line) {
				field = strings.TrimSuffix(strings.TrimSuffix(field, "%"), ".")
				if v, err := strconv.ParseFloat(field, 64); err == nil && v >= 0 && v <= 100 {
					return v
				}
			}
		}
	}
	return 0
}

func toolFuzz(ctx context.Context, args map[string]any) (string, error) {
	pkg := argStr(args, "package")
	if pkg == "" {
		pkg = "./..."
	}
	duration := argStr(args, "duration")
	if duration == "" {
		duration = "30s"
	}
	if _, err := time.ParseDuration(duration); err != nil {
		return "", fmt.Errorf("sin_fuzz: invalid duration %q", duration)
	}
	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "5m"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_fuzz: invalid timeout %q", timeout)
	}

	cctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	cmd := exec.CommandContext(cctx, "go", "test", pkg, "-run=^$", fmt.Sprintf("-fuzz=Fuzz.*"), fmt.Sprintf("-fuzztime=%s", duration))
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	passed := err == nil

	jsonOut := argBool(args, "json", false)
	if jsonOut {
		report := map[string]any{
			"status":   "PASS",
			"package":  pkg,
			"duration": duration,
			"output":   text,
		}
		if !passed {
			report["status"] = "FAIL"
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		return string(b), nil
	}

	status := "PASS"
	if !passed {
		status = "FAIL"
	}
	return fmt.Sprintf("FUZZ %s\n%s", status, text), nil
}

func toolProperty(ctx context.Context, args map[string]any) (string, error) {
	pkg := argStr(args, "package")
	if pkg == "" {
		pkg = "./..."
	}
	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "5m"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_property: invalid timeout %q", timeout)
	}

	cctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	cmd := exec.CommandContext(cctx, "go", "test", pkg, "-run=TestProperty|TestRapid|TestQuick", "-count=1")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	passed := err == nil

	jsonOut := argBool(args, "json", false)
	if jsonOut {
		report := map[string]any{
			"status":  "PASS",
			"package": pkg,
			"output":  text,
		}
		if !passed {
			report["status"] = "FAIL"
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		return string(b), nil
	}

	status := "PASS"
	if !passed {
		status = "FAIL"
	}
	return fmt.Sprintf("PROPERTY %s\n%s", status, text), nil
}
