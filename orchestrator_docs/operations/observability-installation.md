---
id: OPS-OBSERVABILITY-INSTALLATION
artifact: runbook
status: current
last_reviewed: 2026-11-06
related: [ADR-040, UC-09]
---

# Portable observability installation — issue #35 stage 0

User decision 2026-11-06: Bash + Helm tooling for installation on another cluster,
including partial existing stacks. Tooling delivery does not authorize mutation
of an unspecified destination. Do not install from the read-only inventory alone.

[Standalone installer guide](../../scripts/observability/README.md) owns commands,
configuration and recovery details. It is copied with the installer. This runbook
owns operational boundaries; [ADR-040](../architecture/decisions/ADR-040-external-observability.md)
owns architecture. This is operator setup, not Console stack provisioning.

## Installation procedure

1. Select authorized physical cluster/context, installation identity, namespace
   and StorageClass. Inventory existing releases/workloads/CRDs and operators.
   Check node allocations, disk/PVC capacity, quota, DNS/image access and policies.
2. Choose per component install/reuse/skip. Pin chart name/version/archive SHA;
   review company values. Keep credentials, kubeconfig, rendered Secrets, state
   and logs outside Git. Sample values are small-cluster development values.
3. Complete Alloy namespace-scoped RBAC/discovery and Loki push URL; exporters
   need actual targets and monitoring credentials. Do not assume installation
   of an exporter creates application instrumentation or database permissions.
4. Run plan with explicit context from config. Review operations and private
   rendered manifests, including cluster-scoped permissions and scrape selectors.
   Existing components with custom images need manual inventory; discovery is
   deliberately conservative and not complete auto-adoption.
5. Apply with the physical kube-system UID printed by plan; the script validates
   all configured releases before first mutation. Verify the same configuration.
6. Record versions, ownership, UIDs, chart hashes, kept state/PVCs and any partial
   result. Failed/pending or uncertain release outcomes require reconciliation;
   the script never resets state, retries blindly, takes ownership or uninstalls.

## Existing/shared components

Reuse only reads declared controllers and pins their identity. It does not patch
shared Grafana datasources, Alertmanager receivers, Prometheus selectors or an
operator's watch scope. Those changes require separate authorized configuration.
New owned chart components are individually disabled when reused/skipped.

Existing monitoring CRDs require explicit reuse and compatibility review. Basic
served-v1 checks do not validate every CRD schema or operator version. Do not run
force conflicts or CRD upgrade jobs. Before an operator version upgrade, review
upstream CRD instructions and all consumers. The installer refuses automatic
operator/chart upgrades while owning newly installed CRDs.

Existing same-name releases without both receipts and installer ownership labels
are not adopted. Even already deployed legacy development stack releases need
reuse configuration. Helm object ownership checks remain enabled. An object-level
collision can be discovered during Helm apply, leaving earlier owned releases
installed; retain receipts and reconcile before retry. Do not delete shared CRDs,
cluster RBAC, namespaces or PVCs to force a successful rerun.

## Acceptance and cleanup boundaries

Script checks are separate from stage-0 live gates: scrape/metric queries, log
collection/query, Grafana connectivity, retention/persistence after restart and
isolated receiver notification evidence. No mock, Helm template or Ready status
proves these. Existing native history and the retained prototype database are not
modified by this installer. Production notifications are not configured by default.

For UI handoff, use the [Tailscale runbook](tailscale-handoff.md); this installer
does not configure public ingress or claim that an application URL is ready.
Cleanup is explicit operator work after UID/ownership/shared-consumer checks and
backup. Keep shared stack and CRDs; stage test cleanup never removes them.

## Product binding, profiles and resource mappings

After installation, register separate query endpoints and a browser-reachable
Grafana origin in Platform Settings, test connectivity/identity/coverage, then
bind the Environment. Defaults and whole-workload overrides define named Pod
port, scrape path/interval/sample cap and typed metric keys. Rules and PodMonitor
travel through the existing Git/Fleet workload delivery, not Prometheus query
API writes. `APPLIED` requires both expected live objects and the deployment's
acknowledged monitoring revision; telemetry reception and alert delivery remain
separate evidence.

Advanced binding mappings use this shape (replace with actual scoped identity):

```json
{
  "resourceObservations": {
    "RESOURCE_INSTANCE_UUID": {
      "collectorWorkload": "database-exporter",
      "metricKeys": ["database_up", "connections"]
    }
  },
  "dependencyObservations": {
    "declared-backend": {
      "collectorWorkload": "backend-probe",
      "metricKeys": ["endpoint_up", "probe_latency"]
    }
  }
}
```

Resource IDs come from the product State resource list, not Kubernetes names or
an arbitrary external database. Mapped keys must exist in the collector's
effective profile and the resource must belong to the current scope/generation.
A workload collector must already expose actual metrics from its authorized
source. Do not forward a company-wide multi-tenant exporter and pretend its
samples are isolated by an app label; operator setup must limit the original
source/monitoring credential first. The test fixture's metrics forwarding is not
a product proxy. Declared relationships do not prove actual application calls.

Binding/profile edits require Preview and Deploy even if Workload content has
not changed. Preserve a lost response as uncertain and inspect terminal operation,
applied config and Preview before retry. For native transition, select both native
signals, Deploy withdrawal, verify app-owned monitoring objects absent, then unbind.
Keep shared releases, history and resource data according to retention. Removed
source mappings do not authorize deleting a database or exporter outside the app.
