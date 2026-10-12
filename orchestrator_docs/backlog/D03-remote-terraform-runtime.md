---
id: D03
artifact: backlog-item
status: current
last_reviewed: 2026-10-13
related: [ADR-014, UC-03, UC-08]
---

# D03 — Registered immutable Terraform modules: selected for v2

Decision resolved by [ADR-014](../architecture/decisions/ADR-014-terraform-v2-lifecycle.md)
and [module contract](../architecture/contracts/terraform-modules.md): Admin
allowlisted sources, immutable Git commit/artifact digest, verified cache and
transitive content, pinned providers, private credentials and strict input/output
mapping. PE registers modules; workload users select need/class, never source.

Runtime currently executes embedded VPC/EKS/Aurora only. Inspector source support
is not live module execution proof; IMP-007 remains until implementation/validation.
Arbitrary commands, mutable remote modules and custom driver remain outside v2.
