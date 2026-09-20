# Orchestrator Documentation Index

## Use cases

- [Use Case Model và danh sách UC-01 đến UC-09](usecase/README.md)
- Mỗi specification nằm tại `usecase/UC-xx/specification.md`.

## Shared design

Các artifact realization/shared design đã hoàn thành:

- [Architecture baseline](architecture/README.md).
- [Consolidated design class diagram](architecture/domain/design-class-diagram.puml) và [domain/persistence classification](architecture/domain/domain-objects.md).
- [Component diagram](architecture/components/component-diagram.puml).
- [Database schema](architecture/database/schema.md) và [ERD](architecture/database/erd.puml).
- [System operation contracts](architecture/contracts/operation-contracts.md).
- [State machines](architecture/state-machines/README.md).
- [Architecture decisions](architecture/decisions/README.md).
- [Traceability matrix](traceability/matrix.md) và [design gate](traceability/design-gate.md).
- [UC-06 planner reference analysis](implementation/uc06-planner-reference.md).
- [Planned backend/frontend package layout](implementation/package-layout.md).

## Canonical rule

Specification là nguồn yêu cầu chuẩn. Reference code, realization và shared design phải giải thích cách thực hiện specification, không được âm thầm thay đổi nó.

Mọi diagram dùng PlantUML; `.puml` là source chuẩn, `.png` là bản render để review.
