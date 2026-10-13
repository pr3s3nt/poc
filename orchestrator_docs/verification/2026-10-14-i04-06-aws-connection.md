---
id: VERIFY-2026-10-14-I04-06-AWS-CONNECTION
artifact: verification-record
status: evidence
last_reviewed: 2026-10-14
related: [I04-06, I04-07, UC-04, ADR-009, ADR-014, IMP-021]
---

# I04-06 — AWS Connection, kiểm chứng local độc lập

## Kết quả và review

Claude cloud session `session_01UowTTQea8MqLBRaPktRfSX` viết code/test trên
`claude/m04-i04-06`, base `9b00978`; bản cuối `f3ed5bb` đã được Codex review.
[Phiên cloud](https://claude.ai/code/session_01UowTTQea8MqLBRaPktRfSX).
Tên mode quyền nội bộ không đọc được; Bash/Write/Edit/network quan sát chạy
không prompt hoặc blocker. Không suy quyền từ cờ CLI. Nhánh tự sinh
`claude/m04-i04-06-e80y97` có cùng commits.

PE/Admin đăng ký AWS bằng các ô che, strict bounded payload, identity/regions/
capabilities chỉ đọc, scoped immutable Vault object rồi insert READY. Lỗi/đua
không overwrite winner, rollback chỉ xóa object của attempt; Kubernetes flow
và Organization default được giữ. DTO/log/Preview không có key hoặc secret ref.
Allowed regions đi nhất quán qua persisted metadata, target và Terraform binder;
metadata malformed không fallback. Root/temporary key không được initial verifier
hỗ trợ; long-term IAM user cần quyền mô phỏng policy của chính mình.

Review sửa incomplete verification result, missing-context/paging/region checks,
redirect credential destinations và raw Vault dev log. Sửa cuối: IAM global
simulated một lần với `aws:RequestedRegion=us-east-1`, các service regional theo
từng allowed region; có tests cho false READY và false rejection. Endpoint IAM
và condition context theo [AWS documentation](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_condition-keys.html#condition-keys-requestedregion).
Registration không tạo AWS infrastructure; execution capability preflight còn
được tích hợp tại I04-10, không chặn destroy chỉ vì create permission đổi.

## Checks của Codex

Tại checkout riêng `f3ed5bb`, PostgreSQL 16 riêng, Terraform 1.9.8 và score-k8s
thật: `REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 go test ./... -count=1
-timeout=30m -json`: **1082 PASS, 0 FAIL, 0 SKIP**. `go build ./...`,
`go vet ./...` đạt. Frontend typecheck/lint/test/build đạt: **479 tests**, 26 files.
`REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`, `git diff --check` và
`bash -n backend/test/integration/aws-connection-local.sh` đạt.

Logs/result: `/tmp/poc-i0406-codex-checks-r2/`. Cleanup container, private network
và password file được xác minh trong `cleanup.json`. Lượt r1 tại `dac70e7`
1080 tests/0 skip cũng pass, được giữ ở `/tmp/poc-i0406-codex-checks-r1/` nhưng
không thay kết quả bản cuối. Timeout toàn Go suite 30 phút chỉ tránh giới hạn
mặc định 10 phút của harness; không thay runtime deadlines/assertions.

Claude báo full Go còn 5 Terraform tests lỗi do registry network và 9 score-k8s
skips; không dùng làm evidence cuối. Codex đã chạy lại tất cả, không skip.
Cloud frontend count 188 là subset bị ghi nhầm; corrected handoff xác nhận
full run 479. Code/check scope thực tế không bị giảm để đạt.

## Browser và cleanup

Codex chạy runner Claude viết, headed Chromium/Xvfb, cursor/click/paced input,
phụ đề Việt dưới cửa sổ. Product steps qua UI; API read/negative role assertion
chỉ phục vụ kiểm chứng. Bản cuối:

- Video: `/tmp/poc-i0406-local-browser-r2/I04-06-captioned.mp4` và raw MP4.
- Captions/marks/assertions/checks: cùng thư mục, **197.13 giây, 15 mốc**.
- Codec/full decode, 1440x900 raw/1440x996 captioned, frames và captions review đạt;
  khóa được che, clear sau lưu, không secret ở frame/retained text.
- Developer bị từ chối; local invalid không POST; rejected key, thiếu/unprovable
  capabilities và disabled region không lưu; success/list/reload safe metadata.
- Vault có đúng một credential object scoped, KV v2 version 1, không overwrite;
  các failed attempts không để credential. Fake AWS calls chỉ đọc (40 calls).
- `cleanup.txt`: DB `i0406_ff06472b2b74` và 3 roles đã drop, Vault container
  `orch-i0406-20261013191510-26333` đã xóa, private credential directory đã xóa.
- `outer-cleanup.json`: PostgreSQL container/mạng của run không còn, credential
  files không còn. Process quay/backend/Xvfb đã dừng theo runner handles.

REAL: UI/HTTP/service, PostgreSQL, Vault KV v2 và SigV4 verifier code.
FAKE: AWS HTTP endpoints và workload executors. Docker Vault 1.21 path được
Codex kiểm thật; không dùng retained Vault. Video r1 cũng pass, giữ tại
`/tmp/poc-i0406-local-browser-r1/`, không thay bản cuối.

Không AWS apply, không k8s mutation, không V2-A01/A02/A03 live proof trong task
này; các live checks nằm ở I04-11/12. Không có required local check bị bỏ qua.
Retained DB/Compose/k8s-4f/PVC/apps/Vault và user-owned changes được giữ nguyên;
working tree sạch khi bắt đầu. I04-07 tiếp theo; IMP-021 vẫn mở.
