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
Claude implement và chạy code/test được giao, bao gồm Playwright/recording cho
browser scenario. Codex review code và kiểm chứng evidence độc lập. Không deploy
hoặc mutate external state nếu người dùng chưa đưa vào scope; quyền chạy test
local không tự cấp quyền kind/AWS. Task thuần tài liệu không cần Claude.

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
   Brief phải có browser scenario/recording, phụ đề mong muốn, nơi lưu evidence,
   adapter thật/fake, môi trường được phép và ownership/cleanup nếu có integration.
   Brief/log đặt trong thư mục tạm riêng ngoài repository, không chứa secrets.
6. Theo dõi pane bằng `tmux capture-pane` theo nhịp 4 phút (240 giây), không
   capture/poll liên tục khi Claude đang làm việc bình thường. Mỗi lần đọc tail
   ngắn, trạng thái checks và diff liên quan; dùng báo cáo file ngắn để giữ thông
   tin thay vì đọc lại toàn bộ pane. Giữa hai lần kiểm tra tiếp tục phần docs/review
   độc lập hoặc chờ; các lần chờ phải tuân thủ giới hạn môi trường, không dùng
   blocking wait dài hơn giới hạn chỉ để đạt 240 giây. Kiểm tra sớm khi nhận
   completion/lỗi/blocker, user steering hoặc sự cố cleanup. Khi
   Claude báo hoàn thành, Codex review correctness, scope, persistence/security,
   legacy behavior và test coverage. Gửi phản hồi cụ thể vào cùng pane; lặp lại
   khi còn lỗi. Không xem lời báo hoàn thành là bằng chứng checks đã pass.
7. Claude chạy validation trong scope đã giao; Codex xác minh exit status/log/
   artifact theo AGENTS.md và chỉ chạy lại checks cần thiết khi có thay đổi mới,
   failure hoặc evidence chưa đủ. Codex sửa docs/current state/traceability theo
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

## Playwright recording cho task code/test có browser flow

1. Dùng hoặc mở rộng runner/scenario hiện có trong `frontend/test/e2e/` và
   `backend/test/integration/`; Claude chịu trách nhiệm phần code/test/recording.
   Quay headed browser, ưu tiên toàn cửa sổ gồm tab/address bar với runner Xvfb/
   ffmpeg hiện có. Không cần quay terminal hoặc toàn desktop chứa dữ liệu riêng.
2. Scenario phải phản ánh các bước người dùng thực hiện: sign-in, mở màn hình,
   chọn/nhập form, Save, Preview, Deploy nếu được phép, quan sát kết quả. Hiện cursor/
   click, nhập liệu với tốc độ dễ đọc, pause ngắn ở mỗi bước quan trọng. Chờ UI/API/
   readiness có assertion; pause để người xem đọc không thay thế synchronization.
   Không dùng API seed thay cho thao tác UI được tuyên bố đã chứng minh; setup API
   riêng phải được nêu rõ trong evidence.
3. Ưu tiên phụ đề tiếng Việt ở vùng dưới video theo phase marks: số bước, mục đích,
   thao tác và kết quả thật. Mỗi phụ đề ngắn, đủ thời gian đọc, không che nút/form/
   kết quả. Ưu tiên caption trong bản video xem lại; có thể kèm SRT/VTT đồng bộ.
   Overlay/caption chỉ phục vụ recording, không thêm vào UI sản phẩm. Không đưa
   token, password, kubeconfig hoặc secret values vào phụ đề. Nếu runner chưa hỗ
   trợ caption, Claude bổ sung trong task code/test liên quan hoặc handoff nêu rõ
   recording chưa có phụ đề; tài liệu này không tuyên bố helper đã implement.
4. Giữ recording và phase marks trên cả success/failure, cùng kết quả assertion,
   run ID, adapter mode và cleanup. Video là bằng chứng bổ sung; chỉ pass khi
   assertions/checks tương ứng pass. Fake-adapter video không chứng minh live deploy.
   Test chỉ có unit/CLI thì ghi rõ không có browser flow, không mở rộng scope chỉ
   để quay một video không liên quan.
5. Codex review các frame/đoạn chính và đối chiếu kết quả sau khi Claude hoàn tất,
   không xem liên tục khi quay. Với runner MP4 hiện có, kiểm tra full decode,
   resolution, phase marks và frame không trống; không cố kéo dài video để đủ
   thời lượng nếu scenario đã đổi. Kiểm tra caption khớp bước/kết quả, không lộ
   credential và không che UI. Evidence ở ngoài Git; handoff link/path video,
   captions, checks và giới hạn. Không tự publish/upload recording khi chưa được
   yêu cầu. Cleanup chỉ các process/session/resource của task theo authorization.
