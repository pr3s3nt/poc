---
id: VERIFY-2026-10-14-I04-07-DEFINITION-WORKLOAD-UI
artifact: verification-record
status: evidence
last_reviewed: 2026-10-14
related: [I04-07, UC-03, UC-05, UC-09, UC-16, ADR-014, IMP-021]
---

# I04-07 — Definition, class và Xem trước, kiểm chứng local

## Trạng thái review

Code cuối `3ed0a89`, bàn giao `00a8813`, đã review và merge vào refactor_ui.
Standalone và pending Xem trước đều đọc đầu vào cùng baseline trong một snapshot;
kiểm thử PostgreSQL REPEATABLE READ và guard chứng minh concurrent write không
trộn version/token/ý định. Không còn điểm snapshot pending.

Claude viết code/test trên `claude/m04-i04-07`, base `8acf616`.
[Phiên cloud](https://claude.ai/code/session_01B8BFUdSwAFdkASEVQA1oKY);
nhánh tự sinh `claude/m04-i04-07-vsumu1` có cùng commits. Bash/Write/Edit/git/
network quan sát không prompt; tên mode nội bộ không introspect được.
Codex dùng checkout riêng `/tmp/poc-i0407-codex-review`, kiểm tra cloud mỗi
15 phút. Không có thay đổi của người dùng khi bắt đầu.

Review đã yêu cầu sửa public legacy driver pairs, metadata malformed, optional
Kubernetes Connection, standalone intents, tiến độ attempts đã lưu, lọc mã lỗi
không được nhận diện và ý định UPDATE khi cùng Definition key đổi đầu vào.
Hash nội dung Definition chỉ nằm trong plan snapshot nội bộ; kiểm thử pending/
standalone/Deployment detail chứng minh API không trả key hoặc giá trị hash.
Kế hoạch cũ thiếu bằng chứng được báo UNVERIFIED, không đoán RETAIN.

## Checks độc lập của Codex

Code `3ed0a89`/handoff `00a8813`, PostgreSQL 16 riêng, Terraform 1.9.8 và score-k8s 0.15.0 thật:

- `REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 go test ./... -count=1 -timeout=30m
  -json`: **1121 PASS, 0 FAIL, 0 SKIP**; build/vet đạt.
- Frontend typecheck/lint/test/build đạt: **549 tests, 30 files**.
- Docs với `REQUIRE_PLANTUML=1` và `git diff --check` đạt; runner shell đã kiểm
  `bash -n`. Không giảm assertion hoặc validator để đạt.
- Logs/result/cleanup: `/tmp/poc-i0407-codex-checks-r4/`; container PostgreSQL,
  mạng Docker và tệp mật khẩu đã xác minh không còn.

Lượt r1 tại `24bfcfa` đạt 1107 Go/0 skip và 518 frontend tests. Lượt r2 tại
`8a20c58` đạt 1114 Go/0 skip nhưng 6 frontend tests lỗi do fixture Type thiếu
classes; Claude sửa fixture, giữ bộ đọc metadata nghiêm ngặt. Các bước sau
frontend test của harness r2 không chạy; r3 đã chạy đầy đủ. Lưu cả hai lượt tại
`/tmp/poc-i0407-codex-checks-r1/` và `r2/` tương ứng.

Cloud báo Go 1103 pass/18 fail/0 skip do tải provider Terraform từ registry bị
chặn; đây không phải pass. Cloud frontend đủ 549 tests; docs/PlantUML không chạy
ở cloud. Bằng chứng cuối dùng kiểm chứng local độc lập, không dùng kết quả skip.

## Video và dọn dẹp

Codex chạy runner Claude viết trên code cuối `3ed0a89`, Chromium headed/Xvfb, cursor và
thao tác đủ chậm để đọc, phụ đề Việt dưới giao diện:

- `/tmp/poc-i0407-local-browser-r3/I04-07-captioned.mp4` và raw MP4.
- **517,27 giây, 26 mốc, 27 assertions**; giải mã/độ dài/khung hình và phụ đề đạt.
- Type/Definitions/template được đăng ký qua UI; class/default/custom/shared
  ID được giữ; Save chỉ tạo pending; thiếu renderer hoặc class không khớp chặn
  trước Deploy. Metadata lỗi khóa Save và giữ edits; thử lại tường minh.
- Pending và standalone Xem trước trả ý định/module/đích/quyền sở hữu an toàn;
  không tạo Deployment. Tiến độ persisted đọc lại qua reload/restart; mã stage/
  lỗi riêng tư thay bằng nhãn chung; danh sách attempts rỗng khác unavailable.
- REAL: UI/HTTP/service, SQL Console/State login, Vault KV v2, registry đã kiểm
  chứng và score-k8s cài thật. FAKE: executors, một lỗi mạng metadata dựng sẵn;
  attempts cho video được dựng qua stores, không thực thi Terraform. Phụ đề
  nói rõ điều này. Không tính là bằng chứng V2-A01/A02/A03.
- Scan retained text không có Vault tokens hoặc mật khẩu ba SQL logins.
  `cleanup.txt`: DB `i0407_dd785e4b200c` cùng 3 roles đã drop; Vault container
  `orch-i0407-20261014011949-26790` đã xóa, private directory đã xóa.
  `outer-cleanup.json`: PostgreSQL container/mạng/tệp mật khẩu không còn.

Lượt video r1 tại `8a20c58` đạt assertions nhưng validator lỗi: raw 510,266667
giây ngắn hơn mốc đăng xuất 510,4 giây. Bằng chứng giữ tại
`/tmp/poc-i0407-local-browser-r1/`; đã dọn DB/roles/Vault/PG/mạng/tệp riêng tư.
Claude bổ sung khoảng dừng cuối 7 giây; lượt thử lại r2 đạt, validator giữ nguyên.

Không deploy Kubernetes hoặc tạo AWS trong I04-07. Kết nối đầy đủ executor/
namespace/module cho chạy thật còn I04-10; live proof còn I04-11/12. Điểm sửa
snapshot ở đầu biên bản đã sửa và verified; IMP-021 tiếp tục mở cho I04-08..12.

Lượt r3 checks trên `0c870f0` cũng đạt 1115 Go/0 skip, 549 frontend; video r2
519,2 giây đạt sau sửa khoảng dừng cuối. Giữ bằng chứng cũ riêng; không thay
kết quả r4/video r3 trên code cuối. Không có required local check bỏ qua.
Cloud legacy `refactor-local.mjs` chưa chạy vì không thuộc kịch bản mới; live
Deploy-to-attempt browser proof còn I04-10..12, không suy từ attempts dựng sẵn.

Hoàn tất push `b988f27`; nhánh `claude/m04-i04-07` và nhánh tự sinh đã xóa,
`git ls-remote --heads` xác nhận không còn. Checkout review và symlink riêng
đã dọn; `git worktree list` chỉ còn repo chính. I04-08 đã giao phiên cloud mới.
