// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` — interactive REPL and headless one-shot mode.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/commands"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
)

var (
	chatLoadAgentHook   = loadAgentProfile
	chatNewMCPManagerHook = mcpclient.NewManager
	chatConnectAllHook  = func(mgr *mcpclient.Manager, ctx context.Context) error { return mgr.ConnectAll(ctx) }
	chatOpenSessionHook = func(p string) (*session.Store, error) { return session.Open(p) }
	chatStartOrResumeHook = func(s *session.Store, id string) (*session.Session, error) { return s.StartOrResume(id) }
	chatRunOverrideHook   = func(loop *agentloop.Loop, ctx context.Context, sess *session.Session, prompt string) (agentloop.Result, error) {
		return loop.Run(ctx, sess, prompt)
	}
	chatGetwdHook = os.Getwd
)

func init() { rootCmd.AddCommand(newChatCmd()) }

func newChatCmd() *cobra.Command {
	var prompt, sessionID, workspace, model, profile, verifyCmd, verifyMode string
	var headless, newSession, yolo, noSession bool
	var maxTurns int

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Interactive REPL or headless one-shot coding session",
		RunE: func(cmd *cobra.Command, args []string) error {
			if workspace == "" {
				var err error
				workspace, err = chatGetwdHook()
				if err != nil {
					return err
				}
			}

			agent, err := chatLoadAgentHook(profile, model)
			if err != nil {
				return err
			}
			if agent.Model == "" {
				return fmt.Errorf("no model configured; set --model or create a profile")
			}

			mgr, err := chatNewMCPManagerHook()
			if err != nil {
				return err
			}
			defer mgr.Close()
			if err := chatConnectAllHook(mgr, cmd.Context()); err != nil {
				return fmt.Errorf("MCP connect failed: %w", err)
			}

			var sessStore *session.Store
			var sess *session.Session
			if !noSession {
				var err error
				sessStore, err = chatOpenSessionHook(session.DefaultPath())
				if err != nil {
					return err
				}
				defer sessStore.Close()
				if newSession {
					sess = sessStore.New()
				} else {
					sess, err = chatStartOrResumeHook(sessStore, sessionID)
					if err != nil {
						return err
					}
				}
				if sessionID != "" && sess.ID != sessionID {
					return fmt.Errorf("session %q not found", sessionID)
				}
			} else {
				sess = &session.Session{ID: "ephemeral", Workspace: workspace}
			}

			lessonStore, _ := lessons.Open("")
			defer func() {
				if lessonStore != nil {
					lessonStore.Close()
				}
			}()

			loop, cleanup, err := loopbuilder.Build(cmd.Context(), loopbuilder.Config{
				Workspace:   workspace,
				SessionID:   sess.ID,
				Agent:       agent,
				Manager:     mgr,
				MaxTurns:    maxTurns,
				VerifyMode:  verifyMode,
				VerifyCmd:   verifyCmd,
				Headless:    headless,
				YOLO:        yolo,
				ToolFactory: func(mgr *mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {
					return combinedTool(workspace, mgr), combinedSpecs(mgr)
				},
				Style: style.RenderRules{},
			}, lessonStore)
			if err != nil {
				return err
			}
			defer cleanup()

			userPrompt := prompt
			if userPrompt == "" && len(args) > 0 {
				userPrompt = args[0]
			}
			if headless && userPrompt == "" {
				return fmt.Errorf("headless mode requires a prompt")
			}
			if userPrompt == "" {
				userPrompt = "Hello"
			}

			res, err := chatRunOverrideHook(loop, cmd.Context(), sess, userPrompt)
			if err != nil {
				return err
			}
			if headless {
				if res.Verified {
					fmt.Fprintln(cmd.OutOrStdout(), res.Summary)
				} else {
					return fmt.Errorf("verification failed")
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"session_id": sess.ID,
					"summary":    res.Summary,
					"verified":   res.Verified,
					"turns":      res.Turns,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n[session %s, turns %d, verified %v]\n", sess.ID, res.Turns, res.Verified)
			return nil
		},
	}
	cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "headless prompt (one-shot mode)")
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "resume session id")
	cmd.Flags().StringVarP(&workspace, "workspace", "w", "", "workspace directory")
	cmd.Flags().StringVarP(&model, "model", "m", os.Getenv("SIN_MODEL"), "model override")
	cmd.Flags().StringVarP(&profile, "profile", "P", "default", "agent profile name")
	cmd.Flags().StringVar(&verifyCmd, "verify-cmd", os.Getenv("SIN_VERIFY_CMD"), "verification command")
	cmd.Flags().StringVar(&verifyMode, "verify-mode", "poc", "verify mode: off/poc/oracle")
	cmd.Flags().BoolVarP(&headless, "headless", "H", false, "headless one-shot mode")
	cmd.Flags().BoolVarP(&newSession, "new", "n", false, "force a new session")
	cmd.Flags().BoolVar(&yolo, "yolo", false, "auto-allow permission-ask decisions in headless mode")
	cmd.Flags().BoolVar(&noSession, "no-session", false, "do not use session DB (ephemeral)")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 60, "max agent turns")
	return cmd
}

func loadAgentProfile(name, model string) (agentloop.Agent, error) {
	var a agentloop.Agent
	if model != "" {
		a.Model = model
		return a, nil
	}
	if name == "" || name == "default" {
		a.Model = os.Getenv("SIN_MODEL")
		if a.Model == "" {
			a.Model = "openai/gpt-4o"
		}
		return a, nil
	}
	p, err := loadProfile(name)
	if err != nil {
		return a, err
	}
	if p.Model == "" {
		return a, fmt.Errorf("profile %q has no model", name)
	}
	return agentloop.Agent{Model: p.Model, BaseURL: p.BaseURL, APIKey: p.APIKey}, nil
}

func loadProfile(name string) (struct {
	Model, BaseURL, APIKey string
}, error) {
	var p struct {
		Model, BaseURL, APIKey string
	}
	path := filepath.Join(configDir(), "profiles", name+".toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	if err := commands.ParseTOML(data, &p); err != nil {
		return p, err
	}
	return p, nil
}

func configDir() string {
	if d := os.Getenv("SIN_CONFIG_DIR"); d != "" {
		return d
	}
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "sin")
}
