---
id: UC-04-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-12
---

# UC-04 UI states

- List loading/empty/ready/error: READY list; error has Retry, not fake empty.
- Editing: name and upload/paste kubeconfig; context/summary cleared on change.
- Inspecting: disable repeated inspection; stale responses cannot restore an
  old document's context/summary. No secret or Connection is persisted.
- Inspected: one context auto-selected; multiple require selection; summary
  matches chosen context. Changed document requires fresh inspection.
- Unsupported/invalid: safe actionable parser/auth guidance, retain form.
- Verifying/saving: lock submitted form and duplicate submit; API/RBAC then store.
- Registration error: preserve form; safe connectivity/permission/store message.
- Registered: notify committed success, clear credentials/file/form, reload list.
- Registered/list error: keep success distinct from reload failure, offer Retry
  list without repeating registration. Unknown network outcome does not claim
  registration failure as proof that nothing was saved; do not auto-resubmit.

## Secret store registration

Loading/empty/error list states; form validation, verifying, saving, safe failure
and committed-success/list-refresh states. Disable duplicate registration, retain
nonsecret fields after errors, never return/prefill tokens or raw provider errors.

## Kết nối — T06

- Editing/upload: đọc file xong tự inspect; đổi file/source/document xóa ngay
  summary/context cũ. File-read/inspect đến muộn không được sửa draft mới.
- Editing/paste: chờ nội dung ổn định rồi inspect; hủy timer cũ khi document
  đổi/unmount/save. Không tự retry vòng lặp khi inspect lỗi.
- Inspecting: thể hiện trạng thái Việt accessible, khóa inspect lặp; vẫn cho
  đổi document để vô hiệu request cũ. Không persist/verify cluster.
- Inspected: một context tự chọn; nhiều context chưa chọn không có summary/
  gợi ý đích. Gợi ý tên chỉ cập nhật khi người dùng chưa sửa tên.
- Failure/retry: giữ draft, báo hướng dẫn Việt an toàn và có retry inspect
  tường minh; không hiển thị raw parser/provider errors hoặc kubeconfig.
- Saving: chỉ bắt đầu từ nút “Kiểm tra và lưu”, khóa draft/submit lặp.
  Lỗi xác minh giữ form cho sửa; unknown outcome không tự submit lại.
- Success: xóa credential/file/context/name và reset ownership gợi ý tên;
  list refresh lỗi có retry GET riêng, không phủ nhận registration success.
- List loading/empty/error/ready: text và aria-label Việt; error không giả rỗng,
  chỉ READY records được hiển thị, không lộ credential/ref.

## Kho bí mật — T05

- Editing: cơ bản/nâng cao giữ chung draft; checkbox cùng địa chỉ là opt-in.
- Local invalid: lỗi tiếng Việt chỉ rõ field; không POST. Kiểm URL/mount,
  required/name/token limits và CA PEM shape trước request; không xác minh network.
- Verifying/saving: khóa draft và submit lặp; token luôn được che.
- Server failure/unknown outcome: xóa token, giữ metadata và lựa chọn cùng địa
  chỉ/nâng cao; dùng hướng dẫn Việt an toàn, không hiển thị raw provider error,
  không tự submit lại hoặc khẳng định chưa lưu khi response bị mất.
- Success: xóa form/token, báo kho đã sẵn sàng để chọn trong Cài đặt môi trường;
  reload list failure tách khỏi registration success và retry chỉ GET.
- List loading/empty/error: thông báo Việt accessible; lỗi không giả thành rỗng.
- List ready: backend verification và Kubernetes auth hiển thị độc lập;
  absent/unknown không suy CONFIGURED hoặc deploy-ready.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.

## T23 — Ordinary stores only

Store list and Environment choices have no legacy label or flag. READY continues
to describe backend verification, not workload deploy readiness. Loading/error,
explicit Save and selection/version guards are unchanged; no global-token or
old-store fallback is exposed.
