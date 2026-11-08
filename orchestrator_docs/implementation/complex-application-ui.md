---
id: IMP-COMPLEX-APPLICATION-UI
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-08
---

# Complex application deployment through Console — issue #50

## Scope and current stage

The user authorized implementing issue #50 until a product/architecture decision
is needed. Branch: `features/complex_application_ui`, base main `2755c864`.
GitLab is the first acceptance application; the goal is a functioning complex
application entered through Workload forms and deployed in one Environment batch.
This record contains survey evidence and implementation checkpoints. GW-08 in
the Generic Workload contract owns the behavior approved after A selection. The
initial survey itself changed no product code or external application state;
later real UI deployment evidence is linked in the final section.

## Verified upstream baseline

The official chart index identifies chart **10.5.4**, application **v19.5.3**.
The release archive was downloaded from the URL in that index, not from master,
and its SHA-256 matched the index:
`aabc14c99bfc5829a54ecefdc99b2c2f16257e892fddf6425929dfd69375c62c`.
Helm 4.3.1 was downloaded privately and checked against its published checksum.
Local `helm template` with target Kubernetes 1.36.1 succeeded; no Helm install,
upgrade, hooks or Kubernetes writes were executed.

Sources:

- [Chart index](https://charts.gitlab.io/index.yaml).
- [Release archive](https://gitlab-charts.s3.amazonaws.com/gitlab-10.5.4.tgz).
- [Version mappings](https://docs.gitlab.com/charts/installation/version_mappings/).
- [Installation and production boundaries](https://docs.gitlab.com/charts/installation/).
- [Migrations chart](https://docs.gitlab.com/charts/charts/gitlab/migrations/).

This baseline is for the test mapping; production architecture is not claimed.
Archive, extracted third-party sources, rendered YAML, Helm tool and diagnostics
remain outside Git. No external source is copied into product runtime.

## Offline mapping inventory

The survey profile uses placeholder external PostgreSQL/Redis/object-storage
connections, disables optional SSH/KAS/Registry/Runner/metrics/gateway installation,
and assumes credentials will be supplied through Console's approved secret path.
It disables the chart's shared-secret generator only for that mapping. It is
**not deployment-ready**: secret generation/delivery, dependency provisioning,
TLS and sizes remain to be completed in later stages.

| Component | Chart runtime | Image family / structure | Existing form mapping |
|---|---|---|---|
| Webservice + Workhorse | Deployment | Two app containers; certificates/configure/dependency init containers | Multi-container Deployment, init order, probes, shared emptyDir, config/secret files, Service/endpoint |
| Sidekiq | Deployment | One app container; three init containers | Deployment with Redis/database/object-storage config and migration dependency |
| Toolbox | Deployment | One app container; two init containers | Deployment with shared configuration and secret references |
| Gitaly | StatefulSet, one replica | One app container; two init containers; repo-data claim | StatefulSet/claim template/retention; compatible with current one-replica limit |
| Database migration | Job | Toolbox image; certificates/configure init; wait-for-deps and migration script | **No independent Job execution kind in current form/intent** |
| PostgreSQL | External to this chart | Database plus required version/extensions and credentials | Candidate resource Definition or separate stateful workload; exact provisioning unchosen |
| Redis | External to this chart | Redis connection/credentials | Candidate resource Definition or separate workload |
| Object storage | External to this chart | S3-compatible connection and buckets | Candidate resource Definition or separate stateful service; provider unchosen |

All core application images in this render use the chart's v19.5.3 family. No
image equivalence with an all-in-one GitLab container is assumed.

The resulting local render contains 3 Deployments, 1 StatefulSet, 1 Job,
7 ConfigMaps, 2 Services, 3 PDBs and 2 HPAs. This is a **survey render**, not the
final platform inventory: HPA is out of the first acceptance scope and fixed
replicas must be authored explicitly; PDBs require compatibility with existing
platform rules. Database/Redis/object-storage and platform secret/endpoint objects
are not part of those counts. The initial profile still rendered bundled gateway
objects because enabled and install switches are separate; the final survey
sets both off and renders no cluster-scoped resources. Nothing was applied.

Chart app-container requests at one replica, before auxiliary services and init
scheduling overhead:

| Component | CPU requests | Memory requests |
|---|---|---|
| Webservice | 300m | 2.5G |
| Workhorse | 100m | 100M |
| Sidekiq | 900m | 2G |
| Toolbox | 50m | 350M |
| Gitaly | 100m | 200Mi |
| Migration Job | 250m | 200Mi |

These are upstream defaults, not validated final sizing. The chart's default
Gitaly claim is 50Gi; test sizing/retention need explicit configuration before
creation. Cluster capacity/metrics were inspected read-only; actual capacity values remain
in private run evidence. This does not guarantee capacity for GitLab: other workloads, limits,
actual usage, disk backing, init scheduling and all dependencies must be budgeted.
The default local-path storage binds on first consumer and does not advertise
expansion; do not plan to fix undersizing by blindly expanding a claim later.

## Current code contract and relevant reuse

- `frontend/src/features/workloads/ExecutionFields.tsx` exposes only Deployment,
  StatefulSet and CronJob.
- `backend/internal/domain/templatesource/workload_templates.go` accepts only
  those three primary preset kinds. Adding a form option alone cannot add Job.
- `backend/internal/adapters/templatedriver/guard.go` requires a primary workload
  matching declared execution intent. Additional namespaced objects may accompany
  it, subject to platform ownership/secret constraints. A Job cannot currently
  replace the primary object.
- `backend/internal/application/delivery/coordinator.go` already partitions a
  workload's rendered manifests into prerequisite, Job and rollout phases.
- `backend/internal/adapters/kubernetes/deployer.go` already waits for Job
  completion. UC-06 CD-03 requires preserving phase order and forbids Job replay
  on polling, acknowledgement retry or policy changes. This is **not** evidence
  of a first-class Job form or a new cross-workload completion dependency.
- Generic Workload ConfigMap file values are literal nonsecret text, not arbitrary
  expressions. CNG configuration adaptation must preserve its expected file and
  secret formats; service/env bindings do not automatically interpolate every
  file. Raw chart Secret creation cannot bypass platform-owned secret delivery.

## Survey validation

- Official release archive checksum matches the official index; Helm binary
  checksum verified; local reduced-profile render succeeds without cluster access.
- Real authenticated Console browser opens the custom workload form and reveals
  advanced execution options: Deployment, StatefulSet, CronJob only. Cancel
  returns to the original Environment; before/after workload reads are identical.
  Screenshot/receipt remain private. Early inspection selectors were corrected
  to the existing labeled controls/disclosures; no product behavior was modified.
- Node capacity, metrics and StorageClass were read only. No namespace, workload,
  Definition, connection or secret was created, no Helm hook was run, and no
  application Save/Preview/Deploy was performed.
- Documentation checker and whitespace validation pass. Go/frontend product tests
  are not rerun for this documentation-only survey; no product code changed.
  PlantUML is unavailable locally; no diagram changes are included.

## Migration authoring decision — A approved 2026-11-08

The missing decision is **how the developer authors a migration task and how the
Environment expresses waiting for its success**. Both options retain the single
batch Deploy acceptance and require tests for failure and no accidental replay.

| Option | Developer experience | Scope/tradeoff |
|---|---|---|
| A — first-class one-shot Job | Add a migration workload in the form, specify inputs/resources/secrets and select completion dependencies for consumers | Reusable beyond GitLab; extends normalized execution contract, UI, planner/dependency validation, render guards, progress and lifecycle. Job re-execution semantics must be specified; the existing delivery phase is reused where applicable. |
| B — migration companion owned by a template | Select a GitLab-specific template whose reviewed Git source includes a migration Job before its primary rollout | Smaller primary-kind change; migration is not an independent form workload. Must still prove cross-workload ordering for Sidekiq and other consumers, approved secret projection, exact Job identity and no replay. Not automatically safe just because companion manifests are allowed. |

The user selected **A**. The recommendation was based on supporting complex applications rather
than only one GitLab packaging. It keeps migration visible and reusable
as a task. GW-08 now owns the bounded implementation contract: Job plus explicit
waitForJobs completion dependencies, preserving current immutable/no-replay
behavior. Arbitrary dependency conditions or a rerun button are not included.

## Next work after the decision

1. Update the affected UC-05/06/16 specs/contracts and realization, then shared UI
   design only where impacted; define completion/failure/redeploy/no-replay rules.
2. Implement the approved slice with focused unit/integration tests, preserving
   existing ownership/pin/secret safeguards and cycle validation.
3. Finish CNG configuration, approved shared secrets, external dependency versions/
   storage, endpoint/TLS and capacity preflight. Do not claim these are resolved.
4. Use an isolated feature Console and actual UI to author every component, then
   Preview and one batch Deploy, followed by GitLab project/commit/HTTPS Git and
   data-retention acceptance. Stop again if another material decision is missing.
5. Commit/push/comment stage results; user acceptance before main merge.

## First implementation checkpoint — in progress, not runtime acceptance

After A approval, JobPolicy and waitForJobs round-trip through typed intent;
source capability admission and standard Job rendering/guards are implemented.
Completion edges participate in final-candidate and mixed Service/resource cycle
validation, and pending batch ordering puts Job providers before their consumers.
The editor exposes Job policy and explicit completion selection, including missing
references that remain removable. Standard source adds Job preset 1.0.0; existing
preset metadata remains unchanged. Native Job policy is admitted through server
dry-run before publication. Existing Job-phase immutable/no-replay semantics stay.

Focused Go checks cover validation, capability absence, serialization/zero/null,
rendered Job policy tampering, Job ordering, deletion with unchanged consumers
and mixed cycles; affected environment/template-source/workloadspec package suites
and Go build pass. Console focused groups pass; the latest editor/JobFields run
passes 27 tests including Job/consumer Save payloads. Typecheck/lint/build pass,
with the existing PlatformSettings warning and bundle-size advisory. At that checkpoint, runtime Job/GitLab acceptance and the feature full gate
were pending. Those unit/component checks alone are not deployment success; the
subsequent real Job evidence is linked below.

## Real Job proof and next decision

The isolated Console UI proof now passes a complete two-workload batch and a
consumer update with unchanged completed Job UID/time. Platform Git, account, CD,
Definitions, workloads and Deploy used UI; scoped Fleet namespace RBAC preparation
is disclosed separately. See [verification](../verification/2026-11-08-complex-ui-jobs.md).
GitLab itself is not deployed. The user selected Tailscale HTTPS (option 1), then
enabled the account setting after the initial refusal. Certificate issuance and
TLS verification now pass; an HTTPS Serve listener forwards to the existing
Gateway, and comparison verifies all prior Serve entries are preserved. Hostname,
certificates and access details remain private. This is transport setup, not a
working GitLab screen.

All nine component/dependency drafts and a webservice endpoint are authored via
UI before any Deploy. Initial Preview failed because graph discovery interpreted
literal ConfigMap shell text as platform placeholders. GW-06 already requires
literal content: `planning/module_references.go` now excludes only ConfigMap file
values from reference discovery/shared retention without changing authored text
or real container/resource validation. Regression and all planning package tests,
Go build, image rebuild and whole-Environment UI Preview pass. No contract change.

The selected MinIO server/client tags return no manifest; the upstream repository
is archived. The user approved SeaweedFS S3 instead. Objectstore and bucket Job
drafts were edited through UI, preserving Service identity/port, with SeaweedFS
4.52 and AWS CLI 2.31.20 manifests verified. This does not yet prove GitLab S3
compatibility or retention. Next is one batch Deploy and functional acceptance;
full issue acceptance/merge stays pending.
