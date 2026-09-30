---
id: UC-09-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-09-30
related: UC-09
---

# UC-09 UI states

| Screen | State | Behavior |
|---|---|---|
| Recent/history | Loading/empty | Keep selected Environment visible; show loading or no deployments, never mock rows. |
| Recent/history | Ready | Show newest first, status filter, and detail links. |
| Recent/history | Error | Show error and retry without claiming empty history. |
| Detail | Loading | Keep Application/Environment navigation and show an explicit busy state. |
| Detail | Not found | Show scoped not-found without a retry action; this includes a Deployment outside the session Organization/Application/Environment. |
| Detail | API error | Show a retryable error without rendering stale detail. |
| Detail | Failed | Show safe failure summary and last recorded states. Unknown legacy raw reasons become generic safe guidance. If planning failed before Snapshot, leave Delta/graph/batches absent rather than inventing them. |
| Detail | Succeeded | Show persisted plan, graph, batches, resource/workload states and redacted outputs. |
