# orchestrator/episodic.go

Episodic Replay — persist successful plans as searchable episodes in the
existing SQLite memory DB. FTS5 powers similarity retrieval. Both
passed and failed episodes are stored (failed ones become negative
examples for the planner).

## Public surface

- `Episode{ID, Intent, TaskTitle, PlanJSON, Diff, Score, Passed, Rounds, CreatedAt}`
- `EpisodeStore{db}`
  - `NewEpisodeStore(db) *EpisodeStore, error` — schema bootstrap
  - `Record(ctx, ep) error` — append one episode
  - `Similar(ctx, taskTitle, k) []*Episode` — top-k BM25-ranked similar
- `PlanningPrior(episodes []*Episode) string` — render as planner context

## Behavior

- `db == nil` is fully supported: `Record` and `Similar` are no-ops.
  This is the default for tests and for users without a memory DB.
- FTS5 sanitization: query terms shorter than 3 chars and non-alphanumeric
  terms are dropped; remaining terms are quoted and OR-joined.
- `Similar` orders by `passed DESC, bm25(episodes_fts) ASC` — passed
  episodes are always preferred at equal relevance.

## Storage cost

- `episodes` table: ~200 bytes per episode (plan JSON + diff optional).
- `episodes_fts` virtual table mirrors it for search; negligible overhead.
