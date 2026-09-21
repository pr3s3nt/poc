---
id: VERIFY-2026-09-21-WALKING-SKELETON
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-21
---

# 2026-09-21 — Local walking skeleton

## Scope

HTTP API → DeploymentService → Planner → ResourceProvisioningService → fake
ResourceExecutor → output propagation → WorkloadRenderer → fake
WorkloadDeployer → persistence → UC-09 API, cùng React Deploy/Details flow.

## Result

- Go build/vet/test passed; ten packages had tests at this point.
- Frontend typecheck, lint, 23 tests and production build passed.
- Live HTTP smoke and jsdom live-console test passed.
- Three deployments (`backend`, `worker`, `frontend`) reached `SUCCEEDED`.
- Backend and worker referenced shared `acceptance-db`; all workloads were
  `READY`.
- PostgreSQL password appeared as `***redacted***` in deployment view.
- `/ui/`, assets and browser fallback route returned success.

## Boundary at this execution

ResourceExecutor/WorkloadDeployer were fake and state used in-memory + JSON
snapshot. This evidence was superseded operationally by later kind/AWS runs but
remains the walking-skeleton snapshot.
