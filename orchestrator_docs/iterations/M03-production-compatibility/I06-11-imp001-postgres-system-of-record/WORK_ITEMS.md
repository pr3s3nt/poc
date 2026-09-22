---
id: I06-11-WORK
artifact: iteration-work-items
status: deferred
last_reviewed: 2026-09-22
related: I06-11
---

# I06-11 work items

## Implementation order

1. Resolve D08 and revalidate schema/ERD against repository ports.
2. Select migration/tooling/config boundary and add disposable-DB test harness.
3. Implement migrations in dependency order with constraints/indexes.
4. Implement repositories and UnitOfWork transaction boundaries.
5. Add optimistic concurrency, restart/durability and failure-path tests.
6. Add operational configuration/backup/recovery guidance and verification.

## Handoff checklist

- [ ] No secret or Terraform state stored as plaintext/logical JSON state.
- [ ] External calls occur outside DB transactions.
- [ ] Migration up/down or forward-recovery policy is tested/documented.
- [ ] In-memory store is no longer production authoritative state.
- [ ] IMP-001/D04/D08 status matches actual outcome.
