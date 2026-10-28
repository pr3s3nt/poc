---
id: DELIVERY-RECONCILIATION-IMPLEMENTATION
artifact: implementation-plan
status: current
last_reviewed: 2026-10-28
related: [ADR-027, UC-01, UC-04, UC-05, UC-06, UC-07, UC-08, UC-09, UC-18]
---

# Delivery reconciliation — implementation handoff

**Design complete is not runtime delivery.** This plan implements the user's
Environment opt-in decision: DIRECT by default, optional Fleet now, future CD
adapters through common ports. No product code or external deployment is supplied
by the design task. [IMP-039](deviations.md#imp-039--delivery-reconciliation-design-2026-10-28)
tracks the gap. This is the requested feature's work breakdown, not a new roadmap.

## Read in order

1. [ADR-027](../architecture/decisions/ADR-027-delivery-reconciliation.md).
2. Specifications: [UC-01 CD](../usecase/UC-01/specification.md#optional-environment-delivery--adr-027),
   [UC-04 CD](../usecase/UC-04/specification.md#cd-provider-bindings--adr-027),
   [UC-05 CD](../usecase/UC-05/specification.md#delivery-selection-preview--adr-027),
   [UC-06 CD](../usecase/UC-06/specification.md#optional-cd-delivery-and-reconciliation--adr-027),
   [UC-07 CD](../usecase/UC-07/specification.md#provider-neutral-update-and-removal--adr-027),
   [UC-08 INF](../usecase/UC-08/specification.md#infrastructure-drift--adr-027),
   [UC-09 REC](../usecase/UC-09/specification.md#reconciliation-observations--adr-027),
   [UC-18 CD](../usecase/UC-18/specification.md#cd-aware-application-deletion--adr-027).
3. [Delivery port/API contract](../architecture/contracts/delivery-reconciliation.md),
   [Terraform drift contract](../architecture/contracts/infrastructure-drift.md),
   [schema](../architecture/database/schema.md#delivery-and-drift--adr-027-required-design).
4. [Settings UI design](../usecase/UC-01/ui/delivery-reconciliation.md).

Shared collaboration supplements the existing use-case diagrams:
[delivery sequence](../architecture/components/delivery-reconciliation-sequence.puml)
([PNG](../architecture/components/delivery-reconciliation-sequence.png)),
[participants](../architecture/components/delivery-reconciliation-vopc.puml)
([PNG](../architecture/components/delivery-reconciliation-vopc.png)),
[infrastructure sequence](../architecture/components/infrastructure-drift-sequence.puml)
([PNG](../architecture/components/infrastructure-drift-sequence.png)),
[ERD](../architecture/database/delivery-reconciliation-erd.puml)
([PNG](../architecture/database/delivery-reconciliation-erd.png)).

## Current seams and gaps inspected

| Existing code | Reuse / required work |
|---|---|
| `backend/internal/ports/execution/execution.go` | Keep renderer/resource/secret ports; bridge current WorkloadDeployer to typed delivery lifecycle, shared ownership removal |
| `backend/internal/adapters/gitops/deployer.go` | Replace host-context guard architecture, single Deployment assumption, Harbor coupling, process-local last revision and mutex-only publication; do not merely remove guard |
| `backend/internal/adapters/gitops/public_routes.go` | Routes become scoped delivery units; retain complete route-set semantics |
| `backend/internal/bootstrap/adapters.go`, `bootstrap.go` | Registry/composition; remove process-wide Fleet selection and Fleet type assertions from lifecycle wiring |
| `backend/internal/application/deployment`, `pending` | Snapshot pins, durable publication, phased readiness, policy-only actions, ownership transition and route-only retry |
| `backend/internal/adapters/kubernetes` | Preserve explicit credential resolver, inventory/UID checks, VSO/Agent and direct behavior |
| `backend/internal/application/accountdiag` | Reuse lease/checkpoint patterns, not its diagnostic aggregate or authority |
| `backend/internal/application/tfexec`, `adapters/terraform/runner/plan.go` | Persist encrypted input baseline; new nonmutating scan path; parse all drift types, preserve cleanup parser contract |
| `backend/internal/adapters/postgres`, `adapters/store`, `ports/persistence` | New baseline/DTOs/repos and persistence contract tests; no production reset |
| `frontend/src/features/configuration/SettingsPage.tsx` | Existing Environment tabs, section style and explicit save; new opt-in panel |
| `frontend/src/features/applications/ApplicationHomePage.tsx`, `features/preview`, `features/deployments` | Pending selection action, transition review, live observation distinct from historical deployment |

Proposed new packages: `domain/delivery`, `ports/delivery`, `ports/gitpublication`,
`application/delivery`, `application/drift`, `adapters/delivery/direct`,
`adapters/delivery/fleet`, `adapters/gitpublication`. Keep Terraform lifecycle in
tfexec; do not build a second state/credential backend. Paths are planned, not
links to nonexistent code. Retire old gitops adapter only after direct/Fleet parity
and stale-reference checks; do not migrate unrelated E2E runners.

## Ordered work and completion gates

| Step | Deliverables | Exit gate |
|---|---|---|
| R01: domain/storage | Capability model, selection defaults/CAS, unit/revision/outbox, encrypted artifacts, binding versions, operation kinds | PostgreSQL + store contract tests: tenant isolation, restart, immutable pins, owner fencing; no runtime activation |
| R02: direct integration | Registry, direct adapter and neutral routes; persist exact artifacts for future handoff | Existing direct deploy/update/remove and pending tests pass; no user changes required; second fake provider exercises same coordinator |
| R03: Fleet binding/publication | Scoped manager/target verification, private Git credentials, durable publisher, commit-pinned unit GitRepos | Two-writer conflict/lost-ack tests; Fleet conformance on exact installed versions; no host fallback |
| R04: Fleet deploy/observe/remove | Phases, Job receipts, live revision, drift observer, retain/withdraw/UID cleanup, self-heal policy | Disposable real cluster proves retained uninstall and no unsafe native prune before enabling Fleet for users |
| R05: Environment opt-in/UI/handoff | Save → Preview → Deploy; direct ↔ Fleet transition, interruption/retry, routes, application deletion | E01–E08 below; no overlapping writers, no Job replay, stable data/UIDs during handoff |
| R06: infrastructure scan | Immutable private baseline, scheduler, full drift parsing, safe stored reports | E09 read-only scan and persistence tests; state serial and outputs unchanged by scan |
| R07: reviewed repair | Separate review API/UI, encrypted plan, full private digest comparison, in-place constraints | E10: confirmed update only, stale plan and consumer-output changes blocked by automated tests |
| R08: integration handoff | Current state/code map/runbook, provider compatibility report, migration/upgrade limitations | Full required module checks for changed scopes; documented live evidence and cleanup; no hidden unsupported capabilities |

Do not expose Fleet as READY/eligible until R04's semantic gate is proven on the
actual controller/agent/Helm versions. If retained withdrawal, Job idempotency,
HPA delegation or ownership guarantees cannot be implemented, keep the relevant
capability unavailable and revise the design with the user; do not weaken guards
or present a partial adapter as the promised usable release.

## Acceptance matrix (planned, not executed)

| ID | Scenario and assertions |
|---|---|
| E01 | New Application: both Environments DIRECT/off. Enable Fleet only in staging; Save makes zero Git/cluster writes; production remains direct. Preview shows provider change, Deploy activates exact binding. |
| E02 | Fleet new deploy with app/worker, PostgreSQL/Redis dependency and extra Job/CronJob objects. Dependencies remain Terraform-owned; Job completes before rollout; actual SQL/cache job flow passes, not just Ready Pods. |
| E03 | Update image/config through existing form/Preview/Deploy; preserve PostgreSQL/Redis/PVC identities/data; routes and complete inventory match revision. No-op save/retry does not rerun Job. |
| E04 | Controlled owned-workload drift on disposable target: self-heal off reports drift; explicit opt-in policy restores approved continuous configuration. Draft remains unapplied. HPA replicas are not fought. CronJob child jobs are not recreated by Git reconciliation. |
| E05 | Restart backend after push acknowledgement loss; recover exact commit/receipt with no duplicate publish/Job execution. Controller outage yields UNKNOWN with freshness and recoverable progress. |
| E06 | Existing direct Environment → Fleet → direct via Settings. Stable object/PVC/data identity, completion receipts and routes; no competing writer, no template rerender or infrastructure reprovision. Production selection unaffected. |
| E07 | Update removes an extra object, workload removal and Application deletion. Withdraw controller, retire owned release/control metadata, UID-delete only owned objects; verify native absence and retained preexisting controller/registration/credentials. |
| E08 | Fake second provider conformance: different handles, statuses and publication style; identical lifecycle workflow without planner/core provider branches. No claim Argo CD is live-supported. |
| E09 | Real Terraform-managed disposable object: change a reversible non-destructive attribute, scan detects update; canonical state serial, outputs and current Set unchanged. Missing baseline and provider failure represented honestly in automated tests. |
| E10 | Review/confirm eligible in-place repair and verify exact original state identity. Tests prove changed private values invalidate review, replacement/create/delete or consumer-impacting output blocks repair. |

Negative/security/concurrency cases belong in automated tests, not intentional
failure detours in the main business E2E. Controlled drift in E04/E09 is the core
feature assertion and requires explicit permission for those owned targets.
New scripts go under `e2e/scenarios/delivery-reconciliation/` and
`e2e/scenarios/infrastructure-drift/`, platform setup under `e2e/setup/`; those
paths are planned and no runner exists yet. Read [E2E guide](../../e2e/README.md).

## Deployment prerequisites and operator procedure to implement

1. Use the updated fresh database baseline or an independently reviewed retained
   database upgrade. Keep current data; no implicit reset/backfill of old UID proof.
2. Install/pin Fleet controller/agent on authorized infrastructure and register
   each destination. Configure least-scope management and workload service accounts,
   Git reader Secret and dedicated generated repository/branch. Keep credentials
   outside Git. Same-cluster and separate-manager paths must verify target identity.
3. Register and verify binding through new API, record version compatibility and
   private refs. Failure must be actionable before Environment choice can be deployed.
4. Open Application Settings, choose staging/production, enable CD, choose binding,
   optionally enable self-heal, Save, Preview and Deploy. A populated Environment
   uses explicit transfer review; missing baseline first requires a direct redeploy.
5. Observe stored revision/sync/health/drift. Remove/toggle through product workflows;
   do not manually delete Git paths or broad Fleet/Helm releases as cleanup.
6. Enable the independent Terraform scanner only after encrypted baseline support
   is configured. Use read-only scan then explicit repair review when required.

These are the target procedure, not runnable current commands. Implementation
must supply the exact flags, request examples and E2E entrypoints in an operations
runbook once they exist. For AWS verification follow the existing
[AWS runbook](../operations/aws.md) with authorized target/run ID/cost estimate/
cleanup supervisor and native absence proof. This design task grants no such run.

## Validation and handoff obligations

For implementation: Go `go test ./... && go build ./...`; frontend `npm run
typecheck && npm run lint && npm test && npm run build`; required real PostgreSQL
contract tests without skips; docs checker/diff check and PlantUML regeneration.
Use focused runtime runs based on changed impact; historical Fleet evidence is
not evidence for scoped credentials, new lifecycle kinds or current cleanup.

Design-task validation/evidence is recorded separately in
[design verification](../verification/2026-10-28-delivery-reconciliation-design.md).
No browser mock, document, unit fake or successful setup alone means the user can
deploy with Fleet today. “Usable” requires R01–R08 and applicable E01–E10 evidence.
