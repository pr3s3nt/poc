---
id: OBSERVABILITY-CONTRACT
artifact: operation-contract
status: current
last_reviewed: 2026-10-25
related: [ADR-023, UC-01, UC-09]
---

# Native observability contract

[ADR-023](../decisions/ADR-023-native-observability.md) owns architecture;
[UC-09 TEL](../../usecase/UC-09/specification.md#native-application-observability--tel) owns behavior.

## Outbound and storage ports

`observability.Reader.Collect(source, checkpoints)` returns bounded timestamped
logs, one workload sample, detected events, updated stream checkpoints and
separate safe log/metric collection codes. It may return partial observations;
permission/network/metrics errors do not discard successfully read logs.
The source contains the immutable logical scope and opaque Kubernetes target,
never credential bytes or paths. Resolve credentials privately for each call.

`TelemetryRepository` leases/fences sources, commits a batch and its checkpoints
atomically, reads bounded history/aggregates and prunes expired observations.
It is an optional Store port; unsupported stores answer unavailable. Source
leases do not claim an Environment operation or block deploy/configuration.

Lease 60 seconds; reader deadline 20 seconds; polling interval 30 seconds;
at most four concurrent sources. Bound pod and container fan-out. Log requests
use timestamps, inclusive checkpoint overlap, at most 2,000 lines/256 KiB per
container; a message is at most 16 KiB. Adapter output is capped and malformed
or oversized responses become a safe collection code. Cursor keys include pod
UID/container/restart generation; duplicate timestamps retain occurrence order.
Do not advance log checkpoints when storage rejects those logs.

## Read API

Authenticated, same-origin, application-scoped GET endpoints:

- `/api/v1/applications/{id}/environments/{env}/observability/overview`
- `/api/v1/applications/{id}/environments/{env}/observability/logs`
- `/api/v1/applications/{id}/environments/{env}/observability/metrics`

Session supplies organization. `from`/`to` are RFC3339 instants (`from < to`),
default last hour, within retention, with at most one minute future skew.
`workload` is optional; `q` is a literal substring (max 200 characters);
`level` is ALL/ERROR/WARN/INFO/DEBUG/UNKNOWN. ERROR includes FATAL/PANIC.
Log page size defaults 100, maximum 200. Cursor encodes the last `(timestamp,id)`
in descending order; malformed cursors or fields return safe 400. Foreign or
unknown application/environment returns 404. No GET starts collection or writes.

Logs return rows, next cursor, exact matching count and collection coverage.
Each row includes timestamp, workload, pod UID/name, container, restart index,
level and message. Raw provider error messages and private target/lease state
are not public fields. All returned records belong to the selected generation;
history remains when a workload/pod is removed within that generation.

Metrics return time-bucketed series with explicit null gaps (no connecting across
missing samples), per-workload CPU/memory peaks and actual latest samples.
CPU uses mCPU, memory bytes (Console displays MiB). Preserve at most 240 chart
buckets per workload. Absolute values remain useful without a known limit.
Peak percentage uses that sample's own complete limit, not a different revision.
The latest sample is independent of the selected historical window and labelled
separately from interval maxima. Only a fresh provider sample can establish
current recovery. Include actual
provider timestamps and coverage; a successful HTTP response is not fresh data.

Overview returns exact ERROR-line count, bounded histogram, peak facts and
noteworthy events from the selected interval, plus source health. A high metric
means at least 80% of a complete positive workload limit; count each
metric/workload once, plus observed abnormal events. Return at most 200 recent event rows, with the
exact event count so the Console can request a narrower interval/workload. Overview counts and
drill-down detail use the same filters and facts. High peaks that have recovered
are marked separately; no percentage is invented when limits are absent.

## States

No instances/history: NOT_DEPLOYED. Instances but no observations: WAITING.
Verified received logs/metrics: RECEIVING. Missing Metrics Server/forbidden
metrics: METRICS_UNAVAILABLE while logs can still be RECEIVING. Read errors,
late checkpoints, truncation or a missed interval: PARTIAL/GAP; store last
success timestamps and show old data as stale. Quota refusal: STORAGE_LIMIT.
Reader not configured (for example fake execution with no telemetry fixture):
UNAVAILABLE. Never map unknown to zero errors, zero CPU or a healthy app.

No app-readiness guarantee follows from collector availability. An app logging
no lines may still have successfully inspected empty log streams; distinguish
that from “log records received.” Each response reports the last observation,
last received log/metric and missing coverage independently.
