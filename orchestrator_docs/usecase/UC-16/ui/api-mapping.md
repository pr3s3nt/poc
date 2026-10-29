---
id: UC-16-UI-API-MAPPING
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-24
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

## Metadata class và source — I04-07

Editor đọc GET `/api/v1/resource-types` đã authenticate theo Organization của
session. Response `resourceTypes` giữ key/inputs/outputs và bổ sung `classes`
(array string, sorted/unique, rỗng là `[]`) như read projection, không phải
field của Type registration/persistence. Class choices lấy từ verified registry
metadata và class khai báo trong scoped catalog Definitions; không đoán từ tên
module, credential, target mặc định hoặc source bên ngoài. Metadata chỉ hướng
dẫn authoring, không bảo đảm một class match context cụ thể; Preview/Deploy
vẫn kiểm Type/class/profile/criteria và shared-ID consistency độc lập.

Developer chỉ đọc Type schema/class metadata này, không dùng endpoint privileged
`resource-definition-capabilities` để chọn module/provider. Missing session 401,
không org override; metadata failure safe error, không fake empty fallback.
Loading/error/unavailable khóa thao tác cần metadata, giữ edits/imported class,
cho retry GET tường minh. `default` giữ Score semantics, không map sang in-cluster
hoặc cloud-managed. Custom/imported class được giữ nguyên và matching vẫn kiểm;
không tự đổi class/target. Không resource/provider/secret-store/CLI calls hoặc
writes trong query. POST Type vẫn chỉ nhận contract schema đã định nghĩa,
không nhận computed `classes`.


## Unified sources — DEVEX-ENV

Canonical mappings and create-only payload/errors are in
[OC-12/OC-16](../../../architecture/contracts/operation-contracts.md).
Reuse GET configuration, GET resource-types, GET workloads, existing workload
PUT/parse and UC-01 Secret Store selection endpoints. Add POST configuration/keys
for create-only; do not call existing upsert PUT from Tạo và chọn.

Source enum is frontend state only. Send ordinary strings or existing references
in Score; never send secret creation input in workload Save/parse/Preview. Dialog
success consumes the authoritative Configuration/version, selects its new key,
and retains parent draft version unless reload shows a real concurrent change.
Changing config invalidates old Preview even if workload Save has not happened.
Creation timeout cannot be treated as absence: GET/review before new mutation.
Scope/request generation and stable row identity prevent late success selecting
another container/row; clear secret input on cancel/success/scope change.

## ENV-UX — 2026-10-27

Direct-entry rows and the combined reference picker reuse all existing HTTP
contracts and Score projections. No new request field, backend enum, persistence
entity or Secret scope. Literal values stay ordinary workload strings; checkbox
Secret delegates to UC-12 create-only, then binds the created app/environment
key. Local row-to-row references are not produced. Clearing/changing sources
drops inactive payload before Score serialization.


## Workload catalog / scheduled Jobs — ADR-025

Scoped GET workload-templates and existing workload Save carry the immutable
execution pin/typed intent per [WC-C08](../../../architecture/contracts/workload-catalog.md#wc-c08--concrete-git-owned-catalog-contract).
Live GET workloads/{workload}/jobs uses applied inventory/target and returns safe,
bounded Job rows per [WC-C09](../../../architecture/contracts/workload-catalog.md#wc-c09--live-job-inspection).
Missing session/scope uses existing 401/404 policy; runtime unavailable/forbidden
is explicit and never successful empty history. No deploy/Job mutation on GET.
