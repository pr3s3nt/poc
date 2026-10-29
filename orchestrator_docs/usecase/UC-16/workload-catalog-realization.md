---
id: UC-16-WORKLOAD-CATALOG-REALIZATION
artifact: use-case-realization
status: draft
last_reviewed: 2026-10-29
---

# Catalog workload collaboration (proposed)

[Spec](workload-catalog-specification.md) and
[contract](../../architecture/contracts/workload-catalog.md) own requirements/data.

| Phase | Collaboration | Durable boundary |
|---|---|---|
| Select | ApplicationHome → catalog read → Dialog → editor schema/defaults | No desired write |
| Add storage mount | Editor → shared resource dialog filtered to public string claimName/namespace capability → local dependency + mount | No API write/provisioning; Cancel preserves form |
| Save | Editor → workload config service → template/refs validator → draft repository | CAS desired version + pin/extension atomically |
| Preview | Snapshot loader → planner → matching/render capabilities → safe review | Immutable plan/draft/config/targets/template/bundle pins |
| Provision | Deployment service → secret prerequisites + resource graph → Terraform executor | Original state/UID/instance ownership; outputs |
| Render | Resolved outputs + desired model → pinned bundle → invariant guard | One runtime controller, approved refs, safe inventory |
| Deliver | Kubernetes adapter → apply service/controller → runtime readiness | Continuous rollout or schedule registered; not fake Job success |
| Observe | Collector → CronJob UID → Job UID → Pod UID → history/log projection | Applied identity scoped to app/env |
| Remove | Pending removal → stop/delete owned compute per active-job policy → unreference resources | PVC retained; explicit data deletion separate |

Database/backup credential materialization must not depend on database readiness.
PVC WaitForFirstConsumer requires apply consumer before waiting for binding;
resource provisioning acknowledges creation, final readiness verifies consumer.

Before implementation, update existing sequence/VOPC and shared class/schema/ERD
artifacts for accepted operations; this table is design-review collaboration,
not a claim that diagrams/migrations were finalized. Exact API/state decisions
are listed in the [handoff](../../implementation/workload-catalog.md).

Sequence: [source](workload-catalog-sequence.puml), [render](workload-catalog-sequence.png).

Live Job inspection: home row -> scoped HTTP handler -> observability application
service -> applied WorkloadInstance/current generation + inventoried CronJob ->
credential-bound Kubernetes reader. Reader verifies scope and deployment owner,
then joins Job controller owner UID to the verified CronJob. Safe bounded rows
return to the existing Dialog; log navigation reuses UC-09 stored history.
