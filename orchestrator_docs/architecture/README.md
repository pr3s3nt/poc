---
id: ARCHITECTURE-INDEX
artifact: architecture-index
status: current
last_reviewed: 2026-09-21
---

# Shared Architecture Baseline

Shared design này hợp nhất UC-01 đến UC-09 sau realization.

## Artifact map

- [Domain and persistence classification](domain/domain-objects.md)
- [Consolidated design classes](domain/design-class-diagram.puml)
- [Component boundaries](components/component-diagram.puml)
- [Database schema](database/schema.md) và [ERD](database/erd.puml)
- [Operation contracts](contracts/operation-contracts.md)
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
