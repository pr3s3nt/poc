---
id: VERIFY-GIT-BOOTSTRAP-PVC-20261104
artifact: verification-record
status: current
last_reviewed: 2026-11-04
---

# Git-only bootstrap and backend PVC acceptance — issue #33

Scope: [approved follow-up](../implementation/direct-bootstrap.md),
[bootstrap inputs](../../scripts/bootstrap/README.md),
[scenario](../../e2e/scenarios/bootstrap-git-pvc/README.md).
Continued the existing `feat/git-bootstrap-pvc` checkout at `d181e718`, preserving
its unfinished Git-only/PVC changes. Added final-success-only Playwright recording.
No Go product, frontend or database schema change was required.

## Inputs and real environment

Retained dedicated `git-bootstrap-20261104` schema-8 Console (3043/backend 8103),
real PostgreSQL/Vault, authorized kind `k8s-4f`, existing Fleet/VSO and scoped
platform Git credentials. No shared DB reset/migration, cloud, real staging or
production mutation. The Application Environment is named `staging` within this
dev/kind input; this is not a deployment to an external staging installation.

All 11 business files came from HTTPS Git objects pinned to full commit
`d181e7181d7080542b60f148c52d55e7b7119392` with individual SHA-256 checks:
Resource Types, six Definition documents, namespace access rules and three
Workload documents. Four inspected Terraform modules use Git commit
`c78243afdb0173535a02914e5907f9f7ef0b2bec`; the standard workload template source
uses `48035f75af4b3fe7ce1fafd9ebaf41f8b4356d10`, tree digest
`fa4df6f2e54046fe85cefe58c30782663b899cee63ec480047df636be1f55c18`.
Definition registration, Preview and Deploy used real Console forms/read-back,
without an Application Catalog selection or packaged/inline source fallback.
Local config, credentials, kubeconfigs and evidence remain outside Git.

## Earlier failed run retained

Original Application `90aae0fe-e8cb-41fe-b25a-e4bad7c8f984` provisioned storage and
rendered backend successfully. Its prerequisite CD phase reached Git/Fleet, but
rollout Git publication did not reach the remote: local commit `190d050` was ahead
of remote `6eddfcec5356dbc1275aada0eeb8bfc868b0f718`. The rollout intent is
`OUTCOME_UNKNOWN` with no receipt. A later authenticated push dry-run passed;
no manual push, receipt fabrication, lock clearing or replay was performed.
The original Application/data/held operation stay intact for operator attention.
Missing receipt proof prevents the documented recovery path; this remains a
limitation rather than a claim that the original deployment was recovered.

A separate task Application was used for the final acceptance after that read-only
connectivity check. Existing platform Definitions/store/default/account were reused.

## Successful complete scenario

Application `acceptance-git-dev-final`, key `2d5affa1-a9c2-4b75-a86a-29f81f6193cf`, namespace
`app-2d5affa1-a9c2-4b75-a86a-29f81f6193cf-staging`, namespace UID `60e84112-adc2-4f6e-8317-7dcd1fe55d8d`.
The scenario completed platform-only → full acceptance → unchanged rerun →
UID-guarded backend Pod recreation → final unchanged bootstrap.

- Three Workloads ready; five app checks PASS and public job ID `1` DONE.
- All six Fleet units had GitRepo commit equal to the pinned revision and their
  corresponding Bundle ready at that commit.
- Six Definition editor versions/generations stayed at 1, with unchanged immutable
  revision IDs, Git/template/module source pins and resource identities.
- Both unchanged bootstrap reruns reported deployment SKIPPED. Deployment IDs:
- `415f6dcb-7ca3-4a81-9686-0851a6f9b347`
- `aca9b5f9-db4c-4502-8bd2-86370f498f35`
- `b872e4e2-d80f-4ea1-96ff-90a2e4b1e6a6`

Backend PVC `data-0e040a5ea4dc64d9` is managed, Bound and mounted writable
at `/data`, with `ACCEPTANCE_STORAGE_DIR=/data`. UID
`8723c8eb-9afc-4eeb-93ec-728c64a36877` and Deployment UID
`c006e209-b2cd-4796-8571-fb792ed542b4` remained unchanged.
Pod `backend-84cd77c77b-8jfvt` / `a3882324-e18c-4f10-9b7f-21908523787c` was replaced by
`backend-84cd77c77b-vzzk7` / `15a46ee9-b184-46c1-bf6b-7294457fa17b`.
Namespace/PVC ownership and Pod → ReplicaSet → Deployment owner UIDs were checked;
only that exact Pod was deleted with a UID precondition.

Public value `git-dev-persistent-file` and SHA-256
`6554dcc256a636da1fbf6dbc314229bb715abca48c83f9d91c77c1dcfa326732` were identical before/after Pod recreation and final rerun.

## Validation and video

56 bootstrap tests passed (0 failures/skips), including real Chromium video
finalization, discarding failed recordings, preserving a previous success on
failure and replacing it after a later success. Changed runners passed Node
syntax checks. Documentation checker and whitespace checks passed.
Focused actual-backend-snapshot render diagnosis also passed; its temporary
Go diagnostic file was removed. Whole Go/frontend suites were not rerun because
no Go/frontend product source changed.

Private input: `/home/thanhnt1/.local/state/issue33-git/final-config.json`.
State and reports: `/home/thanhnt1/.local/state/issue33-git/final/`.
Only the successful complete scenario published
`reports/videos/backend-pvc-retention.webm` (1440×1000, VP8, 33.68 seconds).
Intermediate bootstrap recordings were disabled; failed recordings are discarded.
Frames were visually checked: original Pod and replacement Pod show the same
file/hash and all five app checks PASS. Console credential-entry pages were never
recorded. Video shows the public workload before/after recreation, not CLI output
or every bootstrap Console form. An MP4 copy is provided in the chat deliverables.

## Verified handoff and retention

Browser and same-origin API checks passed at `2026-11-04T12:10:50.990Z` through the
actual Tailscale MagicDNS URLs from this host:

- [Console Application](http://desktop-bdjp2rq-1.tail342836.ts.net:3043/ui/applications/2d5affa1-a9c2-4b75-a86a-29f81f6193cf?environment=staging) — authenticated, three Workloads ready.
- [Acceptance workload](http://desktop-bdjp2rq-1.tail342836.ts.net:3044) — five checks PASS, job DONE, retained file/hash
  and replacement backend process visible.

App/data, generated private Git repositories, Console and forward are retained.
Unrelated Applications/controllers/RBAC were not removed or replaced.
The final forward can be restored by rerunning the full bootstrap with the private
final config after reviewing target ownership; it verifies process/service identity
before replacing its own stale forward. `tailscale serve status` must continue to
proxy ports 3043 and 3044. This run does not prove access from another tailnet device.
Cleanup requires separately requested product remove/Preview/Deploy and Application
deletion with recorded ownership/UID checks; no PVC deletion was performed.
