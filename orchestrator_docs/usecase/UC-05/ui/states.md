---
id: UC-05-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-13
related: UC-05
---

# UC-05 states

| State | Visible behavior |
|---|---|
| Ready / empty input | Instructions and labeled editors; no fabricated preview. |
| Invalid input | Actionable parse/required-field/Score error; input retained. |
| Previewing | Loading label; duplicate submit disabled. |
| Success | All planning artifacts and version context; no execution controls. |
| No change | Explicit no-change notice with valid planning details. |
| API failure | Error with retry available; no stale success shown. |
| Session expired | Existing UC-00 sign-in flow, no cross-scope artifacts. |
| Input/scope changed | Clear old preview; ignore late obsolete responses. |

Keyboard navigation and accessible labels apply to all inputs/actions.

## Trình bày tiếng Việt — T13

Ready hướng dẫn nhập Score và dùng placeholder cho bí mật; Invalid input giữ
nguyên nội dung và báo lỗi bắt buộc/JSON/YAML/Score bằng tiếng Việt, không echo
giá trị gửi lên. Previewing hiển thị **Đang tính kết quả xem trước…** và khóa
submit trùng. Success nêu rõ chỉ đọc, không lưu/triển khai; No change giải thích
không có thay đổi Workload nhưng vẫn có thể có đồ thị môi trường.
Environment chưa cấu hình hướng dẫn mở Cài đặt môi trường để chọn kết nối.
Lỗi không truy cập được hoặc API failure dùng thông báo Việt an toàn và
**Thử lại**; retry chỉ gọi score-preview. Session expired giữ flow UC-00 và role
gate hiện có. Đổi input/Application/Environment xóa kết quả cũ và bỏ phản hồi
đến muộn; không tự lưu draft, tạo Deployment hoặc provision tài nguyên.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.


## Metadata và tiến trình tài nguyên

Module/class metadata đang tải, lỗi hoặc unavailable chặn thao tác chưa kiểm
chứng; retry giữ edits, không thay bằng default hay replay mutation. Known
input/provider/integrity error dùng code/field an toàn. BUSY giữ dữ liệu đang
sửa; stale yêu cầu review/Preview mới. Progress lấy persisted stage/attempt,
không raw CLI/state/secret; interruption không được coi là cleanup thành công.


## V03 UI/API replacement

[ADR-016](../../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) defines current internal placement. Remove deployment Connection selection after replacement pass; keep Secret Store/configuration/version/error/operation state. Definition authoring supports existing-cluster account choice and workload namespace/cluster references, Type params and provision flags; consumers inherit account. Score import/edit/export preserves metadata.class. Preview shows each workload/resource Definition/account/cluster/namespace and provider-first graph/batches without secrets, and blocks missing/ambiguous/mismatched/unready inputs as appropriate. No Environment target is required. Retired deployment-Connection writes fail explicitly; safe matching and stale-token gates remain. UI multi-cluster V03 unsupported error occurs before execution. Progress/readiness/errors/update/remove use per-workload targets; failure never shown as success.
