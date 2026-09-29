---
id: UC-09-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-28
related: UC-09
---

# UC-09 UI states

| Screen | State | Behavior |
|---|---|---|
| Recent/history | Loading/empty | Keep selected Environment visible; show loading or no deployments, never mock rows. |
| Recent/history | Ready | Show newest first, status filter, and detail links. |
| Recent/history | Error | Show error and retry without claiming empty history. |
| Detail | Loading/not found | Keep Application/Environment navigation; show loading or scoped not-found. |
| Detail | Failed | Show persisted failure reason and last recorded states. If planning failed before Snapshot, leave Delta/graph/batches absent rather than inventing them. |
| Detail | Succeeded | Show persisted plan, graph, batches, resource/workload states and redacted outputs. |
