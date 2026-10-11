---
id: UC-04-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-11
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
