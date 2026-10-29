---
id: VER-2026-10-29-MCP-KIMI-ACCEPTANCE-KIND
artifact: verification-record
status: evidence
last_reviewed: 2026-10-29
related: [ADR-028, MCP-CONTRACT, UC-01, UC-05, UC-06, UC-09, UC-16]
---

# Kimi Code MCP acceptance deployment on kind

Tested commit `b3ed292` with Kimi Code CLI `2.4.0`, model `K3.1 Pro`, in
interactive tmux session `kimi-mcp-acceptance-mcp024435` and Kimi session
`session_120715f3-b476-4ea7-88e5-06d972d79a70`. The run ID was `mcp024435`.
The user authorized a real, disposable kind deployment and Vietnamese
interaction. No AWS resources or retained clusters were changed.

## Boundary and setup

The runner created a run-owned kind cluster, Compose project, workload Vault,
VSO, Envoy Gateway and acceptance images from the checkout. It also registered
the ordinary technical catalog and production render bundle before starting
Kimi. These are prerequisites, not actions attributed to Kimi.

MCP V1 has no Connection, Secret Store, Resource Type/Definition, render
bundle or Environment configuration tools. After Kimi created the Application,
the runner therefore generated the real Score files and configured the staging
variables/secret through the existing Console. The MCP server authenticated as
the synthetic `developer` user, listened only on loopback, allowed mutations
only in `staging`, and wrote a private JSONL audit.

## Kimi actions through MCP

Kimi created Application `ae134b36-b1a8-4122-8081-c8dfd45423df` after a
`confirm:false` dry run and confirmed creation; read-back returned `ACTIVE`.
It read all three Score YAML files, dry-ran and saved backend, worker and
frontend, previewed the three-workload pending delta, then deployed with the
single-use preview token. The preview contained backend, frontend and worker
with action `DEPLOY`.

The first worker save used version 0 and received the real API response
`409 STALE_VERSION`, because saving backend had advanced the shared Environment
draft. Kimi recovered with current draft versions: backend 0, worker 1 and
frontend 2. The audit contains 15 model-issued MCP calls and exactly one such
409; all dry runs and all other calls have their expected outcomes.

The synchronous Deploy and all three subsequent deployment reads succeeded:

| Workload | Deployment ID | Status |
|---|---|---|
| backend | `a2b71357-fe64-44b9-981d-835e1865e458` | `SUCCEEDED` |
| frontend | `a9d384cf-5507-4835-be1f-4c9929ec9bf6` | `SUCCEEDED` |
| worker | `25b51f81-b62e-4772-91e9-ee0143cf4616` | `SUCCEEDED` |

## Independent application checks

Read-only Kubernetes checks found frontend, backend and worker Deployments at
1/1 available, PostgreSQL and Redis StatefulSets at 1/1 ready, the migration
Job complete, and the rendered HPA, PDB, CronJob, bound PVCs and HTTPRoute.
An independent real browser session opened the frontend Service and observed
all five checks PASS: backend connection, environment, secret, database and
cache. It submitted `kimi-mcp-mcp024435`; job 1 reached `DONE` with result
`processed:KIMI-MCP-MCP024435` through the worker.

## Cleanup and retained evidence

The MCP process was stopped and the runner teardown completed with
`final_status=0`. Independent absence checks passed for the run-owned kind
cluster, Compose containers/volumes, workload Vault and all five run-tagged
images. Private inputs were removed by the runner. The tmux session was kept
open for user inspection; the temporary project MCP configuration was removed.

This verifies a real Kimi-driven Application → workload drafts → Preview →
Deploy flow and the acceptance application's database/cache/job path. It does
not claim that MCP V1 alone can bootstrap the technical catalog or Environment
configuration, and it does not exercise update/removal or AWS.
