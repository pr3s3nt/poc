---
id: VERIFICATION-INDEX
artifact: verification-index
status: current
last_reviewed: 2026-09-28
---

# Verification evidence

Mỗi record ghi observation của một lần execution cụ thể. Evidence không định
nghĩa requirement và không chứng minh checkout mới hơn vẫn pass. Run mới phải
tạo file mới; không sửa record cũ để thay ngày/kết quả.

| Date | Scope | Record |
|---|---|---|
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
