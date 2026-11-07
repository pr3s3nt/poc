---
id: VER-OBSERVABILITY-CAPABILITIES-20261107
artifact: verification
status: current
last_reviewed: 2026-11-07
related: [ADR-040, UC-09]
---

# Issue #35 capability execution checkpoints

User authorized continuing through stage 5, stopping before stage 6, and creating
a dedicated kind cluster. No merge into main is authorized. This record separates
implemented mechanisms from completed live gates.

## Installation core checkpoint

A fresh owned two-node kind cluster runs the pinned metrics and logging charts.
Prometheus Operator, Prometheus, Alertmanager, Grafana, kube-state-metrics,
node-exporter, Loki and Alloy reached controller readiness. Metrics were installed
first; the logging installation explicitly reused the six metrics components and
existing monitoring CRDs. Reused controller UIDs and pod templates were unchanged.
Both unchanged repeated applies reported NOOP for every owned release.

Real queries returned cAdvisor CPU/RAM, kube-state-metrics ownership/node data,
and Loki streams carrying the physical cluster UID. These prove source collection,
not complete application acceptance or production compliance. PostgreSQL exporter,
blackbox/ingress coverage, datasource/dashboard checks, notifications and restart
persistence remain separate gates.

The first live run found a Helm defaults merge defect: overriding the Alertmanager
receiver left default child routes pointing at an absent receiver. The values now
set an explicit empty child-route list. Rendered configuration passed the real
Alertmanager `amtool check-config`; the owned release update reconciled both
Prometheus and Alertmanager readiness. No shared cluster was changed or reset.

Local-query isolation needs `cluster_uid` metric relabeling on kube-state-metrics
and kubelet/cAdvisor; the example values require the operator to replace that UID.
External labels alone do not scope local Prometheus queries.

## Connection and binding checkpoint

The isolated feature Docker stack initialized a fresh schema-11 database with
separate owner/runtime identities. Console was opened through the actual Tailscale
feature URL. Its existing shell/platform forms were the visual reference before UI
changes. Browser creation and testing of a real connection returned successful
Prometheus/Alertmanager/Loki connectivity, cluster identity and Kubernetes/log
coverage, with config sync explicitly UNVERIFIED.

Focused Go tests cover source-specific credentials even with identical endpoint
URLs, revision-stable retesting, pre-deployment binding, foreign target rejection,
reserved labels and organization boundaries. Real PostgreSQL focused tests cover
verification CAS, referenced deletion, binding deletion fences and stale writes.
Upgrade/rollback tests preserve populated schema-8/schema-10 identities through
schema 11 and keep State runtime privileges separate. No released migration or
schema reference was changed. The inaccessible historical monitoring-only schema-9
installation has not been opened, reset or declared upgrade-compatible.

Focused existing frontend observability tests passed (8 assertions across two
files). TypeScript checks passed; lint has one pre-existing warning in
PlatformSettingsPage. Documentation checker passed; PlantUML was unavailable.

## Work in progress

Typed profiles/PodMonitor generation, retained Loki queries, live artifact
comparison and native-source withdrawal are being implemented and validated.
These are not yet completed stages 2–5. Actual MCP Gateway/Admin sources are still
missing; generic fixtures must not be counted as MCP-specific acceptance.

Private runtime state, credentials, kubeconfig and screenshots are outside Git.
The isolated kind and feature stack are retained for the remaining checks. Private
network addresses/feature links are deliberately omitted from this public record.

## Additional live checkpoints

The actual HTTP fixture was deployed through Console Preview/Deploy, platform Git
and Fleet into the owned kind cluster. PrometheusRule and PodMonitor reached live
APPLIED comparison; application counters/histograms, Kubernetes samples and Loki
logs were read through the product. Missing OOM observations keep coverage PARTIAL.
The fixture is not MCP Gateway/Admin.

PostgreSQL exporter uses a connection-limited pg_monitor account without superuser,
createdb, createrole or public-schema CREATE. Blackbox and the separate owned
Traefik provide real probe and ingress traffic metrics. These establish source
installation; product resource/dependency views and MCP-specific coverage remain
open gates. Grafana datasource health and the parameterized workload dashboard
were checked through the actual browser access path.

Controlled application failure produced actual HTTP 503/readiness loss. Both the
configured metric alert and WorkloadUnavailable reached the isolated webhook
receiver as firing, then resolved after controlled recovery. This is delivery
proof for those notifications, not evidence that every Alertmanager receiver or
company rule is configured. Grouping/inhibition remain to be verified separately.

Five UID-fenced pod restarts retained PVCs. Historical Prometheus results, retained
Loki rows, Grafana dashboard/annotation, Alertmanager silence and a PostgreSQL
marker row were verified afterwards. This proves restart persistence, not backup
restore or elapsed retention duration.

## Configuration admission and credentials

Preview and operation admission pin monitoring connection/binding revisions and
generation. A monitoring-only change produces UPDATE; stale previews cannot
mutate runtime, and applied signatures produce a subsequent no-op. Focused memory
and real PostgreSQL tests cover those guards, including busy writers. Metadata
locks are released before cluster/credential-provider network calls.

Credential rotation persists superseded private references before cleanup. Failed
cleanup can retry without changing configuration revision. Deletion keeps a
non-reactivatable DELETING tombstone until private credential removal succeeds.
Endpoint changes require an explicit replacement/clear of existing credentials.
Focused tests cover provider failure, rotation, retry and explicit clearing.

The frontend follow/pause check exposed a transient disappearance of rows when
switching from a relative interval to the same fixed interval. The query hook now
keeps that exact scoped snapshot while refreshing it; all 11 observability UI
tests pass. Source failure remains explicit.

The live monitoring-only UPDATE first stopped before mutation because the
rebuild changed the immutable renderer binary pin. Reinspection of identical
content and explicit selection of its new Definition revision restored compatible
rendering. The subsequent actual Console/Git/Fleet deployment succeeded; this
checkpoint does not yet mark the whole stage 5 complete. Known renderer tests and all 29 conformance
failures were also reproduced on the same origin/main baseline; logs remain
private outside Git. Full final gate has not yet been completed.

## Profile and log lifecycle follow-up

The version-2 development HTTP profile was edited through Console, tested, rebound
and deployed through Git/Fleet. Product comparison returned APPLIED. Deployment,
PodMonitor and PrometheusRule UIDs remained stable while the Pod UID changed.
The next unchanged Preview had no changes. Adding a second workload in the same
Application and an explicit whole-profile override produced DEPLOY plus UPDATE;
the resulting PodMonitors use the inherited 30-second interval and overridden
60-second interval respectively. This is not the stage-6 second-Application test.

Real Loki checks cover search/level/container/workload filters, cursor paging
without repeated records and retained logs from multiple Pod UIDs after rollout.
The actual Console log detail shows Pod UID, and follow/pause was exercised by
browser. A separate UID-fenced one-shot probe confirmed JSON severity and plain
levels normalize to WARN/INFO/ERROR/UNKNOWN and representative fake password/token
values are redacted. That probe was removed with its UID precondition.

The profile editor exposed an old connection whose unspecified Loki retention was
rendered as zero in a closed advanced panel, blocking HTML form submission. The
UI now displays the three-day default; a focused legacy-editability test covers
this. Native withdrawal also bypasses unused external-source verification/revision
requirements while retaining generation and cluster guards; a focused test covers
source changes after selecting both native signals.

Actual version-2 rules on both owned workloads reached firing. Alertmanager
reported two active critical alerts and three inhibited warnings; a real webhook
body grouped the critical alerts from both workloads. Console displayed firing
alerts and suppressed processing state, opened actual alert detail, and drilled
to Loki logs preserving workload and time range. After both workloads were
restored, 1,200 actual HTTP 200 requests supplied new recovery data and the
receiver recorded resolved critical alerts for both workloads. Missing data was
not used as proof of recovery.

Native-source withdrawal was performed through Console and Git/Fleet. Both
app-owned PodMonitor/PrometheusRule pairs were absent afterwards, referenced
connection deletion was refused, Console unbind succeeded, native log history
remained readable and Preview was a no-op. All six reused metrics component
controller UIDs remained unchanged. The external binding and override are being
restored for further work.

A feature frontend restart interrupted an open deployment response. The backend
completed normally. Read-only reconciliation confirmed a terminal SUCCEEDED
operation, every expected monitoring configuration APPLIED and no Preview changes
before clearing only the private runner's uncertain-outcome flag. Deployment was
not resent. The runner records pending intent and supports this guarded recovery.

Full frontend execution with four workers completed 982 tests: 979 passed and
three failed. The two historical E2E label/recorded-ID assertions and the Resource
Types deep-link assertion also fail on the same main baseline; focused
observability tests pass, including the legacy editor regression. An initial
unbounded run additionally had transient UI timing failures; bounded execution
removed those without changing assertions. Full Go checks with real PostgreSQL
are still running. Redis was initially missing from the runner PATH; private
packaged binaries/libraries were installed and the affected acceptance workload
packages passed with real PostgreSQL and Redis. This does not declare the entire
feature/full gate green.

## Resource/dependency mapping checkpoint

EXT-OBS-16 now has scoped State-resource authorization, effective profile-key
validation, revisioned optional binding metadata and shared Console charts. Real
PostgreSQL verifies mapping round trips alongside admission/reference fences.
HTTP tests reject unauthenticated/foreign resource reads and selector overrides
before resource/provider reads. Focused UI tests retain scope under late replies
and keep external failures/missing observations explicit.

Four actual workloads run through the product Git/Fleet path: the original two
HTTP fixtures plus collectors forwarding real PostgreSQL-exporter and blackbox
responses without changing their samples. These adapters are test-only, keep
source URLs outside Git and do not simulate telemetry. The PostgreSQL fixture is
an explicitly declared external dependency, not a product-owned PostgreSQL State
resource. The real product namespace State resource is mapped to its workload
request metric solely to verify scoped resource mapping/drill-down; this does not
claim database resource ownership or disk-space coverage.

Browser/API checks verified the real resource panel and two dependency panels,
actual pg_up/probe_success observations, unknown-resource 404 and resource-to-logs
workload/time preservation. Shared resource-detail shell/panels were opened before
implementation and visually compared afterwards. No navigation/branding redesign;
advanced mappings remain collapsed and the panel opens explicitly.

Config APPLIED now additionally requires the workload's persisted monitoring
revision signature, not only equivalent Kubernetes artifact specs. A binding-only
mapping edit therefore stays NOT_APPLIED until deployment acknowledges it. Focused
tests cover absent/current/stale signatures with matching live artifacts.

Controlled source failure on the owned PostgreSQL fixture and isolated blackbox
HTTP target produced actual pg_up=0 and probe_success=0. Both configured below-one
rules fired; the real DB critical notification reached the test receiver while
warning inhibition remained intentional. Restoring replicas with a UID-tested
patch and returning the endpoint to HTTP 200 produced fresh successful samples,
resolved rules, an actual resolved DB notification and the retained PostgreSQL
marker row. No shared database/identity service was stopped.

Final validation follow-up: a full main frontend run reproduces two deterministic
historical E2E label/recorded-ID failures. The Resource Types deep-link assertion
also reproduces on main when using the same external-monitoring isolation mock as
this feature; the untouched main mock can make that assertion pass before the
selection effect completes. It is not counted as a fixed product defect or a green
gate. A pooled Redis auth timing case failed in one full run and passed independently
on both feature and main; affected code is unchanged.

The initial complete PostgreSQL package exhausted default max_locks_per_transaction
while creating/dropping frozen reference schemas, which is an environment failure.
It is being rerun against a separate disposable PostgreSQL instance with 512 locks
per transaction and test-only fsync/synchronous_commit/full_page_writes disabled.
Those settings apply only to the test runner database, not the product/metrics DB.
The future-history tampering test was corrected to mutate the maximum version to
its successor; hardcoded 11 collided with the now-valid schema 11. Released SQL
migrations/references remain unchanged.
