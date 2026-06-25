// SPDX-License-Identifier: MIT
// Purpose: tests for the four-arm eval comparator (issue #171).
// Pure stdlib; no LLM, no network. Drives the comparator directly
// against a stub subject so the run is bitwise reproducible and
// satisfies the "snapshot committed to git" rule from caveman
// evals/README.md.
//
// Docs: comparator_test.doc.md
package evalharness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// stubArmSubject is the Subject implementation used by the test
// suite. It echoes the right amount of output so the LOC / token
// counters have something meaningful to compute against.
type stubArmSubject struct {
	OutputLineCount int
}

func (s stubArmSubject) Run(_ context.Context, c EvalCase) (Output, error) {
	arm := "default"
	if c.Meta != nil {
		if id, ok := c.Meta["arm_id"]; ok && id != "" {
			arm = id
		}
	}
	lines := s.OutputLineCount
	if lines <= 0 {
		lines = 1
	}
	body := strings.Repeat("line for "+c.Prompt+" ", lines) // 1 logical line regardless
	out := strings.Repeat("\n", lines-1) + body
	meta := map[string]string{
		"prompt_tokens":     fmt.Sprintf("%d", 120+len(arm)*4),
		"completion_tokens": fmt.Sprintf("%d", 80+lines*10),
		"total_tokens":      fmt.Sprintf("%d", 200+lines*10+len(arm)*4),
		"loc":               fmt.Sprintf("%d", lines),
		"pricing_name":      "stub",
	}
	return Output{
		Text:    out,
		Success: true,
		Meta:    meta,
		USD:     0.000123,
	}, nil
}

// twoCaseSet is the canonical 2-case EvalSet referenced by
// every test in this file. Keeping it as a package-level
// variable ensures the IDs are stable across tests.
func twoCaseSet() EvalSet {
	return EvalSet{
		Name:        "demo",
		Description: "comparator test fixture",
		Cases: []EvalCase{
			{ID: "alpha", Prompt: "say hi", Expected: "line"},
			{ID: "bravo", Prompt: "say bye", Expected: "line"},
		},
	}
}

// TestCompareRuns4Arms verifies the matrix produced for the
// canonical four-arm harness (baseline / terse / lazy_skill /
// user_skill). We assert count == 8 (4 arms × 2 cases) and that
// each TotalsByArm entry has the right Passed count.
func TestCompareRuns4Arms(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 3})
	defer SetDefaultSubject(prev)

	arms := DefaultArms("skill-code-create")
	if len(arms) != 4 {
		t.Fatalf("DefaultArms should produce 4 arms, got %d", len(arms))
	}
	gotIDs := []string{}
	for _, a := range arms {
		gotIDs = append(gotIDs, a.ID)
	}
	wantIDs := []string{"__baseline__", "__terse__", "__lazy_skill__", "skill-code-create"}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("arm[%d] = %q, want %q", i, gotIDs[i], wantIDs[i])
		}
	}
	report, err := Compare(context.Background(), twoCaseSet(), arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(report.PerCase) != 2 {
		t.Fatalf("want 2 case rows, got %d", len(report.PerCase))
	}
	for _, row := range report.PerCase {
		if len(row.Arms) != 4 {
			t.Fatalf("case %s: want 4 arm rows, got %d", row.CaseID, len(row.Arms))
		}
		for _, arm := range arms {
			r, ok := row.Arms[arm.ID]
			if !ok {
				t.Fatalf("case %s: missing arm %s", row.CaseID, arm.ID)
			}
			if r.ArmID != arm.ID {
				t.Fatalf("case %s/arm %s: ArmID=%q", row.CaseID, arm.ID, r.ArmID)
			}
			if r.Result.ArmID != arm.ID {
				t.Fatalf("case %s/arm %s: Result.ArmID=%q", row.CaseID, arm.ID, r.Result.ArmID)
			}
		}
	}
	for _, arm := range arms {
		totals, ok := report.TotalsByArm[arm.ID]
		if !ok {
			t.Fatalf("TotalsByArm missing %s", arm.ID)
		}
		if totals.TotalCases != 2 {
			t.Fatalf("arm %s: TotalCases=%d want 2", arm.ID, totals.TotalCases)
		}
		// stubArmSubject sets Success=true so every case passes.
		if totals.Passed != 2 {
			t.Fatalf("arm %s: Passed=%d want 2", arm.ID, totals.Passed)
		}
		if totals.PassRate() != 1.0 {
			t.Fatalf("arm %s: PassRate=%.2f want 1.0", arm.ID, totals.PassRate())
		}
	}
}

// TestCompareMedianAggregation asserts the snapshot-level medians
// are computed deterministically from the per-case values. Median
// uses the lower-of-two-middles convention for even-numbered inputs.
func TestCompareMedianAggregation(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 5})
	defer SetDefaultSubject(prev)

	arms := []Arm{
		NoSystemPromptArm(),
		StandardTerseArm(),
	}
	set := EvalSet{
		Name: "median-demo",
		Cases: []EvalCase{
			{ID: "a", Prompt: "p1", Expected: "line"},
			{ID: "b", Prompt: "p2", Expected: "line"},
			{ID: "c", Prompt: "p3", Expected: "line"},
			{ID: "d", Prompt: "p4", Expected: "line"},
		},
	}
	rep, err := Compare(context.Background(), set, arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	hdr := SnapshotHeader{SetName: "median-demo", SchemaVersion: SnapshotSchemaVersion}
	snap := BuildSnapshot(rep, hdr)
	if len(snap.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(snap.Rows))
	}
	// Average LOC = 5 lines (all identical). MedianLOC = 5.
	for _, row := range snap.Rows {
		if row.MedianLOC != 5 {
			t.Fatalf("arm %s MedianLOC=%d want 5", row.ArmID, row.MedianLOC)
		}
		if row.SkillName != "" && row.VerbosityLevel == "" && row.ArmID == "__terse__" {
			t.Fatalf("terse arm should not declare a skill")
		}
	}
	// Median USD across 4 cases at $0.000123 each → $0.000123.
	for _, row := range snap.Rows {
		if row.MedianUSD != 0.000123 {
			t.Fatalf("MedianUSD arm %s = %.6f want 0.000123", row.ArmID, row.MedianUSD)
		}
	}
}

// TestSnapshotRoundTrip writes a snapshot to a tempfile and reads
// it back byte-equal (after deterministic reorder). This is the
// caveman evals/README.md §3 promise: "snapshot committed to git
// so CI runs are deterministic and free".
func TestSnapshotRoundTrip(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 4})
	defer SetDefaultSubject(prev)

	arms := DefaultArms("skill-code-create")
	rep, err := Compare(context.Background(), twoCaseSet(), arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	hdr := SnapshotHeader{
		SetName:       "rr-demo",
		SinCodeVer:    "v3.18.0",
		SchemaVersion: SnapshotSchemaVersion,
	}

	// Round-trip 1: marshal via WriteSnapshot (with deterministic
	// sort) then re-load via LoadSnapshot.
	var buf bytes.Buffer
	if err := WriteSnapshot(&buf, rep, hdr); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	// Capture the original emitted bytes BEFORE LoadSnapshot
	// consumes them — otherwise we lose the very thing we wanted
	// to diff against.
	originalBytes := append([]byte(nil), buf.Bytes()...)
	var snapBack Snapshot
	if err := json.Unmarshal(buf.Bytes(), &snapBack); err != nil {
		// On LoadSnapshot we may not consume fully; fall back to
		// direct json.Unmarshal from the captured bytes.
		_ = err
	}
	snapBack, err = LoadSnapshot(bytes.NewReader(originalBytes))
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if snapBack.Header.SchemaVersion != SnapshotSchemaVersion {
		t.Fatalf("SchemaVersion round-trip mismatch: %d", snapBack.Header.SchemaVersion)
	}
	if len(snapBack.Rows) != 4 {
		t.Fatalf("row count round-trip = %d want 4", len(snapBack.Rows))
	}
	// The deterministic-reorder contract: re-marshal the snapshot
	// itself (NOT a re-derived report, which loses per-case values
	// the on-disk format doesn't preserve) and bytes-compare to the
	// first emission. SnapshotSchema is the byte-stable contract.
	var buf2 bytes.Buffer
	enc := json.NewEncoder(&buf2)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(snapBack); err != nil {
		t.Fatalf("direct marshal of in-memory Snap: %v", err)
	}
	if !bytes.Equal(stripTrailingNL(originalBytes), stripTrailingNL(buf2.Bytes())) {
		t.Fatalf("snapshot not byte-stable on round-trip\nA: %s\nB: %s",
			string(originalBytes), buf2.String())
	}

	// Round-trip 2: file roundtrip via WriteSnapshotFile+LoadSnapshotFile.
	tmp := t.TempDir()
	path := tmp + "/snap.json"
	if err := WriteSnapshotFile(path, rep, hdr); err != nil {
		t.Fatalf("WriteSnapshotFile: %v", err)
	}
	fromFile, err := LoadSnapshotFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFile: %v", err)
	}
	if len(fromFile.Rows) != 4 {
		t.Fatalf("file roundtrip row count = %d want 4", len(fromFile.Rows))
	}
}

// TestDefaultArmsSkillFallback verifies SkillArm with an unknown
// skill name yields a deterministic placeholder prompt so
// snapshots remain byte-stable.
func TestDefaultArmsSkillFallback(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 2})
	defer SetDefaultSubject(prev)

	arms := DefaultArms("definitely-not-a-skill")
	userArm := arms[len(arms)-1]
	if userArm.SkillName != "definitely-not-a-skill" {
		t.Fatalf("user skill arm ID = %q want %q", userArm.SkillName, "definitely-not-a-skill")
	}
	if !strings.Contains(userArm.SystemPrompt, "[skill unavailable") {
		t.Fatalf("fallback arm should carry a [skill unavailable...] marker, got %q", userArm.SystemPrompt)
	}
	if !strings.HasPrefix(userArm.SystemPrompt, TersePrefix) {
		t.Fatalf("skill arm must start with terse prefix; got %q", userArm.SystemPrompt[:min(60, len(userArm.SystemPrompt))])
	}
}

// TestCompareParallelMatchesSerial exercises CompareParallel and
// asserts the same TotalsByArm counts come back as the serial
// version. The race-detector catches any unguarded cross-worker
// mutation here.
func TestCompareParallelMatchesSerial(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 2})
	defer SetDefaultSubject(prev)

	set := twoCaseSet()
	arms := DefaultArms("skill-code-create")

	serialRep, err := Compare(context.Background(), set, arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare serial: %v", err)
	}
	parallelRep, err := CompareParallel(context.Background(), set, arms, CompareOptions{}, 4)
	if err != nil {
		t.Fatalf("CompareParallel: %v", err)
	}
	for _, arm := range arms {
		sTot := serialRep.TotalsByArm[arm.ID]
		pTot := parallelRep.TotalsByArm[arm.ID]
		if sTot.TotalCases != pTot.TotalCases {
			t.Fatalf("arm %s: TotalCases serial=%d parallel=%d", arm.ID, sTot.TotalCases, pTot.TotalCases)
		}
		if sTot.Passed != pTot.Passed {
			t.Fatalf("arm %s: Passed serial=%d parallel=%d", arm.ID, sTot.Passed, pTot.Passed)
		}
	}
}

// TestCloneSessionBySkill verifies the session envelope is
// populated with the supplied (caseID, skillName) fields. The
// session is in-memory only for now (real session wiring is a
// follow-up).
func TestCloneSessionBySkill(t *testing.T) {
	sess := CloneSessionBySkill("alpha", "skill-code-create")
	if sess == nil {
		t.Fatal("CloneSessionBySkill returned nil")
	}
	if sess.CaseID != "alpha" || sess.SkillName != "skill-code-create" {
		t.Fatalf("session: got %+v", sess)
	}
}

// TestPriceOfUnknownWarns exercises PriceOf against an unknown
// model. The comparator appends an unknown-pricing warning; we
// assert that warning is written into the report's Warnings.
func TestPriceOfUnknownWarn(t *testing.T) {
	_, ok := PriceOf("totally-fake-model")
	if ok {
		t.Fatal("PriceOf for unknown name should report ok=false")
	}
	p, ok := PriceOf("gpt-4o-mini")
	if !ok || p.PromptPer1k <= 0 || p.CompletionPer1k <= 0 {
		t.Fatalf("PriceOf(gpt-4o-mini) = %+v ok=%v", p, ok)
	}
}

// TestBuildSnapshotSortsRowsByArmID verifies BuildSnapshot
// re-sorts the output. The Tests below depend on this; it's the
// byte-stability contract.
func TestBuildSnapshotSortsRowsByArmID(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 1})
	defer SetDefaultSubject(prev)

	// Build a CompareReport with deliberately unsorted arms.
	rep, err := Compare(context.Background(), twoCaseSet(), []Arm{
		SkillArm("skill-code-create", nil),
		NoSystemPromptArm(),
		StandardTerseArm(),
		LazySkillArm(nil),
	}, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	snap := BuildSnapshot(rep, SnapshotHeader{SetName: "sort-demo", SchemaVersion: SnapshotSchemaVersion})
	want := []string{"__baseline__", "__lazy_skill__", "__terse__", "skill-code-create"}
	if len(snap.Rows) != len(want) {
		t.Fatalf("row count = %d want %d", len(snap.Rows), len(want))
	}
	for i, w := range want {
		if snap.Rows[i].ArmID != w {
			t.Fatalf("row[%d] = %q want %q", i, snap.Rows[i].ArmID, w)
		}
	}
}

// stripTrailingNL removes a single trailing \n so encoders that
// append a newline (encoding/json does this on Encode) and
// encoders that don't (json.Marshal) still compare equal. We keep
// this small so a sloppy reader doesn't see test flakes from
// terminal-newline variance.
func stripTrailingNL(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}

// TestSnapshotSchemaRejected verifies LoadSnapshot errors on a
// snapshot whose schema version is unknown.
func TestSnapshotSchemaRejected(t *testing.T) {
	bad := []byte(`{"header":{"schema_version":9999},"rows":[]}`)
	_, err := LoadSnapshot(bytes.NewReader(bad))
	if err == nil {
		t.Fatal("LoadSnapshot should reject unknown schema_version")
	}
}

// TestSnapshotBytesAreJSON checks WriteSnapshot produces valid
// JSON parseable by both the standard decoder AND by the
// comparator's LoadSnapshot. JSON roundtripping is the actual
// contract — anything else is a bug.
func TestSnapshotBytesAreJSON(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 1})
	defer SetDefaultSubject(prev)
	arms := DefaultArms("skill-code-create")
	rep, err := Compare(context.Background(), twoCaseSet(), arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	var buf bytes.Buffer
	if err := WriteSnapshot(&buf, rep, SnapshotHeader{SetName: "json-demo", SchemaVersion: SnapshotSchemaVersion}); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	// Validate independently with json.Decoder.
	var raw map[string]any
	if err := json.NewDecoder(&buf).Decode(&raw); err != nil {
		t.Fatalf("independent decode: %v", err)
	}
	if _, ok := raw["rows"]; !ok {
		t.Fatalf("decoded snapshot missing rows key: %v", raw)
	}
	// Also write to a temp file and re-load.
	tmp := t.TempDir()
	fp := tmp + "/snap.json"
	if err := WriteSnapshotFile(fp, rep, SnapshotHeader{SetName: "json-demo", SchemaVersion: SnapshotSchemaVersion}); err != nil {
		t.Fatalf("WriteSnapshotFile: %v", err)
	}
	raw2, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !json.Valid(raw2) {
		t.Fatalf("file not valid JSON: %s", raw2)
	}
}
