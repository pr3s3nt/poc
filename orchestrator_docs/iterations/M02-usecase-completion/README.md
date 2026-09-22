---
id: M02
artifact: milestone-plan
status: current
last_reviewed: 2026-09-22
related: IMP-002, IMP-003, IMP-004, IMP-005
---

# M02 — Use-case completion

## Objective

Hoàn thiện các use case đã có canonical specification/realization nhưng baseline
mới triển khai một phần hoặc còn phụ thuộc seed.

## Iteration order

1. [I06-04 — UC-09 observability](I06-04-uc09-observability/README.md): tiếp tục
   iteration đã được reprioritize sau M01. Current.
2. [I06-08 — UC-05 preview](I06-08-uc05-preview/README.md).
3. [I06-09 — UC-07 update/remove](I06-09-uc07-update-remove/README.md).
4. [I06-10 — UC-01..04 management](I06-10-uc01-04-management/README.md).

## Milestone exit criteria

- IMP-002/003/004/005 được đóng bằng API/UI/tests và traceability tương ứng.
- UC-05 Preview và UC-06 Deploy dùng cùng planning pipeline.
- UC-07 update/remove giữ before/shared/current-set invariants.
- UC-01..04 không còn phụ thuộc seed cho management happy path.
- Backend/frontend validations và relevant kind integration pass; external cloud
  verification chỉ chạy khi behavior cloud thực sự thay đổi.

## Out of scope

- Rollback, retry/resume, RBAC, approval, audit và failure recovery.
- D05 Humanitec external deployment lifecycle.
- Production database/state/remote-source work thuộc M03 hoặc backlog riêng.
