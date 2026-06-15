// Package instinct implements SIN-Code's continuous-learning system, a Go port
// of the ECC "homunculus" model.
//
// An Instinct is a small, confidence-weighted learned behavior (trigger ->
// action) discovered from session activity. Instincts are project-scoped by
// default (keyed on the git remote) and may be promoted to global scope once
// seen across multiple projects. High-confidence clusters can evolve into
// skills, commands, or agents.
//
// Lifecycle:
//
//	observe  -> Observer.Record / Manager.Observe (create or reinforce)
//	mature   -> Reinforce / Contradict / Decay adjust confidence
//	activate -> status flips to active at the configured threshold
//	inject   -> RenderSystemBlock feeds active instincts into the system prompt
//	evolve   -> Evolve graduates clusters into reusable artifacts
//	promote  -> FindPromotable / Promote raise cross-project instincts to global
//
// Storage is plain Markdown-with-frontmatter on disk (ECC-compatible), under
// $SIN_INSTINCT_DIR | $XDG_DATA_HOME/sin-code/instinct | ~/.local/share/...
// Mutations are guarded by cross-process file locks.
//
// SPDX-License-Identifier: MIT
package instinct
