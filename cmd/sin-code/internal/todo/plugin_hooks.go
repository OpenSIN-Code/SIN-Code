package todo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

func firePluginHooks(store *Store, event HookEvent, t *Todo, from, to, note string) {
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
		stdout, stderr, exitCode, err := runPluginHook(h, ctx)
		note := strings.TrimSpace(stdout)
		if note == "" {
			note = strings.TrimSpace(stderr)
		}
		if note == "" {
			note = fmt.Sprintf("exit=%d", exitCode)
		}
		if err != nil {
			note += " err=" + err.Error()
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID:    t.ID,
			Actor:     currentActor(),
			Action:    "plugin_hook:" + h.Event,
			From:      h.Plugin,
			To:        note,
			Timestamp: time.Now(),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "plugin-hook warning: plugin=%s event=%s cmd=%q err=%v stderr=%s\n",
				h.Plugin, h.Event, h.Command, err, stderr)
		}
	}
}

func runPluginHook(h plugins.HookDef, ctx HookContext) (stdout, stderr string, exitCode int, err error) {
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", h.Command)
	cmd.Env = buildEnv(ctx)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if runErr != nil {
		exitCode = -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		return stdout, stderr, exitCode, runErr
	}
	return stdout, stderr, 0, nil
}
