---
id: I06-10
artifact: iteration-plan
status: deferred
last_reviewed: 2026-09-22
related: UC-01, UC-02, UC-03, UC-04, IMP-002
---

# I06-10 — Complete UC-01..04 management flows

## Objective

Thay seed-only management baseline bằng application services, API và Web Console
cho Application/Environment, Resource Type, Resource Definition và Connection
happy paths đã thiết kế.

## In scope

- UC-01 create Application/Environment với immutable Execution Profile.
- UC-02 register Resource Type schemas.
- UC-03 register valid Resource Definition/criteria/contracts.
- UC-04 register/verify AWS or Kubernetes Connection và persist only secret refs.
- Management API/UI, validation, repository tests và planner integration.

## Out of scope

- Update/delete/versioning, fine-grained RBAC, audit và credential rotation.
- Humanitec public boundary D06 hoặc remote Terraform execution D03.
- PostgreSQL adapter IMP-001; repository ports/current store may be used until
  M03.

## Exit criteria

- Bốn management happy paths chạy qua API/UI và dữ liệu mới dùng được bởi
  planner mà không cần restart/seed edit.
- Invalid schemas/criteria/contracts/connections bị từ chối trước persistence.
- Secret value không vào database/state/API/logs.
- Backend/frontend validations pass và IMP-002 được xóa.

## First next action

Lập gap matrix UC-01..04 `MS-nn` → application service/repository/API/UI/test,
sau đó implement theo dependency order UC-01 → UC-02 → UC-03 → UC-04.

## Outcome

Chưa thực hiện.
