// SPDX-License-Identifier: MIT
package gsd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitProject(t *testing.T) {
	dir := t.TempDir()
	if err := InitProject(dir, "TestProject", "A test project"); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	for _, f := range []string{".gsd/PROJECT.md", ".gsd/ROADMAP.md", ".gsd/STATE.md"} {
		p := filepath.Join(dir, f)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %s to exist: %v", f, err)
		}
	}

	proj, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject failed: %v", err)
	}
	if proj.Name != "TestProject" {
		t.Errorf("expected name TestProject, got %s", proj.Name)
	}
	if proj.Description != "A test project" {
		t.Errorf("expected description 'A test project', got %s", proj.Description)
	}
}

func TestAddPhase(t *testing.T) {
	dir := t.TempDir()
	if err := InitProject(dir, "P", "D"); err != nil {
		t.Fatal(err)
	}

	phases := []struct {
		title    string
		priority string
		wantID   string
	}{
		{"Setup", PriorityP0, "1"},
		{"Build", PriorityP1, "2"},
		{"Deploy", PriorityP2, "3"},
	}

	for _, tc := range phases {
		p, err := AddPhase(dir, tc.title, tc.priority)
		if err != nil {
			t.Fatalf("AddPhase(%s) failed: %v", tc.title, err)
		}
		if p.ID != tc.wantID {
			t.Errorf("expected ID %s, got %s", tc.wantID, p.ID)
		}
		if p.Priority != tc.priority {
			t.Errorf("expected priority %s, got %s", tc.priority, p.Priority)
		}
		if p.Status != StatusPlanning {
			t.Errorf("expected status planning, got %s", p.Status)
		}
	}

	list, err := ListPhases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(list))
	}

	content, _ := readFile(roadmapPath(dir))
	if !strings.Contains(content, "## Phase 1: Setup [P0] [planning]") {
		t.Error("ROADMAP.md missing phase 1 line")
	}
	if !strings.Contains(content, "## Phase 3: Deploy [P2] [planning]") {
		t.Error("ROADMAP.md missing phase 3 line")
	}
}

func TestInsertPhase(t *testing.T) {
	dir := t.TempDir()
	if err := InitProject(dir, "P", "D"); err != nil {
		t.Fatal(err)
	}

	if _, err := AddPhase(dir, "Alpha", PriorityP0); err != nil {
		t.Fatal(err)
	}
	if _, err := AddPhase(dir, "Beta", PriorityP1); err != nil {
		t.Fatal(err)
	}

	inserted, err := InsertPhase(dir, "1", "AlphaBeta", PriorityP2)
	if err != nil {
		t.Fatalf("InsertPhase failed: %v", err)
	}

	if !strings.Contains(inserted.ID, ".") {
		t.Errorf("expected decimal ID, got %s", inserted.ID)
	}
	if !strings.HasPrefix(inserted.ID, "1.") {
		t.Errorf("expected ID starting with 1., got %s", inserted.ID)
	}

	list, err := ListPhases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 phases after insert, got %d", len(list))
	}

	found := false
	for _, p := range list {
		if p.ID == inserted.ID && p.Title == "AlphaBeta" {
			found = true
		}
	}
	if !found {
		t.Error("inserted phase not found in list")
	}
}

func TestRemovePhase(t *testing.T) {
	dir := t.TempDir()
	if err := InitProject(dir, "P", "D"); err != nil {
		t.Fatal(err)
	}

	for _, title := range []string{"A", "B", "C"} {
		if _, err := AddPhase(dir, title, PriorityP2); err != nil {
			t.Fatal(err)
		}
	}

	if err := RemovePhase(dir, "2"); err != nil {
		t.Fatalf("RemovePhase failed: %v", err)
	}

	list, err := ListPhases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 phases after remove, got %d", len(list))
	}

	if list[0].ID != "1" {
		t.Errorf("expected first phase ID 1, got %s", list[0].ID)
	}
	if list[1].ID != "2" {
		t.Errorf("expected second phase ID 2 (renumbered), got %s", list[1].ID)
	}
	if list[1].Title != "C" {
		t.Errorf("expected second phase title C, got %s", list[1].Title)
	}
}

func TestEditPhase(t *testing.T) {
	dir := t.TempDir()
	if err := InitProject(dir, "P", "D"); err != nil {
		t.Fatal(err)
	}

	if _, err := AddPhase(dir, "Original", PriorityP2); err != nil {
		t.Fatal(err)
	}

	if err := EditPhase(dir, "1", EditOpts{Title: "Renamed", Priority: PriorityP0}); err != nil {
		t.Fatalf("EditPhase failed: %v", err)
	}

	p, err := GetPhase(dir, "1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Renamed" {
		t.Errorf("expected title Renamed, got %s", p.Title)
	}
	if p.Priority != PriorityP0 {
		t.Errorf("expected priority P0, got %s", p.Priority)
	}

	if err := EditPhase(dir, "1", EditOpts{Status: StatusInProgress}); err != nil {
		t.Fatal(err)
	}
	p, _ = GetPhase(dir, "1")
	if p.Status != StatusInProgress {
		t.Errorf("expected status in-progress, got %s", p.Status)
	}
}

func TestSaveLoadPlan(t *testing.T) {
	dir := t.TempDir()
	if err := InitProject(dir, "P", "D"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddPhase(dir, "TestPhase", PriorityP0); err != nil {
		t.Fatal(err)
	}

	tasks := []Task{
		{ID: "T1", Description: "Setup DB", Effort: EffortSmall, Validation: "db exists"},
		{ID: "T2", Description: "Build API", Effort: EffortMedium, Dependencies: []string{"T1"}, Validation: "api responds"},
		{ID: "T3", Description: "Write tests", Effort: EffortLarge, Dependencies: []string{"T2"}, Validation: "tests pass"},
	}

	if err := SavePlan(dir, "1", tasks); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}

	if !PlanExists(dir, "1") {
		t.Error("PlanExists should return true")
	}

	loaded, err := LoadPlan(dir, "1")
	if err != nil {
		t.Fatalf("LoadPlan failed: %v", err)
	}
	if len(loaded.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(loaded.Tasks))
	}

	if loaded.Tasks[0].ID != "T1" {
		t.Errorf("expected T1, got %s", loaded.Tasks[0].ID)
	}
	if loaded.Tasks[0].Description != "Setup DB" {
		t.Errorf("expected description 'Setup DB', got %s", loaded.Tasks[0].Description)
	}
	if loaded.Tasks[0].Effort != EffortSmall {
		t.Errorf("expected effort S, got %s", loaded.Tasks[0].Effort)
	}

	if len(loaded.Tasks[1].Dependencies) != 1 || loaded.Tasks[1].Dependencies[0] != "T1" {
		t.Errorf("expected T2 to depend on T1, got %v", loaded.Tasks[1].Dependencies)
	}
	if loaded.Tasks[1].Validation != "api responds" {
		t.Errorf("expected validation 'api responds', got %s", loaded.Tasks[1].Validation)
	}

	if len(loaded.Tasks[2].Dependencies) != 1 || loaded.Tasks[2].Dependencies[0] != "T2" {
		t.Errorf("expected T3 to depend on T2, got %v", loaded.Tasks[2].Dependencies)
	}
}

func TestAnalyzeWaves(t *testing.T) {
	plan := &Plan{
		PhaseID: "1",
		Tasks: []Task{
			{ID: "T1", Description: "no deps"},
			{ID: "T2", Description: "depends T1", Dependencies: []string{"T1"}},
			{ID: "T3", Description: "no deps"},
			{ID: "T4", Description: "depends T2", Dependencies: []string{"T2"}},
			{ID: "T5", Description: "depends T1 T3", Dependencies: []string{"T1", "T3"}},
		},
	}

	waves := AnalyzeWaves(plan)

	if len(waves) < 3 {
		t.Fatalf("expected at least 3 waves, got %d", len(waves))
	}

	wave0IDs := taskIDs(waves[0])
	if !contains(wave0IDs, "T1") || !contains(wave0IDs, "T3") {
		t.Errorf("wave 0 should contain T1 and T3, got %v", wave0IDs)
	}

	for _, w := range waves {
		ids := taskIDs(w)
		if contains(ids, "T2") && contains(ids, "T1") {
			t.Error("T1 and T2 should not be in same wave")
		}
		if contains(ids, "T4") && contains(ids, "T2") {
			t.Error("T2 and T4 should not be in same wave")
		}
	}

	totalTasks := 0
	for _, w := range waves {
		totalTasks += len(w)
	}
	if totalTasks != 5 {
		t.Errorf("expected 5 total tasks across waves, got %d", totalTasks)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	dir := t.TempDir()
	if err := InitProject(dir, "P", "D"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddPhase(dir, "Phase", PriorityP0); err != nil {
		t.Fatal(err)
	}

	tasks := []Task{
		{ID: "T1", Description: "task 1", Effort: EffortSmall},
		{ID: "T2", Description: "task 2", Effort: EffortMedium, Dependencies: []string{"T1"}},
	}
	if err := SavePlan(dir, "1", tasks); err != nil {
		t.Fatal(err)
	}

	if err := UpdateTaskStatus(dir, "1", "T1", TaskStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}

	plan, err := LoadPlan(dir, "1")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Tasks[0].Status != TaskStatusDone {
		t.Errorf("expected T1 status done, got %s", plan.Tasks[0].Status)
	}
	if plan.Tasks[1].Status != TaskStatusTodo {
		t.Errorf("expected T2 still todo, got %s", plan.Tasks[1].Status)
	}

	content, _ := readFile(planPath(dir, "1"))
	if !strings.Contains(content, "- [x] T1:") {
		t.Error("plan file should contain [x] for completed T1")
	}
}

func TestProjectStatus(t *testing.T) {
	dir := t.TempDir()
	if err := InitProject(dir, "StatusTest", "Testing status"); err != nil {
		t.Fatal(err)
	}

	if _, err := AddPhase(dir, "Phase1", PriorityP0); err != nil {
		t.Fatal(err)
	}
	if _, err := AddPhase(dir, "Phase2", PriorityP1); err != nil {
		t.Fatal(err)
	}
	if _, err := AddPhase(dir, "Phase3", PriorityP2); err != nil {
		t.Fatal(err)
	}

	if err := EditPhase(dir, "1", EditOpts{Status: StatusCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := EditPhase(dir, "2", EditOpts{Status: StatusInProgress}); err != nil {
		t.Fatal(err)
	}

	report, err := ProjectStatus(dir)
	if err != nil {
		t.Fatalf("ProjectStatus failed: %v", err)
	}

	if report.PhaseCount != 3 {
		t.Errorf("expected 3 phases, got %d", report.PhaseCount)
	}
	if report.CompletedCount != 1 {
		t.Errorf("expected 1 completed, got %d", report.CompletedCount)
	}
	if report.CurrentPhase != "2" {
		t.Errorf("expected current phase 2, got %s", report.CurrentPhase)
	}
	if report.CompletionPct < 33.0 || report.CompletionPct > 34.0 {
		t.Errorf("expected ~33.3%% completion, got %.1f%%", report.CompletionPct)
	}
	if report.Project.Name != "StatusTest" {
		t.Errorf("expected project name StatusTest, got %s", report.Project.Name)
	}
}

func taskIDs(wave []Task) []string {
	ids := make([]string, len(wave))
	for i, t := range wave {
		ids[i] = t.ID
	}
	return ids
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
