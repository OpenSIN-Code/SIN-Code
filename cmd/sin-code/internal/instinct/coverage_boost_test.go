// SPDX-License-Identifier: MIT
// Purpose: coverage boost for the instinct package — covers Evolve,
// RenderArtifact, MarkEvolved, ftoa, LLMExtractor, Store lifecycle
// (Base/ListProjects/Delete/LoadAll), Instinct.Decay/IsActive/
// EligibleForEvolution, envFloat/envInt, Manager.EvolveAll/Prune,
// SystemBlockForProject, and NewCommand construction.
package instinct

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ── evolve.go ──────────────────────────────────────────────────────────────

func TestEvolve_ClusterSizes(t *testing.T) {
	t.Parallel()
	make := func(conf float64, obs int, status Status) *Instinct {
		i := NewInstinct("when doing X", "git", "action X", "obs", ScopeProject)
		i.Confidence = conf
		i.Observations = obs
		i.Status = status
		return i
	}
	cases := []struct {
		name    string
		input   []*Instinct
		wantKind EvolutionKind
		wantN   int
	}{
		{
			"single-eligible→command",
			[]*Instinct{make(0.80, 3, StatusActive)},
			EvolveCommand, 1,
		},
		{
			"two-eligible→skill",
			[]*Instinct{make(0.75, 3, StatusActive), make(0.72, 4, StatusActive)},
			EvolveSkill, 2,
		},
		{
			"four-eligible→agent",
			[]*Instinct{
				make(0.80, 3, StatusActive), make(0.78, 3, StatusActive),
				make(0.76, 3, StatusActive), make(0.74, 3, StatusActive),
			},
			EvolveAgent, 4,
		},
		{
			"none-eligible→empty",
			[]*Instinct{make(0.40, 1, StatusPending)},
			EvolveCommand, 0,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			props := Evolve(c.input)
			if c.wantN == 0 {
				if len(props) != 0 {
					t.Fatalf("expected 0 proposals, got %d", len(props))
				}
				return
			}
			if len(props) != 1 {
				t.Fatalf("expected 1 proposal, got %d", len(props))
			}
			if props[0].Kind != c.wantKind {
				t.Errorf("kind = %q, want %q", props[0].Kind, c.wantKind)
			}
			if len(props[0].Members) != c.wantN {
				t.Errorf("members = %d, want %d", len(props[0].Members), c.wantN)
			}
		})
	}
}

func TestEvolve_MultipleDomains(t *testing.T) {
	t.Parallel()
	mk := func(domain string, conf float64) *Instinct {
		i := NewInstinct("when doing "+domain, domain, "act "+domain, "obs", ScopeProject)
		i.Confidence = conf
		i.Observations = 3
		i.Status = StatusActive
		return i
	}
	props := Evolve([]*Instinct{mk("git", 0.80), mk("testing", 0.70), mk("git", 0.75)})
	if len(props) != 2 {
		t.Fatalf("expected 2 proposals (git + testing), got %d", len(props))
	}
	if props[0].AvgConfidence < props[1].AvgConfidence {
		t.Error("proposals should be sorted by avg confidence descending")
	}
}

func TestRenderArtifact_ContainsFrontmatter(t *testing.T) {
	t.Parallel()
	p := Proposal{
		Kind:          EvolveSkill,
		Domain:        "git",
		Name:          "git-skill",
		AvgConfidence: 0.75,
		Rationale:     "Cluster of 2 high-confidence instincts in domain 'git'.",
		Members: []*Instinct{
			{Trigger: "when committing", Action: "run tests", Confidence: 0.80, Domain: "git"},
			{Trigger: "when pushing", Action: "run linters", Confidence: 0.70, Domain: "git"},
		},
	}
	out := p.RenderArtifact()
	if !strings.HasPrefix(out, "---\n") {
		t.Error("missing frontmatter start")
	}
	if !strings.Contains(out, "name: git-skill") {
		t.Error("missing name in frontmatter")
	}
	if !strings.Contains(out, "kind: skill") {
		t.Error("missing kind in frontmatter")
	}
	if !strings.Contains(out, "# Git Skill") {
		t.Error("missing title header")
	}
	if !strings.Contains(out, "## Learned behaviors") {
		t.Error("missing learned behaviors section")
	}
	if !strings.Contains(out, "Trigger: when committing") {
		t.Error("missing trigger in member listing")
	}
}

func TestMarkEvolved_PersistsAndAudits(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	i := NewInstinct("when committing", "git", "run tests", "obs", ScopeProject)
	i.ProjectID = "test-proj"
	i.Confidence = 0.80
	i.Observations = 3
	i.Status = StatusActive
	if err := store.Save(i); err != nil {
		t.Fatal(err)
	}
	p := Proposal{
		Kind:   EvolveSkill,
		Domain: "git",
		Name:   "git-skill",
		Members: []*Instinct{i},
	}
	if err := MarkEvolved(store, p); err != nil {
		t.Fatalf("MarkEvolved: %v", err)
	}
	loaded, err := store.LoadProject("test-proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 instinct, got %d", len(loaded))
	}
	if loaded[0].Status != StatusEvolved {
		t.Errorf("status = %q, want %q", loaded[0].Status, StatusEvolved)
	}
	events, _ := store.ReadAudit(0)
	found := false
	for _, e := range events {
		if e.Kind == "evolved" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'evolved' audit event")
	}
}

func TestFtoa(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0.75, "0.75"},
		{0.5, "0.50"},
		{0.0, "0.00"},
		{1.0, "1.00"},
		{0.80, "0.80"},
	}
	for _, c := range cases {
		got := ftoa(c.in)
		if got != c.want {
			t.Errorf("ftoa(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{42, "42"},
		{-42, "-42"},
	}
	for _, c := range cases {
		got := itoa(c.in)
		if got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── extract_llm.go ──────────────────────────────────────────────────────────

type mockCompleter struct {
	output string
	err    error
}

func (m mockCompleter) Complete(_ context.Context, _, _ string) (string, error) {
	return m.output, m.err
}

func TestLLMExtractor_NilModelFallsBack(t *testing.T) {
	t.Parallel()
	e := LLMExtractor{}
	obs := []Observation{
		{Tool: "Bash", Action: "commit", Domain: "git", Success: true},
		{Tool: "Bash", Action: "commit", Domain: "git", Success: true},
	}
	cands, err := e.Extract(context.Background(), obs)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate from fallback heuristic, got %d", len(cands))
	}
}

func TestLLMExtractor_ModelErrorFallsBack(t *testing.T) {
	t.Parallel()
	e := LLMExtractor{
		Model: mockCompleter{err: context.DeadlineExceeded},
	}
	obs := []Observation{
		{Tool: "Bash", Action: "commit", Domain: "git", Success: true},
		{Tool: "Bash", Action: "commit", Domain: "git", Success: true},
	}
	cands, err := e.Extract(context.Background(), obs)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected fallback candidate, got %d", len(cands))
	}
}

func TestLLMExtractor_ValidJSONReturnsCandidates(t *testing.T) {
	t.Parallel()
	jsonOut := `{"instincts":[{"trigger":"when committing","domain":"git","action":"run tests","evidence":["a","b"]}]}`
	e := LLMExtractor{
		Model:  mockCompleter{output: jsonOut},
		MaxObs: 10,
	}
	obs := []Observation{
		{Tool: "Bash", Action: "commit", Domain: "git", Success: true},
	}
	cands, err := e.Extract(context.Background(), obs)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if cands[0].Action != "run tests" {
		t.Errorf("action = %q, want 'run tests'", cands[0].Action)
	}
	if cands[0].Domain != "git" {
		t.Errorf("domain = %q, want 'git'", cands[0].Domain)
	}
}

func TestLLMExtractor_EmptyDomainDefaultsToGeneral(t *testing.T) {
	t.Parallel()
	jsonOut := `{"instincts":[{"trigger":"when x","domain":"","action":"do y","evidence":[]}]}`
	e := LLMExtractor{Model: mockCompleter{output: jsonOut}}
	cands, _ := e.Extract(context.Background(), []Observation{{Tool: "Bash", Action: "x", Domain: "d", Success: true}})
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if cands[0].Domain != "general" {
		t.Errorf("domain = %q, want 'general'", cands[0].Domain)
	}
}

func TestLLMExtractor_EmptyActionSkipped(t *testing.T) {
	t.Parallel()
	jsonOut := `{"instincts":[{"trigger":"when x","domain":"git","action":"","evidence":[]}]}`
	e := LLMExtractor{Model: mockCompleter{output: jsonOut}}
	cands, _ := e.Extract(context.Background(), []Observation{{Tool: "Bash", Action: "x", Domain: "git", Success: true}})
	if len(cands) != 0 {
		t.Fatalf("expected 0 candidates (empty action skipped), got %d", len(cands))
	}
}

func TestLLMExtractor_BadJSONFallsBack(t *testing.T) {
	t.Parallel()
	e := LLMExtractor{
		Model: mockCompleter{output: "not json at all"},
	}
	obs := []Observation{
		{Tool: "Bash", Action: "commit", Domain: "git", Success: true},
		{Tool: "Bash", Action: "commit", Domain: "git", Success: true},
	}
	cands, err := e.Extract(context.Background(), obs)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected fallback candidate, got %d", len(cands))
	}
}

func TestLLMExtractor_CodeFenceTolerance(t *testing.T) {
	t.Parallel()
	jsonOut := "```json\n{\"instincts\":[{\"trigger\":\"when x\",\"domain\":\"git\",\"action\":\"do y\",\"evidence\":[]}]}\n```"
	e := LLMExtractor{Model: mockCompleter{output: jsonOut}}
	cands, _ := e.Extract(context.Background(), []Observation{{Tool: "Bash", Action: "x", Domain: "git", Success: true}})
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate (code fence stripped), got %d", len(cands))
	}
}

func TestLLMExtractor_MaxObsTrimsBatch(t *testing.T) {
	t.Parallel()
	jsonOut := `{"instincts":[{"trigger":"when x","domain":"git","action":"do y","evidence":[]}]}`
	e := LLMExtractor{Model: mockCompleter{output: jsonOut}, MaxObs: 2}
	obs := make([]Observation, 10)
	for i := range obs {
		obs[i] = Observation{Tool: "Bash", Action: "x", Domain: "git", Success: true}
	}
	cands, _ := e.Extract(context.Background(), obs)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
}

func TestLLMExtractor_CustomFallback(t *testing.T) {
	t.Parallel()
	called := false
	custom := &fakeExtractor{fn: func(_ context.Context, _ []Observation) ([]Candidate, error) {
		called = true
		return []Candidate{{Trigger: "custom", Domain: "x", Action: "y"}}, nil
	}}
	e := LLMExtractor{Model: mockCompleter{err: context.Canceled}, Fallback: custom}
	cands, _ := e.Extract(context.Background(), nil)
	if !called {
		t.Fatal("custom fallback not called")
	}
	if len(cands) != 1 || cands[0].Trigger != "custom" {
		t.Fatalf("unexpected fallback result: %+v", cands)
	}
}

type fakeExtractor struct {
	fn func(ctx context.Context, obs []Observation) ([]Candidate, error)
}

func (f *fakeExtractor) Extract(ctx context.Context, obs []Observation) ([]Candidate, error) {
	return f.fn(ctx, obs)
}

func TestRenderObservations(t *testing.T) {
	obs := []Observation{
		{Tool: "Bash", Action: "commit", Domain: "git", Success: true},
		{Tool: "Edit", Action: "fix bug", Domain: "code", Success: false},
	}
	out := renderObservations(obs)
	if !strings.Contains(out, "Session events:") {
		t.Error("missing header")
	}
	if !strings.Contains(out, "Bash/git") {
		t.Error("missing tool/domain")
	}
	if !strings.Contains(out, "ok") {
		t.Error("missing success status")
	}
	if !strings.Contains(out, "failed") {
		t.Error("missing failure status")
	}
}

func TestParseCandidatesJSON_EmptyInput(t *testing.T) {
	cands, err := parseCandidatesJSON("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if cands != nil {
		t.Errorf("expected nil candidates, got %v", cands)
	}
}

func TestParseCandidatesJSON_NoInstinctsKey(t *testing.T) {
	cands, err := parseCandidatesJSON(`{"other": []}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(cands))
	}
}

// ── types.go: Decay, IsActive, EligibleForEvolution ─────────────────────────

func TestInstinct_Decay(t *testing.T) {
	t.Parallel()
	i := NewInstinct("when committing", "git", "run tests", "obs", ScopeProject)
	i.Confidence = 0.80
	i.Status = StatusActive
	i.Decay(30) // ~5% loss per 30 days
	if i.Confidence >= 0.80 {
		t.Errorf("confidence should decay: got %v", i.Confidence)
	}
	if i.Confidence < MinConfidence {
		t.Errorf("confidence below floor: got %v", i.Confidence)
	}
}

func TestInstinct_DecayZeroDays(t *testing.T) {
	t.Parallel()
	i := NewInstinct("when x", "git", "act", "obs", ScopeProject)
	i.Confidence = 0.80
	before := i.Confidence
	i.Decay(0)
	if i.Confidence != before {
		t.Errorf("Decay(0) should be no-op: got %v, want %v", i.Confidence, before)
	}
}

func TestInstinct_DecayNegativeDays(t *testing.T) {
	t.Parallel()
	i := NewInstinct("when x", "git", "act", "obs", ScopeProject)
	i.Confidence = 0.80
	before := i.Confidence
	i.Decay(-5)
	if i.Confidence != before {
		t.Errorf("Decay(-5) should be no-op: got %v, want %v", i.Confidence, before)
	}
}

func TestInstinct_IsActive(t *testing.T) {
	t.Parallel()
	active := &Instinct{Status: StatusActive}
	pending := &Instinct{Status: StatusPending}
	if !active.IsActive() {
		t.Error("active should be active")
	}
	if pending.IsActive() {
		t.Error("pending should not be active")
	}
}

func TestInstinct_EligibleForEvolution(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		conf    float64
		obs     int
		status  Status
		want    bool
	}{
		{"eligible", 0.75, 3, StatusActive, true},
		{"low-confidence", 0.50, 3, StatusActive, false},
		{"too-few-obs", 0.80, 2, StatusActive, false},
		{"not-active", 0.80, 3, StatusPending, false},
		{"evolved-excluded", 0.90, 5, StatusEvolved, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			i := &Instinct{Confidence: c.conf, Observations: c.obs, Status: c.status}
			if got := i.EligibleForEvolution(); got != c.want {
				t.Errorf("EligibleForEvolution() = %v, want %v", got, c.want)
			}
		})
	}
}

// ── store.go: Base, ListProjects, Delete, LoadAll ───────────────────────────

func TestStore_Base(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	s := NewStore(tmp)
	if s.Base() != tmp {
		t.Errorf("Base() = %q, want %q", s.Base(), tmp)
	}
}

func TestStore_ListProjects_Empty(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	projects, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestStore_ListProjects_WithMeta(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	proj := Project{ID: "proj-a", Name: "Project A", Remote: "https://github.com/x/y"}
	if err := s.SaveProjectMeta(proj); err != nil {
		t.Fatal(err)
	}
	projects, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "Project A" {
		t.Errorf("name = %q, want 'Project A'", projects[0].Name)
	}
}

func TestStore_Delete(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	i := NewInstinct("when committing", "git", "run tests", "obs", ScopeProject)
	i.ProjectID = "test-proj"
	if err := s.Save(i); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(i); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	loaded, _ := s.LoadProject("test-proj")
	if len(loaded) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(loaded))
	}
}

func TestStore_DeleteNonExistent(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	i := &Instinct{ID: "nonexistent", Scope: ScopeProject, ProjectID: "ghost"}
	if err := s.Delete(i); err != nil {
		t.Errorf("deleting non-existent should be no-op, got %v", err)
	}
}

func TestStore_LoadAll(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	// Save a global
	g := NewInstinct("when global x", "git", "global action", "obs", ScopeGlobal)
	if err := s.Save(g); err != nil {
		t.Fatal(err)
	}
	// Save a project + meta
	proj := Project{ID: "proj-x", Name: "X"}
	if err := s.SaveProjectMeta(proj); err != nil {
		t.Fatal(err)
	}
	p := NewInstinct("when project x", "testing", "project action", "obs", ScopeProject)
	p.ProjectID = "proj-x"
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	all, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 instincts (1 global + 1 project), got %d", len(all))
	}
}

func TestStore_LoadAll_Empty(t *testing.T) {
	t.Parallel()
	s := NewStore(t.TempDir())
	all, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0, got %d", len(all))
	}
}

// ── manager.go: EvolveAll, Prune, SystemBlockForProject ─────────────────────

func TestManager_EvolveAll(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	project := Project{ID: "evo-proj", Name: "evo"}
	mgr := NewManagerWithStore(store, project, nil)
	// Save an eligible instinct
	i := NewInstinct("when committing", "git", "run tests", "obs", ScopeProject)
	i.ProjectID = "evo-proj"
	i.Confidence = 0.80
	i.Observations = 3
	i.Status = StatusActive
	if err := store.Save(i); err != nil {
		t.Fatal(err)
	}
	props, err := mgr.EvolveAll()
	if err != nil {
		t.Fatalf("EvolveAll: %v", err)
	}
	if len(props) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(props))
	}
}

func TestManager_EvolveAll_Empty(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	mgr := NewManagerWithStore(store, Project{ID: "empty", Name: "empty"}, nil)
	props, err := mgr.EvolveAll()
	if err != nil {
		t.Fatalf("EvolveAll: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("expected 0 proposals, got %d", len(props))
	}
}

func TestManager_Prune_StalePending(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	project := Project{ID: "prune-proj", Name: "prune"}
	mgr := NewManagerWithStore(store, project, nil)
	// Save a stale pending instinct (old UpdatedAt)
	i := NewInstinct("when old", "git", "old action", "obs", ScopeProject)
	i.ProjectID = "prune-proj"
	i.Status = StatusPending
	i.UpdatedAt = time.Now().UTC().Add(-60 * 24 * time.Hour) // 60 days ago
	if err := store.Save(i); err != nil {
		t.Fatal(err)
	}
	deleted, err := mgr.Prune(30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
	loaded, _ := store.LoadProject("prune-proj")
	if len(loaded) != 0 {
		t.Errorf("expected 0 after prune, got %d", len(loaded))
	}
}

func TestManager_Prune_DefaultTTL(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	mgr := NewManagerWithStore(store, Project{ID: "p", Name: "p"}, nil)
	// ttl <= 0 should default to 30
	deleted, err := mgr.Prune(0)
	if err != nil {
		t.Fatalf("Prune(0): %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted on empty store, got %d", deleted)
	}
}

func TestManager_Prune_DecaysActive(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	project := Project{ID: "decay-proj", Name: "decay"}
	mgr := NewManagerWithStore(store, project, nil)
	// Save an active instinct (should NOT be deleted, but decayed)
	i := NewInstinct("when active", "git", "act", "obs", ScopeProject)
	i.ProjectID = "decay-proj"
	i.Confidence = 0.80
	i.Status = StatusActive
	i.Observations = 5
	if err := store.Save(i); err != nil {
		t.Fatal(err)
	}
	deleted, err := mgr.Prune(30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 0 {
		t.Errorf("active instinct should not be deleted, got %d deleted", deleted)
	}
	loaded, _ := store.LoadProject("decay-proj")
	if len(loaded) != 1 {
		t.Fatalf("expected 1 instinct remaining, got %d", len(loaded))
	}
}

func TestManager_SystemBlockForProject(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	project := Project{ID: "sb-proj", Name: "sb"}
	mgr := NewManagerWithStore(store, project, nil)
	// No active instincts → empty string
	out, err := mgr.SystemBlockForProject(10)
	if err != nil {
		t.Fatalf("SystemBlockForProject: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty with no active instincts, got %q", out)
	}
	// Save an active instinct
	i := NewInstinct("when committing", "git", "run tests", "obs", ScopeProject)
	i.ProjectID = "sb-proj"
	i.Confidence = 0.80
	i.Status = StatusActive
	if err := store.Save(i); err != nil {
		t.Fatal(err)
	}
	out, err = mgr.SystemBlockForProject(10)
	if err != nil {
		t.Fatalf("SystemBlockForProject: %v", err)
	}
	if !strings.Contains(out, "# Learned instincts") {
		t.Errorf("expected learned instincts header, got %q", out)
	}
}

func TestManager_Active_FiltersPending(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	project := Project{ID: "filt-proj", Name: "filt"}
	mgr := NewManagerWithStore(store, project, nil)
	// Save a pending instinct (should be filtered out)
	i := NewInstinct("when pending", "git", "act", "obs", ScopeProject)
	i.ProjectID = "filt-proj"
	i.Status = StatusPending
	i.Confidence = 0.40
	if err := store.Save(i); err != nil {
		t.Fatal(err)
	}
	active, err := mgr.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active, got %d", len(active))
	}
}

// ── config.go: envFloat, envInt ─────────────────────────────────────────────

func TestEnvFloat_Default(t *testing.T) {
	got := envFloat("SIN_INSTINCT_NONEXISTENT_VAR_XYZ", 0.65)
	if got != 0.65 {
		t.Errorf("envFloat default = %v, want 0.65", got)
	}
}

func TestEnvFloat_Valid(t *testing.T) {
	t.Setenv("SIN_INSTINCT_TEST_FLOAT", "0.42")
	got := envFloat("SIN_INSTINCT_TEST_FLOAT", 0.65)
	if got != 0.42 {
		t.Errorf("envFloat valid = %v, want 0.42", got)
	}
}

func TestEnvFloat_Invalid(t *testing.T) {
	t.Setenv("SIN_INSTINCT_TEST_FLOAT_BAD", "not-a-number")
	got := envFloat("SIN_INSTINCT_TEST_FLOAT_BAD", 0.65)
	if got != 0.65 {
		t.Errorf("envFloat invalid = %v, want 0.65 (default)", got)
	}
}

func TestEnvInt_Default(t *testing.T) {
	got := envInt("SIN_INSTINCT_NONEXISTENT_INT_XYZ", 42)
	if got != 42 {
		t.Errorf("envInt default = %v, want 42", got)
	}
}

func TestEnvInt_Valid(t *testing.T) {
	t.Setenv("SIN_INSTINCT_TEST_INT", "99")
	got := envInt("SIN_INSTINCT_TEST_INT", 42)
	if got != 99 {
		t.Errorf("envInt valid = %v, want 99", got)
	}
}

func TestEnvInt_Invalid(t *testing.T) {
	t.Setenv("SIN_INSTINCT_TEST_INT_BAD", "abc")
	got := envInt("SIN_INSTINCT_TEST_INT_BAD", 42)
	if got != 42 {
		t.Errorf("envInt invalid = %v, want 42 (default)", got)
	}
}

func TestLoadConfig_Overrides(t *testing.T) {
	t.Setenv("SIN_INSTINCT_ACTIVATION", "0.55")
	t.Setenv("SIN_INSTINCT_EVOLVE", "0.65")
	t.Setenv("SIN_INSTINCT_REINFORCE", "0.30")
	t.Setenv("SIN_INSTINCT_CONTRADICT", "0.50")
	t.Setenv("SIN_INSTINCT_PROMOTE_N", "3")
	t.Setenv("SIN_INSTINCT_TTL_DAYS", "45")
	c := LoadConfig()
	if c.ActivationThreshold != 0.55 {
		t.Errorf("activation = %v, want 0.55", c.ActivationThreshold)
	}
	if c.EvolveThreshold != 0.65 {
		t.Errorf("evolve = %v, want 0.65", c.EvolveThreshold)
	}
	if c.ReinforceStep != 0.30 {
		t.Errorf("reinforce = %v, want 0.30", c.ReinforceStep)
	}
	if c.ContradictStep != 0.50 {
		t.Errorf("contradict = %v, want 0.50", c.ContradictStep)
	}
	if c.PromotionThreshold != 3 {
		t.Errorf("promoteN = %v, want 3", c.PromotionThreshold)
	}
	if c.PruneTTLDays != 45 {
		t.Errorf("ttl = %v, want 45", c.PruneTTLDays)
	}
	// Reset tuning for other tests
	ApplyConfig(DefaultConfig())
}

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.ActivationThreshold != ActivationThreshold {
		t.Errorf("activation = %v, want %v", c.ActivationThreshold, ActivationThreshold)
	}
	if c.ReinforceStep != 0.25 {
		t.Errorf("reinforce = %v, want 0.25", c.ReinforceStep)
	}
	if c.PruneTTLDays != 30 {
		t.Errorf("ttl = %v, want 30", c.PruneTTLDays)
	}
}

// ── inject.go: ftoaShort ────────────────────────────────────────────────────

func TestFtoaShort(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0.85, "0.85"},
		{0.70, "0.70"},
		{0.0, "0.00"},
		{1.0, "1.00"},
	}
	for _, c := range cases {
		got := ftoaShort(c.in)
		if got != c.want {
			t.Errorf("ftoaShort(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── manager.go: NewCommand ──────────────────────────────────────────────────

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()
	if cmd.Use != "instinct" {
		t.Errorf("Use = %q, want 'instinct'", cmd.Use)
	}
	subs := cmd.Commands()
	if len(subs) < 8 {
		t.Errorf("expected at least 8 subcommands, got %d", len(subs))
	}
}

// ── project.go: normalizeRemote, repoNameFromRemote, hash12 ─────────────────

func TestNormalizeRemote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://github.com/OpenSIN-Code/SIN-Code.git", "github.com/opensin-code/sin-code"},
		{"git@github.com:OpenSIN-Code/SIN-Code.git", "github.com/opensin-code/sin-code"},
		{"https://user:pass@github.com/foo/bar.git", "github.com/foo/bar"},
		{"ssh://git@github.com/foo/bar", "github.com/foo/bar"},
		{"http://example.com/repo/", "example.com/repo"},
	}
	for _, c := range cases {
		got := normalizeRemote(c.in)
		if got != c.want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRepoNameFromRemote(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"github.com/opensin-code/sin-code", "sin-code"},
		{"github.com/foo/bar", "bar"},
		{"just-a-name", "just-a-name"},
	}
	for _, c := range cases {
		got := repoNameFromRemote(c.in)
		if got != c.want {
			t.Errorf("repoNameFromRemote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHash12(t *testing.T) {
	h := hash12("test-input")
	if len(h) != 12 {
		t.Errorf("hash12 length = %d, want 12", len(h))
	}
	h2 := hash12("test-input")
	if h != h2 {
		t.Error("hash12 should be deterministic")
	}
	h3 := hash12("different-input")
	if h == h3 {
		t.Error("hash12 should differ for different inputs")
	}
}

// ── promote.go: appendUnique ─────────────────────────────────────────────────

func TestAppendUnique(t *testing.T) {
	s := []string{"a", "b"}
	s = appendUnique(s, "a") // duplicate
	if len(s) != 2 {
		t.Errorf("appendUnique duplicate: len = %d, want 2", len(s))
	}
	s = appendUnique(s, "c") // new
	if len(s) != 3 {
		t.Errorf("appendUnique new: len = %d, want 3", len(s))
	}
	if s[2] != "c" {
		t.Errorf("appendUnique: last = %q, want 'c'", s[2])
	}
}

func TestFindPromotable_CrossProject(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	// Two projects with same signature
	for _, pid := range []string{"proj-a", "proj-b"} {
		_ = store.SaveProjectMeta(Project{ID: pid, Name: pid})
		i := NewInstinct("when committing", "git", "run tests", "obs", ScopeProject)
		i.ProjectID = pid
		i.Confidence = 0.70
		i.Status = StatusActive
		if err := store.Save(i); err != nil {
			t.Fatal(err)
		}
	}
	cands, err := FindPromotable(store)
	if err != nil {
		t.Fatalf("FindPromotable: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 promotable candidate, got %d", len(cands))
	}
	if len(cands[0].Projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(cands[0].Projects))
	}
}

func TestPromote_WritesGlobal(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	_ = store.SaveProjectMeta(Project{ID: "proj-a", Name: "A"})
	_ = store.SaveProjectMeta(Project{ID: "proj-b", Name: "B"})
	best := NewInstinct("when committing", "git", "run tests", "obs", ScopeProject)
	best.ProjectID = "proj-a"
	best.Confidence = 0.80
	best.Status = StatusActive
	if err := store.Save(best); err != nil {
		t.Fatal(err)
	}
	second := NewInstinct("when committing", "git", "run tests", "obs", ScopeProject)
	second.ProjectID = "proj-b"
	second.Confidence = 0.60
	second.Status = StatusActive
	if err := store.Save(second); err != nil {
		t.Fatal(err)
	}
	cands, _ := FindPromotable(store)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	g, err := Promote(store, cands[0])
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if g.Scope != ScopeGlobal {
		t.Errorf("scope = %q, want global", g.Scope)
	}
	if g.Source != "promotion" {
		t.Errorf("source = %q, want 'promotion'", g.Source)
	}
	globals, _ := store.LoadGlobal()
	if len(globals) != 1 {
		t.Errorf("expected 1 global instinct, got %d", len(globals))
	}
}

// ── store.go: ResolveBaseDir ─────────────────────────────────────────────────

func TestResolveBaseDir_EnvOverride(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", "/tmp/sin-instinct-test")
	got := ResolveBaseDir()
	if got != "/tmp/sin-instinct-test" {
		t.Errorf("ResolveBaseDir = %q, want /tmp/sin-instinct-test", got)
	}
}

func TestResolveBaseDir_XDG(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-test")
	got := ResolveBaseDir()
	if !strings.Contains(got, "sin-code/instinct") {
		t.Errorf("ResolveBaseDir with XDG = %q, expected sin-code/instinct", got)
	}
}

func TestResolveBaseDir_RelativeEnvIgnored(t *testing.T) {
	t.Setenv("SIN_INSTINCT_DIR", "relative/path")
	t.Setenv("XDG_DATA_HOME", "")
	got := ResolveBaseDir()
	if strings.Contains(got, "relative/path") {
		t.Errorf("relative SIN_INSTINCT_DIR should be ignored, got %q", got)
	}
}

// ── observer.go: Flush empty ─────────────────────────────────────────────────

func TestObserver_FlushEmpty(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	store := NewStore(tmp)
	mgr := NewManagerWithStore(store, Project{ID: "o", Name: "o"}, nil)
	obs := NewObserver(mgr, nil)
	created, reinforced, err := obs.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush empty: %v", err)
	}
	if created != 0 || reinforced != 0 {
		t.Errorf("Flush empty: created=%d reinforced=%d, want 0/0", created, reinforced)
	}
}

// ── extract.go: domainFromPath, triggerFromAction, describe ──────────────────

func TestDomainFromPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/x/foo_test.go", "testing"},
		{"/x/bar.test.ts", "testing"},
		{"/x/foo.spec.js", "testing"},
		{"/x/dockerfile", "infra"},
		{"/x/main.tf", "infra"},
		{"/x/config.yaml", "infra"},
		{"/x/auth.go", "security"},
		{"/x/.env", "security"},
		{"/x/credentials.json", "security"},
		{"/x/main.go", "go"},
		{"/x/lib.rs", "rust"},
		{"/x/app.py", "python"},
		{"/x/app.tsx", "typescript"},
		{"/x/schema.sql", "database"},
		{"/x/README.md", "docs"},
		{"/x/unknown.xyz", ""},
	}
	for _, c := range cases {
		got := domainFromPath(c.path)
		if got != c.want {
			t.Errorf("domainFromPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestTriggerFromAction(t *testing.T) {
	cases := []struct {
		action, domain, want string
	}{
		{"commit", "git", "making a commit"},
		{"test", "testing", "adding or updating tests"},
		{"edit", "code-style", "writing or editing code"},
		{"auth", "security", "handling secrets or auth"},
		{"deploy", "infra", "working in infra"},
		{"x", "unknown-domain", "working in unknown-domain"},
	}
	for _, c := range cases {
		got := triggerFromAction(c.action, c.domain)
		if got != c.want {
			t.Errorf("triggerFromAction(%q, %q) = %q, want %q", c.action, c.domain, got, c.want)
		}
	}
}

func TestDescribe(t *testing.T) {
	if got := describe(Observation{Tool: "Bash", Meta: map[string]string{"path": "/x/y"}}); got != "Bash on /x/y" {
		t.Errorf("describe with path = %q", got)
	}
	if got := describe(Observation{Tool: "Bash", Meta: map[string]string{"command": "ls"}}); got != "Bash: ls" {
		t.Errorf("describe with command = %q", got)
	}
	if got := describe(Observation{Tool: "Bash"}); got != "Bash action" {
		t.Errorf("describe default = %q", got)
	}
}
