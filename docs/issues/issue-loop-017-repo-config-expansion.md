# [loop-017] `.sin-code.yml` expansion — declarative per-repo control of the whole loop

**Labels:** `loop-system` `config` `dx` `p2`
**Branch:** `loop-issues`
**Affects:** `internal/repoconfig/config.go`, `loopbuilder/builder.go`, `cmd/sin-code/daemon_cmd.go`
**Tier:** 3
**Depends on:** #155 (repo config — done), and ideally #010 #013 #016

---

## Problem

`.sin-code.yml` (#155) currently only exposes budgets, verify-mode, and
`disable_checks`. As the loop gains Reflector (#152), sub-agent concurrency
(#009), speculative checks (#013), and model routing (#016), all of those are
hardcoded or global. Teams should be able to declare the entire loop policy
per repo, versioned in git.

---

## Current state

```go
// internal/repoconfig/config.go — today
type Config struct {
    MaxTurns       int      `yaml:"max_turns"`
    MaxStopRejects int      `yaml:"max_stop_rejects"`
    StallThreshold int      `yaml:"stall_threshold"`
    MaxTokens      int      `yaml:"max_tokens"`
    VerifyMode     string   `yaml:"verify_mode"`
    DisableChecks  []string `yaml:"disable_checks"`
}
```

---

## Proposed solution

### 1. Expand the config schema (all additive, all optional)

```go
// internal/repoconfig/config.go
type Config struct {
    // --- existing (loop-155) ---
    MaxTurns       int      `yaml:"max_turns"`
    MaxStopRejects int      `yaml:"max_stop_rejects"`
    StallThreshold int      `yaml:"stall_threshold"`
    MaxTokens      int      `yaml:"max_tokens"`
    VerifyMode     string   `yaml:"verify_mode"`
    DisableChecks  []string `yaml:"disable_checks"`

    // --- new ---
    // Reflection toggles the self-review pass (loop-152). nil = leave default.
    Reflection *bool `yaml:"reflection,omitempty"`

    // ReplanBudget caps stall-triggered re-plans (loop-010).
    ReplanBudget int `yaml:"replan_budget,omitempty"`

    // Subagents controls delegation (loop-009).
    Subagents struct {
        Enabled     *bool `yaml:"enabled,omitempty"`
        Concurrency int   `yaml:"concurrency,omitempty"`
    } `yaml:"subagents,omitempty"`

    // Speculative controls mid-run fast checks (loop-013).
    Speculative struct {
        Enabled *bool `yaml:"enabled,omitempty"`
        Every   int   `yaml:"every,omitempty"`
    } `yaml:"speculative,omitempty"`

    // Models maps turn roles to model identifiers (loop-016).
    Models map[string]string `yaml:"models,omitempty"`

    // CustomChecks lets a repo declare extra deterministic checks inline.
    CustomChecks []CustomCheck `yaml:"custom_checks,omitempty"`

    // Clarify configures the confidence band (loop-014).
    Clarify struct {
        Lo  float64 `yaml:"lo,omitempty"`
        Hi  float64 `yaml:"hi,omitempty"`
        Max int     `yaml:"max,omitempty"`
    } `yaml:"clarify,omitempty"`
}

// CustomCheck is a user-declared deterministic verification command.
type CustomCheck struct {
    Name        string `yaml:"name"`
    Cmd         string `yaml:"cmd"`          // run via `sh -c`
    Speculative bool   `yaml:"speculative"`  // eligible for mid-run runs (loop-013)
    TimeoutSec  int    `yaml:"timeout_sec"`
}
```

### 2. Validation (fail loud on nonsense, never on absence)

```go
// internal/repoconfig/config.go
// Validate returns an error for self-contradictory configs (e.g. negative
// budgets, clarify band where Lo >= Hi). A wholly empty config is valid.
func (c Config) Validate() error {
    if c.MaxTurns < 0 || c.MaxTokens < 0 || c.ReplanBudget < 0 {
        return fmt.Errorf("repoconfig: negative budget value")
    }
    if c.Clarify.Hi != 0 && c.Clarify.Lo >= c.Clarify.Hi {
        return fmt.Errorf("repoconfig: clarify.lo (%.2f) must be < clarify.hi (%.2f)",
            c.Clarify.Lo, c.Clarify.Hi)
    }
    for _, m := range c.Models {
        if strings.TrimSpace(m) == "" {
            return fmt.Errorf("repoconfig: empty model id in models map")
        }
    }
    return nil
}
```

Wire it into `Load`:

```go
// internal/repoconfig/config.go — Load, after unmarshal
if err := cfg.Validate(); err != nil {
    return Config{}, err
}
```

### 3. Apply the expanded config in the builder

```go
// loopbuilder/builder.go — extend the existing override block.
if rc, err := repoconfig.Load(cfg.Workspace); err == nil {
    // existing: MaxTurns / MaxStopRejects / StallThreshold / MaxTokens / VerifyMode ...

    if rc.Reflection != nil && !*rc.Reflection {
        loop.Reflector = nil // explicitly disabled by the repo
    }
    if rc.ReplanBudget > 0 {
        loop.ReplanBudget = rc.ReplanBudget
    }
    if rc.Subagents.Enabled != nil && !*rc.Subagents.Enabled {
        loop.SubagentStore = nil
    }
    if rc.Subagents.Concurrency > 0 {
        loop.SubagentConcurrency = rc.Subagents.Concurrency
    }
    if rc.Speculative.Every > 0 {
        loop.SpeculativeEvery = rc.Speculative.Every
    }
    if rc.Clarify.Hi > 0 {
        loop.ClarifyBand.Lo, loop.ClarifyBand.Hi = rc.Clarify.Lo, rc.Clarify.Hi
    }
    if rc.Clarify.Max > 0 {
        loop.MaxClarifications = rc.Clarify.Max
    }
    if len(rc.Models) > 0 {
        cfg.Models = rc.Models // feed the router builder (loop-016)
    }
} else {
    fmt.Fprintf(os.Stderr, "warn: ignoring invalid %s: %v\n", repoconfig.FileName, err)
}
```

### 4. Custom checks feed the contract

```go
// daemon_cmd.go — translate CustomChecks into orchestrator.Check and append
// to the resolved contract (and mark speculative ones for loop-013).
for _, cc := range repoCfg.CustomChecks {
    timeout := time.Duration(cc.TimeoutSec) * time.Second
    if timeout == 0 {
        timeout = 10 * time.Minute
    }
    resolved.DeterministicChecks = append(resolved.DeterministicChecks, orchestrator.Check{
        Kind: orchestrator.CheckPredicate, Name: cc.Name,
        Cmd: []string{"sh", "-c", cc.Cmd}, Timeout: timeout, Speculative: cc.Speculative,
    })
}
```

### 5. Example `.sin-code.yml`

```yaml
# .sin-code.yml — full policy example
max_turns: 40
max_tokens: 2000000
stall_threshold: 3
replan_budget: 1
verify_mode: strict

disable_checks:
  - no-new-todos

reflection: true

subagents:
  enabled: true
  concurrency: 6

speculative:
  enabled: true
  every: 1

clarify:
  lo: 0.7
  hi: 1.0
  max: 2

models:
  work: openai/gpt-5-mini
  reflection: openai/gpt-5-mini
  stopgate: anthropic/claude-opus-4.6
  work_cheap: openai/gpt-5-mini

custom_checks:
  - name: integration-tests
    cmd: make integration
    speculative: false
    timeout_sec: 600
```

---

## Acceptance criteria

- [ ] `Config` expanded with reflection, replan, subagents, speculative, models, clarify, custom_checks (all optional)
- [ ] `Config.Validate()` rejects contradictory configs; empty config valid
- [ ] `Load` runs validation and returns a clear error on invalid files
- [ ] builder applies every new field, only when set (pointers/zero-guards)
- [ ] custom checks become deterministic checks; speculative flag honored
- [ ] documented example `.sin-code.yml` in `docs/`
- [ ] tests for each new field's override + validation failures
- [ ] `go test -race ./...` green
