// SPDX-License-Identifier: MIT
// Purpose: tests for the superpowers package. All tests are hermetic —
// no network access, no shared state. The "install" path uses a local
// git repo created with `git init` in t.TempDir().
// Docs: superpowers.doc.md
package superpowers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ── Frontmatter parsing ───────────────────────────────────────────────

func TestParseFrontmatterSimple(t *testing.T) {
	body := "---\nname: brainstorming\ndescription: A skill for brainstorming\n---\n# Body\n"
	fm, ok := ParseFrontmatter(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got := fm["name"]; got != "brainstorming" {
		t.Errorf("name: got %q want %q", got, "brainstorming")
	}
	if got := fm["description"]; got != "A skill for brainstorming" {
		t.Errorf("description: got %q want %q", got, "A skill for brainstorming")
	}
}

func TestParseFrontmatterFoldedScalar(t *testing.T) {
	// `>-` is "folded with strip-chomping" — newlines inside the block
	// collapse to single spaces. The test asserts there are NO \n in
	// the resulting description.
	body := "---\n" +
		"name: test\n" +
		"description: >-\n" +
		"  This is a folded scalar.\n" +
		"  Newlines should become spaces\n" +
		"  and a final newline is stripped.\n" +
		"---\n"
	fm, ok := ParseFrontmatter(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	desc := fm["description"]
	if strings.Contains(desc, "\n") {
		t.Errorf("folded scalar should not contain newlines, got %q", desc)
	}
	// Should fold into: "This is a folded scalar. Newlines should become spaces and a final newline is stripped."
	want := "This is a folded scalar. Newlines should become spaces and a final newline is stripped."
	if desc != want {
		t.Errorf("folded scalar: got %q want %q", desc, want)
	}
}

func TestParseFrontmatterLiteralScalar(t *testing.T) {
	// `|-` is "literal with strip-chomping" — newlines are preserved
	// exactly. The test asserts newlines are present.
	body := "---\n" +
		"name: test\n" +
		"description: |-\n" +
		"  Line one.\n" +
		"  Line two.\n" +
		"  Line three.\n" +
		"---\n"
	fm, ok := ParseFrontmatter(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	desc := fm["description"]
	if !strings.Contains(desc, "\n") {
		t.Errorf("literal scalar must preserve newlines, got %q", desc)
	}
	want := "Line one.\nLine two.\nLine three."
	if desc != want {
		t.Errorf("literal scalar: got %q want %q", desc, want)
	}
}

func TestParseFrontmatterNoFrontmatter(t *testing.T) {
	body := "# No frontmatter here\nJust regular text.\n"
	fm, ok := ParseFrontmatter(body)
	if ok {
		t.Errorf("expected ok=false, got true (fm=%v)", fm)
	}
	if len(fm) != 0 {
		t.Errorf("expected empty map, got %v", fm)
	}
}

func TestParseFrontmatterQuoted(t *testing.T) {
	body := "---\n" +
		"name: \"quoted name\"\n" +
		"description: 'single quoted'\n" +
		"---\n"
	fm, ok := ParseFrontmatter(body)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got := fm["name"]; got != "quoted name" {
		t.Errorf("name: got %q want %q", got, "quoted name")
	}
	if got := fm["description"]; got != "single quoted" {
		t.Errorf("description: got %q want %q", got, "single quoted")
	}
}

// ── Overlay idempotency ───────────────────────────────────────────────

func TestOverlayIdempotent(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "superpowers", "my-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	original := "---\nname: my-skill\ndescription: test\n---\n# Body\n"
	if err := os.WriteFile(skillPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	// First call: append.
	if !AppendOverlay(skillPath) {
		t.Fatal("first AppendOverlay should return true")
	}
	firstBody, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(firstBody), OverlayMarker) {
		t.Errorf("overlay marker missing after first call")
	}
	// Second call: must be a no-op.
	if AppendOverlay(skillPath) {
		t.Error("second AppendOverlay should return false (idempotent)")
	}
	secondBody, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBody) != string(secondBody) {
		t.Error("second call must not modify the file")
	}
}

// ── List / Get / Find ─────────────────────────────────────────────────

func TestListGetFind(t *testing.T) {
	// SIN_CODE_HOME drives the *Dir() family. We lay out the canonical
	// "skills/superpowers/<name>/SKILL.md" tree under it so Get/List
	// find our fixtures.
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	root := SkillsDir() // $home/skills/superpowers
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create three skills with distinct names and descriptions.
	skills := []struct {
		dir, name, desc string
	}{
		{"alpha", "alpha", "the first letter"},
		{"beta", "beta-testing", "automated checks"},
		{"gamma", "gamma", "for use in ray tracing"},
	}
	for _, s := range skills {
		skillDir := filepath.Join(root, s.dir)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n# %s\n", s.name, s.desc, s.name)
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	all, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("List: got %d skills, want 3 (%+v)", len(all), all)
	}
	// Get by exact name.
	got, err := Get("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "the first letter" {
		t.Errorf("Get alpha: got %q", got.Description)
	}
	// Find by substring.
	hits, err := Find("beta", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Name != "beta-testing" {
		t.Errorf("Find beta: got %+v", hits)
	}
	// Find with no match.
	none, err := Find("zzz-nothing", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("Find zzz-nothing: expected 0, got %d", len(none))
	}
	// Find with max results.
	hits, err = Find("e", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 2 {
		t.Errorf("Find e max=2: got %d", len(hits))
	}
}

// ── Install with local fixture git repo (NO network) ─────────────────

func TestInstallPinsAndSyncs(t *testing.T) {
	// 1. Build a local fixture git repo that mimics obra/superpowers.
	//    It must contain at least one <skill>/SKILL.md file.
	upstream := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{
			"-C", upstream,
			"-c", "user.email=test@test",
			"-c", "user.name=test",
			"-c", "commit.gpgsign=false",
			"-c", "tag.gpgsign=false",
		}, args...)...)
		c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runGit("init", "--initial-branch=main", "--quiet")
	// Create one skill.
	skillDir := filepath.Join(upstream, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillBody := "---\nname: test-skill\ndescription: a fixture\n---\n# test-skill\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		t.Fatal(err)
	}
	// README so the root is non-empty.
	if err := os.WriteFile(filepath.Join(upstream, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")
	runGit("commit", "--quiet", "-m", "initial commit")

	// 2. Redirect SIN_CODE_HOME to an isolated temp dir, swap DefaultRepoURL
	//    for the local fixture path, and run Install.
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	origRepo := DefaultRepoURL
	DefaultRepoURL = upstream
	t.Cleanup(func() { DefaultRepoURL = origRepo })

	ctx := context.Background()
	res, err := Install(ctx, upstream, "main")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if res.SHA == "" {
		t.Error("Install: empty SHA in result")
	}
	if res.Skills < 1 {
		t.Errorf("Install: expected >=1 skill, got %d", res.Skills)
	}
	// 3. The pin file must exist and match the resolved SHA.
	pin, err := CurrentPin()
	if err != nil {
		t.Fatalf("CurrentPin: %v", err)
	}
	if pin == nil {
		t.Fatal("CurrentPin: nil")
	}
	if pin.SHA != res.SHA {
		t.Errorf("pin SHA %q != Install SHA %q", pin.SHA, res.SHA)
	}
	// 4. The SKILL.md in the CLONED checkout (not the upstream fixture)
	//    must have the overlay marker.
	clonedSkill := filepath.Join(SkillsDir(), "test-skill", "SKILL.md")
	body, err := os.ReadFile(clonedSkill)
	if err != nil {
		t.Fatalf("read cloned SKILL.md: %v", err)
	}
	if !strings.Contains(string(body), OverlayMarker) {
		t.Error("SKILL.md missing overlay marker after install")
	}
	// 5. PROMPT.md exists.
	if _, err := os.Stat(PROMPTFile()); err != nil {
		t.Errorf("PROMPT.md missing: %v", err)
	}
	// 6. List must return our fixture skill.
	all, _ := List("")
	found := false
	for _, s := range all {
		if s.Name == "test-skill" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("List did not include test-skill: %+v", all)
	}
}

// ── RegisterMCP idempotency ───────────────────────────────────────────

func TestRegisterMCPIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	mcpPath := MCPConfigPath()

	// First registration: file does not exist yet.
	if _, err := RegisterMCP(mcpPath); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	// Second registration: must be a no-op.
	if _, err := RegisterMCP(mcpPath); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("RegisterMCP must be idempotent (file changed on second call)")
	}
	// Verify JSON shape: must contain mcpServers.superpowers.command.
	var cfg map[string]any
	if err := json.Unmarshal(second, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	entry, _ := servers["superpowers"].(map[string]any)
	if entry == nil {
		t.Fatal("missing superpowers server entry")
	}
	if entry["command"] != "sin-code" {
		t.Errorf("entry.command: got %v want sin-code", entry["command"])
	}
}

// ── AGENTS.md injection ───────────────────────────────────────────────

func TestInjectAGENTS(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_HOME", dir)
	agentsPath := filepath.Join(dir, "AGENTS.md")

	// Seed a pin so AGENTSSnippet has something to render.
	pin := PinState{SHA: "deadbeef12345678", Branch: "main"}
	if err := WriteJSON(PinFile(), pin); err != nil {
		t.Fatal(err)
	}
	// Seed one skill so the snippet has at least one bullet.
	skillDir := filepath.Join(SkillsDir(), "demo-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: demo-skill\ndescription: a demo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skills, _ := List("")
	snippet := AGENTSSnippet(skills)

	// First injection: file does not exist.
	if err := InjectAGENTS(agentsPath, snippet); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "SIN-Code superpowers:begin") {
		t.Error("first injection missing begin marker")
	}

	// Second injection: must REPLACE the previous block, not duplicate it.
	updatedSnippet := snippet + "\nEXTRA_LINE_TO_FORCE_REPLACEMENT\n"
	if err := InjectAGENTS(agentsPath, updatedSnippet); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	// Must contain the new line, must not have two `begin` markers.
	if !strings.Contains(string(second), "EXTRA_LINE_TO_FORCE_REPLACEMENT") {
		t.Error("second injection did not replace block content")
	}
	if c := strings.Count(string(second), "SIN-Code superpowers:begin"); c != 1 {
		t.Errorf("expected exactly 1 begin marker, got %d", c)
	}
	if c := strings.Count(string(second), "SIN-Code superpowers:end"); c != 1 {
		t.Errorf("expected exactly 1 end marker, got %d", c)
	}
}

// ── Misc helpers / coverage ───────────────────────────────────────────

func TestHomePrefersEnv(t *testing.T) {
	want := t.TempDir()
	t.Setenv("SIN_CODE_HOME", want)
	if got := Home(); got != want {
		t.Errorf("Home: got %q want %q", got, want)
	}
}

func TestFindEmptyQuery(t *testing.T) {
	hits, err := Find("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("Find empty: expected 0, got %d", len(hits))
	}
}

func TestGetNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	_, err := Get("definitely-not-installed")
	if err == nil {
		t.Error("expected error for missing skill")
	}
}

func TestListEmptyDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	all, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("List on empty home: got %d, want 0", len(all))
	}
}

func TestRenderOverlayDeterministic(t *testing.T) {
	a := RenderOverlay(SkillOverlayContext{Path: "/x/y/SKILL.md", SkillsRoot: "/x/y", CommitHint: "abc12345", OverlayKind: SkillOverlay})
	b := RenderOverlay(SkillOverlayContext{Path: "/x/y/SKILL.md", SkillsRoot: "/x/y", CommitHint: "abc12345", OverlayKind: SkillOverlay})
	if a != b {
		t.Error("RenderOverlay not deterministic")
	}
	if !strings.Contains(a, "abc12345") {
		t.Error("overlay missing commit hint")
	}
}

func TestWritePromptRendersSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	pin := PinState{SHA: "0123456789abcdef", Branch: "main"}
	if err := WriteJSON(PinFile(), pin); err != nil {
		t.Fatal(err)
	}
	infos := []SkillInfo{{
		Name:        "demo",
		Path:        "/tmp/demo/SKILL.md",
		Description: "demo skill",
		Hash:        "deadbeef",
	}}
	body, err := WritePrompt(infos)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "demo") {
		t.Error("prompt body missing skill name")
	}
	if !strings.Contains(body, "01234567") {
		t.Error("prompt body missing pinned SHA prefix")
	}
}

// ── MCP server smoke test ─────────────────────────────────────────────

func TestMCPServerInitialize(t *testing.T) {
	// Drive a single initialize request and check the response.
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	var out strings.Builder
	srv := NewServerWithIO(in, &out, &strings.Builder{}, t.TempDir())
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatal(err)
	}
	resp := out.String()
	if !strings.Contains(resp, "\"serverInfo\"") {
		t.Errorf("initialize response missing serverInfo: %s", resp)
	}
	if !strings.Contains(resp, "sin-code-superpowers") {
		t.Errorf("initialize response missing server name: %s", resp)
	}
}

func TestMCPServerListAndCall(t *testing.T) {
	// Use a hermetic SIN_CODE_HOME with one seeded skill.
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	pin := PinState{SHA: "0123456789abcdef", Branch: "main"}
	if err := WriteJSON(PinFile(), pin); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(SkillsDir(), "hello-world")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: hello-world\ndescription: greet the user\n---\n# hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"superpowers_list_skills","arguments":{}}}` + "\n" +
			`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"superpowers_find_skill","arguments":{"query":"greet"}}}` + "\n" +
			`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"superpowers_use_skill","arguments":{"name":"hello-world"}}}` + "\n" +
			`{"jsonrpc":"2.0","id":6,"method":"unknown_method"}` + "\n",
	)
	var out, errOut strings.Builder
	srv := NewServerWithIO(in, &out, &errOut, home)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v (stderr=%s)", err, errOut.String())
	}
	resp := out.String()
	// 5 responses, one per line.
	lines := strings.Split(strings.TrimRight(resp, "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d:\n%s", len(lines), resp)
	}
	// tools/list → 3 tools
	if !strings.Contains(lines[0], "superpowers_list_skills") {
		t.Errorf("tools/list missing superpowers_list_skills: %s", lines[0])
	}
	// list_skills → contains the seeded skill
	if !strings.Contains(lines[1], "hello-world") {
		t.Errorf("list_skills did not include hello-world: %s", lines[1])
	}
	// find_skill "greet" → matches hello-world
	if !strings.Contains(lines[2], "hello-world") {
		t.Errorf("find_skill did not match: %s", lines[2])
	}
	// use_skill hello-world → returns body containing "# hello"
	if !strings.Contains(lines[3], "# hello") {
		t.Errorf("use_skill did not return body: %s", lines[3])
	}
	// unknown method → error code -32601
	if !strings.Contains(lines[4], "-32601") {
		t.Errorf("unknown method: expected -32601, got %s", lines[4])
	}
}

// ── Pin (supplemental coverage) ───────────────────────────────────────

func TestPinRequiresExistingClone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	// No clone yet — Pin must fail cleanly.
	if _, err := Pin(context.Background(), "deadbeef"); err == nil {
		t.Error("expected error when no clone exists")
	}
}

func TestPinHappyPath(t *testing.T) {
	// Build a real local clone (same trick as TestInstallPinsAndSyncs).
	upstream := t.TempDir()
	c := exec.Command("git", "-C", upstream,
		"-c", "user.email=test@test",
		"-c", "user.name=test",
		"-c", "commit.gpgsign=false",
		"init", "--initial-branch=main", "--quiet")
	c.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	skillDir := filepath.Join(upstream, "pinned-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: pinned-skill\ndescription: pinned\n---\n# pinned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds := [][]string{
		{"-C", upstream, "-c", "user.email=test@test", "-c", "user.name=test", "-c", "commit.gpgsign=false", "add", "."},
		{"-C", upstream, "-c", "user.email=test@test", "-c", "user.name=test", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// Capture SHA.
	shaCmd := exec.Command("git", "-C", upstream, "rev-parse", "HEAD")
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.TrimSpace(string(shaOut))

	// Set SIN_CODE_HOME and install first.
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	orig := DefaultRepoURL
	DefaultRepoURL = upstream
	t.Cleanup(func() { DefaultRepoURL = orig })
	if _, err := Install(context.Background(), upstream, "main"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Now Pin to the same SHA (no-op reset) and verify.
	state, err := Pin(context.Background(), sha)
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if state.SHA != sha {
		t.Errorf("Pin SHA: got %q want %q", state.SHA, sha)
	}
}

func TestPinRejectsEmptySHA(t *testing.T) {
	if _, err := Pin(context.Background(), "   "); err == nil {
		t.Error("expected error for empty/whitespace SHA")
	}
}

func TestCurrentPinMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	p, err := CurrentPin()
	if err != nil {
		t.Fatal(err)
	}
	if p != nil {
		t.Errorf("expected nil pin when file missing, got %+v", p)
	}
}

// ── Home fallback (no env) ───────────────────────────────────────────

func TestHomeFallsBack(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "")
	h := Home()
	if h == "" {
		t.Error("Home must never return empty")
	}
}

// ── PromptFromReader (defensive helper) ──────────────────────────────

func TestPromptFromReader(t *testing.T) {
	got := PromptFromReader(strings.NewReader("hello world\n"))
	if got != "hello world" {
		t.Errorf("PromptFromReader: got %q", got)
	}
	// EOF case.
	if got := PromptFromReader(strings.NewReader("")); got != "" {
		t.Errorf("PromptFromReader EOF: got %q", got)
	}
}

// ── InjectAGENTS error path ──────────────────────────────────────────

func TestInjectAGENTSRequiresPath(t *testing.T) {
	if err := InjectAGENTS("", "anything"); err == nil {
		t.Error("expected error for empty path")
	}
}
