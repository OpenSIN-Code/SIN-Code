// SPDX-License-Identifier: MIT
// Purpose: todo event hooks — pre/post shell command execution, TOML config,
// timeout, env-var injection. Inspired by beads (gastownhall/beads).
package todo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

type HookEvent string

const (
	EventPreAdd       HookEvent = "pre_add"
	EventPostAdd      HookEvent = "post_add"
	EventPreUpdate    HookEvent = "pre_update"
	EventPostUpdate   HookEvent = "post_update"
	EventPreClaim     HookEvent = "pre_claim"
	EventPostClaim    HookEvent = "post_claim"
	EventPreComplete  HookEvent = "pre_complete"
	EventPostComplete HookEvent = "post_complete"
	EventPreCancel    HookEvent = "pre_cancel"
	EventPostCancel   HookEvent = "post_cancel"
	EventPreDelete    HookEvent = "pre_delete"
	EventPostDelete   HookEvent = "post_delete"
	EventPreDepAdd    HookEvent = "pre_dep_add"
	EventPostDepAdd   HookEvent = "post_dep_add"
)

func AllEvents() []HookEvent {
	return []HookEvent{
		EventPreAdd, EventPostAdd,
		EventPreUpdate, EventPostUpdate,
		EventPreClaim, EventPostClaim,
		EventPreComplete, EventPostComplete,
		EventPreCancel, EventPostCancel,
		EventPreDelete, EventPostDelete,
		EventPreDepAdd, EventPostDepAdd,
	}
}

func (e HookEvent) Valid() bool {
	for _, v := range AllEvents() {
		if v == e {
			return true
		}
	}
	return false
}

type Hook struct {
	Command string        `toml:"command"`
	Timeout time.Duration `toml:"timeout"`
	OnError string        `toml:"on_error"`
}

func (h Hook) Validate() error {
	if strings.TrimSpace(h.Command) == "" {
		return fmt.Errorf("hook command required")
	}
	if h.Timeout < 0 {
		return fmt.Errorf("timeout must be >= 0")
	}
	if h.OnError != "" && h.OnError != "ignore" && h.OnError != "warn" && h.OnError != "fail" {
		return fmt.Errorf("on_error must be one of ignore|warn|fail")
	}
	return nil
}

type HookConfig struct {
	Hooks map[HookEvent][]Hook `toml:"hooks"`
	path  string
	mu    sync.RWMutex
}

func DefaultHooksPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfg, "sin-code", "hooks.toml")
}

func LoadHooksConfig(path string) (*HookConfig, error) {
	if path == "" {
		path = DefaultHooksPath()
	}
	cfg := &HookConfig{
		Hooks: map[HookEvent][]Hook{},
		path:  path,
	}
	if path == "" {
		return cfg, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("decode hooks.toml: %w", err)
	}
	if cfg.Hooks == nil {
		cfg.Hooks = map[HookEvent][]Hook{}
	}
	for event, hooks := range cfg.Hooks {
		for _, h := range hooks {
			if err := h.Validate(); err != nil {
				return nil, fmt.Errorf("invalid hook for %s: %w", event, err)
			}
		}
	}
	return cfg, nil
}

func (c *HookConfig) Path() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

func (c *HookConfig) Get(event HookEvent) []Hook {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Hooks[event]
}

func (c *HookConfig) Add(event HookEvent, h Hook) error {
	if err := h.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Hooks[event] = append(c.Hooks[event], h)
	return c.save()
}

func (c *HookConfig) Remove(event HookEvent, index int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	hooks, ok := c.Hooks[event]
	if !ok || index < 0 || index >= len(hooks) {
		return fmt.Errorf("hook not found")
	}
	c.Hooks[event] = append(hooks[:index], hooks[index+1:]...)
	if len(c.Hooks[event]) == 0 {
		delete(c.Hooks, event)
	}
	return c.save()
}

func (c *HookConfig) save() error {
	if c.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(c.path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

type HookContext struct {
	Event  HookEvent
	Todo   *Todo
	From   string
	To     string
	Note   string
	Actor  string
}

type HookResult struct {
	Hook    Hook
	Event   HookEvent
	Stdout  string
	Stderr  string
	Err     error
	Elapsed time.Duration
}

func (r HookResult) OK() bool { return r.Err == nil }

func (c *HookConfig) Fire(ctx HookContext) []HookResult {
	hooks := c.Get(ctx.Event)
	if len(hooks) == 0 {
		return nil
	}
	results := make([]HookResult, 0, len(hooks))
	for _, h := range hooks {
		results = append(results, runHook(h, ctx))
	}
	return results
}

func runHook(h Hook, ctx HookContext) HookResult {
	start := time.Now()
	timeout := h.Timeout
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
	elapsed := time.Since(start)
	if execCtx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("hook timed out after %s", timeout)
	}
	return HookResult{
		Hook:    h,
		Event:   ctx.Event,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
		Err:     err,
		Elapsed: elapsed,
	}
}

func buildEnv(ctx HookContext) []string {
	env := os.Environ()
	set := func(k, v string) {
		env = append(env, k+"="+v)
	}
	set("SIN_EVENT", string(ctx.Event))
	set("SIN_ACTOR", ctx.Actor)
	if ctx.Todo != nil {
		set("SIN_TODO_ID", ctx.Todo.ID)
		set("SIN_TODO_TITLE", ctx.Todo.Title)
		set("SIN_TODO_STATUS", string(ctx.Todo.Status))
		set("SIN_TODO_PRIORITY", string(ctx.Todo.Priority))
		set("SIN_TODO_TYPE", string(ctx.Todo.Type))
		set("SIN_TODO_ASSIGNEE", ctx.Todo.Assignee)
		set("SIN_TODO_TAGS", strings.Join(ctx.Todo.Tags, ","))
		set("SIN_TODO_PROJECT", ctx.Todo.Project)
	}
	if ctx.From != "" {
		set("SIN_FROM", ctx.From)
	}
	if ctx.To != "" {
		set("SIN_TO", ctx.To)
	}
	if ctx.Note != "" {
		set("SIN_NOTE", ctx.Note)
	}
	return env
}
