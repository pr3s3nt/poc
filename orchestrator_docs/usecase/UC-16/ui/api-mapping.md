---
id: UC-16-UI-API-MAPPING
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-13
related: UC-12, UC-16, UC-05, UC-07
---

# UC-16 UI API mapping — T30

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

## Terraform v2 contract extension

Existing UI routes and role/org/version guards remain. Required payload/metadata
extensions follow [module contract](../../../architecture/contracts/terraform-modules.md),
[state contract](../../../architecture/contracts/terraform-state.md) and
[operation contracts](../../../architecture/contracts/operation-contracts.md#terraform-v2-scoped-supersession-adr-014).
These are planned v2 fields, not claims the current HTTP handlers accept them.
Workload params/class stay Score-backed references; PE source/revision/mapping
fields are Definition metadata. AWS onboarding belongs to dedicated concealed
Connection write. Public queries return only scoped safe projections; no internal
state endpoint/token exposed. No API bypass of acceptance UI steps is allowed.
