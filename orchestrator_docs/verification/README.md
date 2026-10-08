---
id: VERIFICATION-INDEX
artifact: verification-index
status: current
last_reviewed: 2026-10-08
---

# Verification evidence

Mỗi record ghi observation của một lần execution cụ thể. Evidence không định
nghĩa requirement và không chứng minh checkout mới hơn vẫn pass. Run mới phải
tạo file mới; không sửa record cũ để thay ngày/kết quả.

| Date | Scope | Record |
|---|---|---|
| 2026-10-08 | Ordinary verified Compose Vault seed, legacy-preserving conversion, real two-Vault human UI recording, credential-reference runtime and token refresh | [Normal Compose Vault store](2026-10-08-compose-vault-normal-store.md) |
| 2026-10-08 | Default source-built Compose, automatic platform Vault database seed, real Vault UI secret writes, token ACL isolation and restart persistence | [Compose Vault bootstrap](2026-10-08-compose-vault-bootstrap.md) |
| 2026-10-08 | Editable per-Environment Connections/stores, two real Vaults/VSO, stale-preview rejection, PostgreSQL migration, route/restart/cleanup and reviewed human MP4 | [Environment stores/transitions](2026-10-08-environment-stores-kind.md) |
| 2026-10-07 | Environment Settings set-once targets, legacy migration, independent AWS scope tests and real kind human UI recording | [Environment connection verification](2026-10-07-environment-connection-kind.md) |
| 2026-10-07 | Uploaded nondefault Connection selected through human UI; real kind Deploy/diagnostics, host-fallback rejection control, reviewed MP4 and cleanup | [Live connection selection](2026-10-07-application-connection-selection-kind.md) |
| 2026-10-07 | Application-level connection selection, scoped choices, target guards and fake-adapter Playwright create/deploy/restart | [Connection selection verification](2026-10-07-application-connection-selection-local.md) |
| 2026-10-07 | Four-service Docker Compose, browser kubeconfig Save with simulated API, scoped Vault token and persistence across container replacement | [Local Compose verification](2026-10-07-docker-compose-local.md) |
| 2026-10-06 | UC-04 desktop form/list layout correction, keyboard focus review and revised read-only kind recording | [Connections UI review](2026-10-06-uc04-connections-ui.md) |
| 2026-10-06 | UC-04 kubeconfig upload API/UI, scoped Vault credentials, executor target resolution, Claude review, isolated PostgreSQL and published read-only kind video | [Upload verification](2026-10-06-uc04-kubeconfig-upload.md) |

| 2026-10-06 | Definition-selected score-k8s 0.15.0 registration/Preview/Deploy, CLI semantics and Secret bridge with fake infrastructure; no live mutation | [Local rendering verification](2026-10-06-score-k8s-rendering-local.md) |
| 2026-10-06 | Definition-selected score-k8s rendering through Web Console registration, Preview provenance and real Deploy on kind, with live manifest checks and recording | [score-k8s kind verification](2026-10-06-score-k8s-rendering-kind.md) |
| 2026-10-02 | Human-paced Developer kind deployment with current key picker; Platform Engineer catalog validation and live connection READY, reviewed published MP4s | [Human UI recordings](2026-10-02-human-ui-kind-recordings.md) |
| 2026-10-02 | UC-16 existing-key checklists, aliases, scope/late-response guards and lossless form projection; four-round Claude review | [Local picker verification](2026-10-02-workload-key-picker-local.md) |
| 2026-10-02 | UC-02/03 ID and Driver Inputs policy; UC-16 Save/import params; Claude code-only implementation and two-round review | [Local validation hardening](2026-10-02-catalog-workload-validation-local.md) |
| 2026-09-30 | UC-02..04 insert-only registration, safe errors, PostgreSQL race/rollback/reopen, truthful UI and no-restart matching | [Registration hardening](2026-09-30-registration-hardening-local.md) · [MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc02-04-human-local-20260930-080921.mp4) |
| 2026-09-30 | UC-07 update/remove, shared-resource preservation, PostgreSQL atomic marker/restart, strict scoped API/UI and human-paced UI-only recording | [Update/remove verification](2026-09-30-uc07-update-remove-local.md) · [MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc07-human-local-20260930-073724-10075.mp4) |
| 2026-09-30 | Standalone Score Preview, consistent memory/PostgreSQL snapshot, no mutation, safe API/UI and human-paced UI-only recording | [Preview verification](2026-09-30-uc05-preview-local.md) · [MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc05-human-local-20260930-065714-19834.mp4) |
| 2026-09-30 | Human-paced UC-09 recording with real address bar, visible pointer, clicks, typing and review pauses | [UC-09 review video](2026-09-30-uc09-human-video-local.md) · [MP4](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/uc09-human-local-20260930-043252-720.mp4) |
| 2026-09-30 | UC-09 authenticated scoped history/detail, immutable workload snapshots and JSON/PostgreSQL browser restart verification | [Local observability](2026-09-30-uc09-local-observability.md) |
| 2026-09-30 | UC-00/UC-01 local browser onboarding, sign-out/re-login and backend restart persistence | [Local onboarding](2026-09-30-uc00-uc01-local-onboarding.md) |
| 2026-09-21 | Local walking skeleton and Web Console | [Walking skeleton](2026-09-21-walking-skeleton.md) |
| 2026-09-21 | Internal Kubernetes happy path and cleanup | [kind happy path](2026-09-21-kind-happy-path.md) |
| 2026-09-21 | AWS VPC/EKS/Aurora happy path, cost and cleanup | [AWS happy path](2026-09-21-aws-happy-path.md) |
| 2026-09-21 | Humanitec contract review and planner fixture conformance | [Contract conformance](2026-09-21-contract-conformance.md) |
| 2026-09-21 | Root backend/frontend layout migration | [Layout migration](2026-09-21-root-layout-migration.md) |
| 2026-09-21 | Clean-clone documentation links and product CI | [Clean-clone CI fix](2026-09-21-clean-clone-ci.md) |
| 2026-09-22 | Humanitec Delta, container resources and conformance coverage reconciliation | [Humanitec gap reconciliation](2026-09-22-humanitec-gap-reconciliation.md) |
| 2026-09-22 | I06-05 conformance catalog criteria semantics, 33/33 fixtures | [IMP-010 conformance catalog](2026-09-22-imp010-conformance-catalog.md) |
| 2026-09-22 | I06-06 Deployment Delta Snapshot, 33/33 fixtures with Delta assertion | [IMP-008 Delta Snapshot](2026-09-22-imp008-delta-snapshot.md) |
| 2026-09-22 | I06-07 Score container resources, 33/33 fixtures, kind live resources and cleanup | [IMP-009 container resources](2026-09-22-imp009-container-resources.md) |
| 2026-09-22 | I06-07 kind rerun on final BR-11 renderer policy, seeded resource cases and cleanup | [IMP-009 kind rerun](2026-09-22-imp009-kind-rerun.md) |
| 2026-09-26 | UC-12 persistent Vault installation on kind; uninitialized/sealed handoff | [Vault kind install](2026-09-26-uc12-vault-kind-install.md) |
| 2026-09-26 | UC-12 Vault initialization/unseal and private key-file handoff | [Vault init](2026-09-26-uc12-vault-init.md) |
| 2026-09-27 | UC-12/16 Preview → Deploy, Vault Agent on kind, secret rotation and cleanup | [UC-12/16 kind verification](2026-09-27-uc12-uc16-kind.md) |
| 2026-09-28 | Fleet GitRepo + Harbor-image workload deploy/remove on kind and cleanup | [Fleet GitRepo kind verification](2026-09-28-fleet-gitrepo-kind.md) |
| 2026-09-28 | UC-12 VSO Secret sync, rotation and direct kind deploy/remove | [UC-12 VSO kind verification](2026-09-28-uc12-vso-kind.md) |
| 2026-09-28 | UC-03 profile matching and UC-16 resource-input form local checks | [UC-03/16 resource inputs](2026-09-28-uc03-uc16-resource-inputs.md) |
| 2026-09-28 | UC-16 public Service port → UC-06 Traefik Ingress → kind HTTP and removal | [Public Ingress kind verification](2026-09-28-public-ingress-kind.md) |
| 2026-09-28 | Multi-path direct Ingress, Fleet route bundle/prune, no-op Preview and Pod UID | [Public routes recheck](2026-09-28-public-routes-recheck.md) |
| 2026-09-28 | Official Backstage image import and amd64 Pod smoke test on kind | [Backstage image smoke test](2026-09-28-backstage-image-kind.md) |
| 2026-09-28 | Backstage UC-12/16 Preview → Deploy, PostgreSQL, Ingress and guest API on kind | [Backstage kind end-to-end](2026-09-28-backstage-kind-e2e.md) |
| 2026-09-29 | Dedicated Orchestrator PostgreSQL installation on kind | [PostgreSQL kind installation](2026-09-29-orchestrator-postgres-kind.md) |
| 2026-09-29 | Normalized PostgreSQL repositories, constraints, restart persistence and backup/restore | [PostgreSQL system store](2026-09-29-postgres-system-store.md) |
| 2026-09-29 | UC-04 host kube-context read-only verification on kind | [UC-04 context verifier](2026-09-29-uc04-kind-context.md) |
| 2026-09-29 | Browser-driven acceptance app deployment, environment/secret/DB checks and cleanup on kind | [Playwright acceptance](2026-09-29-acceptance-playwright-kind.md) |
| 2026-09-29 | Continuous Playwright video from sign-in through deployed diagnostic checks on kind | [Recorded Playwright acceptance](2026-09-29-acceptance-playwright-video-kind.md) · [WebM](artifacts/acceptance-playwright-kind-2026-09-29.webm) |
| 2026-09-29 | Review-paced Playwright video with readable pauses at each deployment step | [Review-paced Playwright acceptance](2026-09-29-acceptance-playwright-review-video-kind.md) · [WebM](artifacts/acceptance-playwright-kind-review-2026-09-29.webm) |
| 2026-09-29 | Human-paced Playwright video: visible cursor, typed form input, workload form entry and job submit | [Human-paced Playwright acceptance](2026-09-29-acceptance-playwright-human-video-kind.md) · [WebM release asset](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/acceptance-human-kind-2026-09-29.webm) |
| 2026-09-29 | Orchestrator images deployed by the Orchestrator, then the in-cluster instance deploying the acceptance app; full browser window recorded | [Self-hosted Orchestrator on kind](2026-09-29-self-host-playwright-kind.md) · [MP4 release asset](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/self-host-kind-2026-09-29.mp4) |
