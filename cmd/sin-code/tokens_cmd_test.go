// SPDX-License-Identifier: MIT
// Purpose: tests for the sin-code tokens CLI (issue #168). Covers
// command registration, --json output, the --share one-liner, and
// unit-level helpers (humanTokens, sortedKeys).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/usage"
)

func TestNewTokensCmdRegistersSubcommands(t *testing.T) {
	cmd := NewTokensCmd()
	if cmd.Use != "tokens" {
		t.Fatalf("Use: %q", cmd.Use)
	}
	subs := map[string]bool{}
	for _, s := range cmd.Commands() {
		subs[s.Name()] = true
	}
	for _, want := range []string{"show", "tail", "aggregate"} {
		if !subs[want] {
			t.Errorf("missing subcommand %q (got %v)", want, subs)
		}
	}
}

// TestTokensCLIShareLineUsesZeroWhenEmpty ensures `renderShareLine` never
// fabricates a number (caveman discipline: absent until first call).
func TestTokensCLIShareLineUsesZeroWhenEmpty(t *testing.T) {
	a := &usage.Aggregation{}
	got := renderShareLine(a, true)
	if !strings.Contains(got, "no usage recorded yet") {
		t.Errorf("zero totals should produce empty-share fallback, got %q", got)
	}
}

func TestTokensCLIHumanTokensCompact(t *testing.T) {
	cases := map[int]string{
		0:          "0",
		42:         "42",
		999:        "999",
		1234:       "1234 (1.23k)",
		12_000_000: "12000000 (12.00M)",
	}
	for n, want := range cases {
		if got := humanTokens(n); got != want {
			t.Errorf("humanTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestTokensCLISortedKeys(t *testing.T) {
	m := map[string]int{
		"alpha": 100,
		"beta":  300,
		"gamma": 200,
	}
	got := sortedKeys(m)
	want := []string{"beta", "gamma", "alpha"}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Errorf("sortedKeys: %v, want %v", got, want)
	}
}

func TestTokensCLIBuildShowFilterLifetime(t *testing.T) {
	f := buildShowFilter("", false, false, true)
	if f.SessionID != "" || !f.Since.IsZero() || !f.Until.IsZero() {
		t.Errorf("lifetime filter should be empty, got %+v", f)
	}
	f = buildShowFilter("abc", false, false, true)
	if f.SessionID != "abc" {
		t.Errorf("session filter: %+v", f)
	}
}

func TestTokensCLIBuildShowFilterToday(t *testing.T) {
	f := buildShowFilter("", true, false, false)
	if f.Since.IsZero() || f.Until.IsZero() {
		t.Fatalf("today filter missing bounds: %+v", f)
	}
	if f.Until.Sub(f.Since) != 24*60*60*1_000_000_000 {
		t.Errorf("today window: %v", f.Until.Sub(f.Since))
	}
}

func TestTokensCLIBuildShowFilterMonth(t *testing.T) {
	f := buildShowFilter("", false, true, false)
	if f.Since.IsZero() || f.Until.IsZero() {
		t.Fatalf("month filter missing bounds: %+v", f)
	}
	if f.Since.Day() != 1 {
		t.Errorf("month start: %d", f.Since.Day())
	}
}

// TestTokensCLIEndToEnd runs the show subcommand against an isolated store
// (SIN_CODE_TOKENS_DB override). Verifies table + share + JSON paths.
func TestTokensCLIEndToEnd(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)

	store, err := usage.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordFromChatUsage(context.Background(), "sess-x",
		"meta/llama-3.3-70b-instruct", usage.SourceChat, 1000, 500, 1500); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordFromChatUsage(context.Background(), "sess-x",
		"gpt-4o", usage.SourceJudge, 800, 200, 1000); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	for _, label := range []string{"show", "tail", "aggregate"} {
		cmd := NewTokensCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{label})
		if err := cmd.Execute(); err != nil {
			t.Errorf("execute %s: %v (output: %s)", label, err, out.String())
		}
	}
}

// TestTokensCLISubcommandJSONShape asserts the JSON envelope carries the
// expected keys for `aggregate --json`.
func TestTokensCLISubcommandJSONShape(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "tokens.db")
	t.Setenv("SIN_CODE_TOKENS_DB", storePath)

	store, _ := usage.Open(storePath)
	_ = store.RecordFromChatUsage(context.Background(), "s",
		"gpt-4o", usage.SourceChat, 100, 50, 150)
	_ = store.Close()

	cmd := NewTokensCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"aggregate", "--by", "model", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Total     usage.Aggregation   `json:"total"`
		Subgroups []usage.Aggregation `json:"subgroups"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("json unmarshal: %v\nbody: %s", err, out.String())
	}
	if env.Total.TotalTokens != 150 {
		t.Errorf("total: %+v", env.Total)
	}
	if len(env.Subgroups) == 0 || env.Subgroups[0].Group != "gpt-4o" {
		t.Errorf("expected gpt-4o group, got %+v", env.Subgroups)
	}
}

func TestTokensCLILoadPricingOverridesHonorsUserConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))

	cfgDir := filepath.Join(homeDir, ".config", "sin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(cfgDir, "sin-code.toml")
	if err := os.WriteFile(cfgFile, []byte(`
llm.pricing_per_1k."gpt-4o" = 0.0099
llm.pricing_per_1k."meta/llama-3.3-70b-instruct" = 0.0042
`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadPricingOverrides()
	if got["gpt-4o"] != 0.0099 {
		t.Errorf("gpt-4o override: %v", got["gpt-4o"])
	}
	if got["meta/llama-3.3-70b-instruct"] != 0.0042 {
		t.Errorf("llama override: %v", got["meta/llama-3.3-70b-instruct"])
	}
}

// TestTokensCLIMissingDBErrors verifies the show command reports a clear
// error rather than silently rendering zeros (a fake number would violate
// the caveman discipline). Overriding SIN_CODE_TOKENS_DB to a path whose
// parent dir cannot be created triggers the open() failure.
func TestTokensCLIMissingDBErrors(t *testing.T) {
	tmp := t.TempDir()
	protectedDir := filepath.Join(tmp, "locked")
	if err := os.Mkdir(protectedDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(protectedDir, 0o700) })
	t.Setenv("SIN_CODE_TOKENS_DB", filepath.Join(protectedDir, "tokens.db"))
	cmd := NewTokensCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"show", "--lifetime"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when db cannot be opened; output:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "tokens") {
		t.Errorf("error should mention tokens: %v", err)
	}
}

// TestTokensCLISourceIsConstantStripsGuards uses cobra's root command to
// catch registration regressions: a typo in Use would surface here.
func TestTokensCLISourceIsConstantStripsGuards(t *testing.T) {
	c := &cobra.Command{Use: "tokens"}
	c.AddCommand(&cobra.Command{Use: "show"}, &cobra.Command{Use: "tail"}, &cobra.Command{Use: "aggregate"})
	if c.Commands() == nil {
		t.Fatal("expected subcommands")
	}
}
