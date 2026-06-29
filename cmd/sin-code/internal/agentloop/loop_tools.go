// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package agentloop

import (
	"context"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
)

func (l *Loop) tools() []ToolSpec { return l.LocalSpec }

func (l *Loop) execute(ctx context.Context, tc ToolCall) (out string, injects []string) {
	pre := l.fire(ctx, hooks.ToolPre, tc.Name, map[string]any{"args": tc.Args})
	injects = append(injects, pre.PromptInjects...)
	if pre.Blocked {
		return "BLOCKED by hook: " + pre.BlockReason, injects
	}

	if l.Perm != nil {
		var pol permission.Policy
		if l.Perm.Risk != nil {
			pol = l.Perm.CheckWithArgs(tc.Name, tc.Args)
		} else {
			pol = l.Perm.Check(tc.Name)
		}
		switch pol {
		case permission.Deny:
			l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "deny"})
			l.recordUsage(ctx, tc.Name, ledger.OutcomeDenied)
			return "DENIED by permission policy", injects
		case permission.Ask:
			ask := l.fire(ctx, hooks.PermissionAsk, tc.Name, map[string]any{"args": tc.Args})
			injects = append(injects, ask.PromptInjects...)
			if ask.Blocked {
				l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "ask", "by": "hook"})
				l.recordUsage(ctx, tc.Name, ledger.OutcomeDenied)
				return "DENIED by hook: " + ask.BlockReason, injects
			}
			if l.Ask == nil || !l.Ask(tc) {
				l.fire(ctx, hooks.ToolDenied, tc.Name, map[string]any{"policy": "ask", "by": "user"})
				l.recordUsage(ctx, tc.Name, ledger.OutcomeDenied)
				return "DENIED by user", injects
			}
		case permission.Allow:
		}
	}

	if l.LocalTool == nil {
		return "TOOL ERROR: no LocalTool registered", injects
	}
	if l.BeforeMutate != nil {
		if p := mutatedPath(tc); p != "" {
			l.BeforeMutate(ctx, tc.Name, p)
		}
	}
	start := time.Now()
	l.fireToolStart(ctx, tc)
	l.emitProgress(ProgressEvent{Event: "tool.pre", Tool: tc.Name})
	res, err := l.LocalTool(ctx, tc.Name, tc.Args)
	duration := time.Since(start)
	l.fireToolEnd(ctx, tc, duration, res, err)
	l.emitProgress(ProgressEvent{
		Event: "tool.post",
		Tool:  tc.Name,
		Data: map[string]any{
			"output_bytes": len(res),
			"error":        err != nil,
		},
	})
	if err != nil {
		l.fire(ctx, hooks.ToolError, tc.Name, map[string]any{"error": err.Error()})
		l.record(ctx, ledger.TypeToolError, map[string]any{"tool": tc.Name}, "tool error: "+tc.Name)
		l.recordUsage(ctx, tc.Name, ledger.OutcomeError)
		if l.Lessons != nil {
			_ = l.Lessons.Record(ctx, lessons.Entry{
				Type:      lessons.TypeToolError,
				Workspace: l.Workspace,
				Context:   map[string]any{"tool": tc.Name},
				Lesson:    "Tool " + tc.Name + " failed: " + err.Error(),
			})
		}
		return "TOOL ERROR: " + err.Error(), injects
	}
	postData := map[string]any{"output_bytes": len(res)}
	if p := mutatedPath(tc); p != "" {
		postData["path"] = p
	}
	if l.ResultPolicy != nil {
		action, reason := l.ResultPolicy.ScanResult(tc.Name, res)
		if action != permission.ActionNoOp {
			postData["result_policy_action"] = action.String()
			postData["result_policy_reason"] = reason
			l.record(ctx, ledger.TypePermissionResult, map[string]any{
				"tool":   tc.Name,
				"action": action.String(),
				"reason": reason,
			}, "reactive permission: "+action.String()+" — "+reason)
			if action == permission.ActionEscalate {
				injects = append(injects, "PERMISSION ESCALATION: tool "+tc.Name+" output triggered '"+reason+"'. Stop and review before continuing.")
			} else {
				injects = append(injects, "PERMISSION WARNING: tool "+tc.Name+" output triggered '"+reason+"'.")
			}
		}
	}
	post := l.fire(ctx, hooks.ToolPost, tc.Name, postData)
	injects = append(injects, post.PromptInjects...)
	if tc.Name == "sin_memory_add" {
		l.fire(ctx, hooks.MemoryWrite, tc.Name, map[string]any{
			"insight": tc.Args["insight"],
			"project": tc.Args["project"],
		})
	}
	l.record(ctx, ledger.TypeToolCall, map[string]any{"tool": tc.Name}, "tool call: "+tc.Name)
	l.recordUsage(ctx, tc.Name, ledger.OutcomeOK)
	return res, injects
}

// mutatedPath extracts the target path for tools that mutate the workspace
// so the auto-checkpoint snapshots exactly the file about to change (cheap,
// O(1)). Returns "" for tools that don't mutate the workspace or have no
// "path" argument. (issue #194)
func mutatedPath(tc ToolCall) string {
	switch tc.Name {
	case "sin_write", "sin_edit":
		if p, ok := tc.Args["path"].(string); ok {
			return p
		}
	}
	return ""
}
