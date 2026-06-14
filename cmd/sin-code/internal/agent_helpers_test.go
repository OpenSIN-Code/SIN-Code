// SPDX-License-Identifier: MIT
// Purpose: tests for the agent CLI helpers (agent-dir, sanitize, merge).
package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"coder":    "coder",
		"my-agent": "my-agent",
		"a_b_c":    "a_b_c",
		"a/b":      "ab",
		"a b":      "ab",
		"../etc":   "etc",
		"":         "",
		"x.y.z":    "xyz",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgentDirValid(t *testing.T) {
	dir, err := agentDir("coder")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path: %s", dir)
	}
}

func TestAgentDirRejectsTraversal(t *testing.T) {
	if _, err := agentDir("../foo"); err == nil {
		t.Error("expected error for traversal name")
	}
	if _, err := agentDir("a/b"); err == nil {
		t.Error("expected error for slash")
	}
}

func TestMergeAgentConfigOverride(t *testing.T) {
	base := orchestrator.AgentConfig{Name: "x", Model: "m1", MaxTokens: 1000}
	override := orchestrator.AgentConfig{Model: "m2", MaxTokens: 2000, Provider: "openai"}
	merged := mergeAgentConfig(base, override)
	if merged.Model != "m2" {
		t.Errorf("model: %s", merged.Model)
	}
	if merged.MaxTokens != 2000 {
		t.Errorf("tokens: %d", merged.MaxTokens)
	}
	if merged.Provider != "openai" {
		t.Errorf("provider: %s", merged.Provider)
	}
	if merged.Name != "x" {
		t.Errorf("name overwritten: %s", merged.Name)
	}
}

func TestMergeAgentConfigAllFields(t *testing.T) {
	base := orchestrator.AgentConfig{Name: "x", Model: "m1", MaxTokens: 1000}
	override := orchestrator.AgentConfig{
		Description:   "desc",
		Type:          "type",
		Provider:      "p",
		BaseURL:       "http://x",
		Model:         "m2",
		MaxTokens:     2000,
		Temperature:   0.5,
		SystemFile:    "sys",
		MaxContext:    4000,
		ToolsAllow:    []string{"a"},
		ToolsDeny:     []string{"b"},
		MemoryNS:      "ns",
		RetentionDays: 7,
	}
	merged := mergeAgentConfig(base, override)
	if merged.Description != "desc" || merged.Type != "type" || merged.Provider != "p" ||
		merged.BaseURL != "http://x" || merged.Model != "m2" || merged.MaxTokens != 2000 ||
		merged.Temperature != 0.5 || merged.SystemFile != "sys" || merged.MaxContext != 4000 ||
		merged.MemoryNS != "ns" || merged.RetentionDays != 7 {
		t.Errorf("unexpected merge result: %+v", merged)
	}
	if len(merged.ToolsAllow) != 1 || merged.ToolsAllow[0] != "a" {
		t.Errorf("tools_allow: %v", merged.ToolsAllow)
	}
	if len(merged.ToolsDeny) != 1 || merged.ToolsDeny[0] != "b" {
		t.Errorf("tools_deny: %v", merged.ToolsDeny)
	}
	if merged.Name != "x" {
		t.Errorf("name overwritten: %s", merged.Name)
	}
}

func TestMergeAgentConfigEmptyOverride(t *testing.T) {
	base := orchestrator.AgentConfig{Name: "x", Model: "m1", MaxTokens: 1000}
	merged := mergeAgentConfig(base, orchestrator.AgentConfig{})
	if merged.Name != "x" || merged.Model != "m1" || merged.MaxTokens != 1000 {
		t.Errorf("base should be unchanged: %+v", merged)
	}
}

func TestLoadEffectiveAgentDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	cfg, source, err := loadEffectiveAgent("coder")
	if err != nil {
		t.Fatal(err)
	}
	if source != "default" {
		t.Errorf("source: %s", source)
	}
	if cfg.Name != "coder" {
		t.Errorf("name: %s", cfg.Name)
	}
}

func TestLoadEffectiveAgentUserOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "sin-code", "agents", "coder")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "agent.toml"),
		[]byte("name = \"coder\"\nmodel = \"my-custom-model\"\n"), 0o644)

	cfg, source, err := loadEffectiveAgent("coder")
	if err != nil {
		t.Fatal(err)
	}
	if source != "user (overrides default)" {
		t.Errorf("source: %s", source)
	}
	if cfg.Model != "my-custom-model" {
		t.Errorf("model override: %s", cfg.Model)
	}
}

func TestLoadEffectiveAgentNotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	_, _, err := loadEffectiveAgent("nonexistent-agent-zzz")
	if err == nil {
		t.Error("expected error for unknown agent")
	}
}

func TestLoadAllEffectiveAgents(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	agents, err := loadAllEffectiveAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) < 5 {
		t.Errorf("expected at least 5 defaults, got %d", len(agents))
	}
	for i := 1; i < len(agents); i++ {
		if agents[i-1].Name > agents[i].Name {
			t.Error("agents not sorted by name")
		}
	}
}

func TestOrDash(t *testing.T) {
	if orDash("") != "-" {
		t.Error("empty should be -")
	}
	if orDash("hi") != "hi" {
		t.Error("non-empty should be unchanged")
	}
}

func TestSplitKV(t *testing.T) {
	k, v, ok := splitKV("model=gpt-4o")
	if !ok || k != "model" || v != "gpt-4o" {
		t.Errorf("got %q %q %v", k, v, ok)
	}
	k, v, ok = splitKV("name=Custom Agent")
	if !ok || k != "name" || v != "Custom Agent" {
		t.Errorf("got %q %q %v", k, v, ok)
	}
	if _, _, ok := splitKV("nokeyvalue"); ok {
		t.Error("should fail without =")
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"  a , b ,c  ", []string{"a", "b", "c"}},
		{",,a,,", []string{"a"}},
	}
	for _, c := range cases {
		got := splitCSV(c.in)
		if !equalStrSlices(got, c.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestApplyAgentEditsCreates(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	if err := applyAgentEdits("newagent", []string{"model=gpt-4o", "provider=openai"}); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tmp, "sin-code", "agents", "newagent", "agent.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatal(err)
	}
	var cfg orchestrator.AgentConfig
	_, err := toml.DecodeFile(cfgPath, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "gpt-4o" {
		t.Errorf("model: %s", cfg.Model)
	}
	if cfg.Provider != "openai" {
		t.Errorf("provider: %s", cfg.Provider)
	}
}

func TestApplyAgentEditsUpdates(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "sin-code", "agents", "unique-test-agent-zzz")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "agent.toml"),
		[]byte("name = \"unique-test-agent-zzz\"\nmodel = \"old\"\n"), 0o644)
	if err := applyAgentEdits("unique-test-agent-zzz", []string{"model=new"}); err != nil {
		t.Fatal(err)
	}
	var cfg orchestrator.AgentConfig
	_, _ = toml.DecodeFile(filepath.Join(dir, "agent.toml"), &cfg)
	if cfg.Model != "new" {
		t.Errorf("model: %s", cfg.Model)
	}
}

func TestApplyAgentEditsInvalidField(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	err := applyAgentEdits("unique-agent-x", []string{"nonexistent_field=v"})
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

func TestApplyAgentEditsInvalidMaxTokens(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	err := applyAgentEdits("unique-agent-y", []string{"max_tokens=notanumber"})
	if err == nil {
		t.Error("expected error for non-numeric max_tokens")
	}
}

func TestApplyAgentEdits_DecodeError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "sin-code", "agents", "decode-agent")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("not valid toml = = ="), 0o644)

	err := applyAgentEdits("decode-agent", []string{"model=gpt-4o"})
	if err == nil {
		t.Error("expected error for invalid toml")
	}
}

func TestSetAgentField(t *testing.T) {
	cfg := &orchestrator.AgentConfig{}
	if err := setAgentField(cfg, "name", "foo"); err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "foo" {
		t.Errorf("name: %s", cfg.Name)
	}
	if err := setAgentField(cfg, "temperature", "0.5"); err != nil {
		t.Fatal(err)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("temp: %f", cfg.Temperature)
	}
	if err := setAgentField(cfg, "tools_allow", "a,b,c"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.ToolsAllow) != 3 {
		t.Errorf("tools: %v", cfg.ToolsAllow)
	}
}

func TestBuildAgentSeedForDefault(t *testing.T) {
	seed := buildAgentSeed("coder")
	if seed == "" {
		t.Error("empty seed for default agent")
	}
	if !strContains(seed, "name") {
		t.Errorf("seed missing name: %s", seed)
	}
}

func TestBuildAgentSeedForNew(t *testing.T) {
	seed := buildAgentSeed("nonexistent-zzz")
	if seed == "" {
		t.Error("empty seed for new agent")
	}
}

func TestLoadEffectiveAgent_InvalidName(t *testing.T) {
	_, _, err := loadEffectiveAgent("invalid/name")
	if err == nil {
		t.Fatal("expected error for invalid agent name")
	}
}

func TestLoadEffectiveAgent_DecodeError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", dir)
	agentDirPath := filepath.Join(dir, "sin-code", "agents", "coder")
	if err := os.MkdirAll(agentDirPath, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	cfgPath := filepath.Join(agentDirPath, "agent.toml")
	if err := os.WriteFile(cfgPath, []byte("not valid toml = ["), 0o644); err != nil {
		t.Fatalf("write invalid toml: %v", err)
	}

	_, _, err := loadEffectiveAgent("coder")
	if err == nil {
		t.Fatal("expected error for invalid toml")
	}
}

func TestLoadEffectiveAgent_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", dir)

	_, _, err := loadEffectiveAgent("nonexistent-agent-xyz")
	if err == nil {
		t.Fatal("expected error for agent not found")
	}
}

func strContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRunDoctorNoAgents(t *testing.T) {
	r := runDoctor(nil, true)
	if len(r) != 0 {
		t.Errorf("expected 0, got %d", len(r))
	}
}

func TestRunDoctorUnknownProvider(t *testing.T) {
	r := runDoctor([]orchestrator.AgentConfig{
		{Name: "x", Provider: "nonexistent-provider-zzz"},
	}, true)
	if len(r) != 1 {
		t.Fatal("expected 1 report")
	}
	if r[0].OK {
		t.Error("expected failure for unknown provider")
	}
}

func TestRunDoctorMissingKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("SIN_NIM_API_KEY", "")
	t.Setenv("SIN_LLM_API_KEY", "")
	r := runDoctor([]orchestrator.AgentConfig{
		{Name: "x", Provider: "openai", Model: "gpt-4o"},
	}, true)
	if r[0].OK {
		t.Error("expected failure for missing key")
	}
}

func TestRunDoctorOfflineValid(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	r := runDoctor([]orchestrator.AgentConfig{
		{Name: "x", Provider: "openai", Model: "gpt-4o"},
	}, true)
	if !r[0].OK {
		t.Errorf("expected OK in offline mode, got issues: %v", r[0].Issues)
	}
}

func TestPrintDoctor(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w
	printDoctor([]DoctorReport{
		{Agent: "good", OK: true, Info: map[string]interface{}{"provider": "nim"}},
		{Agent: "bad", OK: false, Issues: []string{"missing key"}, Info: map[string]interface{}{}},
	})
	w.Close()
	os.Stdout = old
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strContains(out, "good") || !strContains(out, "bad") {
		t.Errorf("output missing agents: %s", out)
	}
	if !strContains(out, "missing key") {
		t.Errorf("output missing issues: %s", out)
	}
}

func TestStringInList(t *testing.T) {
	if !stringInList([]string{"a", "b", "c"}, "b") {
		t.Error("should find b")
	}
	if stringInList([]string{"a", "b"}, "z") {
		t.Error("should not find z")
	}
}

func TestLoadAllEffectiveAgents_WithUserOverride(t *testing.T) {
	// On macOS, os.UserConfigDir resolves to $HOME/Library/Application Support.
	// We set HOME to a temp dir so user agents are loaded from there.
	oldHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	defer func() { os.Setenv("HOME", oldHome) }()

	agentsDir := filepath.Join(tmpHome, "Library", "Application Support", "sin-code", "agents")
	os.MkdirAll(filepath.Join(agentsDir, "coder"), 0o755)
	os.WriteFile(filepath.Join(agentsDir, "coder", "agent.toml"), []byte("model = \"custom\"\n"), 0o644)

	agents, err := loadAllEffectiveAgents()
	if err != nil {
		t.Fatalf("loadAllEffectiveAgents failed: %v", err)
	}
	found := false
	for _, a := range agents {
		if a.Name == "coder" && a.Model == "custom" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected coder with custom model in agents, got %v", agents)
	}
}

func TestLoadEffectiveAgent_UserOverride(t *testing.T) {
	old := os.Getenv("SIN_CODE_CONFIG_DIR")
	os.Setenv("SIN_CODE_CONFIG_DIR", t.TempDir())
	defer func() { os.Setenv("SIN_CODE_CONFIG_DIR", old) }()

	dir, _ := agentDir("coder")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent.toml"), []byte("model = \"override\"\n"), 0o644)

	cfg, source, err := loadEffectiveAgent("coder")
	if err != nil {
		t.Fatalf("loadEffectiveAgent failed: %v", err)
	}
	if cfg.Model != "override" {
		t.Errorf("expected model override, got %q", cfg.Model)
	}
	if source != "user (overrides default)" {
		t.Errorf("expected user override source, got %q", source)
	}
}

func TestLoadEffectiveAgent_Default(t *testing.T) {
	old := os.Getenv("SIN_CODE_CONFIG_DIR")
	os.Setenv("SIN_CODE_CONFIG_DIR", t.TempDir())
	defer func() { os.Setenv("SIN_CODE_CONFIG_DIR", old) }()

	cfg, source, err := loadEffectiveAgent("coder")
	if err != nil {
		t.Fatalf("loadEffectiveAgent failed: %v", err)
	}
	if cfg.Name != "coder" {
		t.Errorf("expected coder, got %q", cfg.Name)
	}
	if source != "default" {
		t.Errorf("expected default source, got %q", source)
	}
}
