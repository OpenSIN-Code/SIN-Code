// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` built-in slash commands and TUI launch.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/commands"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/logger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui"
)

// chatSideLLM adapts *llm.Client to the commands.SideLLM interface so
// built-in slash commands (issue #276 /btw) can fire one-shot completions
// without depending on the llm package directly.
type chatSideLLM struct {
	c     *llm.Client
	model string
}

func (a chatSideLLM) Complete(ctx context.Context, system, user string) (string, error) {
	resp, err := a.c.Chat(ctx, llm.ChatRequest{
		Model: a.model,
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ExtractText(), nil
}

// chatUndercover is the session-wide undercover mode shared between the
// /undercover slash command and the commit path. Construction is cheap;
// the chat loop reads .Enabled() before committing (issue #274).
var chatUndercover = commands.NewUndercoverMode()

// newBuiltinCommandRegistry builds the registry of Go-implemented slash
// commands for a chat session. The LLM adapter is wired from the live
// client/model so /btw can answer side questions (issue #276). The
// /undercover command is always available and reuses the package-level
// chatUndercover mode so toggles persist across turns (issue #274).
// This is registration only — the chat loop itself is unchanged.
func newBuiltinCommandRegistry(client *llm.Client, model string) *commands.Registry {
	r := commands.NewRegistry()
	r.Register(commands.NewBTWCommand(chatSideLLM{c: client, model: model}, ""))
	r.Register(commands.NewUndercoverCommand(chatUndercover))
	r.Register(commands.NewInitCommand())
	return r
}

func runChatTUI(ctx context.Context, opts *chatOptions) error {
	logger.SetLevel(logger.LevelError)

	pm := tui.NewModel()
	pm.SwitchView(tui.ViewChat)
	ws, _ := chatGetwdFn()
	pm.Workspace = ws
	pm.SetContextFn(func() context.Context { return ctx })

	maxTurns := opts.maxTurns
	if maxTurns == 0 {
		maxTurns = 80
	}
	pm.AgentConfig = tui.AgentRunnerConfig{
		Yolo:       opts.yolo,
		MaxTurns:   maxTurns,
		Model:      opts.model,
		VerifyMode: opts.verifyMode,
		VerifyCmd:  opts.verifyCmd,
	}

	pm.OnRun = func(name string, args []string) error {
		c := getSubcommand(name)
		if c == nil {
			return fmt.Errorf("unknown subcommand: %s", name)
		}
		c.SetArgs(args)
		c.SetOut(os.Stdout)
		c.SetErr(os.Stderr)
		return c.Execute()
	}

	if ov := loadTUIKeyOverrides(); ov != nil {
		km := tui.DefaultKeymap()
		km.ApplyOverrides(*ov)
		tui.SetKeymap(km)
	}

	guard := tui.SetupPlatformGuard()
	defer guard.Cleanup()

	return tui.RunProgram(pm, tui.ProgramOptions{
		Sigusr2Reload: true,
	})
}
