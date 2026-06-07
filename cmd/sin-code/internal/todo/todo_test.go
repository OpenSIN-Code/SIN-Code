// SPDX-License-Identifier: MIT
// Purpose: comprehensive tests for the todo package: Store CRUD, ID generation,
// filtering, dependency cycle detection, audit log, compaction, and queries.
package todo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestGenerateIDFormat(t *testing.T) {
	resetIDState()
	id := GenerateID()
	if !IsValidID(id) {
		t.Errorf("GenerateID produced invalid id: %q", id)
	}
	if !strings.HasPrefix(id, "st-") {
		t.Errorf("expected prefix st-, got %q", id)
	}
	if len(id) != len("st-")+idBodyLen {
		t.Errorf("expected length %d, got %d (id=%q)", len("st-")+idBodyLen, len(id), id)
	}
}

func TestGenerateIDUniqueness(t *testing.T) {
	resetIDState()
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := GenerateID()
		if seen[id] {
			t.Fatalf("duplicate id at i=%d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestIsValidID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"st-aaaa", true},
		{"st-zzzz", true},
		{"st-0123", true},
		{"", false},
		{"st-", false},
		{"st-AAA", false},
		{"xx-aaaa", false},
		{"st-aaa", false},
		{"st-aaaaa", false},
		{"st-aaa!", false},
	}
	for _, c := range cases {
		if got := IsValidID(c.in); got != c.want {
			t.Errorf("IsValidID(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestAddAndGet(t *testing.T) {
	s := tempStore(t)
	td := &Todo{
		Title:    "Test",
		Priority: PriorityP0,
		Type:     TypeFeature,
		Tags:     []string{"a", "b"},
		Project:  "test-proj",
	}
	if err := s.Add(td); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if td.ID == "" {
		t.Fatal("expected ID to be assigned")
	}
	if !IsValidID(td.ID) {
		t.Errorf("invalid ID: %q", td.ID)
	}
	got, err := s.Get(td.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != td.Title || got.Priority != td.Priority || got.Type != td.Type {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if !got.CreatedAt.Equal(td.CreatedAt) {
		t.Errorf("CreatedAt mismatch")
	}
}

func TestAddDefaults(t *testing.T) {
	s := tempStore(t)
	td := &Todo{Title: "Defaults"}
	if err := s.Add(td); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if td.Priority != PriorityP2 {
		t.Errorf("expected default P2, got %q", td.Priority)
	}
	if td.Type != TypeTask {
		t.Errorf("expected default task, got %q", td.Type)
	}
	if td.Status != StatusOpen {
		t.Errorf("expected default open, got %q", td.Status)
	}
}

func TestAddValidation(t *testing.T) {
	s := tempStore(t)
	if err := s.Add(nil); err == nil {
		t.Error("expected error for nil todo")
	}
	if err := s.Add(&Todo{Title: ""}); err == nil {
		t.Error("expected error for empty title")
	}
	if err := s.Add(&Todo{Title: "X", Priority: "P9"}); err == nil {
		t.Error("expected error for invalid priority")
	}
	if err := s.Add(&Todo{Title: "X", Type: "nope"}); err == nil {
		t.Error("expected error for invalid type")
	}
	if err := s.Add(&Todo{Title: "X", Status: "nope"}); err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestUpdate(t *testing.T) {
	s := tempStore(t)
	td := &Todo{Title: "X", Priority: PriorityP2}
	if err := s.Add(td); err != nil {
		t.Fatal(err)
	}
	td.Status = StatusInProgress
	td.Assignee = "alice"
	if err := s.Update(td); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(td.ID)
	if got.Status != StatusInProgress {
		t.Errorf("Status: got %q, want in_progress", got.Status)
	}
	if got.Assignee != "alice" {
		t.Errorf("Assignee: got %q", got.Assignee)
	}
	if !got.UpdatedAt.After(got.CreatedAt) && !got.UpdatedAt.Equal(got.CreatedAt) {
		t.Errorf("UpdatedAt should be >= CreatedAt")
	}
}

func TestUpdateClosesAt(t *testing.T) {
	s := tempStore(t)
	td := &Todo{Title: "Done", Priority: PriorityP2}
	_ = s.Add(td)
	td.Status = StatusDone
	_ = s.Update(td)
	got, _ := s.Get(td.ID)
	if got.ClosedAt == nil {
		t.Error("expected ClosedAt to be set when done")
	}
	td.Status = StatusOpen
	_ = s.Update(td)
	got, _ = s.Get(td.ID)
	if got.ClosedAt != nil {
		t.Error("expected ClosedAt to be nil when reopened")
	}
}

func TestDeleteSoftAndHard(t *testing.T) {
	s := tempStore(t)
	td := &Todo{Title: "Soft", Priority: PriorityP2}
	_ = s.Add(td)
	if err := s.Delete(td.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(td.ID)
	if got.Status != StatusCancelled {
		t.Errorf("soft delete: status = %q, want cancelled", got.Status)
	}
	td2 := &Todo{Title: "Hard", Priority: PriorityP2}
	_ = s.Add(td2)
	if err := s.Delete(td2.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(td2.ID); err == nil {
		t.Error("expected hard-deleted todo to be gone")
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := tempStore(t)
	if err := s.Delete("st-zzzz", true); err == nil {
		t.Error("expected error for non-existent id")
	}
}

func TestListAndListFiltered(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Priority: PriorityP0, Type: TypeBug, Assignee: "alice", Project: "p1", Tags: []string{"x"}})
	_ = s.Add(&Todo{Title: "B", Priority: PriorityP1, Type: TypeFeature, Assignee: "bob", Project: "p2", Tags: []string{"y"}})
	_ = s.Add(&Todo{Title: "C", Priority: PriorityP2, Type: TypeTask, Assignee: "alice", Project: "p1", Tags: []string{"x"}})

	all, _ := s.List()
	if len(all) != 3 {
		t.Errorf("List: got %d, want 3", len(all))
	}

	ts, _ := s.ListFiltered(ListFilter{Priority: PriorityP0})
	if len(ts) != 1 {
		t.Errorf("Priority filter: got %d, want 1", len(ts))
	}

	ts, _ = s.ListFiltered(ListFilter{Assignee: "alice"})
	if len(ts) != 2 {
		t.Errorf("Assignee filter: got %d, want 2", len(ts))
	}

	ts, _ = s.ListFiltered(ListFilter{Project: "p1"})
	if len(ts) != 2 {
		t.Errorf("Project filter: got %d, want 2", len(ts))
	}

	ts, _ = s.ListFiltered(ListFilter{Tag: "x"})
	if len(ts) != 2 {
		t.Errorf("Tag filter: got %d, want 2", len(ts))
	}

	ts, _ = s.ListFiltered(ListFilter{Search: "B"})
	if len(ts) != 1 {
		t.Errorf("Search filter: got %d, want 1", len(ts))
	}
}

func TestListFilteredPrioritySort(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "P2", Priority: PriorityP2})
	_ = s.Add(&Todo{Title: "P0", Priority: PriorityP0})
	_ = s.Add(&Todo{Title: "P1", Priority: PriorityP1})
	ts, _ := s.ListFiltered(ListFilter{})
	if len(ts) != 3 {
		t.Fatalf("got %d", len(ts))
	}
	if ts[0].Priority != PriorityP0 || ts[1].Priority != PriorityP1 || ts[2].Priority != PriorityP2 {
		t.Errorf("expected P0,P1,P2 order, got %v", []Priority{ts[0].Priority, ts[1].Priority, ts[2].Priority})
	}
}

func TestReadyAndBlocked(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A", Priority: PriorityP0}
	b := &Todo{Title: "B", Priority: PriorityP1}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.AddDep(Dependency{From: b.ID, To: a.ID, Type: DepBlocks})

	ready, _ := s.Ready()
	if len(ready) != 1 || ready[0].ID != a.ID {
		t.Errorf("Ready: expected only A, got %+v", titles(ready))
	}
	blocked, _ := s.Blocked()
	if len(blocked) != 1 || blocked[0].ID != b.ID {
		t.Errorf("Blocked: expected only B, got %+v", titles(blocked))
	}
	a.Status = StatusDone
	_ = s.Update(a)
	ready, _ = s.Ready()
	if len(ready) != 1 || ready[0].ID != b.ID {
		t.Errorf("after completing A, expected 1 ready (B), got %d: %v", len(ready), titles(ready))
	}
}

func TestSelfDependency(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A", Priority: PriorityP0}
	_ = s.Add(a)
	if err := s.AddDep(Dependency{From: a.ID, To: a.ID, Type: DepBlocks}); err == nil {
		t.Error("expected error for self-dependency")
	}
}

func TestCycleDetection(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A", Priority: PriorityP0}
	b := &Todo{Title: "B", Priority: PriorityP1}
	c := &Todo{Title: "C", Priority: PriorityP1}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.Add(c)
	if err := s.AddDep(Dependency{From: a.ID, To: b.ID, Type: DepBlocks}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDep(Dependency{From: b.ID, To: c.ID, Type: DepBlocks}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDep(Dependency{From: c.ID, To: a.ID, Type: DepBlocks}); err == nil {
		t.Error("expected cycle detection error")
	}
}

func TestCycleDetect3Node(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A", Priority: PriorityP0}
	b := &Todo{Title: "B", Priority: PriorityP1}
	_ = s.Add(a)
	_ = s.Add(b)
	if err := s.AddDep(Dependency{From: a.ID, To: b.ID, Type: DepBlocks}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDep(Dependency{From: b.ID, To: a.ID, Type: DepBlocks}); err == nil {
		t.Error("expected 2-cycle detection error")
	}
}

func TestGetDepsAndReverse(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A"}
	b := &Todo{Title: "B"}
	c := &Todo{Title: "C"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.Add(c)
	_ = s.AddDep(Dependency{From: b.ID, To: a.ID, Type: DepBlocks})
	_ = s.AddDep(Dependency{From: c.ID, To: a.ID, Type: DepParentChild})
	deps, _ := s.GetDeps(b.ID)
	if len(deps) != 1 {
		t.Errorf("GetDeps: got %d", len(deps))
	}
	rev, _ := s.GetReverseDeps(a.ID)
	if len(rev) != 2 {
		t.Errorf("GetReverseDeps: got %d", len(rev))
	}
}

func TestBlockingDepsOf(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A"}
	b := &Todo{Title: "B"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.AddDep(Dependency{From: b.ID, To: a.ID, Type: DepBlocks})
	_ = s.AddDep(Dependency{From: b.ID, To: a.ID, Type: DepRelated})
	bd, _ := s.BlockingDepsOf(b.ID)
	if len(bd) != 1 {
		t.Errorf("BlockingDepsOf: got %d, want 1", len(bd))
	}
	if bd[0].Type != DepBlocks {
		t.Errorf("expected only blocks-type, got %q", bd[0].Type)
	}
}

func TestDependencyTree(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A"}
	b := &Todo{Title: "B"}
	c := &Todo{Title: "C"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.Add(c)
	_ = s.AddDep(Dependency{From: b.ID, To: a.ID, Type: DepBlocks})
	_ = s.AddDep(Dependency{From: c.ID, To: b.ID, Type: DepBlocks})
	tree, err := s.DependencyTree(c.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 {
		t.Errorf("tree size: got %d, want 3", len(tree))
	}
}

func TestAuditLog(t *testing.T) {
	s := tempStore(t)
	e := AuditEntry{TodoID: "st-test", Action: "create", To: "hello"}
	if err := s.AppendAudit(e); err != nil {
		t.Fatal(err)
	}
	entries, _ := s.ListAudit("st-test")
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	if entries[0].ID == "" {
		t.Error("ID should be auto-assigned")
	}
	if entries[0].Actor == "" {
		t.Error("Actor should default")
	}
}

func TestAppendAuditDefaults(t *testing.T) {
	s := tempStore(t)
	_ = s.AppendAudit(AuditEntry{TodoID: "x", Action: "test"})
	e, _ := s.ListAudit("x")
	if len(e) != 1 {
		t.Fatal("expected 1 entry")
	}
	if e[0].Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestCountAudit(t *testing.T) {
	s := tempStore(t)
	_ = s.AppendAudit(AuditEntry{TodoID: "x", Action: "a"})
	_ = s.AppendAudit(AuditEntry{TodoID: "y", Action: "b"})
	n, _ := s.CountAudit()
	if n != 2 {
		t.Errorf("CountAudit: got %d", n)
	}
}

func TestCompactOldDone(t *testing.T) {
	s := tempStore(t)
	td := &Todo{Title: "Old", Priority: PriorityP2}
	_ = s.Add(td)
	td.Status = StatusDone
	oldTime := time.Now().Add(-100 * time.Hour)
	td.UpdatedAt = oldTime
	td.ClosedAt = &oldTime
	_ = s.Update(td)
	res, _ := s.Compact(CompactOptions{OlderThan: 24 * time.Hour})
	if res.Compacted != 1 {
		t.Errorf("expected 1 compacted, got %d", res.Compacted)
	}
	got, _ := s.Get(td.ID)
	if !got.Compacted {
		t.Error("expected Compacted=true")
	}
	if got.Summary == "" {
		t.Error("expected non-empty Summary")
	}
}

func TestCompactDryRun(t *testing.T) {
	s := tempStore(t)
	td := &Todo{Title: "X", Priority: PriorityP2}
	_ = s.Add(td)
	td.Status = StatusDone
	oldTime := time.Now().Add(-100 * time.Hour)
	td.UpdatedAt = oldTime
	td.ClosedAt = &oldTime
	_ = s.Update(td)
	res, _ := s.Compact(CompactOptions{OlderThan: 24 * time.Hour, DryRun: true})
	if res.Compacted != 1 {
		t.Errorf("dry-run: got %d", res.Compacted)
	}
	got, _ := s.Get(td.ID)
	if got.Compacted {
		t.Error("dry-run should not modify")
	}
}

func TestCompactKeepsRecent(t *testing.T) {
	s := tempStore(t)
	td := &Todo{Title: "Fresh", Priority: PriorityP2}
	_ = s.Add(td)
	td.Status = StatusDone
	_ = s.Update(td)
	res, _ := s.Compact(CompactOptions{OlderThan: 24 * time.Hour})
	if res.Compacted != 0 {
		t.Errorf("expected 0 compacted, got %d", res.Compacted)
	}
}

func TestMemoryAddAndList(t *testing.T) {
	s := tempStore(t)
	_ = s.AddMemory(&Memory{Insight: "use bbolt for embedded storage", Actor: "jeremy"})
	_ = s.AddMemory(&Memory{Insight: "go-internal/testscript for E2E"})
	ms, _ := s.ListMemories()
	if len(ms) != 2 {
		t.Errorf("got %d memories", len(ms))
	}
	if ms[0].ID == "" {
		t.Error("ID should be set")
	}
}

func TestMemoryDefaults(t *testing.T) {
	s := tempStore(t)
	_ = s.AddMemory(&Memory{})
	ms, _ := s.ListMemories()
	if len(ms) != 1 {
		t.Fatal("expected 1")
	}
	if ms[0].Actor != "system" {
		t.Errorf("expected system actor, got %q", ms[0].Actor)
	}
	if ms[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestMine(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Assignee: "alice"})
	_ = s.Add(&Todo{Title: "B", Assignee: "bob"})
	_ = s.Add(&Todo{Title: "C", Assignee: "alice"})
	ms, _ := s.Mine("alice")
	if len(ms) != 2 {
		t.Errorf("expected 2 for alice, got %d", len(ms))
	}
}

func TestByProject(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Project: "p1"})
	_ = s.Add(&Todo{Title: "B", Project: "p2"})
	ps, _ := s.ByProject("p1")
	if len(ps) != 1 {
		t.Errorf("expected 1 for p1, got %d", len(ps))
	}
}

func TestSearch(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "Fix auth bug", Description: "JWT validation fails"})
	_ = s.Add(&Todo{Title: "Add dark mode"})
	ts, err := s.Search("auth")
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Errorf("expected 1, got %d", len(ts))
	}
	if _, err := s.Search(""); err == nil {
		t.Error("expected error for empty query")
	}
}

func TestStats(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Priority: PriorityP0, Type: TypeBug, Assignee: "alice"})
	_ = s.Add(&Todo{Title: "B", Priority: PriorityP1, Type: TypeFeature, Assignee: "bob"})
	_ = s.Add(&Todo{Title: "C", Priority: PriorityP2, Type: TypeTask})
	st, _ := s.ComputeStats()
	if st.Total != 3 {
		t.Errorf("Total: got %d", st.Total)
	}
	if st.ByStatus["open"] != 3 {
		t.Errorf("ByStatus[open]: got %d", st.ByStatus["open"])
	}
	if st.ByPriority["P0"] != 1 {
		t.Errorf("ByPriority[P0]: got %d", st.ByPriority["P0"])
	}
	if st.ByType["bug"] != 1 {
		t.Errorf("ByType[bug]: got %d", st.ByType["bug"])
	}
	if st.ByAssignee["alice"] != 1 {
		t.Errorf("ByAssignee[alice]: got %d", st.ByAssignee["alice"])
	}
}

func TestStatsReadyAndBlocked(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A"}
	b := &Todo{Title: "B"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.AddDep(Dependency{From: b.ID, To: a.ID, Type: DepBlocks})
	st, _ := s.ComputeStats()
	if st.Ready != 1 {
		t.Errorf("Ready: got %d", st.Ready)
	}
	if st.Blocked != 1 {
		t.Errorf("Blocked: got %d", st.Blocked)
	}
}

func TestMeta(t *testing.T) {
	s := tempStore(t)
	if err := s.SetMeta("k", "v"); err != nil {
		t.Fatal(err)
	}
	v, _ := s.GetMeta("k")
	if v != "v" {
		t.Errorf("got %q", v)
	}
	empty, _ := s.GetMeta("nonexistent")
	if empty != "" {
		t.Errorf("expected empty for missing key, got %q", empty)
	}
}

func TestIndexKeys(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A", Assignee: "alice", Project: "p1", Tags: []string{"x", "y"}}
	b := &Todo{Title: "B", Assignee: "alice", Project: "p2", Tags: []string{"x"}}
	_ = s.Add(a)
	_ = s.Add(b)
	ids, _ := s.IndexKeys(bucketIdxAs, "alice")
	if len(ids) != 2 {
		t.Errorf("expected 2 for alice, got %d", len(ids))
	}
	ids, _ = s.IndexKeys(bucketIdxTg, "x")
	if len(ids) != 2 {
		t.Errorf("expected 2 for tag x, got %d", len(ids))
	}
}

func TestUpdateRemovesOldIndex(t *testing.T) {
	s := tempStore(t)
	td := &Todo{Title: "A", Assignee: "alice"}
	_ = s.Add(td)
	td.Assignee = "bob"
	_ = s.Update(td)
	ids, _ := s.IndexKeys(bucketIdxAs, "alice")
	if len(ids) != 0 {
		t.Errorf("alice should be empty after reassign, got %d", len(ids))
	}
	ids, _ = s.IndexKeys(bucketIdxAs, "bob")
	if len(ids) != 1 {
		t.Errorf("bob should have 1, got %d", len(ids))
	}
}

func TestOpenEmptyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open with empty: %v", err)
	}
	defer s.Close()
	if s.Path() == "" {
		t.Error("Path should not be empty")
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Errorf("file should exist: %v", err)
	}
}

func TestOpenInvalidDir(t *testing.T) {
	_, err := Open("/nonexistent/dir/todo.db")
	if err == nil {
		t.Error("expected error for invalid dir")
	}
}

func TestSplitList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b ,c", []string{"a", "b", "c"}},
		{",,a,,", []string{"a"}},
	}
	for _, c := range cases {
		got := splitList(c.in)
		if !equalSlices(got, c.want) {
			t.Errorf("splitList(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSummarize(t *testing.T) {
	td := &Todo{Title: "Fix bug", Priority: PriorityP0, Assignee: "alice", Tags: []string{"urgent"}}
	got := summarize(td)
	if !strings.Contains(got, "Fix bug") || !strings.Contains(got, "P0") {
		t.Errorf("unexpected summary: %q", got)
	}
}

func TestExportMarkdownPath(t *testing.T) {
	td := &Todo{Title: "X", Priority: PriorityP0, Type: TypeTask, Status: StatusOpen, Assignee: "alice", Tags: []string{"a"}}
	md := exportMarkdown([]*Todo{td})
	for _, want := range []string{"X", "P0", "task", "alice", "a"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q: %s", want, md)
		}
	}
}

func TestPriorityRank(t *testing.T) {
	ranks := map[Priority]int{PriorityP0: 0, PriorityP1: 1, PriorityP2: 2, PriorityP3: 3}
	for p, want := range ranks {
		if p.Rank() != want {
			t.Errorf("Rank(%q) = %d, want %d", p, p.Rank(), want)
		}
	}
	if Priority("P9").Rank() != 99 {
		t.Error("unknown priority should rank 99")
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusOpen, StatusInProgress, StatusDone, StatusCancelled, StatusBlocked} {
		if !s.Valid() {
			t.Errorf("expected %q valid", s)
		}
	}
	if Status("nope").Valid() {
		t.Error("expected 'nope' invalid")
	}
}

func TestDepTypeValid(t *testing.T) {
	for _, d := range []DepType{DepBlocks, DepParentChild, DepRelated, DepDiscoveredFrom, DepDuplicates, DepSupersedes} {
		if !d.Valid() {
			t.Errorf("expected %q valid", d)
		}
	}
	if !DepBlocks.IsBlocking() {
		t.Error("DepBlocks should be blocking")
	}
	if DepRelated.IsBlocking() {
		t.Error("DepRelated should not be blocking")
	}
}

func TestIsOpenAndIsClosed(t *testing.T) {
	open := []Status{StatusOpen, StatusInProgress, StatusBlocked}
	closed := []Status{StatusDone, StatusCancelled}
	for _, s := range open {
		td := &Todo{Status: s}
		if !td.IsOpen() || td.IsClosed() {
			t.Errorf("status %q: open=%v closed=%v", s, td.IsOpen(), td.IsClosed())
		}
	}
	for _, s := range closed {
		td := &Todo{Status: s}
		if td.IsOpen() || !td.IsClosed() {
			t.Errorf("status %q: open=%v closed=%v", s, td.IsOpen(), td.IsClosed())
		}
	}
}

func TestRemoveDep(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A"}
	b := &Todo{Title: "B"}
	_ = s.Add(a)
	_ = s.Add(b)
	_ = s.AddDep(Dependency{From: b.ID, To: a.ID, Type: DepBlocks})
	if err := s.RemoveDep(b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	deps, _ := s.GetDeps(b.ID)
	if len(deps) != 0 {
		t.Errorf("expected 0 deps after remove, got %d", len(deps))
	}
}

func TestAddDepMissingTodo(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A"}
	_ = s.Add(a)
	if err := s.AddDep(Dependency{From: a.ID, To: "st-zzzz", Type: DepBlocks}); err == nil {
		t.Error("expected error for missing to-todo")
	}
}

func TestAddDepInvalidType(t *testing.T) {
	s := tempStore(t)
	a := &Todo{Title: "A"}
	b := &Todo{Title: "B"}
	_ = s.Add(a)
	_ = s.Add(b)
	if err := s.AddDep(Dependency{From: a.ID, To: b.ID, Type: "nope"}); err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestPath(t *testing.T) {
	s := tempStore(t)
	if s.Path() == "" {
		t.Error("Path should be non-empty")
	}
}

func TestCloseIdempotent(t *testing.T) {
	s := tempStore(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestListFilteredSearchTitleAndDesc(t *testing.T) {
	s := tempStore(t)
	_ = s.Add(&Todo{Title: "A", Description: "has a needle in body"})
	_ = s.Add(&Todo{Title: "B with needle", Description: "no match"})
	ts, _ := s.ListFiltered(ListFilter{Search: "needle"})
	if len(ts) != 2 {
		t.Errorf("expected 2 matches, got %d", len(ts))
	}
}

func titles(ts []*Todo) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Title
	}
	return out
}

func equalSlices(a, b []string) bool {
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
