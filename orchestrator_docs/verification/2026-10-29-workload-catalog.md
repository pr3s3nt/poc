---
id: VERIFY-2026-10-29-WORKLOAD-CATALOG
artifact: verification-record
status: current
last_reviewed: 2026-10-29
---

# Workload catalog — acceptance in progress

Task branch `features/24-workload-catalog`; no main merge yet. Existing kind
`kind-idp-internal`, isolated Console Docker project `catalog24`. User authorized
new owned namespaces; original Passbolt is not adopted, migrated or modified.

## Initial generic run — 2026-10-28/29

Source fc05bfb plus local UI locator/accessibility fixes. Real UI registration,
Git inspection, picker, Save/Preview/Deploy used real Terraform/Kubernetes.
14 assertions passed: primary StatefulSet, bound stable PVC, data write/read,
Pod recreation preserves data/PVC UID, standalone CronJob/no Deployment,
scheduled Job completion, explicit suspend, workload removal retains PVC,
compute absent, namespace RETAINED_DATA refusal, explicit PVC deletion and
namespace deletion via confirmed product operations. App key
`df1e6767-405c-447b-ac10-65b713469333`; namespace
`app-df1e6767-405c-447b-ac10-65b713469333-staging` was verified absent.

This run did not cover the latest Job dialog, update/reuse or Passbolt. Logs and
screenshots were under `/tmp/workload-catalog-24`; a subsequent machine restart
cleared them. The result was observed before restart; archived evidence is no
longer available. A new expanded run on persistent evidence storage is required.

## Automated checks observed before restart

Go test/build and frontend typecheck/lint/build/full 921 tests passed before the
last changes. Job dialog/home focused checks: 26 pass. Latest backend focused
planner/environment/Kubernetes/observability/HTTP checks passed. Real PostgreSQL
adapter, pending/deployment and execution JSONB reopen checks passed. A broad
Terraform harness run failed from tmpfs exhaustion and timeout; its retry was
interrupted by machine restart, not counted as pass. Revalidation is in progress.

## Expanded generic run r2 — 2026-10-29

Persistent private artifacts/evidence: `/home/thanhnt1/.cache/workload-catalog-24/`.
Runner: [workload-catalog](../../e2e/scenarios/workload-catalog/README.md).
App `907304df-982d-4387-8ddc-58cbebc914c3`, namespace
`app-907304df-982d-4387-8ddc-58cbebc914c3-staging`. The expanded runner passed:
StatefulSet/Service/PVC authoring, real delayed binding, Pod recreation, config
update with same PVC UID/data, standalone scheduled Job completion, live UID-owned
Job history dialog, suspend/remove, retained storage, explicit re-authoring reuses
the retained instance/data, namespace retained-data guard and separate PVC/namespace
cleanup. Additional scoped reads verified scheduled logs collected into platform
history and both cleanup operations SUCCEEDED/allVerified with no remaining items.
The namespace is absent. No direct kubectl resource cleanup bypass was needed.

Build: fc05bfb plus task-branch local storage graph/Job-history/preflight/UI changes;
immutable module/render Git source remained fc05bfb. Later file-only preflight,
protected-path/numeric grammar and locale formatting changes were checked by
focused tests; they do not change this happy-path fixture's intent. Source build
and published Git module pins are distinct and recorded explicitly.

UI comparison: real original ApplicationHome, existing shell/styles/Dialog and
workload form were used. `job-history.png` was visually inspected: history outcome
and observation time are separate from schedule status; navigation and controls
retain product styling. Context is a retention sentence, not a setup tutorial.
Date formatting was corrected to use the selected locale after visual review.

Current checks: full Go test and build pass; frontend typecheck/lint/build pass;
full frontend retry with maxWorkers=2 passes 925/925 (79 files). Initial unconstrained
suite had one existing live-log timing failure; focused 5/5 and the bounded full
retry passed without weakening assertions. Latest secret-file preflight, domain,
planner and HTTP tests pass. Documentation checker/PlantUML/diff check pass.
Real PostgreSQL adapter (including runtime reopen), pending and deployment pass;
Terraform executor SQL harness was still running at this checkpoint; its later
completion and integrated-main run are recorded below.
No Passbolt, MySQL live secret prerequisites, S3 backup/cleanup or restore success
is claimed. Those acceptance inputs/gaps remain in the implementation handoff.

## Integrated main run r3 — 81f65ac

Main 6004792 was integrated on the task branch, preserving UC-18/ZIP/migrations.
ADR numbering is now ADR-028 (main owns ADR-025/026/027). The task-owned Console
was quiesced and backed up privately; new owner setup upgraded its retained
platform database successfully, without reset. Existing catalog/accounts/apps
remained usable for the next real run.

Expanded r3: **25/25** assertions pass, including actual scheduled log collection,
live Job history, stateful update/reuse, separate persisted allVerified PVC and
namespace cleanup, and confirmed Application deletion/tombstone. App
`1009fb8c-0f93-4bf5-b6c1-5d2babbf4774`; namespace
`app-1009fb8c-0f93-4bf5-b6c1-5d2babbf4774-staging` is absent. Screenshot
`e2e-r3/job-history.png` was visually inspected: selected Vietnamese locale is
used for dates and the original shell/dialog remain. Public Git module/render
pins remain fc05bfb; running product build is integrated 81f65ac.

Integrated source: Go test/build pass; frontend typecheck/lint/build and
**934/934** tests (81 files) pass. Real PostgreSQL adapter tests, including
preserving upgrades/backup-restore and execution JSONB reopen, pass (368.409s).
Application deletion SQL/HTTP tests pass (20.892s). Additional pre-integration
Terraform executor SQL harness completed successfully (920.352s); its earlier
restart-interrupted run is not counted. Documentation/PlantUML/migration checks
pass. CI initially hit PostgreSQL SQLSTATE 53300 (connection slots) in a role
identity test while packages ran concurrently. CI now schedules SQL package
processes with `-p 1`, preserving all tests/assertions and mandatory integration
flags; final CI is required before merge.

Runner hardening after r3 requires private input directory and matching recorded
Application ID before resuming a failed run. Pod restart verifies owner and uses
UID delete preconditions. The actual raw kubectl UID-precondition operation was
verified against a separate task-owned kind fixture; Pod and namespace cleanup
both passed (`uid-delete.log`). No shared namespace or cluster was removed.
