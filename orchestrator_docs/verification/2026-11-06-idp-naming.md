---
id: VERIFY-2026-11-06-IDP-NAMING
artifact: verification-record
status: evidence
last_reviewed: 2026-11-06
related: [ADR-038, UC-01, UC-06, UC-18]
---

# Readable IDP naming — source verification

Branch: `features/rename`; original baseline:
`80315d3132644af5c4415e5a3b183bb23fe494b5`.
Scope: [ADR-038](../architecture/decisions/ADR-038-readable-idp-names.md),
UC-01 BR-15, UC-06 CD-09 and preservation of UC-18 BR-09 recreation.

## Result

New application keys have the shape `idp-<org>-<app>-<random6>`. One cryptographic
lowercase base32 suffix is persisted at creation. For example, the same app has
repository/Fleet name `idp-nangcap-orders-deploy-a7k3m2` and namespace
`idp-nangcap-orders-staging-a7k3m2`. The suffix stays fixed through retry, display
rename and persistence reopen. New app creation after deletion gets fresh UUIDs,
suffix and namespaces; the old tombstone/history remains. Historical names keep
their original resolution. No database schema change is required.

## Checks

- Focused Go tests with `-race -count=1` passed for domain naming, Application
  creation, delivery allocation, Fleet controller lifecycle and infrastructure
  names. They cover Vietnamese normalization, random shape, length boundaries,
  generated-key collision retry/exhaustion, duplicate display-name concurrency,
  normalized aliases, rollback and shared staging/production identity.
- PostgreSQL 17 tests passed with `REQUIRE_POSTGRES=1` against a task-owned local
  disposable server. They use separate real owner/Console/State logins, verify
  unique-key enforcement, concurrent duplicate-name creation and persisted
  identity/namespace stability after display rename and reopen.
- Existing HTTP onboarding and deletion/recreation tests passed with race
  detection. The latter verifies fresh app/Environment UUIDs and namespace names,
  retained history, cleanup ownership and unchanged preexisting resource guards.
- `go build ./...`, `python3 scripts/check_docs.py`,
  `python3 scripts/check_migrations.py` and `git diff --check` passed.

Reproduction after configuring a disposable `ORCHESTRATOR_POSTGRES_TEST_URL`:

```sh
cd backend
REQUIRE_POSTGRES=1 go test -race \
  ./internal/domain/application ./internal/application/application \
  ./internal/application/delivery ./internal/adapters/delivery/fleet \
  ./internal/planning ./internal/adapters/postgres ./test/e2e \
  -run 'Test(NewIDPKey|ManagedNames|CreateApplication|ConcurrentApplicationDisplayName|ManagedRepository|ApplicationPins|RepositoryLost|IDPRepository|ApplicationController|ApplicationRepositoryName|IDPInfrastructure|IDPApplicationNames|UC01CreateApplication|ProductSeedClusterIsVerifyingAndApplicationCreateStillWorks|ApplicationDeletionDestroysOwnedResourcesAndTombstones)' \
  -count=1
go build ./...
```

## Limits and cleanup

The broad `go test -json ./...` command was attempted during implementation on
both the working source and an archive of the original baseline, with the same
local PostgreSQL server. Both failed with existing catalog/render/revision test
failures and the PostgreSQL package's default 10-minute timeout. Two initial
task failures (the old UUID-key HTTP assertion and deleted-name recreation) were
corrected and passed the final focused checks above. The final random-suffix
change was verified by impact; a fully green backend suite is not claimed.

Fleet checks use its existing transport fixture and a real local Git remote;
they do not prove an installed Fleet controller or real GitHub repository
allocation. No Kubernetes/AWS/GitHub deployment, live migration or remote resource
rename was run. No frontend source changed, so frontend/browser checks were not
rerun. PlantUML syntax checking was unavailable locally; no diagram was changed.

Logs and JSON test output are outside Git under `/tmp/idp-rename-checks/`.
The task-owned PostgreSQL server is removed after verification; no existing
application server, database, container or user session is changed by this run.
