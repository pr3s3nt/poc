---
id: IMPLEMENTATION-EXTERNAL-OBSERVABILITY-S1
artifact: implementation-handoff
status: draft
last_reviewed: 2026-11-06
related: [ADR-037, UC-04, UC-09]
---

# Issue #35 Slice 1 — work in progress

This is an implementation checkpoint, not acceptance or completion. User approved
Git/Fleet rules, explicit external metric selection with native logs/history,
shared dev stack on k8s-4f and isolated receiver/failure testing. No other slice or
production notification channel is included.

## Current changes

Canonical ADR/specification/contract/UI-state/sequence and schema/ERD extensions;
new bounded Prometheus reads/query/rule generation; physical target inspection;
PostgreSQL connection/binding repository and immutable schema 9 upgrade; PE
connection/test and Environment-binding APIs; preliminary Console forms and scoped
metrics/alerts; delivery rule hook and scoped Fleet PrometheusRule support.
These remain under review and are not all running in the isolated runtime yet.

Schema 8 files were preserved. A disposable PostgreSQL test passed populated
8 -> 9 installation/app identity preservation, repeated setup, rollback and runtime
privilege checks. More repository/security/target/lifecycle tests and retained state
coverage are required before release. No retained product database was upgraded.

## Actual dev runtime

Private receipts/config/checks under `~/.local/state/observability-s1-20261105/`.
Shared Helm release/namespace `orch-observability`, chart 92.18.0, all component
pods ready, Prometheus 8Gi + Alertmanager/Grafana 1Gi PVCs Bound, dev retention 3d.
Setup-owned physical cluster identity recording rule and isolated bounded Go
receiver are installed. Existing apps and other releases were not intentionally
modified. Cleanup procedure is in the [setup guide](../../e2e/setup/observability-stack/README.md).

Real Prometheus reads returned pod owners, CPU, RAM and restart increase for an
existing app (read-only inspection). Real browser PE connection creation/test
passed on isolated Compose project `observability-s1-live`, fresh schema 9:
connectivity, four required metric families and physical cluster identity. Rule
synchronization explicitly remains UNVERIFIED. Browser screenshot retained privately.

The isolated acceptance app is `00df1898-c61d-448d-9a7f-a19b8997dad6` / staging.
Bootstrap created infrastructure resources and admitted backend deployment, but
Fleet failed to clone its published commit: `lookup github.com on 10.96.0.10:53:
server misbehaving`. The node also cannot resolve github.com; host GitHub access
works. No shared DNS configuration was changed. User decision to expand scope to
repair shared dev DNS is pending. Inspect original operation
`2a834742-6e3d-4208-9f2a-3db6c5fd20d9` and immutable receipts before recovery/retry;
a frontend refresh disconnected the waiting HTTP response with 502, which is not
proof that the deployment stopped or rolled back. Do not blindly replay bootstrap.

## Validation and remaining gates

Focused observability/Kubernetes/Prometheus/delivery/Fleet checks and Go build
passed. Frontend typecheck/lint/build and focused UI/catalog checks passed; first
broad frontend run was overloaded, with timeouts/failures. The bounded rerun
finished with 962 passing and two failing tests: `terraform-v2-labels` and
`v04-internal-full`. Results remain in private logs. Broad Go test run failed with fixture/auth/old contract
failures and PostgreSQL timeout; do not describe those as baseline until reproduced.
Migration immutability guard, documentation checker and diff whitespace pass.

Still required: classify all relevant test failures and review the complete diff;
complete configuration revision/pending/lifecycle behavior, create-before-deploy
binding and newly added Workload target handling, connection removal/unbinding UI,
source timestamp/gap/limit coverage and Grafana scoped dashboard mapping; prove
rules applied through product deploy, no duplicates on redeploy and correct owned
cleanup. Query code's adapter/application dependencies need architecture review.

No controlled app failure, firing/resolved notification, bound-app Console
investigation, cross-app/environment live isolation or app cleanup has passed.
The [scenario](../../e2e/scenarios/kubernetes-monitoring/README.md) currently provides
connection/binding checkpoints only. Do not tick Slice 1 complete in issue #35.

## Services and source preservation

Stack and isolated Compose volumes are retained. Restart only the owned Compose
project using private `start.sh`; never reset retained databases. Private logs
identify scoped Prometheus/Alertmanager/Grafana forwards. Console Tailscale Serve
3055 and Grafana 3065 were configured. The browser verified only the Console
platform settings/connection screen; this is not a ready-to-use workload handoff
or a Grafana dashboard acceptance claim. Keep private receipts before cleanup.

On 2026-11-06 the user requested an issue progress comment and a WIP checkpoint
commit/push. Progress was posted to
[issue #35](https://github.com/pr3s3nt/poc/issues/35#issuecomment-6675824013).
This checkpoint does not mark Slice 1 accepted or ready to release.
Focused Go checks/build, 79 frontend tests across six files, frontend
typecheck/lint/build, shell/runner syntax,
documentation and migration immutability checks were repeated for the checkpoint.
Lint retains one existing effect-cleanup warning; frontend build reports the
existing large-chunk warning. PostgreSQL integration was not rerun in this
checkpoint turn because the prior disposable test server is no longer running;
the earlier successful upgrade/rollback checks above remain the recorded evidence.
Original staged files `INDEX.md`,
`backlog/observability-integration-design.md`, `usecase/UC-09/README.md` remain
user-owned and must not be staged/committed with this task without authorization.
