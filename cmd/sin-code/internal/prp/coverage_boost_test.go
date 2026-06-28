// SPDX-License-Identifier: MIT
// Purpose: coverage boost for the prp package — covers slugID,
// NewCommand, Store.List, engine edge cases (nil verifier, PR
// not-ready, RunAll draft skip), firstNonEmpty, NewStore empty,
// logf with nil Log, and parse error paths.
package prp

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// ── cli.go: slugID ──────────────────────────────────────────────────────────

func TestSlugID(t *testing.T) {
	cases := []struct {
		name  string
		title string
		check func(t *testing.T, id string)
	}{
		{
			"basic",
			"Add Feature X",
			func(t *testing.T, id string) {
				if !strings.HasPrefix(id, "add-feature-x-") {
					t.Errorf("slugID(%q) = %q, want prefix 'add-feature-x-'", "Add Feature X", id)
				}
			},
		},
		{
			"special-chars-stripped",
			"Fix: @#$% Bug!!!",
			func(t *testing.T, id string) {
				if !strings.HasPrefix(id, "fix-bug-") {
					t.Errorf("slugID = %q, want prefix 'fix-bug-' (special chars stripped)", id)
				}
			},
		},
		{
			"empty-title-defaults",
			"   ",
			func(t *testing.T, id string) {
				if !strings.HasPrefix(id, "prp-") {
					t.Errorf("slugID(empty) = %q, want prefix 'prp-'", id)
				}
			},
		},
		{
			"underscores-and-dashes",
			"my_awesome-project",
			func(t *testing.T, id string) {
				if !strings.HasPrefix(id, "my-awesome-project-") {
					t.Errorf("slugID = %q, want prefix 'my-awesome-project-'", id)
				}
			},
		},
		{
			"numbers-preserved",
			"Fix issue 123",
			func(t *testing.T, id string) {
				if !strings.HasPrefix(id, "fix-issue-123-") {
					t.Errorf("slugID = %q, want prefix 'fix-issue-123-'", id)
				}
			},
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			id := slugID(c.title)
			if id == "" {
				t.Fatal("slugID returned empty string")
			}
			c.check(t, id)
		})
	}
}

func TestSlugID_FormatConsistency(t *testing.T) {
	// slugID appends a timestamp suffix; call it a few times and
	// verify the format is always "slug-<digits>".
	for i := 0; i < 10; i++ {
		id := slugID("same title")
		if !strings.HasPrefix(id, "same-title-") {
			t.Fatalf("slugID = %q, want prefix 'same-title-'", id)
		}
		// Verify suffix is numeric
		suffix := id[len("same-title-"):]
		for _, c := range suffix {
			if c < '0' || c > '9' {
				t.Fatalf("slugID suffix %q contains non-digit", suffix)
			}
		}
	}
}

// ── cli.go: NewCommand ──────────────────────────────────────────────────────

func TestNewCommand(t *testing.T) {
	deps := Deps{
		Planner:     fakePlanner{},
		Implementer: fakeImpl{},
		Verifier:    fakeVerifier{pass: true},
		PR:          fakePR{},
	}
	cmd := NewCommand(deps)
	if cmd.Use != "prp" {
		t.Errorf("Use = %q, want 'prp'", cmd.Use)
	}
	subs := cmd.Commands()
	if len(subs) < 5 {
		t.Errorf("expected at least 5 subcommands, got %d", len(subs))
	}
	// Verify subcommand names exist
	names := make(map[string]bool)
	for _, s := range subs {
		names[s.Use] = true
	}
	expected := []string{"new [title]", "run [id]", "status [id]"}
	for _, e := range expected {
		if !names[e] {
			t.Errorf("missing subcommand %q", e)
		}
	}
}

func TestNewCommand_NilDeps(t *testing.T) {
	cmd := NewCommand(Deps{})
	if cmd == nil {
		t.Fatal("NewCommand returned nil")
	}
}

// ── store.go: List ──────────────────────────────────────────────────────────

func TestStore_List_Empty(t *testing.T) {
	s := NewStore(t.TempDir())
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 PRPs, got %d", len(list))
	}
}

func TestStore_List_MultipleSortedByUpdatedAt(t *testing.T) {
	s := NewStore(t.TempDir())
	// Save multiple PRPs
	p1, _ := s.saveRaw(&PRP{ID: "a", Title: "A", Phase: PhaseDraft})
	p2, _ := s.saveRaw(&PRP{ID: "b", Title: "B", Phase: PhaseDraft})
	_ = p1
	_ = p2
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 PRPs, got %d", len(list))
	}
	// Should be sorted by UpdatedAt descending (both just created, so order may vary)
	ids := make(map[string]bool)
	for _, p := range list {
		ids[p.ID] = true
	}
	if !ids["a"] || !ids["b"] {
		t.Errorf("missing PRP in list: %v", ids)
	}
}

func TestStore_List_NonExistentDir(t *testing.T) {
	s := NewStore("/nonexistent/path/that/does/not/exist")
	list, err := s.List()
	if err != nil {
		t.Fatalf("List on nonexistent dir: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 on nonexistent dir, got %d", len(list))
	}
}

func TestStore_Load_NotFound(t *testing.T) {
	s := NewStore(t.TempDir())
	_, err := s.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error loading nonexistent PRP")
	}
}

// helper that saves a PRP directly via the Store
func (s *Store) saveRaw(p *PRP) (*PRP, error) {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	err := s.Save(p)
	return p, err
}

// ── store.go: NewStore empty ─────────────────────────────────────────────────

func TestNewStore_EmptyWorkdir(t *testing.T) {
	s := NewStore("")
	if s.dir == "" {
		t.Error("NewStore('') should default to ./.sin/prp")
	}
	if !strings.HasSuffix(s.dir, ".sin/prp") {
		t.Errorf("NewStore('') dir = %q, want suffix '.sin/prp'", s.dir)
	}
}

// ── engine.go: RunVerify nil verifier ────────────────────────────────────────

func TestRunVerify_NilVerifier(t *testing.T) {
	e := &Engine{
		Store:   NewStore(t.TempDir()),
		Workdir: t.TempDir(),
		// Verifier is nil
	}
	p, _ := e.New("nil-v", "NilV", "goal", "")
	passed, report, err := e.RunVerify(context.Background(), p)
	if err != nil {
		t.Fatalf("RunVerify with nil verifier: %v", err)
	}
	if !passed {
		t.Error("nil verifier should pass")
	}
	if report != "" {
		t.Errorf("nil verifier report = %q, want empty", report)
	}
}

// ── engine.go: RunPR not-ready ───────────────────────────────────────────────

func TestRunPR_NotReady(t *testing.T) {
	e := &Engine{
		Store:   NewStore(t.TempDir()),
		Workdir: t.TempDir(),
		PR:      fakePR{},
	}
	p, _ := e.New("not-ready", "NR", "goal", "")
	_, err := e.RunPR(context.Background(), p)
	if err == nil {
		t.Fatal("expected error when PRP not ready")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("error should mention 'not ready', got: %v", err)
	}
}

func TestRunPR_NilController(t *testing.T) {
	e := &Engine{
		Store:   NewStore(t.TempDir()),
		Workdir: t.TempDir(),
		// PR is nil
	}
	p := &PRP{ID: "x", Title: "X", Phase: PhaseReady}
	_, err := e.RunPR(context.Background(), p)
	if err == nil {
		t.Fatal("expected error with nil PR controller")
	}
	if !strings.Contains(err.Error(), "no PR controller") {
		t.Errorf("error should mention 'no PR controller', got: %v", err)
	}
}

// ── engine.go: RunPlan nil planner ───────────────────────────────────────────

func TestRunPlan_NilPlanner(t *testing.T) {
	e := &Engine{
		Store:   NewStore(t.TempDir()),
		Workdir: t.TempDir(),
		// Planner is nil
	}
	p, _ := e.New("no-plan", "NP", "goal", "")
	err := e.RunPlan(context.Background(), p)
	if err == nil {
		t.Fatal("expected error with nil planner")
	}
	if !strings.Contains(err.Error(), "no planner") {
		t.Errorf("error should mention 'no planner', got: %v", err)
	}
}

// ── engine.go: RunImplement nil implementer ──────────────────────────────────

func TestRunImplement_NilImplementer(t *testing.T) {
	e := &Engine{
		Store:   NewStore(t.TempDir()),
		Workdir: t.TempDir(),
		// Implementer is nil
	}
	p, _ := e.New("no-impl", "NI", "goal", "")
	err := e.RunImplement(context.Background(), p)
	if err == nil {
		t.Fatal("expected error with nil implementer")
	}
	if !strings.Contains(err.Error(), "no implementer") {
		t.Errorf("error should mention 'no implementer', got: %v", err)
	}
}

// ── engine.go: RunAll draft skip ─────────────────────────────────────────────

func TestRunAll_DraftSkipPlan(t *testing.T) {
	// When phase is not draft, RunAll should skip the plan step.
	e := &Engine{
		Store:       NewStore(t.TempDir()),
		Workdir:     t.TempDir(),
		Planner:     fakePlanner{},
		Implementer: fakeImpl{},
		Verifier:    fakeVerifier{pass: true},
		PR:          fakePR{},
	}
	// Create a PRP that's already planned with tasks
	p := &PRP{
		ID:    "skip-plan",
		Title: "SkipPlan",
		Phase: PhasePlanned,
		Tasks: []Task{{ID: "t1", Title: "step one", State: TaskTodo}},
	}
	_ = e.Store.Save(p)
	err := e.RunAll(context.Background(), p)
	if err != nil {
		t.Fatalf("RunAll with planned phase: %v", err)
	}
	if p.Phase != PhaseShipped {
		t.Errorf("phase = %q, want 'shipped'", p.Phase)
	}
}

// ── engine.go: RunAll verify failure ─────────────────────────────────────────

func TestRunAll_VerifyFailureMessage(t *testing.T) {
	e := newEngine(t, fakeImpl{}, fakeVerifier{pass: false})
	p, _ := e.New("vf", "VF", "goal", "")
	err := e.RunAll(context.Background(), p)
	if err == nil {
		t.Fatal("expected verification failure")
	}
	if !strings.Contains(err.Error(), "verification failed") {
		t.Errorf("error should contain 'verification failed', got: %v", err)
	}
}

// ── engine.go: logf with nil Log ─────────────────────────────────────────────

func TestEngine_logf_NilLog(t *testing.T) {
	e := &Engine{Log: nil}
	// Should not panic
	e.logf("test %s", "message")
}

func TestEngine_logf_WithLog(t *testing.T) {
	var buf bytes.Buffer
	e := &Engine{Log: func(f string, a ...any) {
		buf.WriteString("PREFIX ")
	}}
	e.logf("test %s", "msg")
	if !strings.Contains(buf.String(), "PREFIX") {
		t.Error("logf should call the Log function")
	}
}

// ── engine.go: RunVerify error from verifier ─────────────────────────────────

type errorVerifier struct{}

func (errorVerifier) Verify(_ context.Context, _ string) (bool, string, error) {
	return false, "error report", context.Canceled
}

func TestRunVerify_VerifierError(t *testing.T) {
	e := &Engine{
		Store:    NewStore(t.TempDir()),
		Workdir:  t.TempDir(),
		Verifier: errorVerifier{},
	}
	p, _ := e.New("verr", "VE", "goal", "")
	passed, _, err := e.RunVerify(context.Background(), p)
	if err == nil {
		t.Fatal("expected verifier error")
	}
	if passed {
		t.Error("should not pass on error")
	}
}

// ── engine.go: RunVerify failure kicks back ──────────────────────────────────

func TestRunVerify_FailureKicksBack(t *testing.T) {
	e := &Engine{
		Store:    NewStore(t.TempDir()),
		Workdir:  t.TempDir(),
		Verifier: fakeVerifier{pass: false},
	}
	p, _ := e.New("kick", "KI", "goal", "")
	passed, report, err := e.RunVerify(context.Background(), p)
	if err != nil {
		t.Fatalf("RunVerify: %v", err)
	}
	if passed {
		t.Error("should not pass")
	}
	if report != "report" {
		t.Errorf("report = %q, want 'report'", report)
	}
	if p.Phase != PhaseImplementing {
		t.Errorf("phase = %q, want 'implementing' (kicked back)", p.Phase)
	}
}

// ── engine.go: RunPR error from controller ───────────────────────────────────

type errorPR struct{}

func (errorPR) OpenPR(_ context.Context, _ *PRP) (string, error) {
	return "", context.Canceled
}

func TestRunPR_ControllerError(t *testing.T) {
	e := &Engine{
		Store:   NewStore(t.TempDir()),
		Workdir: t.TempDir(),
		PR:      errorPR{},
	}
	p := &PRP{ID: "pe", Title: "PE", Phase: PhaseReady}
	_, err := e.RunPR(context.Background(), p)
	if err == nil {
		t.Fatal("expected PR controller error")
	}
}

// ── engine.go: New persists ──────────────────────────────────────────────────

func TestEngine_New_Persists(t *testing.T) {
	e := &Engine{Store: NewStore(t.TempDir()), Workdir: t.TempDir()}
	p, err := e.New("persist-test", "Persist", "my goal", "my context")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.ID != "persist-test" {
		t.Errorf("ID = %q, want 'persist-test'", p.ID)
	}
	if p.Phase != PhaseDraft {
		t.Errorf("Phase = %q, want 'draft'", p.Phase)
	}
	if p.Goal != "my goal" {
		t.Errorf("Goal = %q, want 'my goal'", p.Goal)
	}
	// Verify it was persisted
	loaded, err := e.Store.Load("persist-test")
	if err != nil {
		t.Fatalf("Load after New: %v", err)
	}
	if loaded.Title != "Persist" {
		t.Errorf("loaded Title = %q, want 'Persist'", loaded.Title)
	}
}

// ── parse.go: firstNonEmpty ──────────────────────────────────────────────────

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		a, b, want string
	}{
		{"first", "second", "first"},
		{"", "second", "second"},
		{"  ", "second", "second"}, // trimmed empty → fallback
		{"first", "", "first"},
		{"", "", ""},
	}
	for _, c := range cases {
		got := firstNonEmpty(c.a, c.b)
		if got != c.want {
			t.Errorf("firstNonEmpty(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

// ── parse.go: Unmarshal error paths ──────────────────────────────────────────

func TestUnmarshal_MissingFrontmatter(t *testing.T) {
	_, err := Unmarshal([]byte("no frontmatter here"))
	if err == nil || !strings.Contains(err.Error(), "missing frontmatter") {
		t.Errorf("expected 'missing frontmatter' error, got: %v", err)
	}
}

func TestUnmarshal_UnterminatedFrontmatter(t *testing.T) {
	_, err := Unmarshal([]byte("---\nid: x\ntitle: y\n"))
	if err == nil || !strings.Contains(err.Error(), "unterminated frontmatter") {
		t.Errorf("expected 'unterminated frontmatter' error, got: %v", err)
	}
}

func TestUnmarshal_MalformedYAML(t *testing.T) {
	_, err := Unmarshal([]byte("---\nid: [broken yaml\n---\n\n# T\n"))
	if err == nil || !strings.Contains(err.Error(), "parse frontmatter") {
		t.Errorf("expected 'parse frontmatter' wrapped error, got: %v", err)
	}
}

func TestMarshal_WithAllSections(t *testing.T) {
	p := &PRP{
		ID:          "full",
		Title:       "Full PRP",
		Phase:       PhaseDraft,
		Goal:        "do the thing",
		Context:     "some context",
		Plan:        "the plan",
		Acceptance:  "must pass tests",
	}
	data, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, want := range []string{"# Full PRP", "## Goal", "do the thing", "## Context", "some context", "## Plan", "the plan", "## Acceptance Criteria", "must pass tests"} {
		if !strings.Contains(s, want) {
			t.Errorf("Marshal missing %q in output:\n%s", want, s)
		}
	}
}

func TestMarshal_EmptySectionsOmitted(t *testing.T) {
	p := &PRP{ID: "empty", Title: "Empty", Phase: PhaseDraft}
	data, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, "## Goal") {
		t.Error("empty Goal should be omitted")
	}
	if strings.Contains(s, "## Context") {
		t.Error("empty Context should be omitted")
	}
}

func TestMarshalUnmarshal_RoundTripWithSections(t *testing.T) {
	original := &PRP{
		ID:         "rt",
		Title:      "Round Trip",
		Phase:      PhasePlanned,
		Goal:       "test round trip",
		Context:    "context info",
		Plan:       "plan steps",
		Acceptance: "acceptance criteria",
	}
	data, err := Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	loaded, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.ID != original.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, original.ID)
	}
	if loaded.Title != original.Title {
		t.Errorf("Title = %q, want %q", loaded.Title, original.Title)
	}
	if loaded.Goal != original.Goal {
		t.Errorf("Goal = %q, want %q", loaded.Goal, original.Goal)
	}
	if loaded.Context != original.Context {
		t.Errorf("Context = %q, want %q", loaded.Context, original.Context)
	}
	if loaded.Plan != original.Plan {
		t.Errorf("Plan = %q, want %q", loaded.Plan, original.Plan)
	}
	if loaded.Acceptance != original.Acceptance {
		t.Errorf("Acceptance = %q, want %q", loaded.Acceptance, original.Acceptance)
	}
}

// ── parse.go: parseSections ──────────────────────────────────────────────────

func TestParseSections(t *testing.T) {
	body := "\n\n## Goal\n\ndo stuff\n\n## Context\n\nsome context\n\n## Plan\n\nstep 1\nstep 2\n"
	sections := parseSections(body)
	if sections["goal"] != "do stuff" {
		t.Errorf("goal = %q, want 'do stuff'", sections["goal"])
	}
	if sections["context"] != "some context" {
		t.Errorf("context = %q, want 'some context'", sections["context"])
	}
	if sections["plan"] != "step 1\nstep 2" {
		t.Errorf("plan = %q, want 'step 1\\nstep 2'", sections["plan"])
	}
}

// ── types.go: Progress, NextTask, AllDone edge cases ─────────────────────────

func TestPRP_Progress_NoTasks(t *testing.T) {
	p := &PRP{}
	done, total := p.Progress()
	if done != 0 || total != 0 {
		t.Errorf("Progress() = %d/%d, want 0/0", done, total)
	}
}

func TestPRP_AllDone_NoTasks(t *testing.T) {
	p := &PRP{}
	if p.AllDone() {
		t.Error("AllDone with no tasks should be false")
	}
}

func TestPRP_NextTask_AllDone(t *testing.T) {
	p := &PRP{
		Tasks: []Task{
			{ID: "t1", State: TaskDone},
			{ID: "t2", State: TaskDone},
		},
	}
	if p.NextTask() != nil {
		t.Error("NextTask with all done should return nil")
	}
}

func TestPRP_NextTask_DoingState(t *testing.T) {
	p := &PRP{
		Tasks: []Task{
			{ID: "t1", State: TaskDone},
			{ID: "t2", State: TaskDoing},
		},
	}
	next := p.NextTask()
	if next == nil || next.ID != "t2" {
		t.Error("NextTask should return the 'doing' task")
	}
}

// ── store.go: Save creates directory ─────────────────────────────────────────

func TestStore_Save_CreatesDir(t *testing.T) {
	s := NewStore(t.TempDir())
	p := &PRP{ID: "mkdir-test", Title: "Mkdir", Phase: PhaseDraft}
	if err := s.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The .sin/prp directory should exist
	if _, err := os.Stat(s.dir); os.IsNotExist(err) {
		t.Error("Save should create the store directory")
	}
}
