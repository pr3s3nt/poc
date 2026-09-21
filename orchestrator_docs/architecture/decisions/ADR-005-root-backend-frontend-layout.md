---
id: ADR-005
artifact: architecture-decision
status: current
last_reviewed: 2026-09-21
---

# ADR-005 — Root backend and frontend source boundaries

## Context

Executable code ban đầu nằm dưới `implementation/`, trong đó Go module, Web
Console, integration tests và acceptance workloads cùng chia một directory.
Tên `implementation` hữu ích trong giai đoạn design-first nhưng tạo thêm một
lớp điều hướng khi code đã trở thành product baseline.

Workspace cũng từng giữ hai ignored reference repositories ở root:
`final_idp/` và `humanitec-planner-challenge-v4/`. Chúng không phải product
dependencies. Planner product conformance chỉ phụ thuộc bản fixed, tracked tại
`orchestrator_reference/humanitec-planner-challenge-v4/`.

`final_idp` được dùng làm reference tổ chức tài liệu/frontend với provenance:

- repository: `https://github.com/pr3s3nt/final_idp.git`;
- branch: `uc03-impl`;
- commit: `e6dc6631ba9db1bc80e2ff56380f50db99d490f9`.

## Decision

1. Go product module nằm tại root `backend/`; module name vẫn là
   `orchestrator`.
2. Orchestrator Web Console nằm tại root `frontend/` và giữ package/build riêng.
3. Acceptance workloads tiếp tục nằm trong Go module tại
   `backend/examples/acceptance-app/`; chúng không phải Web Console.
4. Go tests và kind/AWS verification scripts nằm dưới `backend/test/`.
5. Backend phục vụ production bundle từ sibling `frontend/dist`; runbook chạy
   Go command từ `backend/` và frontend command từ `frontend/`.
6. Xóa hai ignored duplicate/local reference repositories ở root. Giữ
   `orchestrator_reference/` là reference tracked duy nhất cần cho conformance.

## Consequences

- Product source có hai entry point rõ ràng: `backend/` và `frontend/`.
- Mọi build script, runbook, code map và documentation link phải dùng layout
  mới trong cùng change.
- Integration scripts phải resolve frontend bằng path sibling, không dựa vào
  directory cũ.
- Xóa local references không làm mất provenance: `final_idp` có exact Git
  reference ở trên; planner duplicate có bản tracked giống hệt trong
  `orchestrator_reference/`.
- Source-path change không thay product behavior, API contract hoặc Go module
  import path.
