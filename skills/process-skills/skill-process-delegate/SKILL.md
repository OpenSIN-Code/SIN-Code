---
name: skill-process-delegate
description: >
  Master subagent delegation skill. Teaches the orchestrator how to decompose
  complex tasks and delegate to subagents with complete context, precise task
  boundaries, output contracts, and effort scaling. Triggers on "/delegate-subagents",
  "delegate to subagents", "use parallel subagents", "split this task", "fan out",
  "orchestrate subagents", "multi-agent", "parallelize this work", or when the agent
  recognizes 3+ independent subtasks, multiple files to modify, or research +
  implementation + testing phases. DO NOT INVOKE for single-file edits or quick lookups.
license: MIT
lifecycle: native
compatibility:
  - opencode
  - claude-code
  - codex
  - sin-code
metadata:
  audience: agents
  workflow: delegation
  sources: >
    Anthropic multi-agent research system (anthropic.com/engineering/multi-agent-research-system),
    OpenCode agent/subagent architecture (opencode.ai/docs/agents),
    Claude Code subagent patterns, SIN-Code orchestrator DAG
---

## TRIGGER RULES

**UNCONDITIONAL TRIGGER:** When the user types "/delegate-subagents" or
explicitly says "use the delegate-subagents skill", invoke immediately.

**CONDITIONAL TRIGGER:** When ANY of these hold:
- (a) Task has 3+ independent subtasks that could run in parallel
- (b) Task spans multiple files/modules/domains (e.g. backend + frontend + tests)
- (c) Task involves research + implementation + verification phases
- (d) Task is open-ended with multiple viable approaches
- (e) User says "parallel", "simultaneously", "at the same time", "concurrent"
- (f) User says "mit mehreren subagents" or "parallel machen"

**DO NOT INVOKE for:** single-file edits, quick lookups, one-line fixes,
anything where sequential execution is obviously correct.

## MASTER SUBAGENT DELEGATION SKILL

You are now operating in MASTER DELEGATION MODE. Your job is NOT to do the
work yourself. Your job is to ORCHESTRATE subagents to do the work in
parallel, with maximum context, precision, and quality.

---

### 1. CORE PRINCIPLES (never violate these)

#### 1.1 Think like a master, not like a worker
You are the orchestrator. You decompose, delegate, verify, and integrate.
You do NOT write code, run tests, or read files yourself unless it is
necessary to understand the task before delegating.

#### 1.2 Each subagent gets COMPLETE context
A subagent starts with a FRESH context window. It knows NOTHING about your
conversation, your codebase, your conventions, or your goals. You MUST
provide everything it needs in the delegation prompt:

- **Objective**: What exactly should it accomplish (one sentence)
- **Context**: What is the codebase, what file(s) are relevant, what is the
  project structure, what conventions exist
- **Constraints**: What must it NOT do (don't edit file X, don't change API Y,
  don't add dependencies)
- **Output contract**: What format should the output be in (code, report,
  structured JSON, test results)
- **Verification**: How will you (the orchestrator) verify the subagent's work
- **Task boundaries**: What is IN scope and what is OUT of scope

#### 1.3 Scale effort to task complexity
Match the number of subagents and their depth to the task:

| Complexity | Subagents | Tool calls each | When |
|---|---|---|---|
| Simple | 1 | 3-10 | Single focused change, one file |
| Moderate | 2-4 | 10-15 each | Multi-file feature, research + implement |
| Complex | 5-10 | 15-30 each | Cross-module refactor, new system, full feature |
| Massive | 10+ | 25-50 each | Architecture overhaul, multi-repo, platform |

DO NOT spawn 50 subagents for a simple query. DO NOT spawn 1 subagent for
a complex multi-domain task. Match effort to complexity.

#### 1.4 Parallelize aggressively
Launch independent subagents in the SAME message, not sequentially. If
subagent A's output does not depend on subagent B's output, launch them
both at once. This cuts total time by up to 90%.

#### 1.5 The "game of telephone" problem
When a subagent returns results to you, and you then pass those results to
another subagent, information is LOST at each hop. To minimize this:

- Have subagents write their outputs to FILES when possible (code, reports,
  data), then pass only the FILE PATH to the next subagent
- Include the FULL output of one subagent in the next subagent's prompt
  when there is a dependency — do not summarize
- Use structured output formats (JSON, markdown tables) so downstream
  subagents can parse reliably

---

### 2. THE DELEGATION PROTOCOL

#### Step 1: ANALYZE the task
Before spawning any subagent, think through:

1. What is the end goal? (one sentence)
2. What are the independent subtasks? (list them)
3. Which subtasks have dependencies? (DAG)
4. What context does each subtask need?
5. What is the expected output of each subtask?
6. How will I verify each subtask's output?
7. How will I integrate the outputs?

#### Step 2: DECOMPOSE into subtasks
Break the task into independent, parallelizable units. Each subtask should
be:

- **Self-contained**: Can be completed without waiting for another subtask
- **Well-scoped**: Has a clear single objective
- **Verifiable**: You can check if it was done correctly
- **Right-sized**: Not too small (wastes overhead) and not too big (context
  overflow risk)

#### Step 3: CRAFT delegation prompts
For each subagent, craft a prompt using this template:

```
You are working in the [PROJECT NAME] repository at [ABSOLUTE PATH].

Your task: [ONE SENTENCE OBJECTIVE]

## Context
- Project: [what the project is, language, framework]
- Relevant files: [list specific file paths]
- Conventions: [coding conventions, style rules, test framework]
- Existing patterns: [how similar features are implemented in this codebase]

## What to do
[DETAILED step-by-step instructions]

## Constraints
- Do NOT [specific things to avoid]
- Do NOT edit [files that should not be touched]
- Do NOT add [dependencies, imports, etc. that are not wanted]
- Follow [specific conventions or patterns]

## Output
[What the subagent should produce — code, report, test results, etc.]
[If code: specify the exact file path to write to]
[If report: specify the format (markdown, JSON, etc.)]

## Verification
[How the subagent should verify its own work before returning]
[Examples: "run go build ./...", "run go test -race ./...", "check that
the function compiles and passes existing tests"]

## Important
- Do NOT commit anything. Just write the files.
- Do NOT edit AGENTS.md or CHANGELOG.md.
- Report back: [what information to return in the final message]
```

#### Step 4: LAUNCH in parallel
Launch all independent subagents in a SINGLE message with multiple tool
calls. Do not wait for one to finish before launching the next if they
are independent.

#### Step 5: VERIFY each result
When a subagent returns:

1. Check if it achieved the objective
2. Run verification (build, test, lint) if applicable
3. If it failed, diagnose the failure and either:
   a. Re-launch with a corrected prompt
   b. Fix the issue yourself (if trivial)
   c. Escalate to the user ( if fundamental blocker)

#### Step 6: INTEGRATE
Combine all subagent outputs into the final result:

1. Merge code changes (resolve conflicts if multiple subagents edited the
   same file — this is why you assign non-overlapping files)
2. Run full verification suite (build + test + lint)
3. Update docs/changelog if needed
4. Report to the user

---

### 3. ANTI-PATTERNS (never do these)

#### 3.1 Vague delegation
BAD: "Research the semiconductor shortage"
GOOD: "Research the 2025 semiconductor supply chain constraints affecting
the automotive industry. Focus on: (1) which chip categories are constrained,
(2) which manufacturers are affected, (3) expected timeline for resolution.
Return a markdown report with citations."

#### 3.2 Duplicate work
BAD: Two subagents both searching for the same information
GOOD: Subagent A researches backend impact, Subagent B researches frontend
impact — clear division of labor

#### 3.3 Sequential when parallel is possible
BAD: Launch subagent A, wait, launch subagent B, wait, launch subagent C
GOOD: Launch A, B, C in the same message (if independent)

#### 3.4 Over-delegation
BAD: Spawning a subagent to write a single import statement
GOOD: Batch small changes into one subagent task

#### 3.5 Under-delegation
BAD: Doing a 10-file refactor yourself instead of splitting it
GOOD: 5 subagents each handling 2 files with a shared interface contract

#### 3.6 Lost context
BAD: Subagent A returns a 500-line report, you summarize it to 2 lines for
subagent B
GOOD: Subagent A writes report to /tmp/analysis.md, you tell subagent B
"read /tmp/analysis.md for the full context"

#### 3.7 No verification
BAD: Trusting the subagent's "I'm done" without checking
GOOD: Run `go build ./... && go test -race -count=1 ./...` after all
subagents complete

---

### 4. SUBAGENT TYPE SELECTION

Choose the right subagent type for each task:

| Subagent type | When to use | Tools | Can modify files? |
|---|---|---|---|
| `general` | Multi-step tasks, code changes, complex research | Full (except todo) | Yes |
| `explore` | Codebase exploration, "how does X work?", file finding | Read-only | No |
| `scout` | External dependency research, upstream code comparison | Read-only + clone | No |

For code changes: always use `general` (it has write access).
For research only: use `explore` (faster, read-only, can't break anything).
For external research: use `scout` (can clone and inspect dependencies).

---

### 5. EFFORT BUDGETING

Each subagent call costs tokens. Be conscious of the budget:

- A subagent typically uses 5,000-50,000 tokens per task
- A complex task with 5 subagents might use 100,000-250,000 tokens total
- Only use multi-agent for tasks where the value justifies the cost
- For simple tasks (single file, <50 lines changed), just do it yourself

Cost optimization:
- Use `explore` (read-only, cheaper) instead of `general` when possible
- Batch related small tasks into one subagent
- Set `max_steps` on subagents to prevent runaway agents
- Reuse subagent results — don't re-research what a previous subagent found

---

### 6. FILE ASSIGNMENT STRATEGY

When multiple subagents modify code, assign non-overlapping files:

```
Subagent A: cmd/sin-code/doctor_cmd.go + doctor_cmd_test.go
Subagent B: cmd/sin-code/diff_cmd.go + diff_cmd_test.go
Subagent C: cmd/sin-code/benchmark_cmd.go + benchmark_cmd_test.go
Subagent D: cmd/sin-code/main.go (just adding AddCommand lines — LAST, after A-C)
```

If two subagents MUST edit the same file:
1. Have the first subagent make its changes
2. Verify the changes
3. Then launch the second subagent with the updated file as context

---

### 7. COMMUNICATION CONTRACT

When a subagent returns, it should report:

1. **What it did** (summary of changes)
2. **What files it created/modified** (list of paths)
3. **Verification status** (did build/test/lint pass?)
4. **Any issues encountered** (blockers, unexpected findings)
5. **What it did NOT do** (out-of-scope items, skipped steps)

As orchestrator, you should:
1. Read the full report
2. Verify claims (don't just trust — check)
3. Note any issues for the user
4. Integrate with other subagent results
5. Run final verification

---

### 8. CONTEXT WINDOW MANAGEMENT

Subagents start with a fresh context window. They do NOT see:
- Your conversation history
- Other subagents' work
- Previous failed attempts
- User preferences or constraints not in the prompt

You MUST include in every delegation prompt:
- The absolute path to the repository
- The specific files to work on
- All relevant conventions and constraints
- The expected output format
- Verification commands to run

For tasks that need extensive codebase context, have the subagent use
`explore` tools (glob, grep, read) to discover what it needs — but tell
it WHERE to look so it doesn't waste turns searching blindly.

---

### 9. ERROR HANDLING AND RECOVERY

When a subagent fails:

1. **Analyze the failure**: Read the subagent's output to understand what went wrong
2. **Categorize**: Is it a prompt issue (vague instructions), a knowledge gap
   (missing context), or a fundamental blocker (impossible task)?
3. **Respond**:
   - Prompt issue → re-launch with a clearer, more specific prompt
   - Knowledge gap → re-launch with additional context in the prompt
   - Fundamental blocker → escalate to user, do not retry blindly
4. **Never retry more than 2 times** for the same subtask with the same prompt
5. **If a subagent partially completed work**, note what was done and have
   the next subagent continue from there (pass the partial work as context)

---

### 10. WHEN NOT TO DELEGATE

Do NOT use subagents when:

- The task is a single, trivial change (<10 lines in one file)
- The task requires deep conversation context that can't be compressed
- The task is sequential by nature (each step depends on the previous)
- The user wants a quick answer, not a thorough investigation
- You've already done most of the work and just need to finish
- The task is interactive (requires user back-and-forth)

Just do it yourself in these cases. Delegation has overhead — use it
only when the parallelism and context isolation provide real value.

---

### 11. OPencode-SPECIFIC DELEGATION

In OpenCode, subagents are invoked via the `Task` tool:

```
Task({
  description: "Short 3-5 word description",
  prompt: "FULL delegation prompt (see template above)",
  subagent_type: "general" | "explore" | "scout"
})
```

Key OpenCode behaviors:
- Subagents create CHILD SESSIONS — you can navigate to them with
  `session_child_first` keybind
- Subagents inherit the MODEL of the primary agent unless configured otherwise
- Subagents have their own PERMISSION set — configure in opencode.json
- You can launch MULTIPLE Task calls in a SINGLE message for parallelism
- The `task_id` can be reused to continue the same subagent session

---

### 12. OUTPUT FORMAT FOR THE USER

When you finish orchestrating, report to the user:

```
## Delegation Summary

| Subagent | Task | Status | Files | Notes |
|---|---|---|---|---|
| A | [task] | PASS | [files] | [notes] |
| B | [task] | PASS | [files] | [notes] |
| C | [task] | FAIL→FIXED | [files] | [what went wrong + fix] |

## Verification
- Build: PASS
- Tests: PASS (N tests, 0 failures)
- Lint: 0 issues

## Result
[Integrated summary of what was accomplished]
```

---

### 13. QUICK REFERENCE CARD

```
DELEGATE like a master:
1. ANALYZE → decompose into independent subtasks
2. ASSIGN → non-overlapping files, clear boundaries
3. CONTEXT → full repo path, conventions, constraints, output format
4. LAUNCH → all independent subagents in ONE message
5. VERIFY → build + test + lint after all return
6. INTEGRATE → merge, resolve, report
7. REPORT → table + verification status + summary

NEVER:
- Vague prompts ("research X")
- Sequential when parallel is possible
- Over-delegate trivial tasks
- Trust without verifying
- Lose context through summarization
- Retry blindly more than twice
```
