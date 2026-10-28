---
id: UC-18-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-28
related: [UC-18-SPEC, ADR-025]
---

# UC-18 realization — Xóa Application

Diagrams: [sequence](sequence.puml), [VOPC](vopc.puml).

## Participants

| Participant | Kind | Responsibility |
|---|---|---|
| `ApplicationDeletionController` | boundary | `deletion-preview`, `deletions`, operation read; session scope and role |
| `ApplicationDeletionPlanner` | control | Read-only consistent snapshot of both Environments; scope, kept items, blockers, token |
| `ApplicationDeletionService` | control | Admission, stage driver per Environment, Retry, finalization |
| `EnvironmentOperationManager` | control | Existing claim/owner/fence; claims both Environments in one transaction |
| `WorkloadRemoval` | control | Existing UC-07 VAR-02 path with an empty candidate Set |
| `ResourceCleanupService` | control | Existing DEL executor reused with a whole-Environment confirmed scope |
| `SecretPurger` | control | Deletes exact Vault refs through the Secret Store port |
| `TelemetryCollector` | control | Stops sources of a `DELETING` Application |
| `Application` | entity | Lifecycle `ACTIVE`/`DELETING`/`DELETED`, deletion metadata |
| `EnvironmentOperation` | entity | Kind `APPLICATION_DELETION`, stage, confirmed scope, results |

## Collaboration

1. **Preview (MS-01..02).** Controller checks session Organization and role,
   then calls the Planner. The Planner reads in one snapshot: Application and
   Environment versions, current Set, drafts, desired configuration revision,
   `workload_instances` inventories, `terraform_resource_instances` of every
   generation with dependencies and latest attempt ownership, Vault refs from
   configuration revisions/Sets/output metadata. It classifies items as
   *delete*, *keep* (PREEXISTING, platform, catalog) or *blocker*, and signs a
   token over the pinned versions and the delete set hash.
2. **Admission (MS-04).** Service validates token, exact name and role, reloads
   the preview body and compares the token digest; a change of any pinned
   version, workload, instance status or active operation makes it stale. One
   compare-and-set on the Application version writes `status = DELETING` and
   the deletion progress (operation ID, actor, request hash, per-Environment
   stage). Any difference returns 409 with no side effects. Each later stage
   claims its Environment through the existing operation it uses (DEPLOY for
   workload removal, RESOURCE_CLEANUP for destruction), so no second claim
   kind is needed and the existing owner/fence/recovery rules apply as is.
3. **Per-Environment stages (MS-05..08).** The Service drives the Environments
   one after the other in key order (production, staging); each stage runs under
   the owner/fence of the operation it claims. Each stage boundary is persisted
   before the next external call.
   - `REMOVING_WORKLOADS` calls `WorkloadRemoval` with an empty candidate Set,
     which deletes inventory objects by UID, VSO objects and the Environment
     public route, and records a REMOVE Deployment.
   - `DESTROYING_RESOURCES` admits, per target generation, the whole owned set
     as one RESOURCE_CLEANUP operation through `AdmitEnvironmentCleanup`
     (`authorization_mode = USER_CONFIRMED`, actor = deleting user, request hash
     bound to the deletion, generation and attempt). The selection is computed
     and re-checked in the claim transaction by the backend; the engine destroys
     in reverse persisted dependency order and proves absence.
   - `VERIFYING` reuses provider absence verification and inventory absence
     checks; instances become `DESTROYED`.
   - `PURGING_SECRETS` deletes, through each owning store, every Secret value
     ref of every configuration revision and the delivery bundle path of every
     Deployment of the Environment; an already absent path counts as deleted,
     an unreachable store fails the stage.
4. **Finalization (MS-09).** When both Environments are `DONE`, one
   compare-and-set sets `DELETED` with the finished progress. Pending drafts
   were already consumed by the removal Deploy. Partial unique indexes then
   release name/subdomain.
5. **Failure/Retry (VAR-02/03).** A failed stage leaves the Application
   `DELETING` with the Environment's failure code and the per-resource remaining
   IDs of the cleanup operation. A process restart shows the run as
   `INTERRUPTED` (no in-process runner). Retry requires a fresh preview of the
   remainder and the typed name; it keeps the operation ID, increments the
   attempt and continues from the persisted stage.

## Guards reused unchanged

Owner/fence on every write, state locks under the Environment operation,
original target restoration (ADR-016), reverse-graph destroy with shared-consumer
protection (UC-08 V06-01), VSO exact-name removal (UC-07 VSO-01), safe errors
without secret values.

## Persistence impact

- `applications`: `status` (`ACTIVE`/`DELETING`/`DELETED`) and `deletion`
  JSON object (operation ID, request hash, actor, status, attempt, timestamps,
  per-Environment stage/status/failure code, removed workloads, cleanup
  operation IDs, per-resource outcomes, purged secret count). Unique
  `(organization_id, lower(name))` and subdomain are partial indexes
  `WHERE status <> 'DELETED'`. See the [schema](../../architecture/database/schema.md#application-lifecycle--adr-025).
- No new `environment_operations` kind and no new cleanup `authorization_mode`.
- The Application list omits `DELETED`; scoped reads answer 404; the deletion
  progress stays readable by operation ID to the actor's Organization.
- The HTTP lifecycle guard answers 409 `APPLICATION_DELETING` to every mutation
  under the Application while `DELETING`; reads continue.

## ADR-027 collaboration extension

ApplicationDeletionService invokes original applied delivery withdrawal and guarded workload/control-metadata cleanup before Terraform namespace removal. Unknown publication/transition/withdrawal outcomes block downstream cleanup.

Canonical shared collaboration: [delivery sequence](../../architecture/components/delivery-reconciliation-sequence.puml),
[participants](../../architecture/components/delivery-reconciliation-vopc.puml),
[infrastructure sequence](../../architecture/components/infrastructure-drift-sequence.puml).
These supplement the existing direct-path diagrams for this new design slice;
[CDR contract](../../architecture/contracts/delivery-reconciliation.md) owns port/API semantics.
