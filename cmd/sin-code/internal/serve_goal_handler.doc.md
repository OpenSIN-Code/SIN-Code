# serve_goal_handler.doc.md

**Purpose:** MCP handlers for `sin_goal_add`, `sin_goal_list`, `sin_goal_status`, `sin_goal_complete` — asynchronous goal queue management.
**Docs:** Direct API access to `autonomy.Queue` (no subprocess). `handleGoalAdd` supports contract criteria (activates stop-gate). `goalQueuePath` var is overridable for tests.
