---
id: UC-00-UI-API
artifact: use-case-ui-api-mapping
status: current
last_reviewed: 2026-10-12
related: UC-00, OC-00
---

# UC-00 UI API mapping

These are proposed same-origin HTTP contracts for the UI; the canonical service
operation remains [OC-00](../../../architecture/contracts/operation-contracts.md).

| UI action | HTTP contract | Success handling | Failure handling |
|---|---|---|---|
| Submit sign-in | `POST /api/v1/auth/sign-in` with username/password | Backend sets opaque `HttpOnly` cookie; UI redirects to `/ui/applications`. | `401` gives generic invalid-credential state; other failures give API-error state. |
| Restore shell session | `GET /api/v1/auth/session` | Render authenticated shell and User context. | `401` redirects to Sign in. |
| Sign out | `POST /api/v1/auth/sign-out` | Clear local UI context and redirect to Sign in. | Clear local UI context and redirect to Sign in. |

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.
