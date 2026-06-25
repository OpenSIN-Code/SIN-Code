# Anthropic Multi-Agent Research System — Key Findings

Source: https://www.anthropic.com/engineering/multi-agent-research-system (Jun 2025)

## Architecture: Orchestrator-Worker Pattern
- Lead agent coordinates, delegates to specialized subagents in parallel
- Lead agent analyzes query → develops strategy → spawns subagents
- Subagents explore independently → return findings → lead agent synthesizes

## 8 Prompt Engineering Principles

1. **Think like your agents** — simulate prompts step-by-step to find failure modes
2. **Teach the orchestrator how to delegate** — each subagent needs: objective, output format, tool/source guidance, clear task boundaries. Without detailed task descriptions, agents duplicate work or leave gaps.
3. **Scale effort to query complexity** — simple: 1 agent / 3-10 tool calls; moderate: 2-4 subagents / 10-15 calls each; complex: 10+ subagents with clearly divided responsibilities
4. **Tool design is critical** — agents need explicit heuristics: examine all tools first, match tool to intent, prefer specialized over generic
5. **Let agents improve themselves** — Claude 4 models can diagnose why an agent is failing and suggest prompt improvements
6. **Start wide, then narrow down** — broad queries first, then progressively narrow
7. **Guide the thinking process** — extended thinking for planning, interleaved thinking after tool results for evaluation
8. **Parallel tool calling transforms speed** — 3-5 subagents in parallel + 3+ tools per subagent = up to 90% time reduction

## Key Statistics
- Multi-agent (Opus lead + Sonnet subagents) outperforms single-agent Opus by 90.2%
- Token usage explains 80% of performance variance (BrowseComp eval)
- Multi-agent uses ~15x more tokens than single chat
- Agents use ~4x more tokens than chat interactions

## Failure Modes (and fixes)
- Spawning 50 subagents for simple queries → embed scaling rules
- Agents continuing when they have sufficient results → explicit stop conditions
- Vague instructions causing duplicate work → detailed task descriptions with boundaries
- SEO content farms over authoritative sources → source quality heuristics
- Sequential execution → parallelize both subagent spawning AND tool calls

## Production Lessons
- **Errors compound** — minor failures cascade; build resume-from-checkpoint systems
- **Debugging is hard** — agents are non-deterministic; need full production tracing
- **Deployment needs care** — rainbow deployments to avoid disrupting running agents
- **Synchronous execution creates bottlenecks** — async would enable better parallelism

## Subagent Output to Filesystem
- Direct subagent outputs can bypass the main coordinator
- Subagents store work in external systems, pass lightweight references back
- Prevents information loss during multi-stage processing
- Reduces token overhead from copying large outputs through conversation
