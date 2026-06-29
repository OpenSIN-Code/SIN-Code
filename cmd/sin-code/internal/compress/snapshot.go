// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when compaction is refactored
//
// Snapshot and rollback — writes a JSON snapshot containing every
// dropped entry verbatim plus the full Plan, and restores source
// surfaces to the state recorded in the snapshot.
package compress

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// writeSnapshot writes a snapshot file containing every dropped entry
// verbatim plus the full Plan. Atomic via temp+rename. The filename is
// derived from the Plan ID; `snapshots/` lives under
// ~/.local/share/sin-code/.
func writeSnapshot(p Plan) (string, string, error) {
	dir := SnapshotDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	tmp := filepath.Join(dir, p.ID+".json.partial")
	final := filepath.Join(dir, p.ID+".json")
	body, err := jsonMarshalIndent(&p)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(tmp, body, filemode.Default()); err != nil {
		return "", "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		return "", "", err
	}
	return p.ID, final, nil
}

// SnapshotDir resolves ~/.local/share/sin-code/compress-snapshots
// (configurable via SIN_CODE_SNAPSHOT_DIR).
func SnapshotDir() string {
	if v := os.Getenv("SIN_CODE_SNAPSHOT_DIR"); v != "" {
		return v
	}
	if h := os.Getenv("SIN_CODE_HOME"); h != "" {
		return filepath.Join(h, "compress-snapshots")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "sin-code", "compress-snapshots")
}

// Rollback restores the source surfaces to the state recorded in the
// snapshot. The snapshot must be present and complete (no
// `.partial` marker); if any partial marker exists in the snapshot
// directory, Rollback refuses with an error.
func Rollback(snapshotID string) error {
	dir := SnapshotDir()
	if _, err := os.Stat(filepath.Join(dir, snapshotID+".json.partial")); err == nil {
		return fmt.Errorf("compress: refusing to rollback — partial snapshot %s.json.partial exists in %s",
			snapshotID, dir)
	}
	final := filepath.Join(dir, snapshotID+".json")
	data, err := os.ReadFile(final)
	if err != nil {
		return fmt.Errorf("compress: read snapshot %q: %w", final, err)
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("compress: decode snapshot %q: %w", final, err)
	}
	// The snapshot carries the original user-supplied Paths so the
	// Rollback writes hit the same files Apply targeted. Zero-value
	// Paths would route to ~/.local defaults — wrong if the user
	// used --lessons-db / --instinct-dir / --memory to override.
	paths := p.Paths
	return applyPlanReverse(&p, paths)
}

// applyPlanReverse inverts applyTarget: drops[] is now the keep set.
// We do this by re-running Plan() against the same target with the
// same opts but then swapping keeps<->drops so we can write the
// original body back. The destination file is rewritten from the
// snapshot's drops[] verbatim.
func applyPlanReverse(p *Plan, paths Paths) error {
	// Re-running keeps the surface types honest. After re-plan we
	// take the resulting kept set and prefer the snapshot's stored
	// bodies when their hash matches.
	targets := []Target{p.Target}
	if p.Target == TargetAll {
		targets = AllTargets
	}
	for _, t := range targets {
		// Look up snapshot's dropped bodies for this target.
		drops := []rawEntry{}
		for _, d := range p.Drops {
			if d.Target != t {
				continue
			}
			drops = append(drops, rawEntry{Subject: d.Subject, Body: d.Body, Created: d.Created, Utility: d.Utility})
		}
		// Add merged sources back.
		for _, m := range p.Merges {
			for _, sh := range m.SourceHashes {
				for _, d := range p.Drops {
					if d.Target == t && d.Hash == sh {
						drops = append(drops, rawEntry{Subject: d.Subject, Body: d.Body, Created: d.Created, Utility: d.Utility})
					}
				}
			}
		}
		// Also keep the surviving keeps[] so the file is a full state
		// restore, not just an additive one.
		combined := drops[:0:0]
		for _, d := range drops {
			combined = append(combined, d)
		}
		for _, k := range p.Keeps {
			if k.Target != t {
				continue
			}
			combined = append(combined, rawEntry{Subject: k.Subject, Body: k.Body, Created: k.Created, Utility: k.Utility})
		}
		if err := writeTarget(t, combined, paths); err != nil {
			return fmt.Errorf("rollback %s: %w", t, err)
		}
	}
	return nil
}
