---
id: UC-01-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-23
---

# UC-01 UI states

| Screen | State | Behavior |
|---|---|---|
| Create | Loading/validation/submitting/error | Name/Subdomain only; retain input; no target request. |
| Settings | Unconfigured/configured | Two editable scoped Connection selectors; explicit Save, no permanent lock/default auto-select. |
| Choices | Loading/error/empty | Disable corresponding Save; retry or ask PE to register READY target. |
| Save | Submitting/transfer | Disable duplicate action; show stage, scope and affected workloads without secret bytes. |
| Save | Stale version | 409 reloads authoritative selection; user reviews latest before resubmitting. |
| Environment | Busy | Active operation shown; temporarily block conflicting Settings/config/draft writes. |
| Transition | Preview | Show new deployment vs PostgreSQL transfer, resource mapping, downtime and source retention. |
| Transition | Running | Persisted stages, refresh-safe progress; no success before readiness/cutover. |
| Transition | Failed/interrupted | Source authority and compensation results visible; recovery requires confirmation that prior execution stopped; explicit cleanup action. |
| Preview | Stale | Explain configuration changed; require a new Preview. |
| Tabs | Scope switch | Choices/forms/tokens/progress follow selected app/env; ignore obsolete replies. |

## T07 — Draft lựa chọn và phản hồi bất đồng bộ

- Một READY choice, binding trống: gợi ý draft chưa lưu; không gọi mutation.
- Nhiều/không READY choice: không tự chọn; không có choice thì khóa Lưu.
- Binding đã có: giữ target persisted, không thay bằng gợi ý.
- Refresh/retry/late reply: không ghi đè pending edit; bỏ reply sai app/env hoặc
  attempt cũ. Reload authoritative version sau stale không ngầm lưu lại.
- BUSY: giữ nội dung đang sửa, khóa writes cho tới khi operation kết thúc.
- Cùng target: không copy/migrate; thông báo no-op đúng thực tế.
- Loading/error/empty/stale/copy failure/progress/confirmation và aria-label dùng
  tiếng Việt; IDs/enums/payload không dịch. VI-03 áp dụng contract lỗi T20.

## T09 — Gợi ý subdomain và tạo ứng dụng

- Draft chưa sửa subdomain: thay tên cập nhật gợi ý; không gọi API.
- Subdomain đã sửa: thay tên không ghi đè, kể cả khi subdomain bị xóa trống.
- Empty/invalid: lỗi field tiếng Việt, không POST; preview giữ endpoint draft.
- Submitting: khóa form và chặn submit lặp; không tự chọn Connection/provider.
- Duplicate name/subdomain: lỗi đúng field tiếng Việt, giữ nội dung để sửa/retry.
- Lỗi khác: fallback tiếng Việt an toàn, giữ form; retry chỉ khi người dùng submit.
- Success: điều hướng ứng dụng mới ở staging; staging/production, IDs và payload
  giữ nguyên. Không hiển thị raw backend errors; mapping tổng thể theo contract lỗi T20.

## T11 — Trạng thái danh sách ứng dụng

- Loading: skeleton có role status, aria-busy và nhãn “Đang tải ứng dụng”.
- Empty: “Bắt đầu với ứng dụng mới”, hướng dẫn tiếng Việt và nút Tạo ứng dụng.
- Error: alert “Không thể tải danh sách ứng dụng.” và nút “Thử lại”; không
  thay lỗi bằng danh sách giả hoặc hiện raw backend error.
- Retry: chỉ đọc lại danh sách khi người dùng bấm; tải thành công hiển thị
  dữ liệu thật theo thứ tự trả về, giữ tên/URL kỹ thuật.
- Ready: mọi text tĩnh trên trang/cards dùng tiếng Việt; staging/production
  giữ nguyên. Card và nút tạo dùng thao tác accessible hiện có, deep links
  giữ route/Application ID; không tự mutation khi tải hoặc mở ứng dụng.

## T12 — Trạng thái trang ứng dụng

- Loading/empty/read failure: nhãn accessible tiếng Việt; thử lại đọc danh sách,
  không tự replay mutation. Khi reload lỗi, bỏ danh sách cũ và khóa mutation.
- Chưa cấu hình: giải thích cần kết nối và link Cài đặt môi trường; không
  provision. BUSY giữ trạng thái đang chờ và giải thích writes bị tạm dừng.
- Stale: bỏ preview cũ, tải lại danh sách và yêu cầu kiểm tra/xem trước lại;
  giữ confirmation và expected versions hiện hành, không tự triển khai lại.
- No-op: thông báo không có thay đổi; trường hợp chỉ còn public routes giữ
  thao tác thử lại riêng, không restart Workload ngoài semantics hiện tại.
- Failed/partial: báo kết quả từng Workload; thay đổi chưa xong vẫn pending,
  không rollback tự động. Lỗi dùng fallback Việt an toàn, không hiển thị raw
  backend/provider error hoặc giá trị key/bí mật; mapping tổng thể theo contract lỗi T20.
- Đổi app/môi trường: bỏ phản hồi cũ sai context cho list/preview/result/history;
  IDs và graph data giữ nguyên. Triển khai đang chạy giữ các khóa điều hướng.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.


## V03 UI/API replacement

[ADR-016](../../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) defines current internal placement. Remove deployment Connection selection after replacement pass; keep Secret Store/configuration/version/error/operation state. Definition authoring supports existing-cluster account choice and workload namespace/cluster references, Type params and provision flags; consumers inherit account. Score import/edit/export preserves metadata.class. Preview shows each workload/resource Definition/account/cluster/namespace and provider-first graph/batches without secrets, and blocks missing/ambiguous/mismatched/unready inputs as appropriate. No Environment target is required. Retired deployment-Connection writes fail explicitly; safe matching and stale-token gates remain. UI multi-cluster V03 unsupported error occurs before execution. Progress/readiness/errors/update/remove use per-workload targets; failure never shown as success.


## V04–V06 final Settings replacement

After V06.08, Connection-choice and deployment-transition states above are
retired. Loading/empty/error/explicit Save/stale/unknown-outcome/store-copy and
operation-owner guards still apply to the surviving Secret Store/configuration
flow. An Environment with no retired binding is not an error. Missing or invalid
Definition/reference/credential/readiness is reported in its Preview/execution
context; no automatic target selection or mutation is introduced.

## Missing or legacy Environment metadata — 2026-10-23

Use the actual Environment keys advertised by the Application API. An explicitly
missing staging/production Environment must not be fabricated with default
metadata or queried by child views. Show the existing Application identity and
its unsupported/legacy keys with a clear Console-unsupported state. If another
supported Environment exists, offer explicit navigation to it. Do not create,
rename, migrate or delete legacy Environments automatically. API responses that
omit this metadata retain the existing compatibility behavior.
