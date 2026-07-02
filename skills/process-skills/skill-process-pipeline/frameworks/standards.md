# Pipeline Frameworks — Stage Standards

## NEVER-SKIP Systems

| System | Mandate | Stage | Reason |
|---|---|---|---|
| stop-gate / dodone | M3 | 6 | Verification gate is sacred |
| self-review | — | 5 | CEO review is mandatory |
| sin_quality_gate | — | 4, 6 | Build+test+vet is mandatory |
| sandbox + egress | M3/M4 | 0, 4 | Security baseline |
| web search (M8) | M8 | 2 | context7 + web search before coding |

## Checkpoint Strategy

| Checkpoint | Created after | Rewind use case |
|---|---|---|
| pipeline-stage-0 | Pre-flight | Catastrophic failure — start over |
| pipeline-stage-2 | Plan approved | Bad plan execution — re-plan |
| pipeline-stage-3 | GSD setup | GSD corruption — re-init |
| pipeline-stage-4 | All code written | Review/verify failure — keep code |
| pipeline-stage-5 | Review passes | DoDone failure — keep review |

## Loop-Back Logic

Stage 6 fails → findings to Stage 4 → fix → re-run 5+6 → max 3 iterations → escalate to user.
