---
id: IMPLEMENTATION-NATIVE-OBSERVABILITY
artifact: implementation-handoff
status: current
last_reviewed: 2026-10-25
related: [ADR-022, UC-09]
---

# Basic application logging and monitoring — issue #1

Implemented on `features/logging&monitoring`, from approved mock commit
`0b1b58b`. [ADR-022](../architecture/decisions/ADR-022-native-observability.md),
[UC-09 TEL](../usecase/UC-09/specification.md#native-application-observability--tel)
and the [contract](../architecture/contracts/observability.md) define this slice.

## Delivered behavior

Real Kubernetes/AWS bootstrap starts a bounded native collector. It restores
owned current-generation workload targets and the existing scoped credential
resolver, reads pod/container logs and Metrics API, and stores received history
in PostgreSQL. It never installs infrastructure or changes Score/Preview.
Fake execution without a telemetry reader explicitly reports unavailable.

Four new persistence aggregates provide fenced leases/checkpoints, idempotent
log rows, numeric samples and observed pod/container events. Current/previous
container generations have distinct log identities; equal timestamp/message
occurrences remain distinct. History survives backend reopen and pod replacement
within retention. Per-source quotas preserve earlier history and rejected-log
checkpoints; expiration frees retained observation capacity.

Three authenticated stored-query routes serve overview/logs/metrics. Tenant,
application, Environment and generation filter every query; GETs do not collect.
History is independent of current app readiness. Missing values stay null;
percentage uses each sample's complete positive limit. HTTP responses exclude
private source target, credential, checkpoints, lease owner and fence.

Application home contains the approved four summary cards and notable records.
Click-through preserves interval/environment/workload/level. The detail screen
has log search/filter/pagination/follow/detail/copy, CPU/RAM history with null
gaps and keyboard tooltips, actual sample times and readiness/restart counts.
English/Vietnamese use the existing catalogs. Shell, navigation and workload
lifecycle controls retain their existing behavior.

## Entry points

- Domain/ports: `backend/internal/domain/telemetry/`,
  `backend/internal/ports/observability/`, `persistence/telemetry.go`.
- Persistence: `backend/internal/adapters/{store,postgres}/telemetry.go`,
  PostgreSQL baseline/grants and memory snapshot validation.
- Collection/query: `backend/internal/adapters/kubernetes/telemetry.go`,
  `backend/internal/application/observability/`, bootstrap lifecycle.
- HTTP: `backend/internal/delivery/http/observability.go`.
- Console: `frontend/src/features/observability/`, ApplicationHomePage,
  App/routes and existing AppShell active-section mapping.
- Verification: adapter-neutral persistence contract, store/SQL reader tests,
  service/authentication tests, frontend feature tests and
  `frontend/test/e2e/observability.mjs`.

## Operating limits and remaining work

The [runbook](../operations/observability.md) owns prerequisites and procedures.
The baseline and snapshot format changed; retained databases/snapshots have not
been modified. Runtime does not migrate/reset them. Preparing a retained
platform database without losing data requires a separate operator upgrade
procedure. Do not apply the fresh setup/reset path to retained data.

Collection is best effort. Native stdout/stderr are combined; filtering uses
explicit levels, never inferred stream labels. The source observes the deployed
workload's owned pods, not independently named resource pods, database/cache
exporters or CloudWatch. A separate provisioning resource/Definition/driver,
external telemetry stack and advanced integrations remain in issue #1. Existing
workload Definition matching still selects the pinned execution target.

The subsequent authorized [real new-user Kubernetes run](../verification/2026-10-25-native-observability-live.md)
passed 48/48 checks with a new owned kind cluster, Metrics Server and fresh
PostgreSQL. Product UI cleanup and runtime teardown passed. EKS, retained DB
rollout and paid resources were not part of that run.

The earlier [local implementation verification](../verification/2026-10-25-native-observability.md)
used synthetic browser telemetry and disposable real PostgreSQL for persistence
checks. The subsequent live run uses actual application/cluster telemetry. Both
use the real product HTTP/Console; their evidence boundaries remain distinct.
