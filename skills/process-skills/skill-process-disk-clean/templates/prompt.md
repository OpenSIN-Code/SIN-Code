# Prompt Templates — skill-process-disk-clean

## Template A: Full cleanup

```
Clean my Mac disk. First report current capacity with df -h /.
Then delete all safe caches (Yarn, bun, npm, go-build, trivy,
claude transcripts, claude plugin cache, chrome_pipeline, webauto).
Report GB freed. Do NOT stop opencode.
```

## Template B: Max reclamation (VACUUM)

```
My disk is almost full. Stop all opencode processes, then VACUUM
~/.local/share/opencode/opencode.db to reclaim freelist space.
Report before/after size and free GB. No sessions should be lost.
```

## Template C: Diagnostic only

```
Why is my disk 97% full? Survey ~/Library, ~/.local, ~/.config and
report the top 25 directories by size. Also check opencode.db page
and freelist counts to see if VACUUM would help. Do not delete anything.
```

## Template D: Ask-class

```
Check ~/.config/sin-solver/authd/state-backups/ — report size and
age of each backup. Ask me before deleting anything there.
```
