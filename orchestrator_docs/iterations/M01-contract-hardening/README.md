---
id: M01
artifact: milestone-plan
status: current
last_reviewed: 2026-09-22
related: IMP-008, IMP-009, IMP-010
---

# M01 — Contract hardening

## Objective

Đóng ba contract gap làm giảm độ tin cậy của planner/happy path trước khi mở
rộng use case: conformance criteria semantics, Deployment Delta Snapshot và
Score container resources.

## Iteration order

1. [I06-05 — IMP-010](I06-05-imp010-conformance-catalog/README.md): làm
   conformance adapter đáng tin.
2. [I06-06 — IMP-008](I06-06-imp008-delta-snapshot/README.md): thay Delta phẳng
   bằng typed immutable Snapshot.
3. [I06-07 — IMP-009](I06-07-imp009-container-resources/README.md): bảo toàn
   container CPU/memory đến Kubernetes renderer.

## Milestone exit criteria

- IMP-008/009/010 được xóa vì code và tests đã khớp canonical design.
- 33 planner fixtures vẫn pass và các vùng fixture không bao phủ có product
  contract tests riêng.
- Go test/build/vet và documentation validation pass.
- Internal happy-path regression được verify trên kind sau I06-07; AWS không
  cần chạy vì milestone không thay cloud execution contract.
- `CURRENT_STATE.md`, compatibility matrix, code map, traceability và dated
  verification records phản ánh kết quả thực tế.

## Out of scope

- D05 standalone/mutable Delta lifecycle, async hoặc incremental deployment.
- UC-05 Preview API/UI, UC-07 update/remove flow và UC-01..04 management UI.
- PostgreSQL adapter, durable Terraform state và remote Terraform execution.
