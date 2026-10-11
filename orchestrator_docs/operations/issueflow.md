---
id: RUNBOOK-ISSUEFLOW
artifact: operations-runbook
status: current
last_reviewed: 2026-10-11
---

# Chạy worker nhận GitHub Issues

Nguồn chuẩn: [thiết kế workflow](../architecture/issue-workflow.md).
Worker là công cụ phát triển Linux/WSL, không chạy trong Orchestrator backend.
Implementation đã có offline verification; chưa tuyên bố live unattended cloud
đã hoạt động. Gap:
[IMP-020](../implementation/deviations.md#imp-020--unattended-claude-cloud-completion-transport-open).

## Chuẩn bị

Máy cần Go, git, gh, tmux, Codex CLI và `clauded` trong `bash -i`. GitHub CLI
đã đăng nhập bằng account có quyền Issues và Contents trên repository. Cloud
cần account Claude có quyền cloud, kết nối repo và environment. Không copy
token vào config, không bật auto-reload credit.

Build executable bên ngoài Git:

```bash
go -C tools/issueflow build -o /tmp/issueflow ./cmd/issueflow
```

Config JSON ngoài Git, dùng absolute paths, ví dụ:

```json
{
  "repo": "pr3s3nt/poc",
  "repo_path": "/ABS/PATH/poc",
  "base_branch": "main",
  "state_dir": "/ABS/PATH/issueflow-state",
  "worktree_root": "/ABS/PATH/issueflow-worktrees",
  "owner_logins": ["YOUR_GITHUB_LOGIN"],
  "poll_seconds": 240,
  "coder": {"adapter": "claude-local", "command": "clauded"},
  "cloud": {"mode": "off"},
  "evidence": {"publisher": "release", "tag_prefix": "evidence"}
}
```

Thay paths/login bằng giá trị của máy. Worktree/state phải ở ngoài checkout.
Base branch phải chứa code và quy tắc được chọn để task mới đọc; không dùng main
để thử task dựa trên workflow chưa được tích hợp vào main.

## Kiểm tra và chạy

```bash
/tmp/issueflow --config /ABS/PATH/issueflow.json probe
/tmp/issueflow --config /ABS/PATH/issueflow.json status
```

`probe` kiểm tra tools/CLI flags, không tạo cloud session và không chứng minh
account cloud đã dùng được. Trước chạy liên tục, review config và chọn một Issue
smoke. Chỉ khi đã cho phép worker ghi GitHub và chạy model thật mới chạy:

```bash
/tmp/issueflow --config /ABS/PATH/issueflow.json run
```

Worker chạy foreground; có thể giữ trong tmux riêng. Máy Windows/WSL ngủ hoặc
tắt thì worker không chạy; sau khi máy hoạt động lại phải giữ/khởi động supervisor
để reconcile wake đã lưu. Timer không bật được máy Windows đang tắt.

Claude có thể hỏi trust cho mỗi worktree mới. Default chỉ chọn Yes khi thấy
option có số rõ ràng; screen Yes không có số sẽ dừng để tránh chọn sai. Sau khi
kiểm tra layout thực tế, có thể cấu hình `coder.trust_unnumbered_keys` (ví dụ
`["Down", "Enter"]` cho screen đang chọn No và Yes ở ngay dưới). Đây là cấu hình
cho layout đã quan sát; không dùng ví dụ này nếu screen của máy khác.

## Giao việc và trả lời

Issue ghi mục tiêu, phạm vi, tiêu chí hoàn thành, checks và có browser scenario
hay không. Tạo các nhãn `agent:ready`, `agent:active`, `agent:done` trước khi chạy.
Người thuộc `owner_logins` tạo Issue và gắn `agent:ready` sau khi chốt. Worker
nhận số nhỏ nhất, không nhận task thứ hai khi task đầu đang làm/chờ.

Khi cần quyết định, Issue có câu hỏi và ID. Trả lời bằng account được phép:

```text
/answer Q-xxxxxxxx câu trả lời của bạn
```

ID phải là ID thật trong comment. Câu trả lời cho câu hỏi cũ/task khác không
resume. Kiểm tra state/audit local khi cần; không sửa trực tiếp task.json để bỏ
qua lock hoặc writer chưa dừng.

## Evidence và cloud

Browser task giữ video headed, captions, assertions, phase marks và review frame.
Codex dùng control CLI để khai báo evidence trong thư mục task-owned mà worker
cấp; worker validate và upload Release assets, Issue có links. CLI/docs task
ghi rõ không có browser scenario và đưa check logs/assertions làm evidence.
Không nhầm fake-adapter proof với deploy thật.

Browser evidence cần video, `.srt` hoặc `.vtt`, và ba file JSON cùng thư mục:

| File | Contract |
|---|---|
| `assertions.json` | `run_id`, `content_hash`, `gen`, `adapter` (`real`/`fake`), `headed: true`, `overall: "pass"`, `assertions` gồm `name`, `status`, `detail`. |
| `phases.json` | `run_id`, `content_hash`, `duration_seconds`, `phases` gồm `name`, `start`, `end` theo giây trong video. |
| `frame-review.json` | `reviewer: "codex"`, `run_id`, `content_hash`, `video_sha256`, `privacy_ok: true`, `frames` gồm `t`, `note`, `ok`. Codex viết sau khi xem frame thực tế. |

Agent tạo recording/assertions/phase marks theo scenario của task; các recorder
cũ trong project chưa tự phát đầy đủ contract này. Không chỉ đổi tên file hoặc
ghi assertion pass thay cho kiểm chứng thật. Giữ evidence khi fail để review;
chỉ evidence đạt validation và review mới được dùng để hoàn tất task.

Control CLI có `evidence-schema --issue N` để đọc schema/samples và
`evidence-hash` trong context của task để lấy hash nội dung. Brief gửi cho coder
và coordinator chứa contract cùng đường dẫn evidence của task.

Cloud fallback có mode `auto` với report trên branch riêng theo
[protocol thiết kế](../architecture/issue-workflow.md#claude-cloud). Chỉ bật sau
verification; cấu hình không hợp lệ phải báo lỗi, không âm thầm bỏ qua.
Không dùng `Remote Control` để thay cloud. Xác nhận credit còn ở UI Claude;
worker không tự mua credit khi hết. Normal flow không cần bạn báo Claude đã xong:
supervisor tự đọc report đúng run/generation, Codex tự review và kiểm thử lại.
Chưa có live verification cho cơ chế này. Launch mơ hồ, ghi muộn hoặc quyết định
không thể tự giải quyết vẫn dừng hàng chờ và hỏi người dùng.

Để chọn fallback, cấu hình `coder.fallback: ["claude-cloud"]` và
`cloud.mode: "auto"`; default là `off`. Mode `assisted` còn tồn tại để xử lý thủ
công, không phải luồng tự động yêu cầu. Create/send response chưa được kiểm chứng
trên account thật; probe thấy flags không chứng minh quyền cloud. Codex push
checkpoint đã review trước handoff, cloud chỉ push branch riêng, report không
được nhập vào project. Không hardcode số dư credit trong ảnh vào tool.

## Validation

```bash
cd tools/issueflow
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Offline fake tests không dùng quota, không ghi GitHub, không mutate database,
kind/AWS. Verification phải ghi rõ fake/live, evidence path, checks và gap còn lại.
