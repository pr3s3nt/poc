---
id: ADR-040
artifact: architecture-decision
status: current
last_reviewed: 2026-11-07
related: [ADR-014, ADR-027, ADR-030, UC-06, UC-08, UC-09]
---

# ADR-040 — Bounded deployment retry and user-selected Environment rollback

Status: Accepted — 2026-11-07, theo yêu cầu người dùng và issue #49.

## Context

UC-06 deploys multiple workload changes sequentially. A successful workload may
already be committed when a later one fails, so a failed Environment deployment
can leave a partial state. Per-workload Deployment history and Terraform retries
do not by themselves identify one coherent Environment-wide release to restore.

## Decisions

1. A Console Deploy batch is one Environment release. Persist an immutable base
   Environment snapshot and its execution pins before the first external side
   effect. Mark the release successful only after every requested workload and
   required delivery/readiness step succeeds. Keep per-workload Deployment rows
   for detailed progress and diagnostics.
2. Retry only a classified transient operation with a confirmed stopped process
   and safe, acknowledged outcome. Allow at most three attempts total per
   resource operation (initial attempt plus two retries), under the existing
   deadline and Environment claim. Authentication, validation, permission,
   cancellation and unknown outcomes stop without replay. Never retry an
   operation whose external effect is not proven safe.
3. After a release fails, do not automatically roll it back. The user can inspect
   safe failure diagnostics and choose to deploy corrected desired state or
   request rollback for the entire Environment.
4. Rollback is a new, explicitly requested Environment operation. Its target is
   the last successful Environment release snapshot. If no successful release
   exists, the target is the initial empty baseline. The operation is successful
   only after the full target state is observed and committed.
5. Delete only resources proven owned by this Environment/release. Preserve
   shared and PREEXISTING resources. If ownership or external outcome is unknown,
   fail closed, keep the Environment held for recovery, and report partial or
   failed rollback accurately.
6. Snapshots pin the exact non-secret desired state, Definition/catalog/template
   revisions, delivery binding and target identities needed to reproduce the
   target. Never snapshot resolved credentials or secret values. Existing
   historical deployments without a release snapshot are not inferred as a
   successful Environment release.

## Consequences

- Any persistence schema extension must use a versioned PostgreSQL upgrade under
  [ADR-027](ADR-027-postgres-upgrades.md). Existing immutable Deployment Sets and
  Environment operation pins must be reused where their guarantees suffice.
- The Console adds explicit redeploy and rollback actions after failure, with
  clear scope, progress and confirmation for potentially destructive rollback.
- An older Environment with only per-workload history has no proven successful
  whole-Environment release baseline. For its first recorded release, establish
  the empty baseline only after an ownership inventory proves which existing
  resources belong to the Environment; otherwise fail closed and do not delete.
- Rollback does not repair an interrupted writer. Recovery must first prove the
  old operation stopped and safely release its claim.

## Scope

This decision does not add automatic rollback, arbitrary historical version
selection, Terraform orphan import, or recovery from unknown external effects.
It does not change resource ownership or deletion rules.
