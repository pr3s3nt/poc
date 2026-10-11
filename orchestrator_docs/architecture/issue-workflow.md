---
id: DEV-ISSUE-WORKFLOW
artifact: development-workflow-design
status: current
last_reviewed: 2026-10-11
---

# Giao việc qua GitHub Issues

## Phạm vi và trách nhiệm

Đây là công cụ phát triển cho repository này, độc lập với runtime Orchestrator.
Một supervisor Go ở `tools/issueflow/` chạy trên một máy. Người dùng và AI chốt
yêu cầu trên GitHub Issue; supervisor nhận việc và chạy Codex điều phối bằng
`codex --yolo`. Codex viết documentation, giao coding agent thực hiện code/test,
review diff và kiểm chứng checks độc lập, phản hồi đến khi đạt yêu cầu.

Coding agent hiện tại là Claude qua `clauded` trong tmux. Adapter có contract
start/send/status/result/stop/resume; OpenCode/Kimi có thể được bổ sung, không
được coi là đã hỗ trợ khi chưa có implementation và tests.

## Task và hàng chờ

- Repository cố định trong cấu hình; không nhận task repository khác.
- Issue có nhãn `agent:ready`, mục tiêu, phạm vi, acceptance criteria và checks.
  Người cấu hình phải khai báo GitHub login được phép giao việc/trả lời; chỉ task
  được người này tạo và gắn nhãn ready được nhận. Không suy quyền từ nội dung Issue.
- Một task active, chọn Issue số nhỏ nhất trong hàng sẵn sàng. Task mới không
  chạy trước khi task hiện tại done hoặc người dùng explicitly abort.
- Dùng worktree riêng cho từng task từ base branch cấu hình (mặc định main),
  không sửa checkout hoặc thay đổi có sẵn của người dùng.
- Chụp nội dung Issue khi nhận. Thay đổi yêu cầu trong lúc chạy được chuyển về
  Codex để đánh giá; không âm thầm thay acceptance criteria.
- Khi cần quyết định, đăng câu hỏi có ID lên Issue, lưu `WAIT_USER`, dừng cả
  hàng chờ. Chỉ câu trả lời từ login được phép và gắn đúng ID câu hỏi mới resume.

## State và phục hồi

`READY → RUNNING → REVIEW → VALIDATING → PUBLISHING → DONE`.
Trạng thái chờ gồm `WAIT_USER`, `WAIT_QUOTA`, `WAIT_AUTH`; lỗi không thể tự
khắc phục dừng hàng chờ và báo người dùng. Codex không được bỏ qua review.

State ở thư mục ngoài Git, permission private: repo, Issue, run ID, worktree,
base/head SHA, phiên/role, generation, câu hỏi/quyết định, checks, hash nội dung
được review, evidence và wake time UTC. Ghi atomic, khóa một supervisor, intent
trước side effect và reconcile sau restart. GitHub comments có marker ổn định;
trước retry phải tìm marker. Launch không rõ kết quả không được tạo phiên mới
mù quáng. Mọi report phải gắn run ID/generation để loại report cũ.

Epoch/state-file lease là bookkeeping, không cô lập filesystem. Trước chạy
writer mới phải xác minh writer cũ đã dừng. Không đủ bằng chứng thì dừng chờ;
branch im lặng hoặc timeout không chứng minh cloud đã kết thúc. Codex sửa docs
và Claude sửa code theo path được giao; tránh đồng thời sửa cùng file.

Local process ownership dùng PID/start-time/process group được ghi khi launch;
không quét `/proc/*/environ` của process khác để tìm marker. Không kill theo PID
đã được tái sử dụng. Đây là quản lý lifecycle cho agent được tin cậy, không phải
sandbox chống agent độc hại khi đã được cấp `--yolo`.

## Điều phối và quota

Supervisor giữ timer/tiến độ bên ngoài model; không phụ thuộc Codex còn quota.
Codex nhận brief/checkpoint và dùng CLI control của supervisor để giao việc,
nhận report, gửi feedback, hỏi người dùng và báo acceptance. Không chèn phím
vào TUI đang làm việc nếu không xác định được trạng thái nhận input.

Coordinator chạy các turn `codex exec --yolo --json`, resume đúng session ID nếu
có; tmux chứa process turn và log/exit marker. Supervisor chỉ mở turn khi có event
mới hoặc timer đến hạn. Prompt và checkpoint đi qua file/stdin, không dùng shell
interpolation từ nội dung Issue. Nguồn CLI:
[Codex non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode).

Đọc tiến độ Claude bình thường mỗi **240 giây**. Chỉ kiểm tra sớm cho completion,
lỗi/blocker, user steering hoặc sự cố. Watcher completion có thể đọc marker/process
không dùng model; không capture pane mỗi vài giây để giả làm watcher rẻ.

Phân loại quota, context full, auth, network/tool error riêng biệt. Chỉ quota
đặt wake tự động: reset time nếu có + đệm; thiếu reset thì backoff có cấu hình.
Lưu wake trước khi đợi, reconcile khi máy bật lại; không cần model để báo thức.
Context full cần checkpoint/phiên mới, auth cần người dùng; không hẹn giờ nhầm.
Codex hết quota thì coder không nhận việc mới; lưu phần đang làm, resume coordinator
để review lại. Không mất file/patch khi phiên cũ chết. `WAIT_USER` không được timer
quota hoặc resume-now bỏ qua khi chưa có câu trả lời.

## Claude cloud

Người dùng xác nhận có Cloud session credits riêng. Credit tự áp dụng cho cloud;
sau khi hết/hết hạn mới dùng plan thường. Số dư ảnh là observation, không hardcode
vào tool. Không tự mua credit, bật auto-reload hoặc đọc credential.

Local quota → checkpoint → xác minh dừng local → cloud nếu adapter khả dụng.
Cloud không chạy được thì lưu lý do và hẹn tiếp tục, không tạo nhiều phiên fallback.
CLI hỗ trợ create `claude --cloud`, gửi feedback `claude -p --cloud <session-id>`
và import `--teleport`; gửi thành công chỉ chứng minh message được xếp hàng.
Không có command status/result đã kiểm chứng thì trạng thái process vẫn unknown;
không lấy branch SHA đổi làm tín hiệu hoàn thành. Supervisor tự đọc report JSON
trên branch cloud theo protocol bên dưới, không yêu cầu người dùng báo đã xong.
Session create có
kết quả mơ hồ phải reconcile hoặc hỏi người dùng, không retry tạo session mới.

Cloud là phiên mới, brief ghi task, docs, checkpoint, base SHA, phần còn lại và
checks. Codex push checkpoint đã review vào branch handoff riêng; không tự commit
user-owned changes. Người dùng đã cho phép **Claude cloud commit/push chỉ vào
branch riêng của task**. Không push main/branch tích hợp, không tự merge/close
Issue. Codex fetch diff, review và tích hợp kết quả. Đây là exception cho cloud;
Claude local vẫn không commit/push. Không suy Git author là bằng chứng role.

Mỗi vòng cloud có report tại `.issueflow/result-<run-id>-<generation>.json` trên
branch riêng. Report chứa schema version, run ID, generation, checkpoint/base SHA,
commit code cuối, trạng thái `done|blocked|quota`, `writer_state: idle`, summary và
checks (command/exit code). Claude push report sau code và cam kết không sửa tiếp
cho đến feedback. Đây là acknowledgment của agent tin cậy, không phải xác minh
process cloud đã tắt. Supervisor poll không dùng model, fetch đúng branch, đọc blob
tại SHA cố định và kiểm identity, schema, ancestry, commit code và giới hạn kích
thước trước khi phát event cho Codex. Không có report hợp lệ thì tiếp tục chờ.

Report `done` mở review tự động; Codex review code ở SHA cố định, tích hợp code
không mang report vào sản phẩm và chạy lại checks độc lập. Feedback mở generation
mới trên cùng session; report cũ không hoàn thành vòng mới. Report `blocked` đưa
vấn đề cho Codex đánh giá, chỉ hỏi người dùng khi cần quyết định. Report `quota`
lưu wake/checkpoint để tiếp tục tự động. Trước chuyển writer, tích hợp hoặc publish
phải kiểm lại branch đã chốt; ghi muộn làm dừng tích hợp và hàng chờ. Không tự tạo
session thứ hai khi launch/follow-up có kết quả mơ hồ.

Cloud không được giả định có kind/Vault/browser recording trên máy local.
Checks và browser scenario cần môi trường local được chạy lại tại máy khi có
agent khả dụng; task chưa done nếu còn thiếu evidence bắt buộc.

Nguồn: [Claude cloud](https://code.claude.com/docs/en/claude-code-on-the-web),
[Remote Control](https://code.claude.com/docs/en/remote-control). Remote Control
điều khiển local, không thay thế cloud execution.

## Kết quả và evidence

Codex acceptance tham chiếu content hash bao gồm staged/unstaged changes
và file untracked thuộc task; thay đổi sau review làm acceptance stale. Checks
gắn đúng revision, lưu command/exit code/log. Không dùng report pass của model
thay exit status/assertions. Chỉ done sau khi publish kết quả thành công.

Fingerprint nội dung phải ổn định khi cùng bytes/modes được stage/commit: dùng
manifest working tree theo path và mode thay vì ghép git diff với phần untracked.
Head/base/branch được kiểm riêng; commit chỉ thay metadata không làm mất review,
nhưng thay nội dung, thêm/xóa file hoặc đổi mode phải làm review stale.

Scenario Web Console quay headed Playwright với thao tác UI thật, cursor/click,
pause và phụ đề tiếng Việt theo runbook. Giữ video/assertions/phase marks cả khi
fail, ghi rõ adapter thật/fake. Task chỉ CLI/docs ghi lý do không có browser video.
Validation evidence gồm decode, captions/phase marks, review frame và không lộ
secret; không tuyên bố OCR/scan là chứng minh tuyệt đối không có secret.

Video ở ngoài Git. Publisher dùng GitHub Release assets của repo (prerelease
evidence riêng), Issue chứa link và manifest/checks/commit/PR. Không dùng private
attachment API, không commit binary, không tự xóa evidence hoặc tạo storage ngoài.
Upload có manifest/hash và reconcile asset tồn tại trước retry. Visibility theo
repo; chưa có retention policy thì giữ nguyên. Live GitHub/cloud smoke chỉ chạy
khi đã xác nhận kết nối và quyền; fake tests không chứng minh live integration.

Evidence chỉ lấy từ thư mục task sở hữu, không theo symlink ra ngoài. Manifest
gắn content hash/run ID và phân loại browser từ task, không do coder tự hạ xuống
CLI để bỏ video. Assertions/phase marks/captions phải có nội dung hợp lệ; reviewer
ghi kết quả xem frame đúng hash video. Publisher dùng tên asset theo content hash,
rehash khi copy/upload và không clobber asset khác. Lỗi đọc release không được
coi là not-found để tự tạo release. CLI evidence cũng giữ check logs/assertions.

## Validation trước nhận việc thường trực

Offline tests với fake clock/GitHub/agent và temporary git remote phải kiểm tra:
tuần tự, nhiều vòng feedback, WAIT_USER và answer sai ID/actor, quota local/Codex,
credit fallback, restart ở side effects, stale reports/acceptance, unknown cloud
completion, late cloud writes, evidence/publish failure và worktree isolation.
CLI smoke dùng process fake; không quay browser video không liên quan.
Sau checks local, chạy một Issue smoke được chỉ rõ rồi mới bật daemon nhận việc.
