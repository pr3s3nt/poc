---
id: VERIFY-20261025-NATIVE-OBSERVABILITY-LIVE
artifact: verification-record
status: evidence
last_reviewed: 2026-10-25
related: [ADR-022, UC-09]
---

# Native telemetry — real new-user end-to-end acceptance

## Result and scope

**PASS, 48/48 assertions**, run `obs1025r7`, with real Kubernetes/Terraform
adapters, real PostgreSQL and real application stdout/stderr/CPU/RAM. The user
explicitly authorized real-cluster end-to-end verification on 2026-10-25.
Product commit under test: `88fde48` (includes UI fix `2bb0a39`, based on
implementation `3058cb6`). Runner/scenario/workload source hashes are retained
in [source-sha256.txt](artifacts/native-observability-live-20261025/source-sha256.txt).
No fake adapter, telemetry fixture, intercepted response or direct API/SQL write
created catalog/app/workload data for this run.

Runtime: new run-owned kind cluster `logging-obs1025r7`, Kubernetes node image
`kindest/node:v1.36.1`, Metrics Server `v0.8.1`, kind `v0.33.0`, kubectl `v1.36.1`,
Go `1.25.0`, actual score-k8s `0.15.0` and Terraform `1.9.8` from the recorded
local runtime dependency image. Both current backend binary and Console assets
were rebuilt. Separate fresh Compose PostgreSQL/Vault/state/cache volumes were
used; retained user installations were not reset. Metrics Server's insecure
kubelet TLS flag was confined to the disposable kind cluster.

Standard local authentication accounts and the ordinary platform bootstrap were
seeded. Driver Accounts, Types and Definitions started empty. The local bootstrap
also creates an empty `acceptance` Application; its absence of telemetry was
asserted and it was not used by the scenario. PE setup and a **new** Developer
Application were authored through the actual Console UI.

Application: `ca141bd4-6e28-4b97-9f14-a18a101ae7a6`; namespace:
`app-ca141bd4-6e28-4b97-9f14-a18a101ae7a6-staging`. The Go acceptance workload has one ordinary container,
no sidecar/agent/telemetry SDK and no observability dependency in Score.

## Executed user flow

| Step | Actual result |
|---|---|
| PE sign-in → upload the new cluster credential → register Types, existing-cluster/namespace Definitions and installed renderer template | READY account and real Terraform/Kubernetes placement; no feature catalog preseed |
| Developer sign-in → create Application → import Score → Preview | New app, explicitly empty observability, real plan, namespace still absent before Deploy |
| Deploy → open deployed app | Real Pod ready; ordinary stdout/stderr collection starts automatically |
| Emit INFO/WARN/ERROR and a 120-line batch through app buttons | ERROR card equals actual received ERROR count; more than 100 records paginate; details and literal search show real messages |
| Follow new log records → pause | New heartbeat records arrive through stored-history polling; pause retains the latest viewed interval/position |
| Start real CPU/RAM load | CPU peak 99.8% of 250m; RAM peak 82.4% of 256Mi; overview cards, charts, provider timestamps and limits verified |
| Stop load | Fresh CPU/RAM values fall below threshold while interval peaks and at least three real numeric samples remain; UI distinguishes recovery |
| Restart app process | Real container restart count/event received; previous stream is deduplicated and the original ERROR ID remains |
| Change REVISION v1→v2 through Score edit → Preview/Deploy | UPDATE, new Pod UID and v2 startup record; original Pod's received ERROR remains queryable |
| Restart only owned Orchestrator backend | Actual process start changes; PostgreSQL history/IDs persist and collector resumes with a new observation |
| Open production; inspect mobile | Production empty in UI/API; 390px viewport has no horizontal overflow |
| Remove workload via UI Preview/Deploy | Pod absent, stored old ERROR still readable through UI |
| PE delete namespace resource via confirmation UI | Resource DESTROYED and successful independent Kubernetes lookup proves namespace absent |
| Runner teardown | Owned Compose containers/volumes and kind cluster absent; scenario exit and final status both zero |

Original Pod UID `4008d76d-f14f-4742-b6e8-66488f6ee7cc` differs from replacement
`cc34c4d1-c405-4421-9568-bbb52ce1f9c7`. Backend start changed from
`2026-10-25T01:54:31.155439725Z` to `2026-10-25T01:58:29.532380724Z`.
The test deliberately uses successful process restart and bounded load, not OOM,
failed deployment or destructive negative scenarios.

## Defects found and resolved

- Narrow layout: four cards stayed in one row and a real generated namespace
  descriptor widened the viewport. Restored the approved two-column breakpoint,
  stacked charts and wrapping resource text with status preserved. Desktop
  shell/navigation/identity were kept.
- Pause: following switched to a moving interval but pausing returned to the
  original interval. Pause now pins the last displayed interval. A regression
  test and the real browser assert that newly received records remain visible.
- Optional VSO cleanup: credential-backed CLI errors redact remote text; VSO
  cleanup still parsed that text to detect an absent API type. A plain workload
  was physically removed but REMOVE reported FAILED on a cluster without VSO.
  Added a safe typed missing-resource-type category and errors.Is handling.
  Forbidden/unauthorized/unreachable errors continue to fail closed; ownership,
  UID, namespace inventory and shared-object guards were not weakened.

Earlier runs are not accepted gates: r1 failed image context packaging; r2/r3
exposed runner umask permissions for the non-root binary/nginx worker; r4 had an
incorrect empty-Applications assumption against the standard local bootstrap;
r5 reached mobile then found the actual overflow; r6 passed all telemetry and
mobile checks but failed workload cleanup. Each owned runtime was removed and
absence verified before a fresh rerun. R7 runs the corrected full scenario and
passes product cleanup before platform teardown; runner recovery does not count
as product cleanup success.

## Checks and evidence

- Full frontend regression: **73 files / 892 tests PASS**, using two workers and
  the established 60s timeout for slow existing form suites. No business
  assertion removed. Final typecheck/lint/build and focused five observability
  tests pass after the TypeScript test-option correction.
- Kubernetes adapter suite with `-race` passes, including missing VSO API and
  forbidden/unauthorized/unreachable credential-backed cleanup tests. Existing
  deployment workload/removal tests pass; `go build ./...` passes.
- Live command: `OBS_RUN_ID=obs1025r7 OBS_EVIDENCE=/tmp/poc-observability-live-20261025-r7 bash backend/test/integration/observability-kind.sh`, with recorded local dependency-image overrides and task-local Go/kind/kubectl PATH.
- No browser page errors; UI/API credential checks and exact run credential
  scan pass. Private inputs are excluded from published evidence. Required
  documentation and whitespace/shell checks pass.

[Assertions](artifacts/native-observability-live-20261025/assertions.json),
[observed facts](artifacts/native-observability-live-20261025/facts.json),
[manifest](artifacts/native-observability-live-20261025/run-manifest.json),
[versions](artifacts/native-observability-live-20261025/versions.txt),
[cleanup](artifacts/native-observability-live-20261025/cleanup.txt),
[credential scan](artifacts/native-observability-live-20261025/secret-scan.txt).

Screenshots: [new app](artifacts/native-observability-live-20261025/02-new-application.png),
[Preview](artifacts/native-observability-live-20261025/03-preview.png),
[log search](artifacts/native-observability-live-20261025/05-real-log-search.png),
[follow paused](artifacts/native-observability-live-20261025/05-live-follow-paused.png),
[high overview](artifacts/native-observability-live-20261025/06-high-overview.png),
[CPU/RAM history](artifacts/native-observability-live-20261025/06-high-metrics-history.png),
[recovered](artifacts/native-observability-live-20261025/07-recovered-overview.png),
[restart](artifacts/native-observability-live-20261025/08-restart-event.png),
[mobile](artifacts/native-observability-live-20261025/09-mobile-real-history.png),
[retained logs after removal](artifacts/native-observability-live-20261025/10-retained-logs-after-remove.png),
[namespace cleanup](artifacts/native-observability-live-20261025/10-product-cleanup.png).

Visual comparison used the real ApplicationHome/AppShell/shared components and
approved logging-monitoring mock. The narrow view now retains readable two-card
rows and the entire generated descriptor; neither shell nor navigation was
redesigned. Counts/threshold explanations stay behind help, and raw container
information stays in log detail. Screenshots contain real collected data.

## Boundaries

This proves native Kubernetes behavior on a real kind cluster, including real
Terraform namespace provisioning and stored history. It is not EKS/AWS evidence,
a seven/fifteen-day wall-clock retention soak, or a retained-database migration.
Expiry/fencing/capacity remain separately verified by existing SQL/unit tests.
Fresh new-user baseline startup is verified; older retained DB upgrade and the
advanced integrations remain separate work in issue #1.
