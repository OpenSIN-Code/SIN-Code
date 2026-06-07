// SPDX-License-Identifier: MIT
// Purpose: webui cobra command — starts the sin-code web UI HTTP server.
package main

import (
	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/webui"
)

var (
	webuiPort    int
	webuiHost    string
	webuiOpen    bool
	webuiTodoDB  string
	webuiNotifDB string
)

var webuiCmd = &cobra.Command{
	Use:   "webui",
	Short: "Start the sin-code web UI server",
	Long: `Start an HTTP server that exposes the sin-code orchestrator, todo store,
notifications, and EFM stacks through a browser. Uses only the Go standard
library — no web framework, no JavaScript. All templates and static assets
are embedded in the binary.

Defaults:
  --host  127.0.0.1   (loopback only — safe by default)
  --port  27402

Examples:
  sin-code webui
  sin-code webui --port 8080 --host 0.0.0.0
  sin-code webui --open
  sin-code webui --todo-db /tmp/my.db --notif-db /tmp/notif.db

Endpoints:
  GET  /                            Landing page
  GET  /orchestrator                 Prompt form
  POST /orchestrator/run             Run a prompt
  GET  /todos                        List todos
  POST /todos/add                    Add a todo
  GET  /todos/{id}                   Todo detail
  GET  /notifications                List notifications
  GET  /efm                          List EFM stacks
  GET  /efm/{name}                   EFM stack details
  GET  /api/orchestrator/agents.json Agent catalog (JSON)
  GET  /api/todos.json               Todos (JSON)
  GET  /api/notifications.json       Notifications (JSON)
  GET  /static/{file}                Static CSS

The server binds to 127.0.0.1 by default for security. Use --host 0.0.0.0 to
expose on all interfaces.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := webui.Config{
			Host:        webuiHost,
			Port:        webuiPort,
			TodoDB:      webuiTodoDB,
			NotifDB:     webuiNotifDB,
			OpenBrowser: webuiOpen,
		}
		return webui.StartWith(cfg)
	},
}

func init() {
	webuiCmd.Flags().IntVar(&webuiPort, "port", 27402, "Port to listen on")
	webuiCmd.Flags().StringVar(&webuiHost, "host", "127.0.0.1", "Host to bind to (use 0.0.0.0 to expose on all interfaces)")
	webuiCmd.Flags().BoolVar(&webuiOpen, "open", false, "Open browser after starting (xdg-open / open)")
	webuiCmd.Flags().StringVar(&webuiTodoDB, "todo-db", "", "Path to todo bbolt DB (default ~/.config/sin-code/todo.db)")
	webuiCmd.Flags().StringVar(&webuiNotifDB, "notif-db", "", "Path to notifications bbolt DB (default ~/.config/sin-code/notifications.db)")
}
