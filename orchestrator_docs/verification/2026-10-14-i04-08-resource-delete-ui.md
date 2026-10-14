---
id: VERIFY-2026-10-14-I04-08-RESOURCE-DELETE-UI
artifact: verification-record
status: evidence
last_reviewed: 2026-10-14
related: [I04-08, UC-07, UC-09, ADR-014, IMP-021]
---

# I04-08 — Xóa tập tài nguyên đã xác nhận qua Console

## Review và trạng thái

Code `8d213a3` đã review và đạt kiểm chứng local; đã merge tại `b4ed07e`; đang hoàn tất push và dọn nhánh.
[Phiên Claude cloud](https://claude.ai/code/session_015AaNpBjwF2dUGJQHUxUrRv),
base `b988f27`, nhánh `claude/m04-i04-08` và nhánh tự sinh
`claude/m04-i04-08-u9cf6p`. Write/Edit/Read/Bash/git commit/push quan sát không
prompt theo addendum `378d148`; mode nội bộ không introspect được.

Resource list/detail/history links và cleanup queries đọc metadata bằng Console
login với SELECT theo cột. PE/Admin xác nhận đúng tên và selected set; admission
kiểm toàn bộ tập trong cùng giao dịch với Environment claim. REQUESTED lưu
cùng claim trước 202; engine chạy sau commit. Developer bị chặn trước body/plan;
foreign scope 404, thiếu service 503. Không provider/secret/CLI trong query.

Review sửa expiry ngoài token digest, role gate muộn, replay phụ thuộc graph
đã đổi, proof suy từ trạng thái hiện tại của operation khác, thiếu original
target/unavailable eligibility, region lấy nhầm Connection default, nil service
panic, dialog/detail giữ scope cũ, generation lỗi bị cast và success thiếu proof.
Token ghim expiry/nonce/actor/scope/versions nhưng không chứa private payload.
Replay đọc operation đã nhận trước planning, kể cả sau expiry/reopen/destroy;
đổi selected set trả stale. Proof xóa lấy từ DESTROY attempt đã SUCCEEDED của
đúng operation; không sửa kết quả cũ vì một operation sau xóa instance.

DESTROYED là tombstone, không RETAIN/UPDATE trong Xem trước khi khai báo lại;
stable instance/state history được giữ. Marker ActiveResource/current Set/
SUCCEEDED vẫn cùng giao dịch Console. Mirror sang instance chạy qua State dưới
owner sau commit; lỗi ghi số lượng an toàn, query/admission trả
METADATA_SYNC_PENDING và chặn xóa, không giả đồng bộ hoặc tự recovery. Metadata
state_key/module ID nội bộ không cấp quyền đọc blob/capability và không đi ra API.

## Checks của Codex

Checkout riêng code `8d213a3`, PostgreSQL 16 riêng, Terraform 1.9.8 thật:

- `REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 go test ./... -count=1 -timeout=30m
  -json`: **1171 PASS, 0 FAIL, 0 SKIP**; build/vet đạt.
- Frontend typecheck/lint/test/build: **609 tests, 35 files**, đạt.
- Docs với `REQUIRE_PLANTUML=1`, branch/working-tree diff check và `bash -n`
  runner đạt. Không có required local check bỏ qua.
- `/tmp/poc-i0408-codex-checks-r3/`: result/logs/cleanup; PostgreSQL container,
  mạng Docker và tệp mật khẩu đã xác minh không còn.

Lượt r1 `3aab6ca` đạt 1167 Go/0 skip và 606 frontend; r2 `18c387d` đạt 1168
Go/0 skip và 607 frontend. Giữ ở các thư mục r1/r2 tương ứng, không thay proof
bản cuối. Cloud handoff trước sửa cuối báo 1150 pass/18 Terraform provider-download
fail/0 skip; PlantUML thiếu. Cloud dùng Vault/xdotool stand-ins cho script tự kiểm,
không coi đó là real-Vault/real-headed proof. Kết quả cuối dùng local độc lập.

## Video, adapters và cleanup

Codex chạy runner Claude viết trên code `8d213a3`, headed Chromium/Xvfb,
xdotool thật, cursor/click/typing, phụ đề Việt dưới UI:

- `/tmp/poc-i0408-local-browser-r2/I04-08-captioned.mp4` và raw MP4.
- **125,6 giây, 18 mốc, 16 assertions**; decode/size/end mark/secret scan đạt.
- Root-only ban đầu, opt-in ORDERS-NET, SESSIONS không chọn, SHARED-NET bị khóa
  vì còn consumer; Hủy không tạo operation, tên sai bị chặn, stale phải review.
- UI xác nhận đúng ROOT + ORDERS-NET; verified success chỉ cho hai mục này.
  Các mục không chọn và LIVE-CACHE giữ nguyên. Partial failure không success;
  provider AUDIT-NET giữ vì consumer lỗi, remaining IDs được hiển thị.
- Reload/restart trên cùng SQL đọc lại đúng trạng thái và hai operation.
- REAL: UI/HTTP/services, tfexec engine, Terraform 1.9.8 SHA khớp release,
  encrypted state/HTTP backend và SQL Console/State roles, Vault 1.21 thật.
- FAKE: terraform_data objects của module test tự dựng, binder/readiness/
  namespace inspector/absence; topology dựng trước browser, Application fixture
  tạo qua API. Không provider/cloud/cluster hoặc workload thật trong task này.
- Scan 13 text files/7 giá trị bí mật không lộ Vault tokens, key mã hóa hoặc
  mật khẩu SQL. `cleanup.txt`: DB `i0408_c1378dad6084` và 3 roles đã drop,
  Vault `orch-i0408-20261014032315-1295` đã xóa, private directory đã xóa.
  `outer-cleanup.json`: PG container/mạng/tệp mật khẩu không còn.

Video r1 tại `18c387d` cũng pass, 127,6 giây; giữ tại
`/tmp/poc-i0408-local-browser-r1/`. DB/roles/Vault/PG/mạng/tệp riêng tư của r1
đã xác minh dọn sạch. Không tính các lượt này là V2-A01/A02/A03 live proof;
I04-10..12 còn wiring, job/cache và physical absence thật. IMP-021 còn mở.
