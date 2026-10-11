---
id: UC-16-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-12
related: UC-16
---

# UC-16 UI states

| Screen | State | UI behavior |
|---|---|---|
| Application home | Loading/empty | Preserve selected Environment; show list skeleton or `No workloads` and Add action. |
| Application home | Pending | Show added/edited/deleted labels and Preview changes; no runtime-success claim. |
| Application home | API error | Show retry without replacing data with mock workload rows. |
| Form | Loading/new/edit | Show Environment, empty form for Add or existing configuration for Edit. |
| Form | Missing/invalid reference | Identify variable/secret row and source; disable Save until corrected. |
| Form | Missing UC-12 key | Explain the missing key in this Environment; link to Application Variables & Secrets. |
| Form | Invalid Service target | Explain missing target, missing Service/port or target pending deletion; retain other form fields. |
| Form/Preview | Invalid public path | Form requires a valid unique path and declared Service port; Preview rejects duplicate paths across workloads while allowing `/` and `/api`. |
| Preview | No effective change | A draft matching the current module has no affected workload and Deploy is disabled; changed referenced UC-12 key revision remains an update. |
| Preview | Route reconciliation failed | Show route-only pending state and enable `Retry public routes`; no workload restart on retry. |
| Form | Missing/invalid resource input | Identify dependency and required input; retain entered values and prevent invalid save. |
| Form | Secret source | Show name/output classification only; never echo secret value. |
| Import | Parsing/invalid | Show file progress or field-level error; direct literal binding is rejected. |
| Save | Submitting/error/success | Prevent double submit; preserve form on error; return to Application home with pending label on success. |
| Delete | Confirm/cancel/success | Require confirmation; cancel does nothing; success shows pending deletion and Undo. |

## Resource input errors

Save and Score import reject invalid resource params with safe field paths under
UC-16 BR-14. Preserve unsaved editor/import content; do not report a saved draft
or advance displayed draft version after rejection. Backend validation also
covers imported dependencies unused by container bindings.

## Application-key checklist states

| State | Behavior |
|---|---|
| New container | All keys unchecked; show Variables and Secrets separately for the selected Environment. |
| Selected | Same-name binding by default; optional alias input, no value input. |
| Edited/imported | Restore selected keys and all existing aliases, including multiple names for one key. |
| Unselected | Remove only that key's mappings from this container; leave other bindings intact. |
| No keys | Show Settings guidance per group; no automatic selection of future keys. |
| Missing selected key | Keep unavailable reference/name visible; block Save until removed/corrected. |
| Loading/catalog error | Loading or safe error + Retry; do not save using an empty replacement catalog. |
| Duplicate/empty name | Explain container and conflicting name; preserve edits and block Save without submitting. |
| Saving/stale | Freeze checklist, alias and Other sources controls during Save; retain selection and alias edits on failure/conflict. |
| Scope changed | Load only destination Application/Environment/workload state; ignore old import/catalog/save/reload responses. Same-scope retry/reload retains local selection and aliases. |

## T08 — Trạng thái nâng cao và cổng công khai

| State | Behavior |
|---|---|
| Basic/new | Phần nâng cao đóng; tóm tắt request `10m`/`32Mi`, limit chưa đặt, class `default` và target port fallback; không ghi defaults vào capacity fields. |
| Advanced edited/imported | Overrides giữ nguyên khi mở/đóng hoặc sửa trường cơ bản; tóm tắt phản ánh giá trị đang có. |
| Add public route | Chỉ chọn cổng khi thao tác thêm và có đúng một cổng hợp lệ; không tự bật route khi thêm/sửa cổng. |
| Public target invalid | Giữ lựa chọn đích cũ khi port đổi/xóa/không hợp lệ; hiện lỗi tiếng Việt và chặn Save, không chọn đích thay thế. |
| Import/save validation | Giữ nội dung và aliases; params number/bool serialize đúng kiểu; không advance draft sau failure. |
| BUSY/stale/scope | Giữ guards hiện hành và pending edits; không tự gửi lại Save hoặc dùng response của scope cũ. |
| Key catalog | Không tự chọn keys/secrets mới; thiếu key/loading/error khác empty và không lộ giá trị. |

Tất cả nhãn và validation frontend trên editor/key picker dùng tiếng Việt;
backend error envelope/mapping theo contract lỗi T20. API/Score semantics không đổi.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.

## T25 — Field route đã loại bỏ

Import hoặc draft có `service.publicPort` (kể cả rỗng/null hoặc cùng publicRoutes)
bị từ chối với thông báo Việt an toàn; không chuyển sang form/lưu thành công
và không tạo route mặc định. Giữ nội dung import/edits để sửa. Routes hợp lệ
round-trip đầy đủ; đổi/xóa cổng vẫn chặn lưu theo T08, không retarget.
