// SPDX-License-Identifier: MIT
package skillmgr

import (
	"context"
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
