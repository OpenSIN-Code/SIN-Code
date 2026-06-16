// SPDX-License-Identifier: MIT
// Purpose: tests for the spec compiler (issue #164). The
// round-trip test (TestRoundTrip) is the load-bearing one:
// a parsed Config must re-emit to the same bytes (modulo
// field-order, which json.MarshalIndent canonicalizes).
package compiler

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const sampleYAML = `
version: 1
project:
  name: sin-code
  type: go
verify:
  mode: strict
  predicates:
    - name: builds
      command: go build ./cmd/...
      required: true
    - name: tests
      command: go test -count=1 ./cmd/...
      required: true
hooks:
  pre-tool:
    - name: no-no-verify
      when: "tool == 'Bash' and command contains '--no-verify'"
      block: true
      message: "git commit --no-verify is not allowed"
  post-tool:
    - name: gofmt
      when: "tool in ['Edit', 'Write'] and path endswith '.go'"
      run: "gofmt -w $path"
permissions:
  allow:
    - "Bash:go test"
    - "Read:**/*.go"
  ask:
    - "Bash:rm -rf"
  deny:
    - "Bash:curl | sh"
loop:
  max_turns: 12
  max_tokens: 100000
  disable_checks: ["go vet"]
`

func TestParse_Valid(t *testing.T) {
	c, err := Parse([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 1 {
		t.Errorf("expected version 1, got %d", c.Version)
	}
	if c.Project.Name != "sin-code" {
		t.Errorf("expected project.name=sin-code, got %q", c.Project.Name)
	}
	if c.Verify.Mode != "strict" {
		t.Errorf("expected verify.mode=strict, got %q", c.Verify.Mode)
	}
	if len(c.Verify.Predicates) != 2 {
		t.Errorf("expected 2 predicates, got %d", len(c.Verify.Predicates))
	}
	if len(c.Hooks.PreTool) != 1 || len(c.Hooks.PostTool) != 1 {
		t.Errorf("expected 1 pre + 1 post hook, got %d/%d",
			len(c.Hooks.PreTool), len(c.Hooks.PostTool))
	}
	if len(c.Permissions.Allow) != 2 {
		t.Errorf("expected 2 allow, got %d", len(c.Permissions.Allow))
	}
	if c.Loop.MaxTurns != 12 {
		t.Errorf("expected loop.max_turns=12, got %d", c.Loop.MaxTurns)
	}
}

func TestParse_Empty(t *testing.T) {
	// An empty document is a parse failure (per parse.go's
	// errEmpty sentinel intent) — but the YAML parser happily
	// returns a zero Config, which Validate then rejects.
	c, err := Parse([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 0 {
		t.Errorf("expected version 0 from empty doc, got %d", c.Version)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := Parse([]byte("version: : 1\n  bad: indent"))
	if err == nil {
		t.Error("expected error on invalid YAML")
	}
}

func TestParseFile_Missing(t *testing.T) {
	_, err := ParseFile("/nonexistent/.sin-code.yml")
	if err == nil {
		t.Error("expected error on missing file")
	}
}

func TestParseFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFile)
	if err := writeFile(path, sampleYAML); err != nil {
		t.Fatal(err)
	}
	c, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Project.Name != "sin-code" {
		t.Errorf("expected project.name=sin-code, got %q", c.Project.Name)
	}
}

// ── Validate ─────────────────────────────────────────────────────────

func TestValidate_Nil(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Error("expected error on nil config")
	}
}

func TestValidate_MissingVersion(t *testing.T) {
	c := &Config{}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if ve.Path != "version" {
		t.Errorf("expected path=version, got %q", ve.Path)
	}
}

func TestValidate_WrongVersion(t *testing.T) {
	c := &Config{Version: 99}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported version 99") {
		t.Errorf("expected unsupported version, got %v", err)
	}
}

func TestValidate_InvalidProjectType(t *testing.T) {
	c := &Config{Version: 1, Project: Project{Type: "cobol"}}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "project.type") {
		t.Errorf("expected project.type in error, got %v", err)
	}
}

func TestValidate_InvalidVerifyMode(t *testing.T) {
	c := &Config{Version: 1, Verify: Verify{Mode: "extreme"}}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "verify.mode") {
		t.Errorf("expected verify.mode in error, got %v", err)
	}
}

func TestValidate_DuplicatePredicate(t *testing.T) {
	c := &Config{
		Version: 1,
		Verify: Verify{
			Predicates: []Predicate{
				{Name: "x", Command: "true"},
				{Name: "x", Command: "false"},
			},
		},
	}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate, got %v", err)
	}
}

func TestValidate_EmptyPredicateName(t *testing.T) {
	c := &Config{
		Version: 1,
		Verify: Verify{
			Predicates: []Predicate{{Command: "true"}},
		},
	}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected name in error, got %v", err)
	}
}

func TestValidate_HookWithoutBlockOrRun(t *testing.T) {
	c := &Config{
		Version: 1,
		Hooks: Hooks{
			PreTool: []Hook{{Name: "noop", When: "true"}},
		},
	}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error: hook must have block: true or run: <cmd>")
	}
}

func TestValidate_DuplicateHookAcrossGroups(t *testing.T) {
	c := &Config{
		Version: 1,
		Hooks: Hooks{
			PreTool:  []Hook{{Name: "shared", When: "x", Run: "y"}},
			PostTool: []Hook{{Name: "shared", When: "x", Run: "y"}},
		},
	}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error: hook names must be unique across groups")
	}
}

func TestValidate_BadPermissionEntry(t *testing.T) {
	c := &Config{
		Version: 1,
		Permissions: Permissions{
			Allow: []string{":"}, // empty tool, empty pattern
		},
	}
	err := Validate(c)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "permissions.allow[0]") {
		t.Errorf("expected permissions.allow[0] in error, got %v", err)
	}
}

func TestValidate_FullValidSample(t *testing.T) {
	c, err := Parse([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(c); err != nil {
		t.Errorf("sample should validate, got %v", err)
	}
}

// ── Emit + Round-trip ────────────────────────────────────────────────

func TestEmitHooks_Structure(t *testing.T) {
	c, _ := Parse([]byte(sampleYAML))
	b, err := EmitHooks(c)
	if err != nil {
		t.Fatal(err)
	}
	var out HooksOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Version != 1 {
		t.Errorf("expected version 1, got %d", out.Version)
	}
	if len(out.PreTool) != 1 || out.PreTool[0].Name != "no-no-verify" {
		t.Errorf("expected pre-tool[0]=no-no-verify, got %+v", out.PreTool)
	}
	if len(out.PostTool) != 1 || out.PostTool[0].Run != "gofmt -w $path" {
		t.Errorf("expected post-tool[0] with run=gofmt, got %+v", out.PostTool)
	}
}

func TestEmitVerify_Structure(t *testing.T) {
	c, _ := Parse([]byte(sampleYAML))
	b, err := EmitVerify(c)
	if err != nil {
		t.Fatal(err)
	}
	var out VerifyOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Mode != "strict" {
		t.Errorf("expected mode=strict, got %q", out.Mode)
	}
	if len(out.Predicates) != 2 {
		t.Errorf("expected 2 predicates, got %d", len(out.Predicates))
	}
	if out.Predicates[0].Name != "builds" || !out.Predicates[0].Required {
		t.Errorf("expected first predicate builds+required, got %+v", out.Predicates[0])
	}
}

func TestEmitPermissions_Structure(t *testing.T) {
	c, _ := Parse([]byte(sampleYAML))
	b, err := EmitPermissions(c)
	if err != nil {
		t.Fatal(err)
	}
	var out PermissionOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Allow) != 2 || out.Allow[0] != "Bash:go test" {
		t.Errorf("expected allow[0]=Bash:go test, got %+v", out.Allow)
	}
	if len(out.Ask) != 1 || out.Ask[0] != "Bash:rm -rf" {
		t.Errorf("expected ask[0]=Bash:rm -rf, got %+v", out.Ask)
	}
}

func TestEmitLoop_Structure(t *testing.T) {
	c, _ := Parse([]byte(sampleYAML))
	b, err := EmitLoop(c)
	if err != nil {
		t.Fatal(err)
	}
	var out LoopOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.MaxTurns != 12 {
		t.Errorf("expected max_turns=12, got %d", out.MaxTurns)
	}
	if out.MaxTokens != 100000 {
		t.Errorf("expected max_tokens=100000, got %d", out.MaxTokens)
	}
	if len(out.DisableChecks) != 1 || out.DisableChecks[0] != "go vet" {
		t.Errorf("expected disable_checks=[go vet], got %+v", out.DisableChecks)
	}
}

func TestRoundTrip(t *testing.T) {
	// The load-bearing test: parse → emit → parse → emit must
	// produce identical bytes (modulo field-order, which
	// json.MarshalIndent canonicalizes). This is the contract
	// the pre-commit hook relies on.
	c1, err := Parse([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(c1); err != nil {
		t.Fatal(err)
	}
	// First emit
	hooks1, _ := EmitHooks(c1)
	verify1, _ := EmitVerify(c1)
	perms1, _ := EmitPermissions(c1)
	loop1, _ := EmitLoop(c1)
	// The four outputs are JSON; the original is YAML. The
	// round-trip we can test is: emit → parse JSON → emit
	// again must equal. (YAML → JSON → YAML round-trip is
	// lossy because YAML has richer syntax than JSON, and
	// operators edit the YAML not the JSON.)
	c2 := mustParseJSON(t, hooks1, verify1, perms1, loop1)
	hooks2, _ := EmitHooks(c2)
	verify2, _ := EmitVerify(c2)
	perms2, _ := EmitPermissions(c2)
	loop2, _ := EmitLoop(c2)
	if !bytes.Equal(hooks1, hooks2) {
		t.Errorf("hooks: round-trip changed bytes\nfirst:\n%s\nsecond:\n%s", hooks1, hooks2)
	}
	if !bytes.Equal(verify1, verify2) {
		t.Errorf("verify: round-trip changed bytes")
	}
	if !bytes.Equal(perms1, perms2) {
		t.Errorf("perms: round-trip changed bytes")
	}
	if !bytes.Equal(loop1, loop2) {
		t.Errorf("loop: round-trip changed bytes")
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	// The four JSON outputs round-trip cleanly (covered by
	// TestRoundTrip). For YAML, the test is: parse the sample,
	// re-emit to YAML, re-parse, and check that the in-scope
	// fields (Verify, Hooks, Permissions, Loop) survive. Project
	// is metadata, not part of any engine output, so it is
	// intentionally not part of the round-trip.
	c1, err := Parse([]byte(sampleYAML))
	if err != nil {
		t.Fatal(err)
	}
	// Convert to JSON via the emitters.
	hooks, _ := EmitHooks(c1)
	verify, _ := EmitVerify(c1)
	perms, _ := EmitPermissions(c1)
	loop, _ := EmitLoop(c1)
	c2 := mustParseJSON(t, hooks, verify, perms, loop)
	// Re-marshal the second Config to YAML and re-parse. The
	// round-trip may differ in field order but the parsed
	// values must match.
	yamlBytes, err := yamlMarshal(c2)
	if err != nil {
		t.Fatal(err)
	}
	c3, err := Parse(yamlBytes)
	if err != nil {
		t.Fatalf("re-parse failed: %v\nyaml:\n%s", err, yamlBytes)
	}
	// Compare the in-scope fields.
	if c3.Verify.Mode != c1.Verify.Mode {
		t.Errorf("verify.mode changed: %q != %q", c3.Verify.Mode, c1.Verify.Mode)
	}
	if len(c3.Verify.Predicates) != len(c1.Verify.Predicates) {
		t.Errorf("predicate count changed: %d != %d", len(c3.Verify.Predicates), len(c1.Verify.Predicates))
	}
	if len(c3.Hooks.PreTool) != len(c1.Hooks.PreTool) {
		t.Errorf("pre-tool count changed: %d != %d", len(c3.Hooks.PreTool), len(c1.Hooks.PreTool))
	}
	if len(c3.Permissions.Allow) != len(c1.Permissions.Allow) {
		t.Errorf("allow count changed: %d != %d", len(c3.Permissions.Allow), len(c1.Permissions.Allow))
	}
	if c3.Loop.MaxTurns != c1.Loop.MaxTurns {
		t.Errorf("loop.max_turns changed: %d != %d", c3.Loop.MaxTurns, c1.Loop.MaxTurns)
	}
}

// ── InitTemplate ─────────────────────────────────────────────────────

func TestInitTemplate_Default(t *testing.T) {
	b := InitTemplate("", "")
	if len(b) == 0 {
		t.Fatal("expected non-empty template")
	}
	// Must parse and validate.
	c, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(c); err != nil {
		t.Errorf("template must validate, got %v", err)
	}
}

func TestInitTemplate_Custom(t *testing.T) {
	b := InitTemplate("myapp", "python")
	c, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if c.Project.Name != "myapp" || c.Project.Type != "python" {
		t.Errorf("expected myapp/python, got %q/%q", c.Project.Name, c.Project.Type)
	}
}

// ── Helper ────────────────────────────────────────────────────────────

func writeFile(path, content string) error {
	return writeFileImpl(path, []byte(content))
}

// mustParseJSON builds a Config from the four emitted JSON
// files. Used by the round-trip test.
func mustParseJSON(t *testing.T, hooks, verify, perms, loop []byte) *Config {
	t.Helper()
	c := &Config{Version: SchemaVersion}
	// Decode the JSON files into a fresh Config.
	var ho HooksOutput
	if err := json.Unmarshal(hooks, &ho); err != nil {
		t.Fatalf("unmarshal hooks: %v", err)
	}
	c.Hooks.PreTool = make([]Hook, len(ho.PreTool))
	for i, h := range ho.PreTool {
		c.Hooks.PreTool[i] = Hook{
			Name: h.Name, When: h.When, Block: h.Block, Run: h.Run, Message: h.Message,
		}
	}
	c.Hooks.PostTool = make([]Hook, len(ho.PostTool))
	for i, h := range ho.PostTool {
		c.Hooks.PostTool[i] = Hook{
			Name: h.Name, When: h.When, Block: h.Block, Run: h.Run, Message: h.Message,
		}
	}
	var vo VerifyOutput
	if err := json.Unmarshal(verify, &vo); err != nil {
		t.Fatalf("unmarshal verify: %v", err)
	}
	c.Verify.Mode = vo.Mode
	c.Verify.Predicates = make([]Predicate, len(vo.Predicates))
	for i, p := range vo.Predicates {
		c.Verify.Predicates[i] = Predicate{
			Name: p.Name, Command: p.Command, Required: p.Required,
		}
	}
	var po PermissionOutput
	if err := json.Unmarshal(perms, &po); err != nil {
		t.Fatalf("unmarshal perms: %v", err)
	}
	c.Permissions.Allow = po.Allow
	c.Permissions.Ask = po.Ask
	c.Permissions.Deny = po.Deny
	var lo LoopOutput
	if err := json.Unmarshal(loop, &lo); err != nil {
		t.Fatalf("unmarshal loop: %v", err)
	}
	c.Loop = Loop{
		MaxTurns:       lo.MaxTurns,
		MaxStopRejects: lo.MaxStopRejects,
		StallThreshold: lo.StallThreshold,
		MaxTokens:      lo.MaxTokens,
		VerifyMode:     lo.VerifyMode,
		DisableChecks:  lo.DisableChecks,
	}
	return c
}
