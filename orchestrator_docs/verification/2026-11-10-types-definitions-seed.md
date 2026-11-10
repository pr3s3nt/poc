---
id: VERIFY-2026-11-10-TYPES-DEFINITIONS-SEED
artifact: verification
status: current
last_reviewed: 2026-11-10
---

# Resource Types and Definitions seed — issue #65

Branch `features/seed-types-definitions`, based on main `de6c50df`. The operator
CLI now accepts optional `resourceTypes` in the same input as Definitions.
[Input/procedure](../../scripts/bootstrap/README.md#chỉ-seed-resource-definitions-lần-đầu--issue-62).
The existing input without Types remains supported.

## Exact source and runtime result

Used the user's release repository
`https://github.com/pr3s3nt/orchestrator-workload-templates`, exact commit
`2a9be1b1b8150ef146a5e629ea4d818eea03f2a8` (also HEAD when checked).
The input names modules at `terraform/namespace`, `terraform/postgres`,
`terraform/redis`, `terraform/mysql`, `terraform/persistent-storage` and
template bundle `templates/standard`. One repository URL/ref covers all sources;
backend inspection derives the immutable module/template pins.

A fresh isolated kind installation used the real backend image
`orchestrator-backend:main-de6c50df`, PostgreSQL 16 and file-backed Vault.
Reused the local installer manifest generator with private namespace/image
substitution, owner schema setup, separate runtime credentials and retained
cache directories. The local/none seed profile left the Type/Definition catalog
empty; the first Admin used a generated owner-only password. Git credentials
and an uploaded Kubernetes Driver Account were prepared through ordinary APIs
as test prerequisites, before running the command under test. No seed input or
Git/account credentials were uploaded as Kubernetes ConfigMaps or Secrets.
The installation scripts ConfigMap contains only the usual nonsecret scripts.

API reads confirmed **0 Types and 0 Definitions** immediately before seed.
The exact example input, with only target/login/account fields adapted, created
all seven Types and all seven Definitions: cluster, namespace, PostgreSQL,
Redis, **MySQL**, persistent storage and workload. No Definition was omitted.
The seed command finished with `Definitions SEEDED`.

An unchanged rerun reported seven Type `REUSED` entries and seven Definition
`REUSED` entries. Restarted only the isolated backend, reran the same command
and independently compared API records with the receipt captured after the
first run: Type contracts, Definition revisions/pins, Account key/version/status
and Git settings version remained identical. This proves persisted seed and
rerun behavior; it does not claim application/database workload deployment.

The previous shared-dev MySQL Type mismatch is avoided by a fresh catalog; no
existing Type was overwritten. This test did not mutate the shared installation.
No internal company Git endpoint/CA was available. An exact internal mirror
preserving the commit needs only `repository.url` changed; different Git history
also needs the corresponding reviewed full `repository.ref`. Host credentials
remain separate from the platform's stored Git access configuration.

## Automated checks

- Focused Type/Definition and API seed tests: 23/23 pass.
- Full bootstrap suite on kind in the isolated namespace, including existing
  Chromium/video and Git tests: **91/91 pass, zero fail/skip**. Tool image:
  `mcr.microsoft.com/playwright:v1.63.0-noble`; command from `scripts/bootstrap/`:
  `node --test --test-concurrency=1 "lib/**/*.test.mjs"`.
- Tests cover creation before inspection, rerun without duplicate writes, all
  Type conflicts and mapping checks before Type writes, invalid contracts,
  existing Definition conflicts, source pin checks and input compatibility.
- Documentation checker, Node syntax and whitespace checks pass. PlantUML
  is unavailable locally; no diagram changed. Product Go/frontend code and
  database schema were not changed, so their suites/builds were not repeated.

## Retention and cleanup

The run-owned namespace `seed65-runtime` and its storage PV were removed and
independently verified absent after checks. The temporary loopback forward was
stopped. No shared service was restarted or scaled; no workload was deployed.
Private config/credentials, installer manifests, source payload, first-run API
receipt and test log remain outside Git under the operator's local state.
The CLI is handed off for acceptance; no UI preview or product image rollout
is part of this change. Main has not been merged.
