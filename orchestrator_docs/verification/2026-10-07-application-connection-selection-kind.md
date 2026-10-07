---
id: VERIFY-20261007-APPLICATION-CONNECTION-KIND
artifact: verification-record
status: evidence
last_reviewed: 2026-10-07
related: UC-01, UC-03, UC-04, UC-06, UC-08, UC-12, UC-16
---

# Selected uploaded Connection — live Kubernetes and human UI recording

## Outcome

The approved UC-01 Application connection selection was exercised through the
Web Console against the existing real `kind-idp-internal` Kubernetes cluster.
Runtime executors, connection verification, PostgreSQL, Vault/VSO and deployed
workloads were real; no fake/stub executors, mocked responses or API-created
product mutations were used in the successful browser flow.

[Reviewed MP4 (7 minutes 12 seconds)](https://github.com/pr3s3nt/poc/releases/download/acceptance-recordings/application-connection-kind-appconn-kind-20261007102245-10160.mp4)
is published in the existing
[recordings release](https://github.com/pr3s3nt/poc/releases/tag/acceptance-recordings).
The full-window video includes browser chrome/address bar, visible cursor,
clicks, deliberate typing, native dropdown selection and reading pauses.

## Run and assertions

| Item | Observation |
|---|---|
| Baseline product commit | `695e7b1` |
| Successful run ID | `appconn-kind-20261007102245-10160` |
| Context / node | `kind-idp-internal` / `idp-internal-control-plane`, Ready, Kubernetes v1.36.1 |
| Application | `90aab0ff-cad9-4687-8057-3895ba892e33` |
| Namespace | `app-90aab0ff-cad9-4687-8057-3895ba892e33-staging` |
| Selected Connection | `uploaded-kind-510160`, `KUBECONFIG`, READY; different from default `internal-cluster` |
| Definition | `cluster-uploaded-kind-510160`, existing-cluster, resource ID `connections.uploaded-kind-510160` |
| Browser result | PASS backend connectivity, environment, secret, database, job submission |
| Real runtime | backend/frontend Deployments available; Services present; pods Running/Completed; PostgreSQL used by diagnostic backend |
| Persisted binding | Application and cluster/namespace/PostgreSQL Active Resources all use selected Connection key |
| Host fallback control | Private backend host-context token intentionally invalid and rejected; uploaded kubeconfig accepted; live deployment still succeeds |
| Recording | H.264, 1440×900, 15 fps, 432.2 seconds, 18 phase marks, 3,278,157 bytes |
| Validation | Full MP4 decode clean; all 18 settled frames nonblank; coordinator reviewed upload/selector/Deploy/diagnostic frames plus the visible native dropdown |
| Cleanup | Namespace absent via successful API response; run-owned credential Vault container absent; private credential directory and state removed; current context unchanged |

The Platform Engineer uploads the private flattened kind kubeconfig and
registers the matching Definition using UI forms. After sign-out, Developer
selects the new Connection when creating an Application, observes the shared
binding on staging and production tabs, saves Variable/Secret references and
backend/frontend workloads via the current forms, then Previews/Deploys in
staging. Browser navigation opens the real frontend through a local
port-forward and checks diagnostic PASS rows. A submitted job is accepted as
PENDING; worker processing is not exercised in this two-workload recording.

The default and selected Connection refer to the same physical cluster and the
upload originates its existing admin certificate. The deliberately invalid
backend host credential proves selected credential execution, not deployment
between two independent clusters. Production tab binding is observed; this
live run deploys staging only. The earlier
[local run](2026-10-07-application-connection-selection-local.md) covered both
Environment flows and backend restart.

## Procedure and review

[Live runner](../../backend/test/integration/application-connection-kind-video.sh)
and [human Playwright scenario](../../frontend/test/e2e/application-connection-kind-human.mjs)
were written/executed by Claude Code via `clauded` in tmux
`app_connection_live`; the coordinator owns documentation and final review.
Run with `bash backend/test/integration/application-connection-kind-video.sh`
after building the frontend, following the [kind runbook](../operations/kind.md).

Initial recordings found a Driver dropdown locator issue and a video-validator
mark-count mismatch; the harness was corrected and the final run exited 0.
No production source or canonical behavior change was required. Only the
successful reviewed MP4 was published, with a new name; old assets were kept.
The GitHub asset was downloaded and compared byte-for-byte with the reviewed
original; SHA-256 is
`29bd0e3e7daed20b6ec460344414fa357b16c4b6c15944ee6b01e7880e5584ea`.
Private logs/marks/frames are retained outside Git under
`/tmp/poc-application-connection-live-review/`. The final evidence directory is
named by the successful run ID. Credential contents were checked in-process
for absence from UI responses and text artifacts, without printing them.

Validation includes shell/JavaScript syntax, frontend typecheck/lint, 121 tests
and production build, all passing (the coordinator ran the full frontend gates).
Two unused harness declarations were removed after recording without behavior
changes. Documentation checker and whitespace checks also passed.
Full Go gates were not rerun because Go product/test source did not change.

## Lifecycle limits

Namespace teardown removes the run's Kubernetes resources and stops its
port-forward/browser/display/recorder/backend processes. The temporary
credential Vault container and private files are removed. Existing platform
Vault configuration revisions/per-workload policies/auth roles and loaded
acceptance images retain the lifecycle limits described by the runbook;
platform Vault, VSO and existing workloads were not replaced or deleted.
No AWS/DigitalOcean target, cluster creation/deletion, database migration or
current-context change was performed. Generated videos, state, binaries,
credentials and build outputs were not added to Git. The task started with a
clean working tree and no user-owned changes.
