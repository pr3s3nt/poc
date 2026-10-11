---
id: UC-01-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-11
---

# UC-01 UI screens

Create Application contains Name/Subdomain only. Home and Settings have staging/
production tabs and separate target summaries. Settings includes independently
editable Deployment Connection and Secret Store Connection selectors, safe names
and explicit Save; no permanent lock. Ordinary variables work before store set.

For a deployed Environment, changing execution target opens an explicit transition
form: destination, deploy-new or migrate PostgreSQL, logical resource mapping,
impact and downtime acknowledgement. Preview identifies added/updated/removed
workloads, unchanged workloads to redeploy, and pending configuration changes.
Preview then execute with exact token.
Show persisted stages/source-destination status, recovery errors and retained
source cleanup action. Show the persisted authoritative destination and actual
writer/route compensation outcome, including source quiesce failures after
cutover. Never present a failed/partial transition as successful.

Changing secret store explains that Secrets are copied first and running workloads
move after Preview/Deploy. Secret values are never displayed. On 409 show latest
selection and require review/resubmit; active-operation busy is temporary.

## T07 — Cài đặt môi trường (ENV-01, VI-02)

Trang và dialogs dùng nhãn tiếng Việt theo glossary; tab và giá trị kỹ thuật giữ
`staging`/`production`. Hai selector dùng tên an toàn và key kỹ thuật. Khi chưa
có binding và chỉ có một lựa chọn READY hợp lệ, UI gợi ý lựa chọn trong draft,
ghi rõ chưa lưu; đây không phải Organization default hoặc fallback. Chỉ nút
Lưu gửi thay đổi với expectedVersion. Có nhiều lựa chọn thì người dùng tự chọn;
không có lựa chọn thì hướng dẫn đăng ký đích và cho thử lại. Không đổi target
đã cấu hình. Phạm vi triển khai/region do backend suy ra, chỉ đọc.

Các nhãn tiến trình, chuyển đích, tác động, downtime và recovery dùng tiếng Việt.
Giữ Preview rồi xác nhận execution; migration vẫn cần xác nhận downtime và
recovery vẫn cần xác nhận operation cũ đã dừng. Không thay đổi API semantics.
