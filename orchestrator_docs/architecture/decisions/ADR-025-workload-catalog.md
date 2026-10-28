---
id: ADR-025
artifact: architecture-decision
status: current
last_reviewed: 2026-10-28
related: [ADR-021, ADR-024]
---

# ADR-025 — Versioned workload templates, runtime và persistent storage

Người dùng chấp nhận hướng thiết kế và yêu cầu implementation/E2E/merge main
ngày 2026-10-28. Bổ sung ADR-024 cho catalog workload, primary CronJob và storage.
Không triển khai/adopt/migration namespace Passbolt cũ.

## Decisions

- Workload templates có immutable version/schema/defaults/capabilities; PE/Admin
  quản lý, Developer chọn từ Dialog Thêm Workload trên application home hiện hành.
  Role/category (frontend/backend/worker/stateful/scheduled) khác runtime kind.
- Giữ matching Resource Definition và pinned Git bundle; catalog không bypass
  account/target/module selection. Legacy workloads giữ behavior cũ.
- `workloadExecution` version 1 là extension typed trên desired Score và
  Deployment Set module spec. Template pin (ID/version/hash), runtime, job policy,
  storage mounts và service policy được persist và hash cùng desired intent;
  public parser, reconstruction và render guard phải cùng hiểu extension.
- PVC là Terraform-managed resource riêng; renderer dùng claimName output,
  không ghi claim đó vào workload object inventory. Chỉ một owner cho mỗi object.
  Single claim/stateful phiên bản đầu giới hạn một replica.
- Remove workload giữ PVC; chặn Environment/namespace deletion còn retained data.
  Explicit resource deletion giữ confirmation, original state/module/UID guards.
  Delayed PVC binding không được gây deadlock trước consumer apply.
- CronJob là primary controller, không có Deployment giả. Schedule applied không
  đồng nghĩa Job thành công. Theo dõi descendants bằng owner UID; delete chặn
  khi còn active Jobs. Suspend/update lịch không tự cancel Job. V1 chưa có cancel.
- Secret env/file/resource references dùng platform delivery; nested Pod path
  và revision lifecycle được bảo vệ. Không đưa secret bytes vào Terraform inputs.
- Passbolt là acceptance mới có MySQL resource, image storage và hai scheduled
  workloads độc lập; backup S3-compatible. Không hardcode hai job vào MySQL.

Contract chi tiết: [workload catalog](../contracts/workload-catalog.md).
Implementation gates và phần còn phải thiết kế:
[handoff](../../implementation/workload-catalog.md). Quyết định này không xác nhận
code đã implement hoặc E2E pass. Publish API/schema chi tiết phải được ghi trước
khi implement mutation tương ứng. Không reset database staging để đổi schema.
