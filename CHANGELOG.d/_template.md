<!-- one file per PR, named <PR>-<slug>.md -->
<!-- PR is the GitHub PR number (e.g. 222, 235). If you don't have one yet, -->
<!-- use the next free 4-digit slot and rename on PR creation.               -->
<!--
  Slug is a short kebab-case label (e.g. changelog-d, subagent-cli).

  The aggregator sorts fragments by PR number ascending, so the order
  in CHANGELOG.md is deterministic and conflict-free across PRs.

  Two sections are recognised per fragment:

    ### Added — <feature name> (PR #<n>)
    ### Changed — <what changed> (PR #<n>)
    ### Fixed — <what was broken> (PR #<n>)
    ### Removed — <what is gone> (PR #<n>)

  Each section is one line: "<category> — <headline> (PR #<n>)".
  Body lines starting with `-` follow on subsequent lines.

  Example:

    ### Added — Workspace checkpointing + rewind (PR #235)

    - Content-addressed snapshot store backed by modernc.org/sqlite
    - `sin-code checkpoint [label]` captures the dirty set
    - `sin-code rewind [id|--list]` restores the workspace
    - `Loop.BeforeMutate` auto-snapshots `sin_write`/`sin_edit`
-->
