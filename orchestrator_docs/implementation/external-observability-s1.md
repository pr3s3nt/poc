---
id: IMPLEMENTATION-EXTERNAL-OBSERVABILITY-S1
artifact: implementation-handoff
status: draft
last_reviewed: 2026-11-06
related: [ADR-040, UC-04, UC-09]
---

# Issue #35 Slice 1 — work in progress

This is an implementation checkpoint, not acceptance or completion. User approved
Git/Fleet rules, explicit external metric selection with native logs/history,
shared dev stack on k8s-4f and isolated receiver/failure testing. No other slice or
production notification channel is included.

## Current changes

Canonical ADR/specification/contract/UI-state/sequence and schema/ERD extensions;
new bounded Prometheus reads/query/rule generation; physical target inspection;
PostgreSQL connection/binding repository and immutable schema 11 upgrade; PE
connection/test and Environment-binding APIs; preliminary Console forms and scoped
metrics/alerts; delivery rule hook and scoped Fleet PrometheusRule support.
These remain under review and are not all running in the isolated runtime yet.

Before main integration, schema 8 files were preserved and a disposable PostgreSQL test passed populated
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


## Main integration — 2026-11-06

Merged `origin/main` at `b2d605f9` into `features/loggingandmonitoring` at the
user's request. This integrates endpoint delivery, retired public routes,
readable IDP names, template-owned workload defaults and the updated agent workflow.
It does not complete Slice 1 or authorize deployment of this branch.

Main already released schema 9 (endpoint reservations) and 10 (route retirement).
Their migrations and frozen references are preserved byte-for-byte. The WIP
monitoring migration moves from 9 to 11, with a complete schema-11 reference,
owner upgrade from released schema 10, and current version/history checks.
External observability is now ADR-040; ADR-037/038/039 retain main's decisions.
Both translation sets and both sides of the schema/ERD were retained.

The retained isolated `observability-s1-live` database used the earlier WIP
monitoring-only schema 9. It is **not** main's released schema 9 and is not an
accepted source for the new upgrade chain. Preserve its volumes and receipts;
do not restart it on the merged binary, reset it or rewrite its marker/checksums.
A separately reviewed compatibility migration is required before reusing that
retained database. No retained database or cluster was changed by this merge.

Slice 1 still needs pre-deploy/new-workload binding, configuration lifecycle and
removal UI, source coverage/scoped Grafana mapping, deployed rules and the real
firing/notification/recovery/isolation/cleanup gates listed above. The DNS blocker
is historical checkpoint evidence and was not rechecked in this merge task.


Validation for main integration:

- Go 1.25.14 from the existing Go image: product `go build ./...` passed;
  complete observability application, delivery application, Fleet and Prometheus
  package tests passed, plus focused HTTP/Kubernetes monitoring checks.
- Disposable PostgreSQL 17 (`max_locks_per_transaction=256`): fresh setup/reopen,
  active/frozen schema equivalence, populated 8 -> 11 and 10 -> 11, repeated setup,
  migration rollback, runtime privilege checks, populated endpoint 9 -> 11 and
  generic-workload identity/grant preservation passed. All eight retained-state
  and runtime-isolation upgrade cases passed after updating their disposable
  historical-schema fixture to remove the two new monitoring tables.
- Frontend typecheck/lint/build passed (one existing effect-ref lint warning and
  the large-chunk build warning). Five focused files: 70 passed, one failed.
  `App.test.tsx`'s workload resource-type deep-link assertion fails identically
  on unmodified main `b2d605f9`; its implementation was not changed by this merge.
- `TestBaselineEnforcesCurrentConstraints` still inserts the retired `subdomain`
  column and fails identically on unmodified main. The initial combined
  PostgreSQL selection also exceeded its 180-second budget; focused integration
  groups were rerun independently with appropriate bounded timeouts. All 11
  supported populated-source cases then passed (175.50s), including concurrent
  owner setup, repeated execution and retained application/session data. Do not
  describe the overall test suite as green.
- Documentation checker, migration immutability guard against `origin/main`,
  merged translation-preservation checks, shell/runner syntax and diff checks
  passed. ERD PNG was regenerated from the merged source with the available
  PlantUML container; changed ERD/sequence syntax checks passed separately.
- No full product suite, browser acceptance, cluster deployment, DNS repair,
  failure injection or notification send was performed for this merge.
