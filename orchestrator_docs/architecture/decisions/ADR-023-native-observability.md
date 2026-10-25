---
id: ADR-023
artifact: architecture-decision
status: current
last_reviewed: 2026-10-25
related: [UC-01, UC-06, UC-07, UC-09]
---

# ADR-023 — Basic observability collected by the platform

Accepted for the basic slice of issue #1: the user chose collection inside
Orchestrator, accepted Metrics Server, and approved the revised UI mock on
2026-10-25. Logs retain seven days; CPU/memory samples and observed events retain
fifteen days. This is required design, not implementation evidence.

## Scope

The platform automatically observes deployed workload targets on existing
Kubernetes and provisioned EKS. Developer Score stays unchanged. The current
Definition/matching/placement pipeline continues to choose and pin the workload
target; collection restores that target rather than discovering another cluster.
The platform service reads pods, container logs and the Metrics API; the existing
platform PostgreSQL database stores history. No additional observability backend,
agent, sidecar or Terraform module is required. Metrics Server is a cluster
prerequisite installed by an authorized operator; the collector never installs it.

This basic slice does not introduce an `observability` provisioning resource or
its own matching/driver. Those options, external telemetry providers, custom AWS
tags, CloudWatch, notifications, tracing and business instrumentation remain in
the full GitHub issue. Its earlier resource-engine proposal does not require
shipping an unused node for a platform-native read service. Deployment Preview,
execution order and resource ownership remain unchanged.

## Collection and boundaries

- A bounded background worker reads current-generation Workload Instances,
  validates their owning Deployment and restores their opaque scoped target.
  Existing-cluster and EKS credentials use the current private resolver; no host
  kubeconfig, host AWS identity or public credential outputs are used.
- The source identity includes organization, app, environment, generation,
  workload and restored target. Pod UID, container and restart generation
  identify log streams. Redeploy preserves the source and existing history.
- Each source has a short persisted lease and fencing token. Network calls run
  outside database transactions under a shorter deadline. Only the current
  owner may commit its observations/checkpoints; retries are idempotent.
- Poll every 30 seconds, with bounded parallelism and bounded responses. Read
  timestamped container logs with overlap and inspect the previous instance
  after a restart. Preserve repeated equal lines using timestamp occurrence
  ordinals. Truncation, reconnects, retention loss and read failures are coverage
  limits, never proof that the complete log stream was retained.
- Native pod-log responses combine stdout/stderr on ordinary clusters. Expose
  level filtering, not fabricated stream classification. Explicit JSON `level`
  or a recognized leading level identifies ERROR/FATAL/PANIC, WARN, INFO or
  DEBUG; otherwise level is UNKNOWN. stderr alone is not an error level.
- Metrics carry the provider timestamp. Missing, stale or incomplete container
  metrics yield null CPU/memory, not zero. Aggregate workload usage only with
  complete coverage. Percentage needs complete positive limits; absent limits
  show absolute units without a percentage. Pod readiness/restart state is
  available independently of Metrics Server.
- Persist observed restart/waiting failures with deterministic identities and
  safe reason codes. Synthetic counts, example percentages and mock timestamps
  never enter product responses. Collection does not alter workload lifecycle,
  provision resources or change historical deployment snapshots.

## Storage, coverage and access

[Schema](../database/schema.md#native-observability--adr-022) owns four tables.
The Console runtime can read/write telemetry; the Terraform State login cannot.
Runtime never changes a retained database schema. Owner setup still accepts
only the exact fresh baseline; operators must explicitly prepare a compatible
database without an automatic reset/migration.

Per source: at most 100,000 log rows and 64 MiB of log messages, 50,000 metric
samples, and 10,000 events. Remove expired records first. On a capacity limit,
keep already stored history and report STORAGE_LIMIT; do not silently shorten
retention. Message/response/query/page bounds live in the shared contract.
These are basic bounded defaults, not a claim of lossless or unlimited storage.

All queries authorize organization/app/environment before accessing rows, bind
the current target generation, use bounded time ranges and stable cursors, and
read stored data only. Application logs are access-controlled user content,
rendered as text; they may contain secrets printed by the application itself.
Platform diagnostic errors never include credential documents, provider stderr
or log contents. A user must have ordinary application access to read its logs.

## Product experience

The approved mock is the visual reference, not a source of runtime facts.
UC-09 TEL requirements and the
[telemetry contract](../contracts/observability.md) own behavior. Overview shows
error counts, high CPU/memory peaks, unusual events and recent notable records.
Click through with the same environment, workload and interval. Keep current
usage distinct from historical peaks, and collection health distinct from app
health. The illustrative 80%-of-limit rule becomes the fixed basic indicator;
it does not add alert routing, notification delivery or a rules editor.

## References

- [Kubernetes logging architecture](https://kubernetes.io/docs/concepts/cluster-administration/logging/)
  documents log rotation, previous-container limits and optional stream splitting.
- [Resource metrics pipeline](https://kubernetes.io/docs/tasks/debug/debug-cluster/resource-metrics-pipeline/)
  defines the Metrics API and provider timestamp/window.
