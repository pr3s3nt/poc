---
id: ADR-025
artifact: architecture-decision
status: current
last_reviewed: 2026-10-27
related: [UC-18, UC-07, UC-08, UC-12, ADR-012, ADR-014, ADR-016, ADR-023, ADR-024]
---

# ADR-025 — Application deletion as one confirmed cascade operation

## Context

An Application can be created (UC-01) but never removed. Removing a workload
(UC-07 VAR-02) keeps its infrastructure and data; the separate Xóa tài nguyên
flow (UC-07 V2-03) destroys a confirmed set of UNREFERENCED resources in one
Environment and is restricted to PE/Admin. Emptying an Application with those
flows takes many Preview/Deploy and confirmation rounds per Environment, and
still leaves Vault values, configuration and the Application itself.

## Decision — user decisions 2026-10-27

1. **Cascade scope.** One confirmed operation removes everything the platform
   created for the Application in both Environments: applied workloads and their
   inventory/VSO/route objects, every Orchestrator-owned resource instance of
   every target generation (including VPC/EKS/Aurora/ElastiCache and in-cluster
   PostgreSQL/Redis), Vault values and output secrets referenced by the
   Application, then the Application itself.
2. **Actor.** ADMIN, PLATFORM_ENGINEER and DEVELOPER may delete an Application of
   their Organization. This is a deliberate exception to UC-07 V2-03 limited to
   whole-Application deletion with typed confirmation; per-resource deletion stays
   PE/Admin only.
3. **Tombstone.** The `applications` row stays with status `DELETED`, hidden from
   lists and scoped reads; Deployment history, attempts, terminal Terraform state
   snapshots, cleanup results and audit metadata are retained. Name and subdomain
   are released for reuse once the status is `DELETED`, not while `DELETING`.
4. **Vault purge.** Vault KV Secret values referenced by the Application's
   configuration revisions and the delivery bundle of each of its Deployments
   are deleted by exact reference. Secret Store Connections, mounts, auth
   methods and values not referenced by the Application are kept. Platform
   output secrets of destroyed resources and per-workload delivery policies/
   roles are not removed yet (see [IMP-038](../../implementation/deviations.md#imp-038--application-deletion-2026-10-27)).

## Consequences and invariants

- **Reuse of existing machinery.** Workload removal reuses UC-07 VAR-02/RB-03/
  VSO-01 semantics; resource destruction reuses the RESOURCE_CLEANUP executor,
  original target/module/state pins, reverse dependency order and absence
  verification (UC-07 DEL, UC-08 V06-01). No new provider capability is added.
- **Never deleted:** PREEXISTING clusters and objects, Driver Accounts/
  Connections, Secret Store Connections, Resource Types/Definitions/matching
  criteria, Git render bundles, Terraform module bundles, Organization data,
  other Applications' objects. Platform-owned objects are excluded even when
  they live in the same cluster or namespace.
- **No inference of authority.** Ownership comes from persisted instance/attempt
  ownership and workload inventories, never from labels, name prefixes, run IDs or
  UNREFERENCED status. An instance with unknown ownership, missing state/artifact,
  INTERRUPTED or NEEDS_ATTENTION status blocks admission.
- **Freeze.** From admission, every mutation on the Application and both
  Environments is rejected with 409; reads stay available until `DELETED`.
- **No rollback, no undo.** Failure leaves the Application `DELETING` with persisted
  per-Environment stage and remaining IDs; Retry is idempotent and continues from
  the persisted stage under a new owner/fence. Destroyed data is never restored.
- **Success requires proof.** `DELETED` only after both Environments verified
  absence of every owned physical resource and workload object, and Vault
  deletions were confirmed.
- **Telemetry.** Collection stops at admission; stored telemetry is not shown for
  a deleted Application and expires by the ADR-023 retention.
- **Schema.** `applications` gains `status` and `deletion` columns and partial
  unique indexes. Stages reuse the DEPLOY and RESOURCE_CLEANUP operations, so
  no operation kind or cleanup authorization mode is added. Fresh-baseline
  policy applies: no automatic migration or reset of retained databases.

## Alternatives considered

- **Require empty Environments first** (metadata-only delete): safest and smallest,
  rejected because it leaves a long manual sequence and Vault residue.
- **Hard delete of all rows:** rejected; it removes audit and terminal state the
  Terraform v2 design requires to keep.
- **PE/Admin only:** rejected by user decision; Developer access is compensated by
  preview, typed confirmation and the same backend guards.

## Status

Accepted and implemented on `features/xoa_app`. Behavior is owned by
[UC-18](../../usecase/UC-18/specification.md); implementation status and
evidence are in [CURRENT_STATE](../../CURRENT_STATE.md).
