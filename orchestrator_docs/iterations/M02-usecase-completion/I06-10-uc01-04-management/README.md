---
id: I06-10
artifact: iteration-plan
status: deferred
last_reviewed: 2026-09-30
related: UC-02, UC-03, UC-04, IMP-002
---

# I06-10 — Complete remaining UC-02..04 management flows

## Objective

Hoàn thiện application services, API và Web Console còn thiếu cho Resource
Type, Resource Definition và Connection lifecycle đã thiết kế.

## In scope

- UC-02 register Resource Type schemas.
- UC-03 register valid Resource Definition/criteria/contracts.
- UC-04 register/verify AWS or Kubernetes Connection và persist only secret refs.
- Management API/UI, validation, repository tests và planner integration.

## Out of scope

- Update/delete/versioning, fine-grained RBAC, audit và credential rotation.
- Humanitec public boundary D06 hoặc remote Terraform execution D03.
- Production-grade authorization, credential lifecycle và remote source
  execution.

## Exit criteria

- Ba management happy paths chạy qua API/UI và dữ liệu mới dùng được bởi
  planner mà không cần restart/seed edit.
- Invalid schemas/criteria/contracts/connections bị từ chối trước persistence.
- Secret value không vào database/state/API/logs.
- Backend/frontend validations pass và IMP-002 được xóa.

## First next action

Lập gap matrix UC-02..04 `MS-nn` → application service/repository/API/UI/test,
sau đó hoàn thiện theo dependency order UC-02 → UC-03 → UC-04. UC-01 đã hoàn
thành ở I00-00 và không được mở lại trong iteration này.

## Outcome

Chưa thực hiện.
