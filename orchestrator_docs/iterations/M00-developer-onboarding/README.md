---
id: M00-A
artifact: milestone-plan
status: historical
last_reviewed: 2026-09-30
related: UC-00, UC-01, IMP-002
---

# M00-a — Developer onboarding

## Objective

Deliver một vertical slice để Developer dùng thử Web Console: đăng nhập bằng
fixed test account, tạo Application self-service và thấy `staging`/
`production` context. Thiết kế UI cho UC-00 và UC-01 là phần đầu của cùng
milestone; implementation backend/frontend theo ngay sau thiết kế được duyệt.

## Iteration order

1. [I00-00 — UC-00 and UC-01 developer onboarding](I00-00-uc00-uc01-developer-onboarding/README.md). Completed 2026-09-30.

## Milestone exit criteria

- UI flow, screen states và API contract của UC-00/UC-01 map đúng canonical
  specification trước khi implementation.
- Fixed test account đăng nhập được qua Web Console; session context xác định
  User, Organization và role ở backend boundary.
- Developer tạo Application từ Name/Subdomain; hệ thống tạo ID, `staging` và
  `production` atomically mà không provision runtime infrastructure.
- Web Console có sign-in, Application empty/list/create/success/error states;
  không còn bắt user bắt đầu tại form deploy kỹ thuật.
- UC-00 targeted design-gate review, backend/frontend tests/build và docs
  validation pass; IMP-012 được xóa, IMP-002 được cập nhật cho phần UC-01 còn
  lại hoặc xóa nếu UC-01 hoàn tất.

## Out of scope

- Account management, self-registration, RBAC chi tiết và external IdP.
- Workload editor, Score persistence, Preview UC-05 và thay đổi execution của
  UC-06/UC-08.
- Deployment history/filter của UC-09, rollback, audit hoặc production-grade
  account/RBAC lifecycle.

## Outcome

M00-a hoàn thành 2026-09-30. I00-00 đã pass toàn bộ exit criteria; IMP-012 được
đóng và IMP-002 được thu hẹp còn phần chưa hoàn tất của UC-02..UC-04. Xem
[dated local onboarding evidence](../../verification/2026-09-30-uc00-uc01-local-onboarding.md).
