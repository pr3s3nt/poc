---
id: VERIFY-2026-11-07-ORGANIZATION-SEED
artifact: verification
status: current
last_reviewed: 2026-11-07
---

# Configurable Organization seed — issue #47

## Scope and result

Branch `features/organization_seed`, based on `origin/main` commit `b2d605f9`.
External API seed supports `organization: {key, name}`. Preparation exports only
that identity to a new owner-only JSON file; it neither reads credential files
nor calls APIs. Operator mounts it read-only and configures backend/first-Admin
with `-organization-file` or `ORCHESTRATOR_ORGANIZATION_FILE`.

Backend creates a missing identity transactionally or reuses its ID/name. An
explicit conflicting name fails before other seed writes. It never renames an
existing Organization or moves accounts. Import checks session Organization
key/name before platform mutations. Legacy expectedOrganization-only inputs and
backend startup without a file retain their previous behavior. The new API is
read-only and session-scoped; it does not create Organizations.

## Checks

- API seed tests: 14 pass, including credential-free preparation, permissions,
  exclusive output creation, invalid identity, key conflict, name conflict before
  writes, rerun reuse and compatibility.
- All bootstrap tests: 82 pass, zero skip.
- Focused Go tests pass for CLI file wiring, identity reuse/conflict, file
  validation, scoped API authorization and existing staging seeding.
- Full affected Go suites (`cmd/orchestrator`, `internal/seed`,
  `internal/delivery/http`) pass in disposable tooling containers.
- Real PostgreSQL: REQUIRE_POSTGRES=1 with a new disposable server; identity
  name/ID survive pool reopen, conflicting name is refused, and existing
  catalog-free concurrent fixture test passes. Database fixtures clean themselves.
- Product build passes. Docs/whitespace checks pass; optional PlantUML is not
  installed and no diagram was changed. Frontend is unchanged and was not tested.
- Full backend checks run in a run-owned kind namespace on task and unmodified
  main, with identical `go test -p 4 -timeout 2m ./...` package deadlines. All
  packages are included. The first unbounded attempt was stopped after retaining
  partial logs because unrelated suites stalled; the bounded run includes runtime
  prerequisites git/tzdata before starting. PostgreSQL is not configured for those
  full-suite pods, so optional SQL suites skip there; required real SQL coverage
  for this change is the separate zero-skip PostgreSQL run above.
- Complete-repository kind run: focused checks and build pass. Full task and
  unmodified main both fail in the same 16 packages; task has no additional
  failing package or named test. Package deadlines mean this is a bounded
  comparison, not a green full-suite claim. The affected CLI/seed/HTTP packages
  pass. Shared failing packages are:
  - adapters: `kubernetes`, `templatesources`, `terraform/registry`;
  - application: `catalog`, `configuration`, `deployment`, `pending`, `preview`,
    `provisioning`; plus `bootstrap`;
  - tests: `conformance`, `e2e`, `integration/v03run`, `integration/v04run`,
    `integration/v2cleanup`, `integration/v2run`.
  Reasons include existing worker timeouts, old rendering/planning contracts and
  scenario assertions. Earlier backend-only payload attempts lacked sibling
  fixtures and were discarded; final comparison includes the complete tracked
  repository plus task changes for both trees.

The local host has no Go toolchain; Go checks use the same Go 1.25 Alpine builder
as the product Dockerfile. Disposable tooling additionally installs git and tzdata,
matching runtime prerequisites. Initial tooling attempts without those packages
failed before application checks; they were corrected before rerunning.

## Runtime

Fresh durable snapshot with a production-profile backend and a first Admin was
created from the prepared identity. Real HTTP importer creates a missing Type,
then reports REUSED on rerun and after process restart. Organization key/name
and ID remain equal across restart. Both wrong key and wrong name stop before
platform writes. No browser/Console UI, infrastructure provisioning or application
deploy was required or claimed. PostgreSQL coverage is separate from this snapshot
HTTP wiring check.

Temporary credentials/input/state are owner-only outside Git under
`~/.local/state/issue47-organization/`; logs are under `/tmp/issue47-*`.
No shared user stack, account, namespace or database was modified. Disposable HTTP/PostgreSQL containers and their test network were stopped and
removed after validation; private input/state remains for reproduction. The run-owned kind namespace was deleted after verifying its ownership label
and collecting logs; an independent API read confirms it is absent. Images,
private snapshot input and local build caches remain for reproduction; no global
Docker-volume prune was performed.
