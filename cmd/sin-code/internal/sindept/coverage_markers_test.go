package sindept

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScan_AllCommentFamilies(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name   string
		path   string
		source string
	}{
		{
			name:   "go-slash",
			path:   "go_fixture.go",
			source: "package x\n// sin-debt: medium, upgrade: review quarterly\n",
		},
		{
			name:   "py-hash",
			path:   "py_fixture.py",
			source: "# sin-debt: low\n",
		},
		{
			name:   "sql-dash",
			path:   "sql_fixture.sql",
			source: "-- sin-debt: high, upgrade: add tests\n",
		},
		{
			name:   "c-block",
			path:   "c_fixture.c",
			source: "/* sin-debt: medium */\n",
		},
		{
			name:   "html",
			path:   "html_fixture.html",
			source: "<!-- sin-debt: low -->\n",
		},
	}
	wantReasons := map[string]string{
		"go_fixture.go":  "medium",
		"py_fixture.py":  "low",
		"sql_fixture.sql": "high",
		"c_fixture.c":    "medium",
		"html_fixture.html": "low",
	}
	wantUpgrades := map[string]string{
		"go_fixture.go":   "review quarterly",
		"sql_fixture.sql": "add tests",
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(dir, c.path)
			if err := os.WriteFile(p, []byte(c.source), 0o644); err != nil {
				t.Fatal(err)
			}
			opts := DefaultOptions()
			opts.IncludeExt = nil
			opts.SkipSuffixes = nil
			mk, err := ParseFile(p)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if len(mk) != 1 {
				t.Fatalf("expected 1 marker in %s, got %d", c.path, len(mk))
			}
			m := mk[0]
			if m.Reason != wantReasons[c.path] {
				t.Errorf("%s reason=%q want %q", c.path, m.Reason, wantReasons[c.path])
			}
			upg, ok := wantUpgrades[c.path]
			if ok {
				if !m.HasUpg || m.Upgrade != upg {
					t.Errorf("%s upgrade=%q (has=%v) want %q", c.path, m.Upgrade, m.HasUpg, upg)
				}
			} else if m.HasUpg {
				t.Errorf("%s unexpected upgrade clause %q", c.path, m.Upgrade)
			}
		})
	}
}

func TestScan_NoUpgradeClause_IsRotRisk(t *testing.T) {
	mk := parseGolden(t)
	var rot []Marker
	for _, m := range mk {
		if !m.HasUpg || m.Upgrade == "" {
			rot = append(rot, m)
		}
	}
	if len(rot) != 4 {
		t.Fatalf("expected 4 rot-risk markers, got %d", len(rot))
	}
	stats := AggregateStats(rot)
	report := RenderStatsString(stats)
	for _, m := range rot {
		if !strings.Contains(report, m.Reason) {
			t.Errorf("rot-risk row missing reason %q in report", m.Reason)
		}
	}
	if !strings.Contains(report, "Without upgrade (rot-risk):") {
		t.Fatalf("rot-risk section missing from report:\n%s", report)
	}
	if !strings.Contains(report, "## Rot-risk markers") {
		t.Fatalf("rot-risk heading missing from report:\n%s", report)
	}
}

func TestScan_DefaultReasonsCatalogue(t *testing.T) {
	mk := parseGolden(t)
	catalog := DefaultPolicy().DefaultReasons
	catalogSet := make(map[string]struct{}, len(catalog))
	for _, reason := range catalog {
		catalogSet[strings.ToLower(strings.TrimSpace(reason))] = struct{}{}
	}
	hits := []string{}
	for _, m := range mk {
		key := strings.ToLower(strings.TrimSpace(m.Reason))
		if _, ok := catalogSet[key]; ok {
			hits = append(hits, key)
		}
	}
	if len(hits) == 0 {
		t.Fatalf("no fixture marker matched DefaultReasons catalogue: %v", catalog)
	}
	stats := AggregateStats(mk)
	report := RenderStatsString(stats)
	if !strings.Contains(report, "## By reason") {
		t.Fatalf("By reason section missing:\n%s", report)
	}
	for _, hit := range hits {
		if !strings.Contains(report, "| "+hit+" |") {
			t.Errorf("catalogue hit %q missing from By reason table:\n%s", hit, report)
		}
	}
}

func TestScan_ByteStable(t *testing.T) {
	root := filepath.Join("testdata")
	opts := DefaultOptions()
	opts.SkipSuffixes = nil
	mk1, err := ParseDir(root, opts)
	if err != nil {
		t.Fatalf("ParseDir #1: %v", err)
	}
	mk2, err := ParseDir(root, opts)
	if err != nil {
		t.Fatalf("ParseDir #2: %v", err)
	}
	if len(mk1) == 0 {
		t.Fatalf("no markers parsed; fixture missing?")
	}
	if len(mk1) != len(mk2) {
		t.Fatalf("marker counts differ across runs: %d vs %d", len(mk1), len(mk2))
	}
	r1 := RenderStatsString(AggregateStats(mk1))
	r2 := RenderStatsString(AggregateStats(mk2))
	if r1 != r2 {
		t.Fatalf("Render output byte-mismatch across two invocations:\n---1---\n%s\n---2---\n%s", r1, r2)
	}
	if !bytes.Equal([]byte(r1), []byte(r2)) {
		t.Fatalf("Render output []byte form byte-mismatch across runs")
	}
}

func TestScan_PolicyGate_Deny(t *testing.T) {
	dir := t.TempDir()
	sinDir := filepath.Join(dir, ".sin-code")
	if err := os.Mkdir(sinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	policyTOML := "[sin-debt]\nmax_no_upgrade = 0\nrequire_upgrade = true\n"
	policyPath := filepath.Join(sinDir, "debt-policy.toml")
	if err := os.WriteFile(policyPath, []byte(policyTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(dir, "src")
	if err := os.Mkdir(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(srcDir, "hot.go")
	if err := os.WriteFile(srcFile, []byte("// sin-debt: legacy retry loop\npackage hot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicyFile(policyPath)
	if err != nil {
		t.Fatalf("LoadPolicyFile: %v", err)
	}
	if policy.Source == "" {
		t.Fatalf("policy.Source empty after LoadPolicyFile")
	}
	if policy.RequireUpgrade != true {
		t.Fatalf("RequireUpgrade=%v want true", policy.RequireUpgrade)
	}
	if policy.MaxNoUpgrade != 0 {
		t.Fatalf("MaxNoUpgrade=%d want 0", policy.MaxNoUpgrade)
	}
	if got := parseBool("yes"); !got {
		t.Fatalf("parseBool(yes)=false")
	}
	if got := parseBool("on"); !got {
		t.Fatalf("parseBool(on)=false")
	}
	if got := parseBool("off"); got {
		t.Fatalf("parseBool(off)=true")
	}
	if got, err := parseInt("42"); err != nil || got != 42 {
		t.Fatalf("parseInt(42)=%d,%v want 42,nil", got, err)
	}
	if got := parseStringList(`["a", "b", "c"]`); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("parseStringList bracket: %v", got)
	}
	if got := parseStringList("a, b, c"); len(got) != 3 || got[1] != "b" {
		t.Fatalf("parseStringList bare: %v", got)
	}
	if got := parseStringList("[\"x\",\"y\"]"); len(got) != 2 || got[1] != "y" {
		t.Fatalf("parseStringList tight: %v", got)
	}
	opts := DefaultOptions()
	opts.SkipSuffixes = nil
	mk, err := ParseDir(srcDir, opts)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	if len(mk) == 0 {
		t.Fatalf("expected at least one marker in hot.go")
	}
	res := policy.RunCheck(mk)
	if res.Ok {
		t.Fatalf("gate accepted non-zero markers: %+v", res)
	}
	if len(res.Failed) == 0 {
		t.Fatalf("Failed slice empty on deny")
	}
	if res.Missing < 1 {
		t.Fatalf("Missing=%d want >=1", res.Missing)
	}
	formatted := FormatCheckResult(res)
	if !strings.Contains(formatted, "FAIL") {
		t.Errorf("FormatCheckResult missing FAIL banner: %s", formatted)
	}
	if !strings.Contains(formatted, "require_upgrade") {
		t.Errorf("FormatCheckResult missing require_upgrade reason: %s", formatted)
	}
	walked, err := LoadPolicyForRoot(srcDir)
	if err != nil {
		t.Fatalf("LoadPolicyForRoot: %v", err)
	}
	if !walked.RequireUpgrade {
		t.Fatalf("LoadPolicyForRoot RequireUpgrade=%v want true", walked.RequireUpgrade)
	}
	if walked.Source == "" {
		t.Fatalf("LoadPolicyForRoot Source empty for nested project")
	}
	isolated := t.TempDir()
	defaulted, err := LoadPolicyForRoot(isolated)
	if err != nil {
		t.Fatalf("LoadPolicyForRoot missing: %v", err)
	}
	if defaulted.Source != "" {
		t.Fatalf("default policy should have empty Source, got %q", defaulted.Source)
	}
	if defaulted.MaxNoUpgrade != DefaultPolicy().MaxNoUpgrade {
		t.Fatalf("default policy MaxNoUpgrade=%d want %d", defaulted.MaxNoUpgrade, DefaultPolicy().MaxNoUpgrade)
	}
}

func TestScan_FiltersHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	mkReal := filepath.Join(dir, "real.go")
	if err := os.WriteFile(mkReal, []byte("package r\n// sin-debt: keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hidden := []struct {
		subdir string
		files  map[string]string
	}{
		{
			subdir: ".git",
			files: map[string]string{
				"HEAD":    "ref: refs/heads/main\n// sin-debt: skip\n",
				"config":  "[core]\n# sin-debt: skip\n",
			},
		},
		{
			subdir: "node_modules",
			files: map[string]string{
				"index.js": "// sin-debt: skip\nconsole.log(1);\n",
			},
		},
		{
			subdir: "vendor",
			files: map[string]string{
				"lib.go": "package v\n// sin-debt: skip\n",
			},
		},
		{
			subdir: "dist",
			files: map[string]string{
				"bundle.js": "// sin-debt: skip\n",
			},
		},
	}
	for _, h := range hidden {
		p := filepath.Join(dir, h.subdir)
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range h.files {
			if err := os.WriteFile(filepath.Join(p, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	opts := DefaultOptions()
	opts.SkipSuffixes = nil
	mk, err := ParseDir(dir, opts)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	if len(mk) != 1 {
		t.Fatalf("expected only 1 marker (real.go), got %d: %+v", len(mk), filesOf(mk))
	}
	if !strings.HasSuffix(mk[0].File, string(filepath.Separator)+"real.go") {
		t.Fatalf("unexpected survivor: %s", mk[0].File)
	}
	for _, h := range hidden {
		if shouldSkipDir(h.subdir, map[string]bool{}) {
			if !strings.HasPrefix(h.subdir, ".") && !mapContains(DefaultOptions().Skip, h.subdir) {
				t.Errorf("non-dot subdir %q should not be a hidden skip", h.subdir)
			}
		}
	}
	if !shouldSkipDir(".sin-code", map[string]bool{}) {
		t.Errorf(".sin-code should be a hidden skip")
	}
	if !shouldSkipDir("vendor", map[string]bool{"vendor": true}) {
		t.Errorf("explicit vendor skip should fire")
	}
	cfg := DefaultOptions()
	cfg.SkipSuffixes = []string{"_skipme.go"}
	if err := os.WriteFile(filepath.Join(dir, "doc_skipme.go"), []byte("// sin-debt: skip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !shouldSkipSuffix("doc_skipme.go", map[string]bool{"_skipme.go": true}) {
		t.Errorf("suffix skip did not match")
	}
	mk2, err := ParseDir(dir, cfg)
	if err != nil {
		t.Fatalf("ParseDir suffix: %v", err)
	}
	if len(mk2) != 1 {
		t.Fatalf("suffix skipper expected 1 marker, got %d", len(mk2))
	}
	rlPath := filepath.Join(dir, "real.go")
	rel, err := filepath.Rel(dir, rlPath)
	if err != nil {
		t.Fatal(err)
	}
	rooted, err := ParseFile(rlPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rooted) != 1 || !strings.HasSuffix(rooted[0].File, rel) {
		t.Fatalf("ParseFile real.go strange: %+v", rooted)
	}
	if ok := strings.Contains(filepath.Ext(""), "."); ok {
		t.Errorf("empty extension should not contain a dot")
	}
	_ = cfg
}

func filesOf(mk []Marker) []string {
	out := make([]string, 0, len(mk))
	for _, m := range mk {
		out = append(out, m.File)
	}
	return out
}

func mapContains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
