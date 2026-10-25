---
id: OPS-NATIVE-OBSERVABILITY
artifact: operational-runbook
status: current
last_reviewed: 2026-10-25
related: [ADR-022, UC-09]
---

# Native application logging and monitoring

Use [ADR-022](../architecture/decisions/ADR-022-native-observability.md) and the
[contract](../architecture/contracts/observability.md) for design. This runbook
prepares the basic collector; it does not authorize changing a cluster/database.

## Prerequisites and rollout

1. Build the Console and Go backend as in their READMEs. Native collection starts
   with the real Kubernetes/AWS adapter. Fake execution has no telemetry reader
   and reports unavailable; it never manufactures records.
2. Prepare a compatible **new-baseline** platform database using the existing
   owner setup process. The four telemetry tables and Console grants belong to
   the baseline. Runtime does not migrate or reset retained data; an older
   database fails the existing baseline check. Do not run reset/setup against a
   retained database to enable this feature. A data-preserving upgrade of such
   a database needs a separately reviewed operator procedure; this change does
   not supply one. The memory snapshot format also adds four required maps and
   rejects an older snapshot without rewriting it.
3. An authorized operator enables Metrics Server on each selected internal/EKS
   cluster if it is absent. Use the cluster's supported installation procedure.
   The collector does not install Metrics Server, Prometheus, Loki, Grafana,
   node agents, sidecars or an application SDK.
4. The existing scoped deployment credential must additionally allow namespace
   `get/list` of core `pods`, `get` of `pods/log`, and namespace `get/list` of
   `metrics.k8s.io` `pods`. No `watch`, Kubernetes Events, Secrets read, node or
   cluster-wide metrics permission is added by the collector. Keep the existing
   deployment credential resolver and its private credential storage. Do not
   replace a scoped credential with host kubeconfig/admin identity.
5. Existing deployed Workload Instances already carrying a valid owned
   Deployment and scoped target are picked up automatically. Allow two or more
   30-second polls before assessing history. Console GETs read PostgreSQL and
   never contact the cluster. No historical CPU/RAM can be backfilled from
   Metrics Server; only received observations become history.

This implementation observes the deployed workload's owned pods/containers.
Specialized database/cache exporters, independently named resource pods and
cloud-service metrics are deferred in issue #1. It does not claim metrics for
all infrastructure resources merely because an app uses them.

## Check collection

Open Application → Environment. The overview has ERROR-line counts, unusual
observations, CPU/RAM peaks, recent notable records and links to stored detail.
Select an interval, then click a card to retain that interval and its filter.
Logs support workload, level, literal search, custom time, older pages, follow,
record details and copying. Follow polls stored history; source collection is
still every 30 seconds. Metrics use absolute mCPU/MiB, null gaps, provider
sample times, per-workload lines, peak/limit facts and independent pod counts.

`RECEIVING` means source reads work, not that the app is ready. A quiet log
stream may have zero received lines after a successful inspection. ERROR means
an explicit recognized level, not every stderr line or a semantic diagnosis.
The ordinary pod log API combines stdout/stderr. Unusual events are observed
pod/container restart/waiting/termination facts, not the entire Kubernetes
Events feed. One high signal counts one workload/metric with an interval peak
at least 80% of a complete positive limit; it is not an alert notification.

Use the keyboard to explore chart tooltips; log details close with Escape.
Timestamps display in the browser timezone; API intervals use RFC3339 instants.
At most 200 recent events are returned; narrow time/workload to inspect more.

## Missing data and bounded capacity

- Missing/forbidden Metrics API: retain logs and readiness/restarts; CPU/RAM are
  unavailable. Check Metrics Server and the scoped namespace permissions.
- Pod/log permission, credential or network failure: preserve stored history;
  display partial/stale coverage and last observation. Provider stderr and
  credential documents are never stored as diagnostic events or returned by API.
- Missing/incomplete limits: use absolute metrics and omit percentages/high
  indicators for that metric. Missing values are never zero-filled. Historical
  high values that have recovered are distinct from current high values; stale
  samples cannot establish recovery.
- Rotation, a deleted pod before collection, more than one missed restart,
  reconnects or truncation can lose unreceived log lines. Read current and one
  previous container with overlap. Received rows survive pod removal/redeploy;
  this polling design is best effort, not a lossless log transport.
- Per source: logs 7 days, at most 100,000 rows/64 MiB; samples/events 15 days,
  at most 50,000 samples/10,000 events. A message is capped at 16 KiB. At quota,
  keep stored history and report STORAGE_LIMIT; rejected logs do not advance
  checkpoints. Expiration frees capacity on later collection rounds.
- Read fan-out is bounded to eight pods/eight containers per source, 2,000
  lines/256 KiB per container and 8,000 lines per round. Excess data is marked
  truncated. Queries over more than 128 stored sources require a workload
  filter. These defaults target a basic deployment, not high-volume logging.

Application logs are user content accessible to users with app access. An app
can itself print a secret; the platform does not fetch its environment/Secrets
or promise to detect/redact arbitrary text printed by that app. Render/copy
records as text and keep the existing session/organization boundary.

## Local verification without cluster mutation

Run the normal Go/frontend checks. PostgreSQL adapter tests use a disposable,
password-authenticated PostgreSQL URL through `ORCHESTRATOR_POSTGRES_TEST_URL`
and `REQUIRE_POSTGRES=1`; never point them at retained product data.

For the real HTTP/Console browser check, build `frontend/` then run from `backend/`:

```bash
ORCHESTRATOR_OBSERVABILITY_BROWSER_SCRIPT="$PWD/../frontend/test/e2e/observability.mjs" \
  go test ./internal/delivery/http -run TestObservabilityConsoleHistory -count=1 -v
```

The script uses `/usr/bin/google-chrome` (override `OBSERVABILITY_CHROMIUM`) and
writes screenshots/result JSON to `/tmp/poc-observability-browser` (override
`OBSERVABILITY_EVIDENCE_DIR`). This is **product UI with synthetic received
telemetry**, backed by real authenticated Go HTTP endpoints; it proves no live
cluster/cloud behavior. Live verification requires an explicitly selected
cluster/environment and authorization for any Metrics Server/RBAC changes.
