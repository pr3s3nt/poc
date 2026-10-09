---
id: RUNBOOK-CLAUDE-TMUX
artifact: operations-runbook
status: current
last_reviewed: 2026-10-09
---

# Codex điều phối Claude qua tmux

## Vai trò và phạm vi

[AGENTS.md](../../AGENTS.md) sở hữu quy tắc làm việc. Codex phân tích, sửa tài
liệu chuẩn, giao task, review, validation, cập nhật trạng thái và commit/push.
Claude chỉ implement code/test được giao. Không deploy hoặc mutate external
state nếu người dùng chưa đưa vào scope. Task thuần tài liệu không cần Claude.

## Procedure

1. Codex kiểm tra git status, đọc INDEX/nguồn chuẩn, ghi nhận user-owned changes.
2. Chốt canonical docs trước code; ghi deviation khi design chưa được implement.
3. Kiểm tra `tmux` và `clauded` bằng shell tương tác. `clauded` có thể là alias;
   chạy trong `bash -i`, không giả định nó là executable trên PATH.
4. Tạo session riêng với tên duy nhất và working directory đúng repository.
   Giữ nguyên mọi session có sẵn. Ví dụ `tmux new-session -d -s codex-task-<id>
   -c <repo> 'bash -i'`, sau đó gửi `clauded` vào pane đó.
5. Gửi brief gồm mục tiêu, docs chuẩn, file/phạm vi được phép sửa, acceptance
   criteria, checks, giới hạn external mutation và user-owned changes. Nêu rõ
   Claude không sửa docs, không commit/push và không tự đổi requirements.
   Brief/log đặt trong thư mục tạm riêng ngoài repository, không chứa secrets.
6. Theo dõi pane bằng `tmux capture-pane`; kiểm tra diff thực tế độc lập. Khi
   Claude báo hoàn thành, Codex review correctness, scope, persistence/security,
   legacy behavior và test coverage. Gửi phản hồi cụ thể vào cùng pane; lặp lại
   khi còn lỗi. Không xem lời báo hoàn thành là bằng chứng checks đã pass.
7. Codex chạy validation theo AGENTS.md, sửa docs/current state/traceability theo
   kết quả thực tế. Ghi checks chưa chạy và lý do; không tuyên bố live proof từ
   local fake tests. Không tự triển khai database/cluster/cloud để chạy checks. Khi chỉ chạy local
   checks, unset các opt-in như `ORCHESTRATOR_POSTGRES_TEST_URL` và
   `ORCH_KIND_VERIFY` cho subprocess test, không đọc/hiển thị giá trị.
8. Stage rõ các file thuộc task, review staged diff, commit và push theo branch/
   upstream đã kiểm tra. Không dùng force-push; nếu remote thay đổi, kiểm tra và
   tích hợp an toàn trước khi thử lại. Không reset/revert công việc của người dùng.
9. Sau completion, đóng session của task bằng `tmux kill-session -t <owned-name>`
   và xác nhận đã đóng. Nếu bị chặn/hủy, lưu handoff trước cleanup và báo trạng
   thái chưa hoàn thành. Không kill tmux server hoặc session khác.

## Handoff

Báo outcome, vùng thay đổi, checks, giới hạn, commit/push result và session đã
đóng. Nếu `clauded` không chạy hoặc bị giới hạn dịch vụ, Codex báo rõ; không âm
thầm chuyển code sang agent khác. Authorization commit/push không cấp quyền deploy.
