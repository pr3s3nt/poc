---
id: VERIFY-STAGING-CATALOG-20261026
artifact: verification
status: current
last_reviewed: 2026-10-26
---

# Staging bootstrap and operator catalog

User scope: remove Connections UI, stop sample account/Application/catalog seed,
seed Kubernetes contracts, and supply Playwright registration after the operator
creates Internal Driver Account `k8s-4f`. No staging deployment or database cleanup.

## Checks

- `cd backend && go test ./... && go build ./...`: PASS. Integration tests retain
  their existing environment requirements; no live PostgreSQL fixture was provisioned.
- `cd frontend && npm run typecheck && npm run lint`: PASS.
- `cd frontend && npm test`: final run PASS, 920 tests / 77 files. The initial
  run failed one observability follow-interval test; its isolated rerun passed
  5/5 and the final full run passed without changing observability code/tests.
- `cd frontend && npm run build`: PASS; existing large-chunk advisory remains.
- Focused final App/routes/bootstrap tests: PASS, 46/46.
- `bash -n backend/deploy/docker-entrypoint.sh`, Node script syntax: PASS.
- Documentation checker and `git diff --check`: PASS.

## Browser verification

Real Console UI and backend ran locally with fake execution adapters, disposable
JSON state outside Git and a synthetic READY `k8s-4f` account. Installed renderer
used the real local pinned score-k8s binary; no cluster credential was read and
no cluster/database/cloud resource was provisioned.

From an empty Definition catalog, operator Playwright created all five entries
(cluster, namespace, PostgreSQL, Redis, renderer), read back exact pins, mappings,
references and staging criteria, then reran and retained all five unchanged.
The partial-run resume path was also exercised during locator corrections.
No writes were made to the user's staging platform.

UI source/reference: existing AppShell, shared styles/Button and current Driver
Accounts navigation. Browser screenshots before/after outside Git were visually
compared at 1440x1000: Connections is removed, Infrastructure accounts remains,
and shell/layout/styles are retained. Content review: removal needs no added
product instructions; operator details live in the runbook.

## Limits and handoff

[Runbook](../operations/staging-catalog.md) supplies inputs and execution commands.
Operator must deploy the new images, register the real account and run the script.
Live kubeconfig verification, Vault delivery, module provisioning, registry pull
and acceptance Application deployment were not run: external verification is
outside this task. Existing PVC/database data is preserved; reapplying manifests
with the same PostgreSQL PVC does not remove old seed records. No preexisting
user changes were present at task start.
