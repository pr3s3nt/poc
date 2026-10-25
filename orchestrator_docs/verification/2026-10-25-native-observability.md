---
id: VERIFY-20261025-NATIVE-OBSERVABILITY
artifact: verification-record
status: evidence
last_reviewed: 2026-10-25
related: [ADR-022, UC-09]
---

# Basic native application observability — issue #1

## Scope and evidence boundary

Branch `features/logging&monitoring`, implementation based on approved mock
`0b1b58b`. Required behavior lives in UC-09 TEL and ADR-022, not this record.

Real: Go collector/query/HTTP code, PostgreSQL persistence and runtime roles,
Console production bundle, Chromium and its HTTP calls. PostgreSQL tests use
owned disposable PostgreSQL 17 containers on loopback; the final SQL fixture
requires password authentication. Go 1.25.0 and Terraform 1.9.8 were installed
outside the repository for verification; acceptance tests also use temporary
Redis 7.0.15 tools outside the repository.

Synthetic: the kubectl transport tests' pods/logs/Metrics API responses and the
browser's explicitly synthetic received-history fixture. Browser requests reach
the real authenticated Go HTTP server and stored-query service; telemetry
responses are not intercepted by Playwright. The SQL adapter tests separately
exercise real persistence/reopen/roles. No live cluster/EKS, Metrics Server/RBAC
installation, retained DB rollout, cloud resource or user credential was changed.

## Focused checks

- Adapter-neutral memory/PostgreSQL history: stable duplicate-timestamp log
  IDs/cursors, idempotent replay, competing/expired fenced leases, checkpoint
  reopen, tenant/environment isolation, actual peaks versus latest samples,
  null samples, Unix-aligned arbitrary chart buckets and expiration.
- Quota checks submit a bounded batch crossing the log byte cap: stored history,
  last received log time and rejected-log checkpoint stay unchanged; limit is
  explicit. Same contract runs against memory and real PostgreSQL.
- Native reader: mCPU/byte quantity conversion, provider timestamp, complete
  coverage/limits, duplicate equal lines, current/previous restart streams,
  deterministic OOM event, missing metrics with logs/readiness retained,
  foreign/host target rejection, partial-line rejection and bounded stdout/stderr.
  The cap test found/fixed an embedded `bytes.Buffer.ReadFrom` bypass; the
  capped writer now exposes only its explicit bounded Write implementation.
- Service/API: automatic immediate collection and cancellation/released lease,
  owned persisted target restoration, no provider calls from GET, retained
  history after reader failure, private diagnostic exclusion, query bounds,
  session 401 and foreign-organization 404. Collector lifecycle passes `-race`.
- Console: real summary values, retained filters and scoped deep links, explicit
  level filtering, raw text rendering, late Environment response rejection,
  null gaps, no invented limit percentage and no recovery from stale samples.
  Route round-trip and vi/en catalog/placeholder parity pass.
- Bootstrap tests pass against the disposable authenticated PostgreSQL fixture;
  production starts the real reader and stops collection before database pools.

Focused command patterns (all passed):

```text
go test ./internal/adapters/postgres ./internal/adapters/store ./internal/adapters/kubernetes \
  ./internal/application/observability ./internal/delivery/http ./internal/domain/telemetry \
  -run 'Telemetry|Observability|SweepUses|QueryRejects|LogLevel' -count=1
go test ./internal/application/observability -race -count=1
go test ./internal/bootstrap -count=1
npm run typecheck
npm run lint
npm run build
```

## Broader regression checks

The required `go test ./... -count=1` ran with real disposable PostgreSQL and
Terraform. Its first fixture used local trust authentication and lacked Redis;
acceptance-app tests exposed those setup limitations. A separate authenticated
fixture and temporary Redis binaries resolved them: the complete
`examples/acceptance-app/...` suite passes, including wrong-password rejection.
The full PostgreSQL adapter suite also passes with authenticated connections.
One Environment fencing test exceeded its deliberately short lease under the
initial parallel load; the unchanged complete `application/envops` suite passes
when rerun with the corrected fixture.

The full Terraform executor package reached Go's default ten-minute package
limit after earlier tests had passed, while running
`TestSafetyNetReverseDestroysAnAbandonedRunThroughTheEngine`. The interrupted
and subsequent 14 tests passed in a focused continuation with a 15-minute
timeout (194.627 seconds). Earlier passing tests were reused. This is
local `terraform_data` verification, not paid AWS execution.

Frontend full run: 73 files / 891 tests. Added overview requests initially
invalidated old lifecycle tests that counted every fetch/role globally; those
tests now isolate the independently tested OverviewPanel, keeping their existing
workload/deploy assertions. The affected ApplicationHome/App suites pass.
The unchanged DefinitionV3 suite exceeded its existing five-second per-test
limit under load; all three tests pass alone with `--testTimeout=60000`.
Other passing files are reused; no business assertion was removed or relaxed.

`go build ./...`, docs validation and `git diff --check` pass. The docs checker
reports that local PlantUML is absent; the changed ERD, telemetry sequence and
shared class diagram are instead successfully rendered with Docker PlantUML
`-failfast2 -pipe -tpng` and verified as PNG output. The production build retains
the existing large-chunk advisory.

## Browser and visual comparison

[Browser script](../../frontend/test/e2e/observability.mjs) runs through
`TestObservabilityConsoleHistory` against the real local API. Eight groups pass:
overview counts; ERROR drill-down; record detail/Escape; history charts and
keyboard tooltip; deep-link reload; Environment isolation; 390 px mobile with
no horizontal overflow; no page errors or unsuccessful telemetry responses.

This is **product UI with fixture telemetry**, not live application evidence.
The unavailable-reader warning intentionally remains visible with retained
history. Sparse isolated samples render points, never a fabricated continuous
line across missing intervals.

UI sources: existing AppShell, ApplicationHomePage, Button/Status, shared styles,
[shared UI design](../architecture/ui/README.md) and
[approved mock](../../frontend/design/logging-monitoring/README.md). Compared
approved and product screenshots in the browser: sidebar/identity, typography,
spacing, Environment tabs, four summary cards, row drill-down and chart layout
are preserved. Product adds real collection warnings/provider sample times and
uses level filtering supported by the native API. No scenario switch or mock
values enter the product bundle.

Concision review: counting/threshold details stay behind “Cách đọc”; repeated
last-observation text was removed from the warning, retention appears once per
panel, and raw pod/container information stays in record detail. Existing
workload/deployment controls were kept.

Evidence: [overview](artifacts/native-observability-20261025/home.png),
[logs](artifacts/native-observability-20261025/logs.png),
[metrics](artifacts/native-observability-20261025/metrics.png),
[mobile](artifacts/native-observability-20261025/mobile.png),
[assertions and HTTP status](artifacts/native-observability-20261025/result.json).

## Handoff limits

Live internal/EKS telemetry remains unverified. Runtime never installs Metrics
Server or mutates a cluster. The four new tables and memory snapshot maps change
the baseline; retained platform state was not reset/migrated. A non-destructive
operator upgrade is still needed before using an older retained database with
this build. Native polling cannot recover lines deleted/rotated before a read.
Advanced provisioning/Definition integrations, specialized resource telemetry,
CloudWatch, alert notifications and tracing remain open in issue #1.


## Local cleanup

Both task-owned disposable PostgreSQL containers were stopped and their removal
verified by container ID/ownership label. Their ephemeral volumes were not
retained. No cluster/cloud resources were created. The existing approved mock
server/session and all user sessions were left running. No user-owned change
was present at task start or included in the task commit.
