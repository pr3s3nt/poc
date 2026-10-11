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
