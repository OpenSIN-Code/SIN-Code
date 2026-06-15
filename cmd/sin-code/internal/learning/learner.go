// SPDX-License-Identifier: MIT
// Purpose: bridge between SIN-Code's existing agent loop (cmd/sin-code/
// internal/agentloop) and the new learning subsystem (instinct +
// hooklife). This package is the only place that knows about both.
// Docs: learner.doc.md
package learning

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/adapters"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/instinct"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// Options configures a Learner. The zero value is fully usable:
// all subsystems are wired to safe no-op defaults.
type Options struct {
	Workdir    string
	LLM        *llm.Client
	Model      string // background model for LLMExtractor; default haiku
	Memory     *memory.Store
	VerifyGate *verify.Gate
}

// Learner is the single entry point for the agent loop. It owns
// the Instinct manager, the heuristic/optional-LLM observer, and
// the hooklife runner. It exposes session-lifecycle methods the
// loop calls around its main Run.
type Learner struct {
	opts    Options
	manager *instinct.Manager
	obs     *instinct.Observer
	hooks   *hooklife.Registry
	runner  *hooklife.Runner
}

// New constructs a Learner with all subsystems wired. Errors are
// returned for genuine failures (store open etc.) — but missing
// optional subsystems (no LLM, no memory) are silently downgraded
// to no-ops.
func New(opts Options) (*Learner, error) {
	store := instinct.NewStore("")
	proj := instinct.DetectProject(opts.Workdir)
	_ = store.SaveProjectMeta(proj)

	var sink instinct.MemorySink
	if opts.Memory != nil {
		sink = adapters.MemoryBridge{Store: opts.Memory}
	}
	mgr := instinct.NewManagerWithStore(store, proj, sink)

	var ex instinct.Extractor
	if opts.LLM != nil {
		ex = instinct.LLMExtractor{
			Model:    adapters.BackgroundCompleter{Client: opts.LLM, Model: opts.Model},
			MaxObs:   40,
			Fallback: instinct.HeuristicExtractor{MinRepeats: 2},
		}
	} else {
		ex = instinct.HeuristicExtractor{MinRepeats: 2}
	}
	obs := instinct.NewObserver(mgr, ex)

	reg := hooklife.NewRegistry()
	reg.Register(hooklife.BlockNoVerify{})
	reg.Register(hooklife.ConfigProtection{Protected: []string{".git/", "go.sum", ".env", ".sin/prp/"}})
	reg.Register(hooklife.PostEditFormat{Formatter: hooklife.DefaultFormatters()})
	if opts.VerifyGate != nil {
		reg.Register(hooklife.QualityGate{Verifier: adapters.VerifyGate{Gate: opts.VerifyGate}})
	}
	reg.Register(hooklife.SuggestCompact{Threshold: 150000})

	runner := hooklife.NewRunner(reg).WithTimeout(10 * time.Second)

	return &Learner{
		opts:    opts,
		manager: mgr,
		obs:     obs,
		hooks:   reg,
		runner:  runner,
	}, nil
}

// Manager exposes the instinct manager (for the CLI + tests).
func (l *Learner) Manager() *instinct.Manager { return l.manager }

// Observer exposes the observer (for the CLI + tests).
func (l *Learner) Observer() *instinct.Observer { return l.obs }

// Hooks exposes the hook registry (for `sin hooks list`).
func (l *Learner) Hooks() *hooklife.Registry { return l.hooks }

// BeforeTurn returns the active-instinct system-prompt block to
// prepend to the model's context. Returns "" when no instincts are
// active.
func (l *Learner) BeforeTurn(_ context.Context, _ *session.Session) string {
	block, _ := l.manager.SystemBlockForProject(15)
	return block
}

// BeforeTool runs the PreToolUse hook chain. Returns (allowed, message).
// The loop MUST NOT run the tool when allowed == false.
func (l *Learner) BeforeTool(ctx context.Context, name string, args map[string]any) (bool, string) {
	flat := flattenArgs(args)
	d := l.runner.Dispatch(ctx, hooklife.Event{
		Phase:   hooklife.PreToolUse,
		Tool:    name,
		Args:    flat,
		Workdir: l.opts.Workdir,
	})
	return d.Verdict != hooklife.Block, d.Message
}

// AfterTool runs the PostToolUse hook chain AND feeds the instinct
// observer. Always returns ok=true (the tool already ran; warnings
// are non-fatal).
func (l *Learner) AfterTool(ctx context.Context, name string, args map[string]any, ok bool, errMsg string) {
	flat := flattenArgs(args)
	d := l.runner.Dispatch(ctx, hooklife.Event{
		Phase: hooklife.PostToolUse,
		Tool:  name,
		Args:  flat,
		Meta: map[string]string{
			"success": boolStr(ok),
			"error":   errMsg,
		},
	})
	if d.Verdict == hooklife.Warn && d.Message != "" {
		fmt.Fprintf(os.Stderr, "hook warning: %s\n", d.Message)
	}
	// Feed the observer regardless of warning state.
	l.obs.Record(instinct.Observation{
		Tool:    name,
		Action:  describeTool(name, flat),
		Domain:  instinct.Classify(name, flat),
		Success: ok,
		Meta:    flat,
	})
}

// EndTurn flushes the observer. Returns (created, reinforced) for
// telemetry. The caller may log these.
func (l *Learner) EndTurn(ctx context.Context) (int, int, error) {
	return l.obs.Flush(ctx)
}

// PreCompact runs the PreCompact hook chain AND flushes the
// observer so learning is captured before context is dropped.
func (l *Learner) PreCompact(ctx context.Context) (int, int, error) {
	created, reinforced, err := l.obs.Flush(ctx)
	_ = l.runner.Dispatch(ctx, hooklife.Event{Phase: hooklife.PreCompact, Workdir: l.opts.Workdir})
	return created, reinforced, err
}

// --- helpers ---

func flattenArgs(args map[string]any) map[string]string {
	out := make(map[string]string, len(args))
	for k, v := range args {
		switch x := v.(type) {
		case string:
			out[k] = x
		case bool:
			if x {
				out[k] = "true"
			} else {
				out[k] = "false"
			}
		case float64:
			out[k] = fmt.Sprintf("%v", x)
		case nil:
			out[k] = ""
		default:
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}

func describeTool(tool string, args map[string]string) string {
	if c := args["command"]; c != "" {
		return tool + ": " + c
	}
	if p := args["path"]; p != "" {
		return tool + " " + p
	}
	return tool
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Wrap is a convenience for chat/dialog commands that already have
// a loop and want a place to invoke the learner lifecycle from.
// Returns the loop unchanged — the caller is responsible for calling
// BeforeTurn / BeforeTool / AfterTool / EndTurn / PreCompact.
func (l *Learner) Wrap(loop *agentloop.Loop) *agentloop.Loop { return loop }
