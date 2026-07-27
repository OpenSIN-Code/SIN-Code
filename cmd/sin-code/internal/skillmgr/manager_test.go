// SPDX-License-Identifier: MIT
package skillmgr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnownSkillsCoversRegistry(t *testing.T) {
	// Sanity: every shortname from mcpclient.DefaultServers must have an
	// entry here (registry↔skillmgr parity). The CI sync gate enforces this
	// upstream; here we just guard the list is non-empty.
	ks := KnownSkills()
	if len(ks) < 10 {
		t.Fatalf("expected at least 10 known skills, got %d", len(ks))
	}
	for _, name := range []string{"websearch", "browser", "simone", "honcho"} {
		if _, ok := ks[name]; !ok {
			t.Errorf("expected %q in KnownSkills", name)
		}
	}
}

func TestKnownSkillsHasShopEntries(t *testing.T) {
	// Issue #142 fusion: the three shop skills (CJ Dropshipping,
	// Stripe, TikTok Shop) are in KnownSkills so `sin-code skill
	// install <name>` works. The mapping is shortname -> repo.
	ks := KnownSkills()
	for name, want := range map[string]string{
		"shop-cj-dropshipping": "cj-dropshipping-skill",
		"shop-stripe":          "SIN-Stripe-Bundle",
		"shop-tiktok":          "SIN-eCommerce-Scraper-Bundle",
	} {
		got, ok := ks[name]
		if !ok {
			t.Errorf("expected %q in KnownSkills (issue #142)", name)
			continue
		}
		if got != want {
			t.Errorf("KnownSkills[%q] = %q, want %q", name, got, want)
		}
	}
}

func TestInstallUnknownSkillFails(t *testing.T) {
	_, err := Install(context.Background(), "no-such-skill-xyz")
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
}

func TestStatusOnEmptyDir(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())
	sts := Status(context.Background())
	if len(sts) == 0 {
		t.Fatal("expected status for known skills")
	}
	for _, st := range sts {
		if st.Installed {
			t.Errorf("skill %q should NOT be installed in tempdir", st.Name)
		}
	}
}

func TestStatusReportsPathBinaryAsInstalledAndRunnable(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		if name == "sin-scheduler" {
			return "/opt/bin/sin-scheduler", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "scheduler" {
			if !st.Installed {
				t.Fatalf("scheduler should be installed via PATH binary")
			}
			if !st.Runnable {
				t.Fatalf("scheduler should be runnable via PATH fallback")
			}
			if st.Detail == "" {
				t.Errorf("expected PATH detail for scheduler")
			}
			return
		}
	}
	t.Fatal("scheduler not found in status")
}

func TestStatusReportsEcosystemSkillsInstalledOnPath(t *testing.T) {
	wantPathInstalled := map[string]bool{
		"browser":   true,
		"codocs":    true,
		"frontend":  true,
		"goalmode":  true,
		"scheduler": true,
	}
	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		for skill := range wantPathInstalled {
			if name == canonicalBinary(skill)+"-mcp" {
				return "/opt/bin/" + name, nil
			}
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Status(context.Background())
	for _, st := range sts {
		if !wantPathInstalled[st.Name] {
			continue
		}
		if !st.Installed {
			t.Errorf("%s should be installed via PATH binary", st.Name)
		}
		if !st.Runnable {
			t.Errorf("%s should be runnable via PATH binary", st.Name)
		}
		delete(wantPathInstalled, st.Name)
	}
	for skill := range wantPathInstalled {
		t.Errorf("expected status entry for %s", skill)
	}
}

func TestStatusReportsNonMcpPathBinaryAsInstalled(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		// Only mock the canonical non-mcp binary name for scheduler.
		if name == "sin-scheduler" {
			return "/usr/local/bin/sin-scheduler", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "scheduler" {
			if !st.Installed {
				t.Fatalf("scheduler should be installed via non-mcp PATH binary")
			}
			if !st.Runnable {
				t.Fatalf("scheduler should be runnable via non-mcp PATH binary")
			}
			if !strings.Contains(st.Detail, "available on PATH") {
				t.Errorf("expected PATH detail, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("scheduler not found in status")
}

func TestStatusFallsBackToPathWhenRepoDirHasNoEntrypoint(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		if name == "sin-scheduler" {
			return "/opt/bin/sin-scheduler", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })

	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)
	// Create the repo directory but leave it empty so no entrypoint is found.
	if err := os.MkdirAll(filepath.Join(dir, "SIN-Code-Scheduler-Skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "scheduler" {
			if !st.Installed {
				t.Fatalf("scheduler should be installed (repo dir exists)")
			}
			if !st.Runnable {
				t.Fatalf("scheduler should be runnable via PATH fallback")
			}
			if !strings.Contains(st.Detail, "available on PATH") {
				t.Errorf("expected PATH fallback detail, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("scheduler not found in status")
}

func TestStatusDetectsNestedMcpServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)
	repo := "SIN-Code-Scheduler-Skill"
	script := filepath.Join(dir, repo, "src", "sin_scheduler", "mcp_server.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "scheduler" {
			if !st.Installed {
				t.Fatalf("scheduler should be installed in tempdir")
			}
			if !st.Runnable {
				t.Fatalf("nested mcp_server.py should be runnable")
			}
			if st.Detail == "" {
				t.Errorf("expected entrypoint detail for scheduler")
			}
			return
		}
	}
	t.Fatal("scheduler not found in status")
}

func TestVerifyEntrypointGoBinaryName(t *testing.T) {
	if got, want := goBinaryName("web_search_bundle"), "sin-websearch"; got != want {
		t.Errorf("goBinaryName(web_search_bundle) = %q, want %q", got, want)
	}
	if got, want := goBinaryName("sin-analyse-suite"), "sin-analyse"; got != want {
		t.Errorf("goBinaryName(sin-analyse-suite) = %q, want %q", got, want)
	}
	if got, want := goBinaryName("SIN-Analyse-Suite"), "sin-analyse"; got != want {
		t.Errorf("goBinaryName(SIN-Analyse-Suite) = %q, want %q", got, want)
	}
}

func TestFindPythonCliEntrypoint(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "scripts", "sin_context_bridge.py")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := findPythonCliEntrypoint(dir, "contextbridge"); got != script {
		t.Errorf("findPythonCliEntrypoint = %q, want %q", got, script)
	}
	if got := findPythonCliEntrypoint(dir, "no-such-skill"); got != "" {
		t.Errorf("findPythonCliEntrypoint should return empty for missing skill, got %q", got)
	}
}

func TestRepoNameFromRepoAnalyse(t *testing.T) {
	for _, repo := range []string{"sin-analyse-suite", "SIN-Analyse-Suite"} {
		if got, want := repoNameFromRepo(repo), "analyse"; got != want {
			t.Errorf("repoNameFromRepo(%q) = %q, want %q", repo, got, want)
		}
	}
}

func TestDoctorReportsAllSkills(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Doctor(context.Background())
	if len(sts) == 0 {
		t.Fatal("expected doctor entries for known skills")
	}
	for _, st := range sts {
		if st.Detail == "" {
			t.Errorf("doctor should always report a detail for %q", st.Name)
		}
		if st.Installed && st.Runnable && st.Detail == "" {
			t.Errorf("runnable skill %q should have a detail", st.Name)
		}
	}
}

func TestDoctorReportsNotInstalled(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { _execLookPath = orig })
	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)

	sts := Doctor(context.Background())
	found := false
	for _, st := range sts {
		if st.Installed {
			continue
		}
		found = true
		want := "not installed: " + filepath.Join(dir, st.Repo)
		if st.Detail != want {
			t.Errorf("doctor detail for %q = %q, want %q", st.Name, st.Detail, want)
		}
	}
	if !found {
		t.Fatal("expected at least one not-installed skill")
	}
}

func TestDoctorReportsRunnableOnPath(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		if name == "sin-scheduler" {
			return "/opt/bin/sin-scheduler", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Doctor(context.Background())
	for _, st := range sts {
		if st.Name == "scheduler" {
			if !st.Installed || !st.Runnable {
				t.Fatalf("scheduler should be installed and runnable via PATH, got %+v", st)
			}
			if !strings.Contains(st.Detail, "/opt/bin/sin-scheduler") {
				t.Errorf("expected PATH detail for scheduler, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("scheduler not found in doctor output")
}

func TestCanonicalBinarySimoneAndSymfony(t *testing.T) {
	for name, want := range map[string]string{
		"simone":      "simone-cli",
		"symfonylens": "symfony-lens",
		"honcho":      "sin-honcho-rollback",
	} {
		if got := canonicalBinary(name); got != want {
			t.Errorf("canonicalBinary(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestStatusDetectsSimoneCliEntrypoint(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { _execLookPath = orig })

	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)
	repoDir := filepath.Join(dir, "Simone-MCP")
	cli := filepath.Join(repoDir, "src", "cli.py")
	if err := os.MkdirAll(filepath.Dir(cli), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cli, []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "simone" {
			if !st.Installed || !st.Runnable {
				t.Fatalf("simone should be installed and runnable, got %+v", st)
			}
			if !strings.Contains(st.Detail, "src/cli.py") {
				t.Errorf("expected simone CLI detail, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("simone not found in status")
}

func TestStatusDetectsSymfonyLensModule(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { _execLookPath = orig })

	dir := t.TempDir()
	t.Setenv("SIN_SKILLS_DIR", dir)
	pkgDir := filepath.Join(dir, "SIN-Code-Symfony-Lens", "symfony_lens")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "__init__.py"), []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "server.py"), []byte("# mock"), 0o644); err != nil {
		t.Fatal(err)
	}

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "symfonylens" {
			if !st.Installed || !st.Runnable {
				t.Fatalf("symfonylens should be installed and runnable, got %+v", st)
			}
			if !strings.Contains(st.Detail, "symfony_lens.server") {
				t.Errorf("expected symfony module detail, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("symfonylens not found in status")
}

func TestStatusReportsSimoneCliOnPath(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		if name == "simone-cli" {
			return "/opt/bin/simone-cli", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "simone" {
			if !st.Installed || !st.Runnable {
				t.Fatalf("simone should be installed and runnable via PATH, got %+v", st)
			}
			if !strings.Contains(st.Detail, "/opt/bin/simone-cli") {
				t.Errorf("expected simone PATH detail, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("simone not found in status")
}

func TestStatusReportsSymfonyLensOnPath(t *testing.T) {
	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		if name == "symfony-lens" {
			return "/opt/bin/symfony-lens", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "symfonylens" {
			if !st.Installed || !st.Runnable {
				t.Fatalf("symfonylens should be installed and runnable via PATH, got %+v", st)
			}
			if !strings.Contains(st.Detail, "/opt/bin/symfony-lens") {
				t.Errorf("expected symfony PATH detail, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("symfonylens not found in status")
}

func TestStatusHonchoReportsServerUnreachable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()
	origURL := os.Getenv("HONCHO_SERVER_URL")
	t.Setenv("HONCHO_SERVER_URL", ts.URL)
	t.Cleanup(func() { os.Setenv("HONCHO_SERVER_URL", origURL) })

	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		if name == "sin-honcho-rollback" {
			return "/opt/bin/sin-honcho-rollback", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "honcho" {
			if !st.Installed {
				t.Fatalf("honcho should be installed via PATH")
			}
			if st.Runnable {
				t.Fatalf("honcho should not be runnable when server is unreachable, got %+v", st)
			}
			if !strings.Contains(st.Detail, "unreachable") {
				t.Errorf("expected unreachable detail, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("honcho not found in status")
}

func TestStatusHonchoReportsServerReachable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	origURL := os.Getenv("HONCHO_SERVER_URL")
	t.Setenv("HONCHO_SERVER_URL", ts.URL)
	t.Cleanup(func() { os.Setenv("HONCHO_SERVER_URL", origURL) })

	orig := _execLookPath
	_execLookPath = func(name string) (string, error) {
		if name == "sin-honcho-rollback" {
			return "/opt/bin/sin-honcho-rollback", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "honcho" {
			if !st.Installed || !st.Runnable {
				t.Fatalf("honcho should be installed and runnable when server is reachable, got %+v", st)
			}
			if !strings.Contains(st.Detail, "/opt/bin/sin-honcho-rollback") {
				t.Errorf("expected PATH detail, got %q", st.Detail)
			}
			return
		}
	}
	t.Fatal("honcho not found in status")
}

func TestStatusHonchoReportsNotInstalledWhenServerUnreachable(t *testing.T) {
	origURL := os.Getenv("HONCHO_SERVER_URL")
	t.Setenv("HONCHO_SERVER_URL", "http://127.0.0.1:1")
	t.Cleanup(func() { os.Setenv("HONCHO_SERVER_URL", origURL) })

	orig := _execLookPath
	_execLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { _execLookPath = orig })
	t.Setenv("SIN_SKILLS_DIR", t.TempDir())

	sts := Status(context.Background())
	for _, st := range sts {
		if st.Name == "honcho" {
			if st.Installed {
				t.Fatalf("honcho should not be installed when neither repo nor PATH present")
			}
			return
		}
	}
	t.Fatal("honcho not found in status")
}

func TestBundledToolsAreNotExternalSkillRepositories(t *testing.T) {
	for _, info := range KnownSkillsInfo() {
		switch info.Name {
		case "marketplace", "mcpbuilder":
			t.Errorf("bundled tool %q must not be cloned as external repo %q", info.Name, info.Repo)
		}
	}
}
