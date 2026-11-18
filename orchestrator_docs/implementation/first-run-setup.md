---
id: IMP-FIRST-RUN-SETUP
artifact: implementation-note
status: current
last_reviewed: 2026-11-18
---

# First-run setup — issue #89

Feature branch `features/setup`; not merged or deployed to the main Console.
Approved behavior: [UC-00 SETUP](../usecase/UC-00/first-run-setup.md).
Operation and restart: [runbook](../operations/first-run-setup.md).

## Code map and traceability

| Requirement | Implementation | Verification |
|---|---|---|
| SETUP-01/02 | `backend/internal/application/installationsetup`, optional persistence InstallationSetup, PostgreSQL advisory transaction lock, bootstrap/CLI token file | initialization race/restart, two PostgreSQL backend pools, HTTP and bootstrap tests |
| SETUP-03/04 | `frontend/src/features/setup/InitializePage.tsx`, `SetupPage.tsx`, `/platform/setup` progress; current product forms | sign-in/route/shell component tests, real browser scenario |
| SETUP-05 | registry `setup_discovery.go`, hardened GitCLI, scoped access | local HTTPS Git snapshot/ref/traversal integration test, user Git browser discovery |
| SETUP-06/07 | catalog `setup_preview.go`, prospective Type overlay, normal prepare/import APIs; `GitInit.tsx` | no-business-write/reuse/conflict tests, UI ordering/invalidation/conflict tests, real Git Terraform/template inspection |
| SETUP-08 | no-store APIs, concealed operator token/password, existing credential forms/transport | HTTP authorization, no browser storage state, private evidence |

No migration, runtime DDL, fixed repository/resource list, cloud provisioning,
Application/workload instance import or controller installation is included.
Init-enabled startup suppresses fixture seeds, including the bundled template/Vault
seed; after initialization, platform entries are configured explicitly in the guide.

Source selection/editor drafts are transient. Durable resume means rerun the same
pinned source against saved configuration, not persistence of browser form contents.
The guide reflects saved verification, not continuous liveness or deploy acceptance.
