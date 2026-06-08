// SPDX-License-Identifier: MIT
// Purpose: plugin hook wiring — fires the [[hooks]] declared in plugin.toml
// for the same todo events the built-in HookConfig fires. Plugin hooks run
// as subprocesses (sh -c) with the same SIN_TODO_* env vars so the plugin
// can react to todo state changes identically to a user-configured hook.
package todo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/plugins"
)

var (
	pluginRegOnce sync.Once
	pluginReg     *plugins.Registry
)

func pluginRegistry() *plugins.Registry {
	pluginRegOnce.Do(func() {
		pluginReg = plugins.NewRegistry()
		_ = pluginReg.LoadFromDir("")
	})
	return pluginReg
}

// firePluginHooks runs every enabled plugin hook registered for the given
// event, with the same HookContext semantics the built-in hooks use. Errors
// are logged to stderr but never block the caller — the primary op already
// succeeded by the time hooks fire.
func firePluginHooks(event HookEvent, t *Todo, from, to, note string) {
	reg := pluginRegistry()
	if reg == nil {
		return
	}
	hooks := reg.HooksFor(string(event))
	if len(hooks) == 0 {
		return
	}
	ctx := HookContext{Event: event, Todo: t, From: from, To: to, Note: note, Actor: currentActor()}
	for _, h := range hooks {
		runPluginHook(h, ctx)
	}
}

func runPluginHook(h plugins.HookDef, ctx HookContext) {
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", h.Command)
	cmd.Env = buildEnv(ctx)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "plugin-hook warning: plugin=%s event=%s cmd=%q err=%v stderr=%s\n",
			h.Plugin, h.Event, h.Command, err, stderr.String())
	}
}
