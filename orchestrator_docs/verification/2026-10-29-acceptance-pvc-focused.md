---
id: VERIFY-20261029-ACCEPTANCE-PVC-FOCUSED
artifact: verification-record
status: evidence
last_reviewed: 2026-10-29
related: UC-16, UC-05, UC-06, UC-08
---

# Focused catalog, workload and PVC acceptance

The user corrected the earlier broad recording: test quickly in headless mode,
only the catalog/workload/PVC behavior. This execution updates the existing
acceptance Application, not another whole-app scenario. No repeated app creation,
settings, async job, PostgreSQL/Redis acceptance or scheduled execution checks.

Runner: [acceptance-pvc](../../e2e/scenarios/acceptance-pvc/README.md).
Procedure: [acceptance storage](../operations/acceptance-storage-schedule.md).
Runtime remains frontend/backend built from `f9addce`; no product code changed.

## Catalog diagnosis and correction

The deployed Console had current code but its only registered render bundle was
`score-k8s-internal-v1`, without workload templates. Its actual authenticated
`workload-templates` response was empty; rebuilding UI alone could not populate it.

Reused the existing [registration script](../../e2e/setup/register-render-bundles/README.md)
in headless mode with zero pacing: registered the `persistent-storage` Type,
Terraform Definition `acceptance-pvc-storage` and Git bundle/renderer Definition
`acceptance-pvc-catalog`, pinned to
`f9addce86c183ce575586edc4a4f3117dcc1d285`. Templates are published to the
Organization catalog. Storage selection is scoped to this app/staging/default;
renderer matching is scoped to this app/staging and `res_id=modules.backend`.
The temporary broader criterion was narrowed before any workload deployment.
No existing platform Definition was edited. The setup verified five real catalog
cards and selection of Backend API → Add volume mount using the actual UI.

## Focused execution

Application `057436dd-04a9-440d-98f1-5d208d666fda`, environment staging, context
`kind-k8s-4f`; existing app URL `http://100.77.224.24:18482`.
`ORCH_HEADLESS=1`, no artificial waits. First focused attempt passed, exit0;
Console recording is 55.12seconds. Its eight assertions establish:

1. Catalog shows five actual verified templates, including Backend API.
2. Existing backend configuration preserves its references and adds the Backend
   API execution pin, literal `ACCEPTANCE_STORAGE_DIR=/data` and 1Gi managed
   storage via the product Add volume mount dialog.
3. Preview changes **backend only**; refuses another workload before Deploy.
4. PVC provisioning and backend deployment succeed through Console.
5. Acceptance storage form writes and reads a real value and SHA-256.
6. PVC is Bound; verified backend Pod → ReplicaSet → Deployment owner UIDs.
7. Delete only that Pod using Kubernetes DeleteOptions UID precondition; observe
   a different backend process and the same value/hash and PVC UID.
8. Frontend/worker Score state and Deployment UIDs remain unchanged.

PVC: `data-e5eaaa3803e1f1dd`, UID
`fa3c0c47-daca-42d4-aeca-79915a125b56`.
Backend Deployment UID: `846ccfd4-37f5-4e28-8135-53d683bc2f20`.
Backend Pod changed from `backend-6d87479d97-xw465`
(`8b8b9966-33fa-4ced-990a-691f39808c8a`) to
`backend-6d87479d97-v68wp`. Retained public value:
`pvc-proof-1793285508179`, SHA-256
`ff451089a04a6e5bc4ca8a6616c62b41b66da93f92ebb3bc37fd9334353632e2`.
Independent GET of the existing application URL after the run confirms
`configured=true`, `present=true`, the same value/hash and new process name.

Evidence outside Git:
`~/.local/state/orch-acceptance/pvc-focused-20261029/`, with setup config/log,
`run-r1.log`, and `run-r1/` containing catalog/mount/retained screenshots,
Console/app videos, result.json (8 passed) and identities.json. The actual catalog
and retained-storage screenshots were visually inspected. Existing shared
resources, Application, new PVC and public file are retained for user inspection;
only the verified owned backend Pod was deleted and replaced. No namespace/PVC
or unrelated resources were deleted. The existing frontend port-forward stays up.

## Checks and limits

Changed runner syntax, all eight focused runtime assertions, documentation
validation and whitespace checks passed. No product Go/React code changed;
no broad unit/integration suite or unrelated runtime scenario was rerun.
This proves file retention through one actual backend Pod recreation, not
scheduled jobs, backups, volume expansion, update/removal or deletion lifecycle.

Published [focused workload/PVC clip](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/acceptance-pvc-workload-focused-20261029.mp4):
66.64seconds, H.264, 1440×1000, 2,495,829 bytes. Full ffmpeg decode with
`-xerror` passed; sampled Console frames and the actual catalog/storage
screenshots were reviewed. Downloaded the release asset and `cmp` verified exact
bytes against the local file. SHA-256:
`e673462a8b5f93e93c211dcf8ea71e53b96c1a110ba8f396993c0ed2d325e0b3`.
Only the reviewed clip was uploaded; no credential, session, logs or state.
