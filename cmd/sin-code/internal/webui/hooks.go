// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when webui is refactored
// Purpose: test hooks for the web UI server. These indirection points allow
// error-path coverage without heavy refactoring or real network/subprocess
// dependencies.
package webui

import (
	"context"
	"html/template"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

var (
	netListenHook     = net.Listen
	signalNotifyHook  = signal.Notify
	openBrowserHook   = openInBrowser
	userConfigDirHook = os.UserConfigDir
	osTempDirHook     = os.TempDir
	goosHook          = func() string { return runtime.GOOS }
	lookPathHook      = exec.LookPath
	readDirHook       = os.ReadDir
	browserExecHook   = exec.Command
	readFileHook      = os.ReadFile
	// execCommandRunner hard-caps each invocation at 2s so a hung docker
	// daemon (or any unresponsive subprocess on `ps`/`docker`/`which`)
	// cannot wedge the request goroutine or inflate the test suite.
	execCommandRunner = func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, name, args...).Output() // #nosec G204
	}
	orchestratorRunFunc = func(ctx context.Context, prompt string) (*orchestrator.Result, error) {
		return orchestrator.New().Run(ctx, prompt)
	}
	todoOpenHook  = todo.Open
	notifOpenHook = notifications.Open
	todoListHook  = func(s *todo.Store) ([]*todo.Todo, error) { return s.List() }
	todoAddHook   = func(s *todo.Store, t *todo.Todo) error { return s.Add(t) }
	notifListHook = func(s *notifications.Store, filter notifications.ListFilter, limit int) ([]*notifications.Notification, error) {
		return s.List(filter, limit)
	}
	templateCloneHook = func(t *template.Template) (*template.Template, error) { return t.Clone() }
	templateParseHook = func(t *template.Template, text string) (*template.Template, error) { return t.Parse(text) }
	templateExecHook  = func(t *template.Template, wr io.Writer, name string, data interface{}) error {
		return t.ExecuteTemplate(wr, name, data)
	}
	parseFSHook = func(t *template.Template, f fs.FS) (*template.Template, error) { return t.ParseFS(f, "*.html") }
)
