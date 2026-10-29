---
id: VERIFY-20261029-ACCEPTANCE-HEADLESS-RECORDING
artifact: verification-record
status: evidence
last_reviewed: 2026-10-29
related: UC-01, UC-05, UC-06, UC-08, UC-12, UC-16, ADR-027
---

# Paced headless acceptance application recording

Requested scope: create a new acceptance application through Playwright, slow
workload creation for review, save/upload the video and diagnose/fix failures.
The user additionally required the current main Console and headless execution.
Runner and configuration: [acceptance-app](../../e2e/scenarios/acceptance-app/README.md).

## Runtime provenance and preparation

`git fetch origin main` confirmed HEAD and origin/main both `f9addce`, with
zero divergence before this task's script/documentation commit. The Console at
`http://localhost:3001` initially served `thieu-staging` images created October27;
checkout freshness did not establish runtime freshness. Those images were
replaced with locally built `orchestrator-frontend:main-f9addce` and
`orchestrator-backend:main-f9addce`, both labeled
`org.opencontainers.image.revision=f9addce`. No frontend/backend source changed.

The existing workload Vault had exited because file bind mounts still referenced
the retired `projects/poc` path, now containing directories instead of files.
Recovery retained its existing data/bootstrap/token/anonymous volumes and Docker
network, corrected only the three source-file binds, and preserved the scoped
token byte-for-byte. The previous stopped container is retained as
`k8s-4f-workload-vault-pre-recovery-20261029`; the replacement is Healthy.

Console upgrade followed [ADR-027](../architecture/decisions/ADR-027-postgres-upgrades.md)
and the [database upgrade wrapper](../operations/postgres-upgrades.md): build,
stop old writers/frontend, private custom-format backup plus archive inspection,
owner-only setup, then start/health-check frontend/backend without restarting
PostgreSQL or platform Vault. The recognized source checksum was
`c8d5170caaba9bce4fb41d99f562a31442a157cdb39235f6dcf2b6ff0a1eb161`.
Schema advanced to `orchestrator-schema-v1`, preserving installation ID
`932893b5-0180-41a9-94e2-b3ce1566bdee` and creation time
`2026-10-27 02:37:43.362088+00`. No reset or marker editing occurred.

Private build/upgrade logs, Compose revision overlay and the mode0600 backup
are under `~/.local/state/orch-acceptance/main-f9addce-runtime/`, outside Git.
Existing Compose environment settings were compared before replacement; none
of its configured backend values changed. The render-cache setting remains in
the new image. Existing database, Vault, credential, state-key and cache volumes
were preserved.

## Execution observations

Final run: `acceptance-20261029-r4-headless`, `ORCH_HEADLESS=1`,
`ORCH_WORKLOAD_DELAY_MS=2500`, Console `http://localhost:3001`, cluster context
`kind-k8s-4f`, image tag `pvc-cron-20261029`. Three acceptance images were loaded
into that exact existing kind cluster. No cluster or cloud infrastructure created.

Application: `acceptance-2610291429`, ID
`057436dd-04a9-440d-98f1-5d208d666fda`. The runner signs in as Developer,
creates Application/settings through UI, imports backend/worker/frontend Score,
opens the supported product form to show the imported configuration, then
Preview/Deploy. Workload actions pause before/after clicks. Secret input must
have `type=password` before filling; responses and credential values are not
logged. The catalog's custom-configuration choice is handled explicitly.

The process exited 0. Preview showed three affected workloads; all three deploy
results succeeded. Acceptance UI showed backend connection, environment, secret,
database and cache PASS; the submitted job reached DONE and identifies its real
worker Pod. `result.json` records eight assertions. Independent native reads
confirm three ready Deployments, one ready PostgreSQL StatefulSet and one ready
Redis StatefulSet, with both database/cache PVCs Bound. The namespaces for R1/R2
do not exist; those attempts never deployed resources.

Run evidence: `~/.local/state/orch-acceptance/acceptance-20261029-r4-headless/`,
including `console.webm`, `application.webm`, final screenshots, `result.json`
and `native-resources.txt`. The two recordings were concatenated without changing
playback speed into H.264 MP4, 1440×1000 at 25fps, 259.68 seconds. Full ffmpeg
decode produced no errors. Contact sheets every eight seconds, additional frames
from login/settings and the final acceptance screenshot were visually inspected:
current catalog/editor/preview UI, references instead of secret values, masked
login and a real DONE result. No generated screenshot/video/session is in Git.

Published [MP4 recording](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/acceptance-main-f9addce-headless-20261029-r4.mp4)
to the existing GitHub release without replacing another asset. Downloaded it
again and `cmp` matched the local MP4 byte-for-byte (7,049,236 bytes); SHA-256:
`aa4b63a4d3c4f5af5a7422691848b0b337d473143eaa0f611844a4ac3d592888`.
Only the reviewed MP4 was uploaded, not logs, backups, tokens or browser sessions.

## Earlier attempts and retained state

- R1: Vault unavailable during secret save; Application
  `acceptance-2610291419` (`c3e99062-528a-40b4-85b8-ab7e206823f2`) retained.
- R2: secret save passed after Vault recovery, but the old Console disabled
  form conversion for advanced imported Score. The runner was corrected to
  read that Score instead of clicking the disabled conversion control.
  Application `acceptance-2610291421`
  (`720cab11-d9c0-440e-b929-1e5f9d0ff12f`) retained.
- R3: old Console deployment passed all five app checks and job DONE.
  Application `acceptance-2610291424`
  (`3e3d27e4-6466-4e8a-97a5-dc62a5be6843`) retained. It completed before
  Console replacement; its recording is not the current-main evidence.

All attempt videos/screenshots remain under `~/.local/state/orch-acceptance/`.
Applications/resources are deliberately retained for inspection; no preexisting
Application/namespace/PVC is deleted. Temporary Score files and each run's
frontend port-forward are removed/stopped by the runner.

## Validation and limits

Changed runner syntax, E2E helper tests (21/21, zero skips), documentation checker
and whitespace checks pass. After the live run, only port-forward formatting
and writing the failure summary before session-dependent cleanup were refined.
A local media lifecycle probe found that `saveAs` must run after context close
and before browser close; the final runner preserves this already-passed live
ordering. The optional keep-browser-open mode was removed; this task closes the
browser automatically and records with headless enabled. Syntax checks were
rerun without another deployment of unchanged product code.
No product source or schema artifact was changed in Git, so full
backend/frontend suites were not repeated. Runtime owner setup and Console
health/login use the real retained PostgreSQL/Vault installation.

This scenario covers frontend/backend/worker, shared PostgreSQL/Redis, five
application checks and one async job. It does not verify PVC file retention,
CronJob receipts, update/restart lifecycle, deletion or AWS. The latest acceptance
binary includes optional storage/schedule panels; these are not enabled by the
three-workload fixture and are not counted as passed checks.
