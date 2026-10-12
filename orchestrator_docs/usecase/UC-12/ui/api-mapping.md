---
id: UC-12-UI-API-MAPPING
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-12
related: UC-12, UC-16, UC-05, UC-07
---

# UC-12 UI API mapping — T30

[OC-12 T30](../../../architecture/contracts/operation-contracts.md#oc-12-atomic-desired-reference-rename--t30--cfg-01)
sở hữu payload/CAS/error contract. Settings GET configuration lấy config/env/draft
versions cùng snapshot. Rename PATCH gửi `newName`, `version`; checkbox opt-in
thêm `updateReferences: true`, `environmentVersion`, `draftVersion`. Checkbox
mặc định bỏ chọn; false/omitted giữ rename-only.

Workload editor tiếp tục đọc desired Score qua UC-16 API hiện hành; rename không
thêm endpoint deploy hoặc copy giá trị. Save editor với draft version cũ trả
STALE_VERSION và giữ edits. Xem trước thay đổi trên trang ứng dụng gọi pending
Preview hiện hành; token cũ sau rename không dùng được.

BUSY/stale/duplicate và REFERENCE_REWRITE_UNSAFE dùng thông báo Việt theo shared
error contract. Failed mutation không mất dialog/aliases; reload sau commit
thất bại báo tải lại, không tự replay rename. Scope change invalidate replies.

Sau rename ở `production`, CTA Xem trước mở trang ứng dụng đúng `production`
qua query `environment=production`; route không có query vẫn mặc định staging.
Không chuyển người dùng sang scope khác để Preview/Deploy.
