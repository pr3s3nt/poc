---
id: VERIFY-T29-DEV-REBUILD-20261012
artifact: verification-record
status: evidence
last_reviewed: 2026-10-12
---

# T29 — Dev rebuild

T29 / LEG-08 / VI-04/05, branch `refactor_ui`. Initial working tree was clean.
Task completed after live authorization, verified backup/restore and IDP reset,
a preserved pre-deploy locator failure and one explicit same-run continuation.
Earlier preparation sections describe their checkpoint only.
The [runbook](../operations/k8s4f-dev.md#t29-backupreset-gate) owns the procedure.

## Read-only preflight

On 2026-10-12, Docker reports retained IDP PostgreSQL/backend/frontend/Vault
and workload Vault healthy. `kind-k8s-4f` node is Ready; VSO Deployment is 1/1.
Both retained staging namespaces contain backend/frontend Deployments at 1/1,
PostgreSQL StatefulSet `modules-backend-externals-db` at 1/1 and a Bound 1Gi PVC
`data-modules-backend-externals-db-0`. Both workload DBs are PostgreSQL 16 with
DB/user `acceptance`; their PV reclaim policy is `Delete`.

| Source | Namespace / PV |
|---|---|
| final app | `app-3c5e4c33-b724-4032-80b1-d270e5c32a35-staging` / `pvc-d7cb93b1-e930-478c-9907-0b586b5ceb80` |
| earlier app | `app-a43bb834-d2d7-42cd-a035-41841155b5f7-staging` / `pvc-f7b74c53-97b3-4c7f-9288-93741bb8ed2d` |

Safe authenticated API metadata confirms both staging bindings are
`k8s-4f-2` / `k8s-4f-vault`, ENVIRONMENT, generation 0. Both production
Environments are unconfigured. Listed staging operations are SUCCEEDED;
no active operation is observed. This preflight must be repeated at the live gate.
Connection `k8s-4f` targets the unrelated old `kind-idp-internal`; the running
historical IDP still exposes HOST_CONTEXT and LEGACY_APPLICATION seed metadata.
The new baseline must not be started against this retained database before reset.
No credential, secret value, raw configuration entry or kubeconfig was displayed.

IDP DB/user: `orchestrator` in `orchestrator-local-postgres-1`, PostgreSQL 16,
volume `orchestrator-local_postgres-data`. Recovery image IDs observed:

- Backend: `sha256:049cb601e1b21fd6ed0713d730d4093012448d4e154c99457f1d31a18dbfa01e`.
- Frontend: `sha256:f589c953813635023a23a47ca666b35d9929c6a0b5872ffe871336fe0f9fbabe`.

Port 18480 is occupied by the retained access path; 18481 is currently free.
Both Deployment selectors are `app.kubernetes.io/name=backend` / `frontend`.
Vault data/bootstrap and IDP credential-token volumes are inventoried for preservation.
No cluster, operator, volume or workload was changed by these checks.

## Preparation and pending gates

Codex clarified clean-baseline runner procedure in the canonical runbook before
assigning code/test preparation to Claude in `codex-T29-20261012`.
Claude may update only runner code/tests and execute local fake checks; it has
no authorization to run backups, restart Compose or mutate retained infrastructure.
Preparation brief and draft run/recovery manifest are outside Git at
`/tmp/poc-T29-prep-20261012/`. The draft is not an approved executable run.

Data treatment and backup retention await a user decision. After that, complete
and review exact backup/restore-smoke/reset/recovery/cleanup commands, runner
changes and appropriate checks; present the concrete run for separate explicit
approval immediately before mutation. Real backups/restore proof, new baseline,
headed real video, diagnostics, cleanup and final IDs/URLs remain unverified.
No commit/push or T29 DONE claim is made during this preparation record.

## Local preparation validation

Claude's full frontend typecheck/lint/tests/build and Go tests/build passed in
`/tmp/poc-T29-prep-20261012/vfe.log` / `vbe.log`; Go conformance passed, with SQL
and kind opt-ins unset. Shell syntax and JavaScript parse checks pass. Offline
runner dry-run and rejection of in-repo evidence, nonprivate inputs and missing
resume keys were checked; the real K8S-4F browser path has not been run.

The required headed fake T01 regression initially failed because the readiness
fixture lacked its owning Organization after T28's strict snapshot validation.
Claude fixed only the fake readiness harness to create that owner before its
Connection. Failed evidence is retained at
`/tmp/poc-T29-prep-20261012/fake-T01-failed-readystate`; the rerun at
`/tmp/poc-T29-prep-20261012/fake-T01` passes all sign-in/session/navigation/logout
assertions. Codex reviewed result, assertions, phase marks and Vietnamese captions,
fully decoded the captioned MP4 and inspected the Connections frame and caption
strip. The 41-second video shows a headed browser with visible cursor; captions
sit below the UI. Fake adapters prove only this local regression, not real rebuild.

Independent documentation checker, whitespace, shell syntax and JS parsing pass.
Runner review requested isolated port-forward PID ownership and snapshots that
exclude approved resume keys only during explicit resume. Live backup/reset and
all real rebuild checks remain pending the user's data decision and final run approval.


Final preparation review confirms run-specific PID files preserve the old supervisor
handle, existing PID files are refused in shell and immediately before JS spawn,
and fresh-run snapshots exclude no existing keys. Updated FE/Go checks pass;
independent shell syntax, JS parse, docs and whitespace checks pass. The earlier
fake T01 video remains valid because this follow-up changes only the live runner.
No disposable SQL or real K8S-4F runner was invoked. No user-owned changes were
present initially; every current diff belongs to this T29 preparation.

Saved handoff: `/tmp/poc-T29-prep-20261012/status.txt`, draft plan
`run-plan.md`, checked fake video `fake-T01/T01-captioned.mp4`. No commit/push:
T29 requires one completion commit with DONE/evidence, and is not complete.
The task-owned tmux session is closed after saving this handoff; unrelated
sessions are preserved. Resume by resolving data/retention, completing executable
backup/reset/recovery preparation and presenting the final explicit run gate.


## Data gate answer and refreshed preparation

The user selected retention of both old namespaces/PVC, old FE/BE stopped,
new workload DB empty, three backups retained 30 days, without data restore.
The user explicitly withheld live-run approval pending concrete commands.
No backup or retained mutation is authorized by this answer.

Read-only revalidation found the IDP containers recreated independently of this
T29 preparation. Their database still exposes the same retained app IDs/bindings
and historical seed metadata. Current recovery images are backend
`sha256:61b2fed3dc59ff9425199bce2439025ec151462e51c811d41a9cc0f8fc103571` and
frontend `sha256:de449a00db689693084656eeb9cd2f121cc7cf31dfb75eaef28516fa6a2763b9`;
the earlier IDs are historical preflight observations. Compose source at
`a0ef186` matches the tracked current Compose configuration. Both old PVC UIDs,
replicas and loaded acceptance image tags remain as inventoried. Cluster/VSO
are Ready. Workload Vault token exists with mode 0400; its content was not read.
The former default kubeconfig upload is absent; generation is planned only after
final approval. Port 18481 is free at this recheck.

Codex updated canonical procedure with the data decision before assigning the
concrete wrapper/command manifest to Claude in `codex-T29-manifest-20261012`.
This assignment permits only code/preparation and local checks, not Docker builds,
backup, restart, kubeconfig extraction, cluster exec/scale or live UI execution.
Artifacts are at `/tmp/poc-T29-manifest-20261012/`.


## Concrete preparation review and live authorization

Claude completed wrapper/runner preparation in
`/tmp/poc-T29-manifest-20261012/`. FE typecheck/lint/429 tests/build and Go
suite/build (conformance included, external opt-ins unset) pass. Shell syntax,
JS parse and offline dry-run/rejections pass. The 42-assertion offline flow uses
fake binaries in an isolated mount/user namespace, exercising fail-before-reset,
smoke mismatch/residue/ownership, environment retarget rejection, dump failure
retention, EndpointSlice failed-read/serving guards, before/after reset recovery
and recovery idempotence/drift refusal. Codex reviewed logs, source/seed/schema
assumptions, generated manifest and wrapper-copy equality; independently verified
syntax/parse/dry-run. These are preparation checks, not real backup proof.

On 2026-10-12 the user explicitly authorized this T29 live run after the prepared
plan, requiring backup first, retained recovery, and limiting scope to this repo's
dev K8S-4F. The user also permits dev data deletion; run `t29-20261012-r1` follows
the prepared retention treatment: old namespaces/PVC remain, old FE/BE stop,
new DB empty. No unrelated system mutation is authorized. Concrete entrypoint:
`bash backend/test/integration/k8s4f-dev-reset.sh --apply --run-id t29-20261012-r1`.
Scoped recovery is available as separate `--recover` with the same run ID.
Live results are not yet claimed; backup readability must pass during the run
before dropdb. Backups remain private for 30 days; no evidence upload.


## First live phase and same-run continuation

Run `t29-20261012-r1` began 2026-10-12T04:05:19Z. Safe preflight passed;
old backend/frontend in both retained app namespaces were scaled to zero,
while PostgreSQL remained 1/1 and both PVC identities stayed Bound/unchanged.
Three private 0600 dumps were created under the approved 0700 directory.
All restored public-table counts match (IDP 24 tables, each workload 1 table),
and both workload jobs checksums match. Codex independently recomputed dump
checksums, compared source/restored count/job files and checked empty restore
error logs. Only the label-owned smoke container was removed; no residue remains.

Backup creation: `2026-10-12T04:05:55Z`; retention expiry:
`2026-11-11T04:05:55Z`. No automatic deletion is scheduled. Exact checksums and
recovery configuration stay in the private directory.

IDP reset proceeded only after verified backups, at 04:06:06Z. Current
backend/frontend became healthy; fresh seed-only baseline assertions pass
(including the seed's own empty Deployment Set, without historical deployments).
The headed UI phase registered verified KUBECONFIG Connection `K8S-4F`, key
`k8s-4f`, in the fresh database. This includes scoped private Connection credential
storage in IDP Vault; Vault volumes/bootstrap/token lifecycle remain preserved.

The first UI phase failed before Store registration, app creation or Deploy:
`Địa chỉ backend` label matched both textbox and same-address checkbox. This is
a runner locator defect; the textbox selector was made explicit. No deployment
was replayed. Codex approved explicit partial continuation of the same authorized
run, reusing only its identity-checked Connection and recording to new
`/tmp/poc-T29-live-20261012-r1/ui-resume1` evidence. Failed phase evidence stays
at `.../ui/`, including raw/captioned video, marks, assertions and FAIL result;
Codex fully decoded its captioned video. The wrapper is not rerun/reset again.
Continuation results remain pending at this checkpoint.


## Successful continuation and final independent review

Single-use `--continue --run-id t29-20261012-r1` completed with exit 0:
resume phase started at 04:17:20Z, UI phase passed at 04:25:32Z, final checks
passed at 04:25:44Z. Recovery's owner-only phase marker is `ui-resume1` and
stage is `final-checks-done`. Original FAIL evidence remains intact.

| Artifact | Actual result |
|---|---|
| Application / ID | `k8s4f-t29-20261012-r1` / `47fb34b4-2d6e-493e-a2c5-880837abfc1c` |
| Namespace | `app-47fb34b4-2d6e-493e-a2c5-880837abfc1c-staging` |
| Connection / Store | `K8S-4F` / `k8s-4f`; `K8S-4F Vault` / `k8s-4f-vault`, READY |
| Binding | staging, ENVIRONMENT, `internal-k8s`, generation 0 |
| Workloads / DB | frontend/backend/PostgreSQL 1/1 Ready; new database initially empty |
| PVC | `data-modules-backend-externals-db-0`, Bound, PV `pvc-24ecc993-ed14-49c3-9e92-4537b178b678` |
| Access | Console `http://localhost:3001/ui/`; app `http://localhost:18481/` |
| Data | four diagnostics PASS, first job accepted PENDING, preserved on reload |
| Persistence | IDP backend restarted, healthy; new app plus seed metadata preserved |

Store registration, app creation, Settings/keys/workload save, Preview and Deploy
were UI actions with real adapters. The reused Connection was explicitly pinned
by approved key, name, auth, context and endpoint; it was registered through UI
in the first phase. Pre-existing seed Connection/Store metadata is unchanged.
No deployment replay occurred. Scoped credential/secret-delivery Vault writes
are part of the approved run; no Vault reset/refresh/bootstrap was invoked.

Independent Codex review checked safe authenticated API binding metadata after
restart, live diagnostics JSON, Ready/Bound cluster metadata, old FE/BE at 0,
old PostgreSQL 1/1, retained UID/PV identities, preserved unrelated containers
and zero smoke-container residue. Three dump hashes/count files/jobs checksums
were independently compared and restore logs checked; directory/dump permissions
are 0700/0600. Backups expire 2026-11-11T04:05:55Z and are not auto-deleted.
Historical pinned images/configuration remain prepared for recovery. Live full
recovery was not run because it would undo successful state; its offline guards
and the real backup restore-smoke are the evidence for that path.

Evidence paths outside Git:

- `/tmp/poc-T29-live-20261012-r1/steps.log`, `result.txt` (PASS), plus private
  stage/phase markers under the approved backup directory.
- Failed first phase: `/tmp/poc-T29-live-20261012-r1/ui/`, including FAIL result,
  video/captions/marks/assertions and the safe Connection registration proof.
- Successful continuation: `/tmp/poc-T29-live-20261012-r1/ui-resume1/` with
  PASS result, `run.json`, assertions, `cluster-proof.json`, phase marks and
  `k8s4f-dev-raw.mp4` / `k8s4f-dev-captioned.mp4`.
- Preparation, offline test/flow and execution logs:
  `/tmp/poc-T29-manifest-20261012/`; earlier fake T01 regression remains at
  `/tmp/poc-T29-prep-20261012/`.

Codex fully decoded both captioned real videos. Continuation has H.264, 15 fps,
443.73 seconds / 6656 frames, browser 1440x900 and captioned canvas 1440x996.
Reviewed Connection upload and continuation Store/masked-token, Preview, successful
Deploy and four-diagnostic/job frames; Vietnamese captions stay below the UI,
cursor is visible and no credential content is exposed. These are two separate
recording phases, not a single uninterrupted run; no live rerun for aesthetics.
The acceptance workload's existing English labels are technical test UI, separate
from the Vietnamese Web Console. No worker is deployed, so the job is PENDING.

Final code scope is runner/test harness only; product behavior/matcher/fixtures
are unchanged. Full FE (429 tests)/Go/build/conformance passed during preparation;
follow-up JS/shell parsing and offline flow guards passed (59 assertions), with
runner guard rejections checked before continuation. Final docs and whitespace
pass. No additional AWS/production/disposable full SQL suite was run: T29 validates
actual scoped backup/rebuild, while T28 already verifies the full fresh SQL suite.
All initial/current diffs belong to T29; no pre-existing user source change was
staged or modified. Completion commit uses prefix `T29:` and includes DONE/evidence;
find the actual SHA with `git log --grep '^T29:'`. Push destination is
`origin/refactor_ui`. The owned tmux session is closed at handoff; current app's
run-owned port-forward is intentionally retained.
