// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package agentloop

import (
	"context"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

func (l *Loop) record(ctx context.Context, typ ledger.EntryType, data map[string]any, summary string) {
	if l.Ledger == nil || l.SessionID == "" {
		return
	}
	_, _ = l.Ledger.Record(ctx, ledger.Entry{
		SessionID: l.SessionID,
		Type:      typ,
		Data:      data,
		Summary:   summary,
	})
}

func (l *Loop) recordUsage(ctx context.Context, name string, outcome ledger.UsageOutcome) {
	if l.Ledger == nil || l.SessionID == "" {
		return
	}
	_ = l.Ledger.RecordUsage(ctx, ledger.UsageRecord{
		ToolName:  name,
		Outcome:   outcome,
		SessionID: l.SessionID,
		GoalID:    l.GoalID,
	})
}

func (l *Loop) fire(ctx context.Context, event, name string, data map[string]any) hooks.Result {
	if l.Hooks == nil {
		return hooks.Result{}
	}
	return l.Hooks.Fire(ctx, hooks.Payload{
		Event:     event,
		SessionID: l.SessionID,
		Workspace: l.Workspace,
		Name:      name,
		Data:      data,
	})
}

func (l *Loop) fireToolStart(ctx context.Context, tc ToolCall) {
	if l.ToolStart != nil {
		l.ToolStart(ctx, tc)
	}
}

func (l *Loop) fireToolEnd(ctx context.Context, tc ToolCall, d time.Duration, out string, err error) {
	if l.ToolEnd != nil {
		l.ToolEnd(ctx, tc, d, out, err)
	}
}

func (l *Loop) emitProgress(ev ProgressEvent) {
	if l == nil || l.ProgressWriter == nil {
		return
	}
	ev.SessionID = l.SessionID
	l.ProgressWriter.Write(ev)
}
