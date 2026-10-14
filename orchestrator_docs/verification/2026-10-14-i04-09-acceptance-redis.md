---
id: VERIFY-I04-09-20261014
artifact: verification-record
status: historical
last_reviewed: 2026-10-14
related: [I04-09, M04, IMP-021, ADR-014]
---

# I04-09 — Redis cho ứng dụng kiểm chứng

Code `65223ef`, handoff message-only `67e26a7` (cùng tree), đã review và merge
`45006fd`. Phiên [session_019GgofsetFjxTDq6V83vv6Q](https://claude.ai/code/session_019GgofsetFjxTDq6V83vv6Q)
quan sát Read/Write/Edit/Bash/git push chạy; network qua proxy, không introspect
mode nội bộ. Hai nhánh `claude/m04-i04-09` và `-m1sahg` đã xóa sau push `6907de3`;
remote query xác minh vắng mặt, checkout review đã dọn.

## Kết quả và phạm vi

Go frontend/backend/worker dùng Redis chung và PostgreSQL bền vững. Worker
commit DB rồi write-through; reader hit/miss/DB/refill với key app/env/run/job,
TTL ngắn, không cache pending. Auth/TLS/CA/hostname/SNI bắt buộc, public roots
nhúng cho scratch, DSN PostgreSQL escape đúng ký tự. Giữ UI job và thêm dòng
cache trong bảng kiểm tra hiện có, không thêm màn hình riêng.

Review đã sửa ba lỗi false-pass: readiness bỏ qua marker FAILED/thiếu; PING đạt
nhưng cache ACL từ chối thao tác; job B thành công xóa lỗi của job A. DONE thiếu
marker chưa xác minh, FAILED giữ trong run, không xử lý lại hoặc duplicate.
Probe key TTL tối đa10s; failure theo job+operation tối đa64, overflow fail closed,
recheck tối đa16 trong deadline và không tăng bộ đếm hoặc xử lý lại job.
Bộ đếm process-local là giới hạn rõ; live runner còn đọc khóa/TTL Redis thật.
Public CA bundle đã pin bản tương thích Go1.25 (20250305); TLS AWS thật chưa
được kiểm ở task này. Root trust fixtures và thiếu system CA đã được kiểm local.

## Kiểm tra độc lập local

`/tmp/poc-i0409-codex-checks-r3/`: **1254 Go PASS/0 FAIL/0 SKIP** với
`REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 REQUIRE_REDIS=1`, Go build/vet;
frontend typecheck/lint/**609 tests/35 files**/build; docs với
`REQUIRE_PLANTUML=1`, diff-check và bash-n script thay đổi đều đạt.
PostgreSQL16 Docker cô lập và Redis8.0.5 native là adapter thật: TTL/refill,
post-commit cache failure/no duplicate, auth, trusted TLS/wrong CA/hostname,
ACL read/write, nhiều job không che lỗi, migration đồng thời và marker.

Cloud: riêng app84 pass/0fail/0skip trên PG16+Redis7.0.15. Full Go cũ1205pass/
18fail/21skip do provider download/score-k8s thiếu không được tính đạt;
PlantUML cloud thiếu không tính kiểm tra diagram đạt. Kết quả local thay thế
bằng kiểm tra đầy đủ, không bỏ qua. R1 `419478c`:1244Go; r2 `7984f1f`:1249Go,
fullchecks đều đạt nhưng chỉ bản cuối đã sửa toàn bộ findings được merge.

Không dựng video giả cho unit/CLI. Nghiệp vụ browser thật thuộc I04-11/12;
không task-owned Kubernetes/AWS resource hoặc v2 live proof ở I04-09.

## Ảnh thật và cleanup

Sáu ảnh scratch đã build từ code65223ef cho amd64/arm64, unique tags/content IDs
và manifest digests thật ghi `/tmp/poc-i0409-image-build/images.json`. Digest
thu từ push vào registry local tạm thời; live target publication và kiểm tra
matching digest thuộc I04-10..12, không giả local imageID là registry digest.

| Kiến trúc | Workload | Manifest digest |
|---|---|---|
| amd64 | acceptance-frontend | `sha256:e06d251281a53cbab5a7eed2f4764398040feb4daf4c645b9e2c10a3ad738eff` |
| amd64 | acceptance-backend | `sha256:f5c61b7d55accf19cbc22ab3b31ce622541cb2e652126ac601a9e4c5295589bf` |
| amd64 | acceptance-worker | `sha256:8d0500e061c63c3028b0ce976d8c111f77ca6cecad4b08abae02e70039cc9bfa` |
| arm64 | acceptance-frontend | `sha256:fe2513df913b92bc60aa66ddf420cbbcf5f8233b63a42158913c281dca159e1f` |
| arm64 | acceptance-backend | `sha256:bd6b8d17189f5b9a9d1fd82d2c6f356c48c9e80b5871a112c9748e621ceb5c1b` |
| arm64 | acceptance-worker | `sha256:eedf635439402e6ab98c8ef0fdba8e2959bda858499a1668e2f4183c04731bc3` |

Registry local/private Docker config đã xóa: `registry-cleanup.json` cả hai
absence true. Unique local images/binaries giữ để tích hợp, không chứa secret.
PG/network/password file cả ba lượt absent trong `cleanup.json`; Redis fixture
directories và native child processes uid1000 absent trong `redis-cleanup.json`.
Redis uid999 có sẵn giữ nguyên. Không đọc credential môi trường, kubeconfig hoặc
state để gửi cloud. Evidence ngoài Git; log password PG đã redacted.

Tiếp theo I04-10: Score/Definition bind Redis và cache scope cho backend/worker;
seed/kind v1 chỉ bind PG hiện báo cache fail đúng, không được coi live-ready.
