# OpenCode Subagent Architecture

Source: https://opencode.ai/docs/agents/ + https://opencode.ai/docs/skills/

## Agent Types

### Primary Agents
- Main assistants you interact with directly
- Switch with Tab key
- Built-in: Build (full tools), Plan (read-only)

### Subagents
- Specialized assistants that primary agents invoke
- Can also be @mentioned manually
- Built-in: General (full tools, multi-step), Explore (read-only, fast), Scout (external deps)

## Subagent Configuration

### JSON (opencode.json)
```json
{
  "agent": {
    "code-reviewer": {
      "description": "Reviews code for best practices",
      "mode": "subagent",
      "model": "anthropic/claude-sonnet-4-20250514",
      "prompt": "You are a code reviewer...",
      "permission": { "edit": "deny" }
    }
  }
}
```

### Markdown (~/.config/opencode/agents/review.md)
```yaml
---
description: Reviews code for quality
mode: subagent
model: anthropic/claude-sonnet-4-20250514
temperature: 0.1
permission:
  edit: deny
  bash: deny
---
You are in code review mode...
```

## Key Options
- `description` (required): What the agent does and when to use it
- `mode`: `primary` | `subagent` | `all`
- `model`: Override model per agent (format: `provider/model-id`)
- `temperature`: 0.0-1.0 (lower = focused, higher = creative)
- `steps`: Max agentic iterations before forced text response
- `permission`: Per-tool allow/ask/deny with glob patterns
- `hidden`: Hide from @ autocomplete (still invocable via Task tool)
- `prompt`: Custom system prompt file (`{file:./prompts/build.txt}`)

## Permission System
```json
{
  "permission": {
    "edit": "allow",        // write, edit, apply_patch
    "bash": {
      "*": "ask",           // default: ask for all bash
      "git status *": "allow"  // exception: allow git status
    },
    "task": {
      "*": "deny",              // default: deny all subagents
      "orchestrator-*": "allow"  // exception: allow orchestrator-* subagents
    }
  }
}
```
Rules: last matching rule wins. Glob patterns supported.

## Task Tool (how subagents are invoked)
```
Task({
  description: "Short 3-5 word description",
  prompt: "Full delegation prompt",
  subagent_type: "general" | "explore" | "scout"
})
```
- Multiple Task calls in ONE message = parallel execution
- `task_id` can be passed to continue the same subagent session
- Subagents create child sessions (navigable via keybinds)

## Skill Discovery
Skills are discovered from:
- `~/.config/opencode/skills/<name>/SKILL.md`
- `.opencode/skills/<name>/SKILL.md`
- `~/.claude/skills/<name>/SKILL.md`
- `.agents/skills/<name>/SKILL.md`

Skills are loaded on-demand via the `skill` tool.
Agent sees available skills in `<available_skills>` section.
