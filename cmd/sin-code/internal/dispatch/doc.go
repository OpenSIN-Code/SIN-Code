// Package dispatch turns loaded command and agent assets into executable
// actions, completing the pipeline: load (assets) -> select (Selector) ->
// dispatch (this package) -> run.
//
// Slash commands ("/tdd add login") are resolved against the asset registry,
// with ECC-style placeholders ($ARGUMENTS, $1..$9, $@, ${flag}) substituted,
// then submitted to the main agent loop via PromptSink. Agent assets are
// resolved into AgentInvocations (system prompt + model hint + tool whitelist)
// and run by the orchestrator via SubagentRunner, either by explicit name or
// by best-match selection for a task Context.
//
// SPDX-License-Identifier: MIT
package dispatch
