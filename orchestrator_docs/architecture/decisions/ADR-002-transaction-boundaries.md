---
id: ADR-002
artifact: architecture-decision
status: current
last_reviewed: 2026-10-12
---

# ADR-002 — External Calls Outside Database Transactions

Status: Accepted — 2026-09-20.

## Decision

Never hold a PostgreSQL transaction while Terraform, cloud or Kubernetes calls run. Persist an immutable plan first, persist each resource result in a short transaction, then use one optimistic final transaction to commit Environment current-set pointer and Deployment success.

## Consequences

- No false distributed-transaction guarantee.
- Deployment/Resource progress is observable through UC-09.
- Executor operations require logical idempotency/state references.
- Retry/resume policy remains future scope, but persisted state is sufficient to design it later.

## Environment transition extension

[ADR-012](ADR-012-environment-stores-and-transitions.md) adds atomic persisted
operation admission and explicit stage recovery/compensation/cleanup. External
calls still remain outside transactions; no distributed atomicity is promised.

## Atomic desired rename extension — T30

UC-12 rename opt-in commits immutable configuration revision and all affected
UC-16 desired drafts in one short metadata transaction under expected config,
Environment and draft versions plus BUSY/current-set guards. Prepare lossless
reconstruction and reference rewrites before commit; no Secret/provider read,
external execution or runtime mutation belongs to this command. Any failure
rolls back the complete metadata change; Preview/Deploy remain separate.
