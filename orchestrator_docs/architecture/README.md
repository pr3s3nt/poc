---
id: ARCHITECTURE-INDEX
artifact: architecture-index
status: current
last_reviewed: 2026-10-13
---

# Shared Architecture Baseline

Shared design này hợp nhất UC-00 đến UC-09 sau realization.

## Artifact map

- [Domain and persistence classification](domain/domain-objects.md)
- [Consolidated design classes](domain/design-class-diagram.puml)
- [Component boundaries](components/component-diagram.puml)
- [Database schema](database/schema.md) và [ERD](database/erd.puml)
- [Connection credentials and execution identity](connection-credentials.md)
- [Operation contracts](contracts/operation-contracts.md)
- [Workload rendering contract](contracts/workload-rendering.md) — Definition-selected
  score-k8s rendering for internal-k8s.
- [Shared Web Console UI](ui/README.md)
- [State machines](state-machines/README.md)
- [Architecture decisions](decisions/README.md)
- [Implementation index](../implementation/README.md) và [package layout](../implementation/package-layout.md)

## Dependency rule

```text
delivery -> application -> domain
application -> ports
adapters -> ports + domain contracts
domain -> no delivery/application/adapter package
```

Planning là deterministic core, provisioning thực thi resource nodes, workload deployment chỉ chạy sau khi resource outputs đã sẵn sàng.

Orchestrator Web Console là React + TypeScript application riêng, chỉ giao tiếp với Go backend qua same-origin JSON API. Nó khác với acceptance frontend workload mà orchestrator triển khai lên Kubernetes.

Internal cluster binding follows [ADR-013](decisions/ADR-013-implicit-existing-cluster.md); cloud infrastructure and application resources retain Definition matching.

## Terraform v2 accepted design

- [ADR-014](decisions/ADR-014-terraform-v2-lifecycle.md)
- [Module/provider contract](contracts/terraform-modules.md)
- [Durable state/locking/cleanup contract](contracts/terraform-state.md)
- [V2 acceptance scope](../operations/terraform-v2-acceptance.md)

These are required v2 design; current implementation remains tracked separately.
