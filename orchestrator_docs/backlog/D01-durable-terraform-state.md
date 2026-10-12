---
id: D01
artifact: backlog-item
status: current
last_reviewed: 2026-10-13
related: [ADR-014, D12, D13, D14]
---

# D01 — Durable Terraform state: selected for v2, implementation pending

Decision resolved by [ADR-014](../architecture/decisions/ADR-014-terraform-v2-lifecycle.md):
Orchestrator HTTP backend, encrypted PostgreSQL state, stable resource identity/
physical names and two-level locking. Required protocol is in the
[state contract](../architecture/contracts/terraform-state.md).

Code baseline still uses temporary run directory/state and run-derived physical
names; do not claim this gap closed until implementation and real durable-state
checks pass. No migration/reconciliation of v1 resources is required. Automatic
recovery/orphan reconciliation and platform backup are separate deferred D12–D14.
