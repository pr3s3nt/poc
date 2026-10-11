---
id: UC-01-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-11
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
  tiếng Việt; IDs/enums/payload không dịch. VI-03 mapping toàn backend thuộc T20.

## T09 — Gợi ý subdomain và tạo ứng dụng

- Draft chưa sửa subdomain: thay tên cập nhật gợi ý; không gọi API.
- Subdomain đã sửa: thay tên không ghi đè, kể cả khi subdomain bị xóa trống.
- Empty/invalid: lỗi field tiếng Việt, không POST; preview giữ endpoint draft.
- Submitting: khóa form và chặn submit lặp; không tự chọn Connection/provider.
- Duplicate name/subdomain: lỗi đúng field tiếng Việt, giữ nội dung để sửa/retry.
- Lỗi khác: fallback tiếng Việt an toàn, giữ form; retry chỉ khi người dùng submit.
- Success: điều hướng ứng dụng mới ở staging; staging/production, IDs và payload
  giữ nguyên. Không hiển thị raw backend errors; mapping tổng thể thuộc T20.

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
