// SPDX-License-Identifier: MIT
package skillmgr

import (
	"context"
	"os"
	"path/filepath"
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

func TestStatusReportsPathFallbackWhenNotInstalled(t *testing.T) {
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
			if st.Installed {
				t.Errorf("scheduler should not be installed in tempdir")
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
}
