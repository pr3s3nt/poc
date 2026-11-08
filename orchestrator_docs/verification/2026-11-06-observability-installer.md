---
id: VERIFY-20261106-OBSERVABILITY-INSTALLER
artifact: verification
status: current
last_reviewed: 2026-11-06
related: [ADR-041, OPS-OBSERVABILITY-INSTALLATION]
---

# Portable observability installer tooling checkpoint

Scope: issue #35 stage 0, portable Bash + Helm scripts. No application/backend/UI
change, retained database upgrade or cluster mutation. This is not stage-0 live
acceptance or proof of compatibility with every Kubernetes distribution.

## Implementation

Standalone `scripts/observability/` supplies plan/apply/verify, explicit context
and apply cluster UID, install/reuse/skip, individually disabled bundled metrics
components, chart version/SHA pins and private durable receipts. Existing release
adoption, automatic uninstall/reset/CRD upgrades and blind failure retries are
excluded. Managed components cannot be silently removed by changing mode.

Operators may add components or edit owned values after review. Ready checks cover
every installed component, including Prometheus/Alertmanager operator-owned
controllers; an operator Ready alone is insufficient. Failed outcomes retain
intent and require reconciliation. Exact limits are in the
[guide](../../scripts/observability/README.md).

## Executed checks

- `bash -n scripts/observability/setup.sh scripts/observability/test/run.sh`.
- ShellCheck v0.11.0: both scripts passed. Scoped SC2016 annotations explain jq
  variables intentionally passed as literal expressions to the state helper.
- 23 command-contract cases passed via `bash scripts/observability/test/run.sh`:
  read-only plan, explicit target UID, initial install, repeat NOOP, read-only
  verify, owned update, drift, foreign ownership, partial reuse/component toggles,
  UID pinning/replacement, uncertain failure, private errors, checksum/RBAC/CRD
  failures, operator ownership, removal protection, all-release preflight,
  missing controllers, spaces in paths, unrelated state-directory protection and
  retaining the previous completed receipt on a failed upgrade.
  These use simulated CLI/cluster behavior, not mock evidence of real collection.
- Downloaded upstream charts and checked SHA-256 against repository-index pins:
  kube-prometheus-stack 92.18.0, Loki 7.5.0, Alloy 1.15.0,
  prometheus-postgres-exporter 8.2.3, prometheus-blackbox-exporter 11.21.1.
- Helm 3.19.4 lint/render passed for all five pinned charts/sample values.
  Exporter lint emitted informational icon recommendations only.
- Real mixed metrics render disables Grafana, Operator and CRDs and retains the
  physical-cluster identity recording rule. No resources were applied.
- Alloy v1.21.3 (pinned chart appVersion) `validate` accepted the sample River
  config after replacing namespace/push-URL placeholders in a private temporary
  file. This validates config syntax, not RBAC or successful log ingestion.
- Documentation checker and diff whitespace passed. PlantUML executable absent;
  no diagram was changed.

## Live limits and next gate

Read-only access to historical `kind-k8s-4f` was attempted but the context is not
present in the active kubectl configuration. No kubeconfig/secret was searched or rewritten, no
current context was switched and no cluster resource was changed. Full backend,
frontend and browser suites were not run: the change is operator scripts/docs.
No full kind deployment gate was claimed.

Next live gate requires a supplied/authorized target: fresh installation and
repeat NOOP, partial existing-stack reuse with before/after UID/config comparison,
actual metrics/logs queries, Grafana datasource access, persistence after restart,
isolated firing/resolved notification and ownership-aware test cleanup. Keep
shared stack/data. Do not tick stage 0 from this checkpoint.
