---
id: VERIFY-PLATFORM-MANAGED-CD-20261102
artifact: verification-record
status: current
last_reviewed: 2026-11-02
related: [ADR-033, ADR-027, UC-01, UC-04, UC-05, UC-06, UC-07, UC-18]
---

# Platform-managed CD repositories

Scope: platform default, private GitHub repo allocation per Application, hidden
Application destinations, environment paths, schema 5 and the existing Console.
No old-Application migration or test-data deletion. Terraform retains dependency
ownership. [Decision](../architecture/decisions/ADR-033-platform-managed-delivery.md),
[operations](../operations/delivery-reconciliation.md).

## Evidence boundaries

GitHub tests use an explicit local HTTPS fake server; hosting service tests use
explicit fake collaborators. They cover private initialized creation, actual branch,
conflict/lost response, retry without duplicate, refusal of foreign/replaced remote
identity, no creation from reads, immutable allocation, owner loss and env paths.
They do not prove real GitHub permissions or Fleet synchronization.

JSON and PostgreSQL share the same tenant/CAS/concurrent allocation/reopen contract.
Database tests use a task-owned disposable PostgreSQL 16 server and separate owner,
Console and State login roles. The retained Console database is not a test fixture.

UI reference: existing CD configuration and Application Settings in the current
AppShell, plus `architecture/ui/README.md`. Main form uses the existing controls;
advanced connection settings stay collapsed. Application repo/CD authoring is
removed. Read-only browser access is checked without API interception through the
actual Tailscale URL. Browser evidence and database credentials/backups remain
outside Git under the private local task evidence directory.

## Validation record

- Backend: `go test ./...` and `go build ./...` passed. PostgreSQL-dependent tests
  in this ordinary run are supplemented by the mandatory real-database run below.
  Focused Store/service/HTTP/Fleet/bootstrap checks passed after final owner-fencing
  and configuration guards.
- Real PostgreSQL: full adapter suite passed with `REQUIRE_POSTGRES=1`, explicit
  disposable test URL/container and `-count=1 -timeout=25m` (696.747 seconds).
  Populated registered sources through version 4, state/domain identities,
  repeat setup, atomic rollback, grant isolation and backup/restore passed.
  A final focused real-PostgreSQL/JSON run passed after owner-fencing changes,
  including creation pin persistence, concurrent allocation/reopen and denied
  State reads/Console deletion. The first full attempt hit Go's default ten-minute
  timeout and had an outdated fixture constraint name; both were corrected before
  the successful run. No timeout/failed attempt is counted as acceptance.
- Frontend: all 81 files / 773 tests passed; TypeScript, lint and production build
  passed. Lint retains one preexisting PlatformSettingsPage hook warning; build
  retains the existing large-chunk warning. The log-follow test now explicitly
  cleans up its render and synchronizes its pause event across async refreshes;
  no observability product behavior was changed.
- Documentation/PlantUML and migration immutability guard passed; `git diff --check`
  passed. Released SQL migrations/references 1–4 are unchanged.
- Read-only browser scenario `e2e/scenarios/platform-managed-cd/run.mjs` passed on
  the real Tailscale Console/API without interception. Its first attempt overlapped
  backend restart (502); rerun after readiness passed.

## Retained Console and handoff

Console: <http://desktop-bdjp2rq-1.tail342836.ts.net:3001/ui/platform/cd-providers>.
Verified using a real browser on the host through this exact Tailscale route,
including authenticated policy/API reads, opening the GitHub-owner/token form and
Application Settings without CD authoring. No separate tailnet-device claim.
Visual comparison with the existing AppShell/CD/Settings reference passed: same
navigation/forms/styles, technical connection fields collapsed, no additional
Application repo form.

Runtime images: `orchestrator-backend:managed-cd-20261102` and
`orchestrator-frontend:managed-cd-20261102`, based on the existing pinned runtime
images. Private restart script:
`/home/thanhnt1/.local/state/orch-acceptance/platform-managed-cd-20261102/start.sh`.
It reuses Compose `thieu-staging`, its existing dependencies/volumes and Tailscale
Serve; it recreates only backend/frontend. Services remain running for handoff.

Before owner upgrade, both writers were stopped and a private v4 dump was archived,
inspected and successfully restored into a separate disposable database. New owner
setup upgraded the retained installation to v5; runtime retains ordinary credentials
and no DDL. Browser comparison preserved all 15 Applications' environment namespace,
current deployment-set and target-generation identities. No test apps were deleted.
Private backup, browser screenshots/JSON and run scripts remain outside Git under
`/home/thanhnt1/.local/state/orch-acceptance/platform-managed-cd-20261102`.

The retained platform currently has **zero READY CD bindings**. Users can review
and configure the new form; real Deploy needs verified Fleet prerequisites and
private GitHub credentials, followed by an explicit platform default. Existing
test Applications remain available for inspection and are not automatically moved.
Create new Applications after setting the default.
Live GitHub repository creation/Fleet deploy/self-heal/cleanup is outside this
run's verification; the new allocation path still requires that live acceptance.

## Shared Git and Fleet setup refinement — 2026-11-02

CD registration supports `usePlatformGit`, mutually exclusive with submitted or
retained separate credentials. It resolves current Organization Git settings on
subsequent host/publication opens, validates destination scope and durable
credential storage, and changes publisher cache identity on token rotation.
Tokens never return to the form. Existing explicit credentials remain supported.
Account selection automatically discovers Fleet and shows workspace/cluster and
controller; Advanced defaults and explicit READY/default selection remain.
Verification gives bounded remediation text for inaccessible reader Secret,
release namespace, ServiceAccount, controller/version, target mapping and Git.

Validation: backend full `go test ./...` / `go build ./...` passed, followed by
focused checks after final edits. The new Git writer test covers rotation,
tenant isolation and destination scope. Frontend full suite passed (81 files,
773 tests before adding the new form test); final focused form/catalog checks
passed (11 tests), with typecheck, lint and production build passing. Existing
hook and chunk-size warnings remain. Docs checker and `git diff --check` passed.
No schema changes or live GitHub creation/Fleet deployment were exercised.

Updated retained `thieu-staging` backend/frontend only, using images
`orchestrator-backend:shared-git-cd-20261102` and
`orchestrator-frontend:shared-git-cd-20261102`. Initial host-linked backend binary
failed to start in the container; rebuilt with `CGO_ENABLED=0`, restarted and
confirmed healthy. Private restart/evidence directory:
`/home/thanhnt1/.local/state/orch-acceptance/shared-git-cd-20261102` (`start.sh`).
No dependencies, volumes, schema or workloads were recreated.

Real authenticated browser verification passed through
<http://desktop-bdjp2rq-1.tail342836.ts.net:3001/ui/platform/cd-providers>, including
Git metadata/API reads, source-selection form and Application Settings. Compared
before/after identities for all 15 Applications and retained deployment state;
no browser runtime errors occurred. Visually compared with existing CD form:
AppShell, layout and collapsed Advanced styling preserved. Current platform Git
has no configured token, and there are zero READY CD bindings; live reuse and
Fleet deployment require operator-provided credentials/prerequisites.
