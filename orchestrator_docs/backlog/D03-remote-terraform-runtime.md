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

I04-02 registry verifies packaged modules and allowlisted HTTPS Git commits,
including transitive pins. The executor accepts a verified artifact but still
uses workspace state and refuses modules with children. Durable/transitive
execution remains I04-03; registry tests are not live module execution proof.
See [registry evidence](../verification/2026-10-13-i04-02-module-registry.md).
IMP-007 remains until executor implementation/validation.
Arbitrary commands, mutable remote modules and custom driver remain outside v2.
