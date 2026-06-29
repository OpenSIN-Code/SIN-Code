// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package agentloop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// buildCompactorConfig materialises a CompactorConfig snapshot from the
// loop-level fields. Defaults are applied via CompactorConfig.Normalize
// inside Configure().
func (l *Loop) buildCompactorConfig() CompactorConfig {
	mode := l.ContextCompactionMode
	if mode == "" {
		mode = ContextCompactionOff
	}
	trigger := l.CompactionTrigger
	if trigger == "" {
		trigger = CompactionTriggerTokens
	}
	if l.CompactionMaxTokens <= 0 {
		l.CompactionMaxTokens = 8000
	}
	return CompactorConfig{
		Mode:             mode,
		Trigger:          trigger,
		Threshold:        l.CompactionThreshold,
		ContextWindow:    l.effectiveContextWindow(l.CompactionMaxTokens),
		MaxTokens:        l.CompactionMaxTokens,
		PreserveEvidence: l.CompactionPreserveEvidence,
		RecentTurns:      l.CompactionRecentTurns,
	}
}

// effectiveContextWindow resolves the loop's ContextWindow field. Zero
// means auto: derive a sensible cap from CompactionMaxTokens so the
// token trigger still emits a meaningful signal.
func (l *Loop) effectiveContextWindow(maxTkns int) int {
	if l.ContextWindow > 0 {
		return l.ContextWindow
	}
	if maxTkns <= 0 {
		maxTkns = 8000
	}
	return maxTkns * 4
}

// shouldFireCompaction combines turns- and tokens-based triggers. Any
// trigger that fires returns true; when both are off (the default) the
// compactor never runs.
//
// Backward-compat: legacy callers configure the threshold directly on the
// Compactor (compactor.Threshold); the loop-level CompactionThreshold is
// only used when the user opted in to the compaction-modes flow. When
// neither is set, we fall back to the original threshold default so a
// single-shot integration test like TestCompactionIntegration_TriggeredAtThreshold
// keeps firing.
func (l *Loop) shouldFireCompaction(maxTurns int, msgs []session.Message) bool {
	if l.Compactor == nil {
		return false
	}
	threshold := l.CompactionThreshold
	if threshold <= 0 && l.Compactor.Threshold > 0 {
		threshold = l.Compactor.Threshold
	}
	if threshold <= 0 && l.Compactor != nil {
		if cfg := l.Compactor.config(); cfg.Threshold > 0 {
			threshold = cfg.Threshold
		}
	}
	if threshold <= 0 {
		threshold = DefaultCompactionThreshold
	}
	trigger := l.CompactionTrigger
	if trigger == "" {
		// Legacy callers that set Compactor.Threshold but neither
		// CompactionStrategy turned-on nor CompactionTrigger want turns
		// semantics to keep the old single-knob behaviour.
		if l.Compactor.Threshold > 0 {
			trigger = CompactionTriggerTurns
		} else {
			trigger = CompactionTriggerTokens
		}
	}
	switch trigger {
	case CompactionTriggerTurns:
		return ShouldCompact(len(msgs), maxTurns, threshold)
	case CompactionTriggerTokens:
		ctxWin := l.effectiveContextWindow(l.CompactionMaxTokens)
		return ShouldCompactTokens(estimateTokens(msgs), ctxWin, threshold)
	case CompactionTriggerBoth:
		ctxWin := l.effectiveContextWindow(l.CompactionMaxTokens)
		return ShouldCompact(len(msgs), maxTurns, threshold) ||
			ShouldCompactTokens(estimateTokens(msgs), ctxWin, threshold)
	}
	return false
}

// compactionSnapshot bundles the inputs/outputs of Compact2 so the loop
// can route between request-only compaction (mode-based) and in-place
// (legacy strategy) with the same code path.
type compactionSnapshot struct {
	mode   ContextCompactionMode
	result CompactResult
}

// compactionSnapshot runs Compact2 with the loop's configured inputs. The
// returned result carries the kept/down/summary fields and may have
// SnapshotID populated when a sidecar was written.
func (l *Loop) compactionSnapshot(ctx context.Context, sess *session.Session, msgs []session.Message) compactionSnapshot {
	mode := l.ContextCompactionMode
	if mode == "" {
		mode = ContextCompactionOff
	}
	maxTkns := l.CompactionMaxTokens
	if maxTkns <= 0 {
		maxTkns = 8000
	}
	res, _ := l.Compactor.Compact2(ctx, CompactInput{
		Messages:        msgs,
		Mode:            mode,
		MaxTokens:       maxTkns,
		EvidenceIndices: identifyEvidence(msgs),
		SessionID:       sess.ID,
	})
	return compactionSnapshot{mode: mode, result: res}
}

// writeCompactionSidecar writes a JSON snapshot of the dropped messages
// to ~/.local/share/sin-code/context-snapshots/<session-hash>/<turn>.json
// so lossy compaction is reversible for debugging and audit (mandate M3:
// verification evidence is preserved AND traceable). Failure to write is
// non-fatal: the loop keeps running and logs the path for forensic use.
func (l *Loop) writeCompactionSidecar(sess *session.Session, turn int, result CompactResult) string {
	if sess == nil || sess.ID == "" {
		return ""
	}
	if len(result.Dropped) == 0 {
		return ""
	}
	if result.Mode == "" || result.Mode == ContextCompactionOff || !result.Mode.IsLossy() {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".local", "share", "sin-code", "context-snapshots", sessionIDHash(sess.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	id := fmt.Sprintf("turn-%05d", turn)
	path := filepath.Join(dir, id+".json")
	payload := map[string]any{
		"session_id":    sess.ID,
		"turn":          turn,
		"mode":          result.Mode.String(),
		"snapshot_id":   id,
		"tokens_before": result.TokensBefore,
		"tokens_after":  result.TokensAfter,
		"summary":       result.Summary,
		"dropped_count": len(result.Dropped),
		"kept_count":    len(result.Kept),
		"dropped":       result.Dropped,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, filemode.Default()); err != nil {
		return ""
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	return path
}

// identifyEvidence scans msgs for the canonical evidence markers and
// returns the matching message indices. Used to seed the retain filter
// for callers that do not pass an explicit map.
func identifyEvidence(msgs []session.Message) map[int]bool {
	out := make(map[int]bool)
	for i, m := range msgs {
		if containsEvidence(m.Content) {
			out[i] = true
		}
	}
	return out
}
