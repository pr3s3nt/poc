---
id: VERIFY-2026-10-13-I04-03-EXECUTOR
artifact: verification-record
status: evidence
last_reviewed: 2026-10-13
related: [I04-03, I04-04, ADR-014, IMP-021]
---

# I04-03 — Executor và xác minh độc lập

## Outcome

Executor dùng registered modules, durable HTTP state, stable identity/password
refs, scoped AWS/Kubernetes/EKS bindings và provider dependency edges. Binary,
module và transitive sources được copy riêng, kiểm digest trước execution.
UPDATE destructive bị chặn; failed mutation chưa có state acknowledgement hoặc
outcome chưa rõ giữ Environment hold. Process waits và retry được giới hạn.

Destroy dùng historical release/target, kiểm ownership/consumer/generation và
reverse graph. Namespace kiểm UID, owner chain, defaults và foreign contents.
Failed consumer giữ provider; cleanup run không xóa preexisting/platform hoặc
tài nguyên do run khác cập nhật. Remaining IDs được giữ khi chưa chứng minh hết.

Review bốn vòng bổ sung các regression trên và sửa retry cleanup: bản state
trước destroy được lưu trong artifact mã hóa riêng tư trước effects. Retry và
restart vẫn query original IDs khi terminal state đã rỗng; mất hoặc không đọc
được artifact thì chặn. Không lấy destroy exit 0 làm bằng chứng physical absence.

## Validation và bằng chứng

Codex chạy `REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 go test ./... -count=1 -json`
trên PostgreSQL 16 disposable riêng: **970 PASS, 0 FAIL, 0 SKIP**.
`go build ./...` và `go vet ./...` exit 0. Evidence:
`/tmp/poc-i0403-final-review/tests.log`, `build.log`, `vet.log`, `result.json`.
Lượt review trước có 959 tests; không dùng kết quả cũ thay final validation.

Claude vòng 4: full tests/build/vet đạt; focused race runner/recovery/readiness/
envops/tfexec đạt. Logs tại `/tmp/poc-i0403-executor/logs/full-8.log` và
`race5.log`. Một heartbeat test lỗi khi full suite chạy đồng thời với race,
sau đó đạt khi chạy riêng; các logs fail `full-6.log`, `full-7.log`, `race4.log`
được giữ. Codex final suite chạy riêng và không gặp lỗi này.

Actual Terraform CLI 1.9.8, PostgreSQL và HTTP state backend được dùng cho local
fixtures terraform_data/output-only. Physical provider/readiness trong một số
tests là seam fake; restart test dùng store fixture, encrypted recovery adapter
được kiểm riêng. Không tuyên bố live AWS/k8s provision hoặc live absence proof.

Không chạy frontend checks/video vì task chỉ thay backend, không có Console
scenario. Không thay shell scripts hoặc PlantUML sources. Documentation checker
với `REQUIRE_PLANTUML=1` và `git diff --check` được chạy trước commit.

## Cleanup và task tiếp

Codex đã xóa `poc-i0403-codex-final-review-pg`, tmpfs data và credential file;
cleanup exit 0, không còn container label của lượt chạy. Evidence:
`/tmp/poc-i0403-final-review/cleanup-proof.txt`. Claude báo PG `i0403-pg` và
credential files đã dọn. Không mutate retained DB, cluster hoặc AWS.

I04-04/05 còn packaged Kubernetes/AWS modules và release pins; I04-06 phải dùng
AWS credential JSON contract của resolver. I04-07/08 còn form/class/rendering
và selected-set Delete UI/API. Redis app/integration/live scenarios thuộc các
task sau. IMP-021 và gate 2 vẫn mở; AWS provisioning chưa được phép.
