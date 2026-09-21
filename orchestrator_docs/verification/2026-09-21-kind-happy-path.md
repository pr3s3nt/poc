---
id: VERIFY-2026-09-21-KIND
artifact: verification-evidence
status: evidence
last_reviewed: 2026-09-21
---

# 2026-09-21 — Internal Kubernetes happy path

## Environment

- Existing kind cluster `idp-internal`, context `kind-idp-internal`.
- Latest post-review run: `kind-20260921071211-11863`.
- A run-specific namespace was used; the cluster was not created or deleted.

## Result

- Acceptance frontend/backend/worker images built and loaded locally.
- Namespace executor, PostgreSQL StatefulSet/Service executor, existing-cluster
  adapter and workload deployer ran through the HTTP API.
- Backend, worker, frontend and `acceptance-db-0` became ready.
- End-to-end job flow returned a `processed:KIND-...` result.
- Password was stored in Kubernetes Secret, absent from Deployment plaintext and
  redacted in the deployment view.
- Go tests, frontend live-console test and integration test passed.

## Cleanup proof

The run namespace was deleted in the EXIT trap. Queries for namespaces and
objects labeled with the run ID returned empty. The pre-existing kind cluster
and unrelated namespaces/resources were preserved.
