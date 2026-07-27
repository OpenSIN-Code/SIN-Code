// SPDX-License-Identifier: MIT
// Purpose: tests for the built-in ecosystem registry in mcpclient.
package mcpclient

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultServersWebsearchUsesLocalBinaryWhenPresent(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "web_search_bundle", "sin-websearch")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SIN_SKILLS_DIR", dir)
	for _, s := range DefaultServers() {
		if s.Name != "websearch" {
			continue
		}
		if s.Command != bin {
			t.Fatalf("websearch command should use local binary %q, got %q", bin, s.Command)
		}
		if len(s.Args) != 1 || s.Args[0] != "serve" {
			t.Fatalf("websearch args should be [serve], got %v", s.Args)
		}
		return
	}
	t.Fatal("websearch server not found in DefaultServers")
}

func TestDefaultServersWebsearchFallsBackToPathBinary(t *testing.T) {
	// Use a HOME that has no sin-code skills checkout so the default skills
	// dir check fails and the registry falls back to the binary on PATH.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIN_SKILLS_DIR", "")
	for _, s := range DefaultServers() {
		if s.Name != "websearch" {
			continue
		}
		if s.Command != "sin-websearch" {
			t.Fatalf("websearch command should fall back to %q, got %q", "sin-websearch", s.Command)
		}
		return
	}
	t.Fatal("websearch server not found in DefaultServers")
}

func TestDefaultServersWebsearchUsesDefaultSkillsDir(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "share", "sin-code", "skills", "web_search_bundle", "sin-websearch")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SIN_SKILLS_DIR", "")
	t.Setenv("HOME", home)
	for _, s := range DefaultServers() {
		if s.Name != "websearch" {
			continue
		}
		if s.Command != bin {
			t.Fatalf("websearch command should use default skills dir binary %q, got %q", bin, s.Command)
		}
		return
	}
	t.Fatal("websearch server not found in DefaultServers")
}

func TestDefaultServersFallbacksWhenNoSkillsDir(t *testing.T) {
	// Force UserHomeDir to fail so skillsDirOrDefault returns empty and both
	// py and goNative fall back to the binary-on-PATH command.
	orig := userHomeDirHook
	userHomeDirHook = func() (string, error) { return "", errors.New("no home") }
	t.Cleanup(func() { userHomeDirHook = orig })
	origLP := lookPathHook
	lookPathHook = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPathHook = origLP })
	t.Setenv("SIN_SKILLS_DIR", "")

	for _, s := range DefaultServers() {
		switch s.Name {
		case "websearch":
			if s.Command != "sin-websearch" {
				t.Fatalf("websearch fallback command mismatch: %q", s.Command)
			}
		case "scheduler":
			if s.Command != "sin-scheduler" {
				t.Fatalf("scheduler fallback command mismatch: %q", s.Command)
			}
		}
	}
}

func TestDefaultServersPythonSkillWithSkillsDir(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPathHook = orig })

	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)

	// Create the root mcp_server.py so the registry selects the local script.
	script := filepath.Join(dir, "SIN-Code-Scheduler-Skill", "mcp_server.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, s := range DefaultServers() {
		if s.Name != "scheduler" {
			continue
		}
		if s.Command != "python3" || len(s.Args) != 1 || s.Args[0] != script {
			t.Fatalf("scheduler should use python3 + skills dir, got %+v", s)
		}
		return
	}
	t.Fatal("scheduler server not found in DefaultServers")
}

func TestDefaultServersPythonSkillDiscoversNestedMcpServer(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPathHook = orig })

	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "SIN-Code-Scheduler-Skill", "src", "sin_scheduler")
	script := filepath.Join(pkgDir, "mcp_server.py")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_SKILLS_DIR", dir)

	for _, s := range DefaultServers() {
		if s.Name != "scheduler" {
			continue
		}
		if s.Command != "python3" || len(s.Args) != 2 || s.Args[0] != "-m" || s.Args[1] != "sin_scheduler.mcp_server" {
			t.Fatalf("scheduler should run as module, got %+v", s)
		}
		wantDir := filepath.Join(dir, "SIN-Code-Scheduler-Skill")
		if s.Dir != wantDir {
			t.Fatalf("scheduler dir mismatch: got %q, want %q", s.Dir, wantDir)
		}
		if s.Env == nil || s.Env["PYTHONPATH"] != filepath.Join(wantDir, "src") {
			t.Fatalf("scheduler PYTHONPATH mismatch: got %v", s.Env)
		}
		return
	}
	t.Fatal("scheduler server not found in DefaultServers")
}

func TestDefaultServersPythonSkillFallsBackToPath(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(name string) (string, error) {
		if name == "sin-scheduler" {
			return "/opt/bin/sin-scheduler", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPathHook = orig })

	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)

	for _, s := range DefaultServers() {
		if s.Name != "scheduler" {
			continue
		}
		if s.Command != "/opt/bin/sin-scheduler" {
			t.Fatalf("scheduler should fall back to PATH binary, got %q", s.Command)
		}
		return
	}
	t.Fatal("scheduler server not found in DefaultServers")
}

func TestDefaultServersPythonSkillPrefersRepoOverPath(t *testing.T) {
	// When a local checkout entrypoint exists AND a console script is on PATH,
	// the repo-cloned skill must be the preferred source.
	orig := lookPathHook
	lookPathHook = func(name string) (string, error) {
		if name == "sin-scheduler" {
			return "/opt/bin/sin-scheduler", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPathHook = orig })

	dir := t.TempDir()
	script := filepath.Join(dir, "SIN-Code-Scheduler-Skill", "mcp_server.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_SKILLS_DIR", dir)

	for _, s := range DefaultServers() {
		if s.Name != "scheduler" {
			continue
		}
		if s.Command != "python3" || len(s.Args) != 1 || s.Args[0] != script {
			t.Fatalf("scheduler should prefer local repo script, got %+v", s)
		}
		return
	}
	t.Fatal("scheduler server not found in DefaultServers")
}

func TestShortNameDefaultReturnsRepo(t *testing.T) {
	if got := shortName("Unknown-Repo-Name"); got != "Unknown-Repo-Name" {
		t.Fatalf("expected repo name unchanged, got %q", got)
	}
}

func TestShortNameAnalyseCanonicalCasing(t *testing.T) {
	for _, repo := range []string{"sin-analyse-suite", "SIN-Analyse-Suite"} {
		if got := shortName(repo); got != "analyse" {
			t.Fatalf("shortName(%q) = %q, want %q", repo, got, "analyse")
		}
	}
}

func TestDefaultServersAnalysePrefersLocalCheckout(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "SIN-Analyse-Suite")
	bin := filepath.Join(repoDir, "sin-analyse")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SIN_SKILLS_DIR", dir)
	for _, s := range DefaultServers() {
		if s.Name != "analyse" {
			continue
		}
		if s.Command != bin {
			t.Fatalf("analyse command should use local binary %q, got %q", bin, s.Command)
		}
		if len(s.Args) != 1 || s.Args[0] != "serve" {
			t.Fatalf("analyse args should be [serve], got %v", s.Args)
		}
		return
	}
	t.Fatal("analyse server not found in DefaultServers")
}

func TestDefaultServersContextBridgeUsesCliEntrypoint(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPathHook = orig })

	dir := t.TempDir()
	repoDir := filepath.Join(dir, "SIN-Code-Context-Bridge-Skill")
	script := filepath.Join(repoDir, "scripts", "sin_context_bridge.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also create lib/mcp_server.py to ensure the CLI script is preferred over
	// the module entrypoint (which only defines the server without running it).
	libScript := filepath.Join(repoDir, "lib", "mcp_server.py")
	if err := os.MkdirAll(filepath.Dir(libScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libScript, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(libScript), "__init__.py"), []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_SKILLS_DIR", dir)

	for _, s := range DefaultServers() {
		if s.Name != "contextbridge" {
			continue
		}
		if s.Command != "python3" {
			t.Fatalf("contextbridge command should be python3, got %q", s.Command)
		}
		if len(s.Args) != 2 || s.Args[0] != script || s.Args[1] != "serve" {
			t.Fatalf("contextbridge should use CLI script with serve arg, got %+v", s.Args)
		}
		if s.Dir != repoDir {
			t.Fatalf("contextbridge dir mismatch: got %q, want %q", s.Dir, repoDir)
		}
		if s.Env == nil || s.Env["PYTHONPATH"] != repoDir {
			t.Fatalf("contextbridge PYTHONPATH mismatch: got %v", s.Env)
		}
		return
	}
	t.Fatal("contextbridge server not found in DefaultServers")
}

func TestDefaultServersSimoneUsesLocalCli(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPathHook = orig })

	dir := t.TempDir()
	repoDir := filepath.Join(dir, "Simone-MCP")
	cli := filepath.Join(repoDir, "src", "cli.py")
	if err := os.MkdirAll(filepath.Dir(cli), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cli, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_SKILLS_DIR", dir)

	for _, s := range DefaultServers() {
		if s.Name != "simone" {
			continue
		}
		if s.Command != "python3" {
			t.Fatalf("simone command should be python3, got %q", s.Command)
		}
		if len(s.Args) != 2 || s.Args[0] != cli || s.Args[1] != "serve-mcp" {
			t.Fatalf("simone should use src/cli.py serve-mcp, got %+v", s.Args)
		}
		if s.Dir != repoDir {
			t.Fatalf("simone dir mismatch: got %q, want %q", s.Dir, repoDir)
		}
		return
	}
	t.Fatal("simone server not found in DefaultServers")
}

func TestDefaultServersSimoneFallsBackToPath(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(name string) (string, error) {
		if name == "simone-cli" {
			return "/opt/bin/simone-cli", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { lookPathHook = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	for _, s := range DefaultServers() {
		if s.Name != "simone" {
			continue
		}
		if s.Command != "/opt/bin/simone-cli" {
			t.Fatalf("simone command should fall back to PATH binary, got %q", s.Command)
		}
		if len(s.Args) != 1 || s.Args[0] != "serve-mcp" {
			t.Fatalf("simone args should be [serve-mcp], got %v", s.Args)
		}
		return
	}
	t.Fatal("simone server not found in DefaultServers")
}

func TestDefaultServersSymfonyLensUsesLocalModule(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPathHook = orig })

	dir := t.TempDir()
	repoDir := filepath.Join(dir, "SIN-Code-Symfony-Lens")
	pkgDir := filepath.Join(repoDir, "symfony_lens")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "server.py"), []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_SKILLS_DIR", dir)

	for _, s := range DefaultServers() {
		if s.Name != "symfonylens" {
			continue
		}
		if s.Command != "python3" {
			t.Fatalf("symfonylens command should be python3, got %q", s.Command)
		}
		if len(s.Args) != 2 || s.Args[0] != "-m" || s.Args[1] != "symfony_lens.server" {
			t.Fatalf("symfonylens should run as module, got %+v", s.Args)
		}
		if s.Dir != repoDir {
			t.Fatalf("symfonylens dir mismatch: got %q, want %q", s.Dir, repoDir)
		}
		if s.Env == nil || s.Env["PYTHONPATH"] != repoDir {
			t.Fatalf("symfonylens PYTHONPATH mismatch: got %v", s.Env)
		}
		return
	}
	t.Fatal("symfonylens server not found in DefaultServers")
}

func TestDefaultServersSymfonyLensFallsBackToPath(t *testing.T) {
	orig := lookPathHook
	lookPathHook = func(name string) (string, error) {
		if name == "symfony-lens" {
			return "/opt/bin/symfony-lens", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { lookPathHook = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	for _, s := range DefaultServers() {
		if s.Name != "symfonylens" {
			continue
		}
		if s.Command != "/opt/bin/symfony-lens" {
			t.Fatalf("symfonylens command should fall back to PATH binary, got %q", s.Command)
		}
		if len(s.Args) != 0 {
			t.Fatalf("symfonylens args should be empty, got %v", s.Args)
		}
		return
	}
	t.Fatal("symfonylens server not found in DefaultServers")
}

func TestDefaultServersBundledConsolidatedTools(t *testing.T) {
	want := map[string]string{
		"marketplace": "sin_code_bundle.tools.marketplace.server",
		"mcpbuilder":  "sin_code_bundle.tools.mcp_server_builder.mcp_server",
	}
	for _, server := range DefaultServers() {
		module, ok := want[server.Name]
		if !ok {
			continue
		}
		if server.Command != "python3" {
			t.Errorf("%s command = %q, want python3", server.Name, server.Command)
		}
		if len(server.Args) != 2 || server.Args[0] != "-m" || server.Args[1] != module {
			t.Errorf("%s args = %v, want [-m %s]", server.Name, server.Args, module)
		}
		delete(want, server.Name)
	}
	for name := range want {
		t.Errorf("bundled server %q missing from DefaultServers", name)
	}
}
