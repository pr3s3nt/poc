---
id: ARCHITECTURE-INDEX
artifact: architecture-index
status: current
last_reviewed: 2026-10-28
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

## Infrastructure accounts (V01)

[ADR-015](decisions/ADR-015-infrastructure-driver-accounts.md) adds the account
catalog while preserving current execution consumers until V02–V03.

V02 Definition authoring/matching replaces profile filtering and Environment AWS
identity per [UC-03](../usecase/UC-03/specification.md#hợp-đồng-v02--áp-dụng-từ-2026-10-17).
Definition edits are version CAS; historical execution/cleanup pins remain.
Actual V02 delivery is tracked in CURRENT_STATE/evidence, not implied by design.

- [Deployment review artifact/progress contract](contracts/deployment-review.md) — ADR-019, UC-05/09 desktop visual review.

## Git-owned manifest policy (accepted design)

- [ADR-021](decisions/ADR-021-git-render-bundles.md).
- [Git bundle source/engine/API contract](contracts/git-render-bundles.md).
- [Implementation handoff](../implementation/git-render-bundles.md) and
  [real acceptance](../operations/git-render-bundles-acceptance.md).

Terraform planning/provisioning and resource lifecycle stay unchanged. The
installed-only renderer remains current code until IMP-034 is implemented.

- [Workload catalog contract — proposed, issue #24](contracts/workload-catalog.md).

## Optional CD and reconciliation

[ADR-030](decisions/ADR-030-delivery-reconciliation.md),
[delivery contract](contracts/delivery-reconciliation.md),
[infrastructure drift](contracts/infrastructure-drift.md),
[implementation route](../implementation/delivery-reconciliation.md).
Environment opt-in, provider-neutral lifecycle, Fleet first; design only.
