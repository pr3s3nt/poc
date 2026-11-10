---
id: VERIFY-2026-11-10-DEFINITION-SEED
artifact: verification
status: current
last_reviewed: 2026-11-10
---

# Definition-only Git seed — issue #62

Implementation commit `24e6505b`, branch `features/seed-resource-definitions`,
based on main `e7eea986`. Procedure and input:
[bootstrap guide](../../scripts/bootstrap/README.md#chỉ-seed-resource-definitions-lần-đầu--issue-62).

## Runtime

The importer used the existing dev Console API and real PostgreSQL/Vault-backed
installation. Resource Types, READY Kubernetes Driver Account and platform Git
credentials were already configured. The backend image was built from
`cc4320af`; no product image/schema upgrade was needed for this script.

Source: `pr3s3nt/orchestrator-workload-templates`, commit
`2a9be1b1b8150ef146a5e629ea4d818eea03f2a8`. Backend inspection fetched the private
Git module/template sources using the stored platform credential. The script
validated returned origin/commit/path/digest and used the exact server-returned
module/template identities. It did not use a GitHub token or Git CLI on the host.

Six task-prefixed Definitions were created: cluster, namespace, PostgreSQL,
Redis, storage and workload. The final import printed `Definitions SEEDED`;
unchanged rerun printed six `REUSED` entries and `Definitions SEEDED`. Existing
Types/Accounts/Git settings and old Definitions were preserved. No workload,
Terraform execution, CD setup or infrastructure provisioning was performed.

The first seven-entry input reached MySQL after creating the first four
Definitions and received HTTP 400. The existing dev MySQL Type has legacy
`auth_secret_name`, `root_password_key`, `user_password_key` inputs and lacks
the new password/URI outputs. The current module uses executor-owned passwords.
That Type was left unchanged and MySQL was omitted from the runtime input;
the six-entry input reused the first four and created the last two. The final
script now detects Type mapping mismatches before creating any Definitions.
The seven-entry example includes MySQL for installations with its corresponding
Type contract; successful live MySQL registration is not claimed by this run.

Changing only `repository.url` supports an exact internal mirror preserving the
commit. A mirror with different Git history also requires its reviewed full
commit in `repository.ref`. Real internal Git/CA connectivity was not available
and is not claimed as verified.

## Checks

- Focused API/importer tests: 20 pass, zero skip.
- Full bootstrap tests in an isolated kind namespace: 88 pass, zero fail/skip,
  including the existing Chromium video tests. Used Node/Chromium from
  `mcr.microsoft.com/playwright:v1.63.0-noble` and installed project dependencies.
  Command: `node --test --test-concurrency=1 "lib/**/*.test.mjs"` from
  `scripts/bootstrap/`. Serial execution limits test-worker memory without
  removing tests or assertions.
- Initial tooling attempts hit a 1 GiB worker OOM and lacked Chromium in the
  Alpine image. Corrected to a 2 GiB Playwright pod; the final complete run passed.
- Documentation checker, `git diff --check` and Node syntax checks pass.
  PlantUML is unavailable locally; no diagram changed. Product Go/frontend
  suites were not run because their code/contracts were unchanged.

## Retention and cleanup

The run-owned kind test namespace is deleted and independently verified absent.
The script-specific loopback forward is stopped. The dev services were temporarily
resumed and returned to their previously observed zero replicas after runtime;
later shared-stack replica changes were left untouched. Six task-prefixed
Definitions and source-inspection metadata/cache remain in the dev database.
Private config, credentials, test payload/log and downloaded sources remain
outside Git under the user's local state directories. No secret, internal URL
or credential was published in the issue. No UI handoff or workload acceptance
is part of this command-line-only change. Main is not merged or deployed.
