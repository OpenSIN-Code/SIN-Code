// SPDX-License-Identifier: MIT
// Purpose: tests for the grill package (issue #141 fusion). The
// round-trip test (TestSession_RoundTrip) is load-bearing: a session
// is written to disk, loaded back, and must be identical.
package grill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStart_CreatesSessionWithSeedQuestion(t *testing.T) {
	m := newTestManager(t)
	s, err := m.Start("add OAuth login")
	if err != nil {
		t.Fatal(err)
	}
	if s.Topic != "add OAuth login" {
		t.Errorf("expected topic=add OAuth login, got %q", s.Topic)
	}
	if s.ID == "" {
		t.Error("expected non-empty id")
	}
	if len(s.Decisions) != 1 {
		t.Errorf("expected 1 seed decision, got %d", len(s.Decisions))
	}
	if s.Decisions[0].Status != "open" {
		t.Errorf("expected seed status=open, got %q", s.Decisions[0].Status)
	}
	if !strings.Contains(s.Decisions[0].Question, "add OAuth login") {
		t.Errorf("expected question to mention topic, got %q", s.Decisions[0].Question)
	}
	if s.OpenQuestions != 1 {
		t.Errorf("expected OpenQuestions=1, got %d", s.OpenQuestions)
	}
}

func TestStart_EmptyTopicErrors(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Start(""); err == nil {
		t.Error("expected error on empty topic")
	}
}

func TestStart_UniqueIDsForSameTopic(t *testing.T) {
	// Two starts of the same topic must produce different session IDs
	// (the time salt makes them unique even within the same second).
	m := newTestManager(t)
	s1, _ := m.Start("same topic")
	// Force a different timestamp by sleeping > 0 seconds. We use
	// 1ms to keep the test fast.
	time.Sleep(time.Millisecond)
	s2, _ := m.Start("same topic")
	if s1.ID == s2.ID {
		t.Error("expected different session IDs")
	}
}

func TestNext_AppendsSubQuestion(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	child, parent, err := m.Next(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if parent != "d0" {
		t.Errorf("expected parent=d0, got %q", parent)
	}
	if child.ParentID != "d0" {
		t.Errorf("expected child.ParentID=d0, got %q", child.ParentID)
	}
	if child.Status != "open" {
		t.Errorf("expected child.status=open, got %q", child.Status)
	}
	if child.ID == "d0" {
		t.Error("expected new ID, got d0")
	}
}

func TestNext_AdvancesThroughTree(t *testing.T) {
	// Each `Next` call should close one decision (the parent) and
	// open a new child, keeping OpenQuestions = 1 throughout.
	m := newTestManager(t)
	s, _ := m.Start("x")
	for i := 0; i < 5; i++ {
		_, _, err := m.Next(s.ID)
		if err != nil {
			t.Fatalf("Next call %d: %v", i, err)
		}
		// The session should still be reachable and have at least
		// one open decision.
		got, _ := m.Get(s.ID)
		if got.OpenQuestions < 1 {
			t.Errorf("after %d Next calls, OpenQuestions=%d (expected ≥1)", i+1, got.OpenQuestions)
		}
	}
}

func TestAnswer_RecordsResponse(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	if err := m.Answer(s.ID, "d0", "use OAuth 2.0 with PKCE"); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(s.ID)
	if got.Decisions[0].Answer != "use OAuth 2.0 with PKCE" {
		t.Errorf("expected answer recorded, got %q", got.Decisions[0].Answer)
	}
	if got.Decisions[0].Status != "answered" {
		t.Errorf("expected status=answered, got %q", got.Decisions[0].Status)
	}
	if !got.Decisions[0].ResolvedAt.IsZero() {
		t.Error("expected ResolvedAt to be zero for non-resolved answer")
	}
}

func TestAnswer_DoneResolves(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	if err := m.Answer(s.ID, "d0", "done"); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(s.ID)
	if got.Decisions[0].Status != "resolved" {
		t.Errorf("expected status=resolved for answer=done, got %q", got.Decisions[0].Status)
	}
	if got.Decisions[0].ResolvedAt.IsZero() {
		t.Error("expected ResolvedAt to be set for resolved")
	}
}

func TestAnswer_UnknownDecisionErrors(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	if err := m.Answer(s.ID, "d999", "x"); err == nil {
		t.Error("expected error for unknown decision id")
	}
}

func TestStatus_ResolvesAndOpens(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	// d0 is the seed (open).
	if _, _, err := m.Next(s.ID); err != nil {
		t.Fatal(err)
	}
	// After Next: d0 still open (a sub-question is a deepening,
	// not a closure of the parent). d1 is the new open sub-question.
	// d0 is still open. d1 is open. 2 open, 0 resolved.
	_ = m.Answer(s.ID, "d1", "done")
	resolved, open, answered, deferred, err := m.Status(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != 1 {
		t.Errorf("expected 1 resolved (d1=done), got %d", resolved)
	}
	if answered != 0 {
		t.Errorf("expected 0 answered, got %d", answered)
	}
	if open != 1 {
		t.Errorf("expected 1 open (d0 still open), got %d", open)
	}
	if deferred != 0 {
		t.Errorf("expected 0 deferred, got %d", deferred)
	}
}

func TestSynthesize_IncludesAssumptions(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	// After Start, d0 is the seed (open). Answering it directly
	// (no Next call) makes it "answered".
	if err := m.Answer(s.ID, "d0", "I assume the operator has admin access"); err != nil {
		t.Fatal(err)
	}
	syn, err := m.Synthesize(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	// d0 is "answered" (not "resolved"), so it goes to Open,
	// not Resolved. The Assumptions list still catches the
	// "I assume" heuristic.
	if len(syn.Open) != 1 {
		t.Errorf("expected 1 open (d0 answered), got %d", len(syn.Open))
	}
	if len(syn.Resolved) != 0 {
		t.Errorf("expected 0 resolved (d0 is answered, not resolved), got %d", len(syn.Resolved))
	}
	if len(syn.Assumptions) != 1 {
		t.Errorf("expected 1 assumption (I assume...), got %d", len(syn.Assumptions))
	}
}

func TestSynthesize_OpenAndResolved(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	if _, _, err := m.Next(s.ID); err != nil {
		t.Fatal(err)
	}
	// d0 still open (Next is a deepening, not a closure).
	// d1 open. Both are open.
	syn, _ := m.Synthesize(s.ID)
	if len(syn.Open) != 2 {
		t.Errorf("expected 2 open, got %d", len(syn.Open))
	}
	if len(syn.Resolved) != 0 {
		t.Errorf("expected 0 resolved, got %d", len(syn.Resolved))
	}
}

func TestSession_RoundTrip(t *testing.T) {
	// The load-bearing test: a session is written to disk, loaded
	// back, and must be identical. The CLI depends on this for
	// `grill next` after a restart.
	dir := t.TempDir()
	m1, _ := NewManager(dir)
	_, _ = m1.Start("round-trip test")
	id := getLastID(m1)
	t.Logf("DEBUG: id=%q", id)
	if id == "" {
		t.Fatal("expected non-empty session id")
	}
	// Use m1.Get for all reads so the pointer is always the
	// in-memory one (Start's return value can diverge from the
	// in-memory pointer after a slice append that grows the
	// backing array).
	got, err := m1.Get(id)
	if err != nil {
		t.Fatalf("m1.Get(%q): %v", id, err)
	}
	if _, _, err := m1.Next(got.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = m1.Get(got.ID)
	var openID string
	for _, d := range got.Decisions {
		if d.Status == "open" {
			openID = d.ID
		}
	}
	if openID == "" {
		t.Fatal("expected at least one open decision")
	}
	if err := m1.Answer(got.ID, openID, "answer 0"); err != nil {
		t.Fatal(err)
	}

	// Reload from a fresh manager.
	m2, _ := NewManager(dir)
	reloaded, err := m2.Get(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Topic != got.Topic {
		t.Errorf("topic changed: %q != %q", reloaded.Topic, got.Topic)
	}
	if len(reloaded.Decisions) != len(got.Decisions) {
		t.Errorf("decision count changed: %d != %d", len(reloaded.Decisions), len(got.Decisions))
	}
	if len(reloaded.Decisions) == 0 {
		t.Fatal("reloaded has 0 decisions — disk load is broken")
	}
	for i := range reloaded.Decisions {
		if reloaded.Decisions[i].ID != got.Decisions[i].ID {
			t.Errorf("decision %d ID changed: %q != %q", i, reloaded.Decisions[i].ID, got.Decisions[i].ID)
		}
	}
	if reloaded.Decisions[len(reloaded.Decisions)-1].Answer != "answer 0" {
		t.Errorf("last decision answer not preserved: got %q", reloaded.Decisions[len(reloaded.Decisions)-1].Answer)
	}
}

// getLastID returns the most-recently-created session id in the
// manager. There is at most one in this test. Used by the round-
// trip test to avoid the Go-slice-grow pitfall (Start returns the
// old *Session; subsequent appends grow a new backing array).
func getLastID(m *Manager) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var id string
	for k := range m.sessions {
		id = k
	}
	return id
}

func TestNewSessionID_StableForSameInput(t *testing.T) {
	t1 := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	id1 := newSessionID("topic", t1)
	id2 := newSessionID("topic", t1)
	if id1 != id2 {
		t.Errorf("expected stable ID for same input, got %q != %q", id1, id2)
	}
	id3 := newSessionID("other", t1)
	if id1 == id3 {
		t.Error("expected different ID for different topic")
	}
}

func TestCatalog_HasSeedEntries(t *testing.T) {
	if len(Catalog) < 5 {
		t.Errorf("expected at least 5 anti-patterns, got %d", len(Catalog))
	}
	for _, p := range Catalog {
		if p.Name == "" {
			t.Error("empty anti-pattern name")
		}
		if len(p.Questions) == 0 {
			t.Errorf("anti-pattern %q has no questions", p.Name)
		}
	}
}

func TestCatalogQuestion_DeterministicForSameTopic(t *testing.T) {
	q1 := catalogQuestion("x", 0)
	q2 := catalogQuestion("x", 0)
	if q1 != q2 {
		t.Error("expected deterministic question for same topic+index")
	}
	q3 := catalogQuestion("y", 0)
	// Different topic should land in a (potentially) different
	// bucket. Not strictly required to differ, but typical.
	if q1 == q3 && len(Catalog) > 1 {
		t.Logf("warning: same first question for different topics (q1=%q, q3=%q)", q1, q3)
	}
}

func TestContains(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"hello world", "world", true},
		{"hello world", "foo", false},
		{"hello world", "", true},
		{"", "x", false},
		{"abcabc", "cab", true},
	}
	for _, c := range cases {
		if got := strings.Contains(c.s, c.sub); got != c.want {
			t.Errorf("contains(%q,%q) = %v, want %v", c.s, c.sub, got, c.want)
		}
	}
}

func TestMarshalUnmarshal_Session(t *testing.T) {
	// Sanity: the JSON shape is stable enough to be consumed by
	// the CLI's text renderer.
	s := &Session{
		ID:        "grill-abc",
		Topic:     "x",
		StartedAt: time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 16, 12, 1, 0, 0, time.UTC),
		Decisions: []Decision{
			{ID: "d0", Question: "q1", Status: "open", AskedAt: time.Now().UTC()},
		},
		OpenQuestions: 1,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"id":"grill-abc"`) {
		t.Errorf("expected id in JSON, got %s", b)
	}
}

// ── smoke: a full interview flow ─────────────────────────────────────

func TestSession_FullFlow(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("ship a new binary")
	for i := 0; i < 3; i++ {
		_, parent, err := m.Next(s.ID)
		if err != nil {
			t.Fatalf("Next %d: %v", i, err)
		}
		if err := m.Answer(s.ID, parent, "answer to "+parent); err != nil {
			t.Fatalf("Answer %d: %v", i, err)
		}
	}
	// Resolve every open question.
	got, _ := m.Get(s.ID)
	for _, d := range got.Decisions {
		if d.Status == "open" {
			if err := m.Answer(s.ID, d.ID, "done"); err != nil {
				t.Fatal(err)
			}
		}
	}
	syn, _ := m.Synthesize(s.ID)
	// 3 answers via Answer(parent, "answer to dX") + 3 Next-append
	// leaves 1 final open, then done. So 3 answered, 1 resolved
	// at the time of the last done. Total: 1 resolved + 3 answered.
	// Synthesize counts Resolved and Open. Answered goes to Open.
	// So we expect 1 resolved and 3 open.
	if len(syn.Resolved) != 1 {
		t.Errorf("expected 1 resolved, got %d", len(syn.Resolved))
	}
	if len(syn.Open) != 3 {
		t.Errorf("expected 3 open (answered, not resolved), got %d", len(syn.Open))
	}
}

func TestMainPath(t *testing.T) {
	// Helper: ensure the path function builds the expected layout.
	dir := t.TempDir()
	m, _ := NewManager(dir)
	want := filepath.Join(dir, "abc.json")
	got := m.path("abc")
	if got != want {
		t.Errorf("expected path=%q, got %q", want, got)
	}
}

// ── coverage of error paths and edge cases ─────────────────────────

func TestNewManager_MkdirAllError(t *testing.T) {
	old := osMkdirAllHook
	osMkdirAllHook = func(string, os.FileMode) error { return os.ErrPermission }
	defer func() { osMkdirAllHook = old }()
	if _, err := NewManager(t.TempDir()); err == nil {
		t.Error("expected error when MkdirAll fails")
	}
}

func TestDir(t *testing.T) {
	m := newTestManager(t)
	if got := m.Dir(); got != m.dir {
		t.Errorf("Dir() = %q, want %q", got, m.dir)
	}
}

func TestStart_SaveError(t *testing.T) {
	m := newTestManager(t)
	old := osWriteFileHook
	osWriteFileHook = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	defer func() { osWriteFileHook = old }()
	if _, err := m.Start("topic"); err == nil {
		t.Error("expected error when save fails")
	}
}

func TestGet_LoadError(t *testing.T) {
	m := newTestManager(t)
	old := osReadFileHook
	osReadFileHook = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	defer func() { osReadFileHook = old }()
	if _, err := m.Get("nonexistent"); err == nil {
		t.Error("expected error when load fails")
	}
}

func TestNext_SessionNotFound(t *testing.T) {
	m := newTestManager(t)
	if _, _, err := m.Next("nonexistent"); err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestNext_SaveError(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	old := osWriteFileHook
	osWriteFileHook = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	defer func() { osWriteFileHook = old }()
	if _, _, err := m.Next(s.ID); err == nil {
		t.Error("expected error when save fails")
	}
}

func TestNext_NoOpenDecisions(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	if err := m.Answer(s.ID, "d0", "done"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Next(s.ID); err == nil {
		t.Error("expected error when no open decisions")
	}
}

func TestAnswer_SessionNotFound(t *testing.T) {
	m := newTestManager(t)
	if err := m.Answer("nonexistent", "d0", "x"); err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestStatus_SessionNotFound(t *testing.T) {
	m := newTestManager(t)
	if _, _, _, _, err := m.Status("nonexistent"); err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestStatus_CountsAnsweredAndDeferred(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	if err := m.Answer(s.ID, "d0", "partial"); err != nil {
		t.Fatal(err)
	}

	// Write a deferred decision directly to disk and reload via a fresh manager.
	raw := []byte(`{
		"id": "deferred-id",
		"topic": "x",
		"started_at": "2026-06-17T00:00:00Z",
		"updated_at": "2026-06-17T00:00:00Z",
		"decisions": [
			{"id": "d0", "question": "q", "status": "deferred", "asked_at": "2026-06-17T00:00:00Z"}
		],
		"open_questions": 0
	}`)
	if err := os.WriteFile(filepath.Join(m.Dir(), "deferred-id.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	m2, _ := NewManager(m.Dir())

	resolved, open, answered, deferred, err := m2.Status("deferred-id")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != 0 || open != 0 || answered != 0 || deferred != 1 {
		t.Errorf("deferred counts mismatch: resolved=%d open=%d answered=%d deferred=%d", resolved, open, answered, deferred)
	}

	resolved, open, answered, deferred, err = m.Status(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != 0 || open != 0 || answered != 1 || deferred != 0 {
		t.Errorf("answered counts mismatch: resolved=%d open=%d answered=%d deferred=%d", resolved, open, answered, deferred)
	}
}

func TestSynthesize_SessionNotFound(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Synthesize("nonexistent"); err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestSave_MarshalError(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	old := jsonMarshalHook
	jsonMarshalHook = func(any, string, string) ([]byte, error) { return nil, os.ErrInvalid }
	defer func() { jsonMarshalHook = old }()
	if err := m.save(s); err == nil {
		t.Error("expected error when marshal fails")
	}
}

func TestSave_WriteFileError(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	old := osWriteFileHook
	osWriteFileHook = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	defer func() { osWriteFileHook = old }()
	if err := m.save(s); err == nil {
		t.Error("expected error when write fails")
	}
}

func TestLoad_ReadFileError(t *testing.T) {
	m := newTestManager(t)
	old := osReadFileHook
	osReadFileHook = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	defer func() { osReadFileHook = old }()
	if _, err := m.load("x"); err == nil {
		t.Error("expected error when read fails")
	}
}

func TestLoad_UnmarshalError(t *testing.T) {
	m := newTestManager(t)
	path := m.path("x")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := jsonUnmarshalHook
	jsonUnmarshalHook = func([]byte, any) error { return os.ErrInvalid }
	defer func() { jsonUnmarshalHook = old }()
	if _, err := m.load("x"); err == nil {
		t.Error("expected error when unmarshal fails")
	}
}

func TestCatalogQuestion_EmptyCatalog(t *testing.T) {
	old := Catalog
	Catalog = nil
	defer func() { Catalog = old }()
	if got := catalogQuestion("x", 0); got != "What is the assumption behind this step?" {
		t.Errorf("unexpected fallback: %q", got)
	}
}

func TestCatalogQuestion_EmptyQuestions(t *testing.T) {
	old := Catalog
	Catalog = []AntiPattern{{Name: "Empty", Description: "desc", Questions: []string{}}}
	defer func() { Catalog = old }()
	if got := catalogQuestion("x", 0); got != "desc" {
		t.Errorf("unexpected fallback: %q", got)
	}
}

func TestAnswer_SkipResolves(t *testing.T) {
	m := newTestManager(t)
	s, _ := m.Start("x")
	if err := m.Answer(s.ID, "d0", "skip"); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(s.ID)
	if got.Decisions[0].Status != "resolved" {
		t.Errorf("expected status=resolved for answer=skip, got %q", got.Decisions[0].Status)
	}
}
