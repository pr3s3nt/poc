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

## Custom workload expansion — GW-06 / WC-16..19

The editor owns local role/order/volume/mount controls for every workload, not
only selected presets. Save/parse validates the same domain intent and scoped
Secret refs; existing desired JSON persists it under draft CAS. The planner
checks the matched immutable source capabilities before executing dependencies.
The renderer builds ordered init/sidecar specs, per-container mounts and owned
nonsecret ConfigMaps; semantic guards verify all declarations and references.
Existing applied snapshots, inventory and cleanup retain their current ownership
boundaries. No separate volume/sidecar API, SQL column or runtime plugin is added.

HostPath (WC-21) follows the existing named-volume flow: Editor → node path/type
validation → draft CAS → selected source host-path-volumes-v1 capability gate →
render path/type + per-container mode → semantic source/mount guard → safe Preview.
It adds no Terraform resource, filesystem operation, scheduling or cleanup step.

WC-22 places the source/mount picker in each container. Create local source +
mount in one local update; reuse points to the same workload source. Source
editing remains shared and removal detaches only one mount. Existing storage/UC-12
creation dialogs receive the invoking container and preserve scoped reply guards.
Serialization and runtime pipeline remain unchanged; no API/schema change.

WC-23 projects the existing workload/configuration catalogs into reusable source
choices before opening creation controls. Shared PVC reuse resolves the existing
validated sharedResource choice, preserving identity/params/reuse guard, then
adds dependency + targeted mount in one local update. Runtime/persistence unchanged.
