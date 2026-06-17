// SPDX-License-Identifier: MIT
// Purpose: tests for the SIN Fusion verify-tournament arm surface
// (issue #290). Verifies FusionArm sets FusionEnabled, Compare
// records FirstToPassRate on the fusion arm's Totals, and the
// snapshot round-trip remains byte-stable with the new omitempty
// field.
package evalharness

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestFusionArmSetsFusionEnabled(t *testing.T) {
	base := StandardTerseArm()
	if base.FusionEnabled {
		t.Fatal("base terse arm should not have FusionEnabled")
	}
	fusion := FusionArm(base)
	if !fusion.FusionEnabled {
		t.Fatal("FusionArm should set FusionEnabled=true")
	}
	if fusion.ID != base.ID {
		t.Fatalf("FusionArm ID = %q, want %q", fusion.ID, base.ID)
	}
	if fusion.SystemPrompt != base.SystemPrompt {
		t.Fatalf("FusionArm SystemPrompt = %q, want %q", fusion.SystemPrompt, base.SystemPrompt)
	}
}

func TestFusionArmCompareAndSnapshot(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 2})
	defer SetDefaultSubject(prev)

	baseline := NoSystemPromptArm()
	terse := StandardTerseArm()
	fusion := FusionArm(StandardTerseArm())
	fusion.ID = "__terse_fusion__"

	arms := []Arm{baseline, terse, fusion}

	if !fusion.FusionEnabled {
		t.Fatal("fusion arm must have FusionEnabled=true")
	}
	if baseline.FusionEnabled || terse.FusionEnabled {
		t.Fatal("non-fusion arms must not have FusionEnabled")
	}

	set := twoCaseSet()
	rep, err := Compare(context.Background(), set, arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(rep.PerCase) != 2 {
		t.Fatalf("want 2 case rows, got %d", len(rep.PerCase))
	}

	fusionTotals, ok := rep.TotalsByArm[fusion.ID]
	if !ok {
		t.Fatalf("TotalsByArm missing fusion arm %s", fusion.ID)
	}
	var _ float64 = fusionTotals.FirstToPassRate
	if fusionTotals.TotalCases != 2 {
		t.Fatalf("fusion arm TotalCases = %d, want 2", fusionTotals.TotalCases)
	}

	terseTotals, ok := rep.TotalsByArm[terse.ID]
	if !ok {
		t.Fatalf("TotalsByArm missing terse arm %s", terse.ID)
	}
	var _ float64 = terseTotals.FirstToPassRate

	hdr := SnapshotHeader{SetName: "fusion-demo", SchemaVersion: SnapshotSchemaVersion}
	var buf bytes.Buffer
	if err := WriteSnapshot(&buf, rep, hdr); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	originalBytes := append([]byte(nil), buf.Bytes()...)

	snap, err := LoadSnapshot(bytes.NewReader(originalBytes))
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if len(snap.Rows) != 3 {
		t.Fatalf("want 3 snapshot rows, got %d", len(snap.Rows))
	}

	var buf2 bytes.Buffer
	enc := json.NewEncoder(&buf2)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(snap); err != nil {
		t.Fatalf("re-marshal snapshot: %v", err)
	}
	if !bytes.Equal(stripTrailingNL(originalBytes), stripTrailingNL(buf2.Bytes())) {
		t.Fatalf("fusion snapshot not byte-stable on round-trip\nA: %s\nB: %s",
			originalBytes, buf2.String())
	}
}

func TestFusionArmSnapshotOmitsZeroField(t *testing.T) {
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 1})
	defer SetDefaultSubject(prev)

	baseline := NoSystemPromptArm()
	terse := StandardTerseArm()
	fusion := FusionArm(StandardTerseArm())
	fusion.ID = "__terse_fusion__"

	arms := []Arm{baseline, terse, fusion}
	rep, err := Compare(context.Background(), twoCaseSet(), arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	hdr := SnapshotHeader{SetName: "fusion-omit-demo", SchemaVersion: SnapshotSchemaVersion}
	var buf bytes.Buffer
	if err := WriteSnapshot(&buf, rep, hdr); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("first_to_pass_rate")) {
		t.Fatalf("zero FirstToPassRate should be omitted via omitempty; got: %s", buf.String())
	}
}
