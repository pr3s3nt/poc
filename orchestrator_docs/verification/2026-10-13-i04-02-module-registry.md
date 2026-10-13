---
id: VERIFY-2026-10-13-I04-02-REGISTRY
artifact: verification-record
status: evidence
last_reviewed: 2026-10-13
related: [I04-02, I04-03, ADR-014, IMP-021]
---

# I04-02 — Registry module và xác minh độc lập

## Outcome

Registry hỗ trợ packaged module và HTTPS Git trong Admin allowlist, pin full
commit/digest, kiểm contract và lưu bundle bất biến. Registration ghi bundle
và Definition trong một transaction; unique pin từ chối hai digest cho cùng
revision/path. Preview/capability/planner chỉ đọc metadata, không fetch hoặc
chạy provider. Module con local và HTTPS Git được pin, cache và kiểm lại digest
trước execution. Inputs/outputs có mapping; output sensitive không bị hạ cấp.

Review bổ sung deep copy cho release và allowlist tại ranh giới Config/Release,
để caller không thay được provider hashes, nguồn Git hoặc packaged digest sau
validation. Test bỏ fix bắt lại lỗi, sau đó Claude khôi phục fix và kiểm thử.

Code chính: [registry](../../backend/internal/adapters/terraform/registry/),
[module domain](../../backend/internal/domain/tfmodule/),
[catalog](../../backend/internal/application/catalog/terraform.go),
[PostgreSQL bundles](../../backend/internal/adapters/postgres/modulebundles.go),
[planner metadata](../../backend/internal/planning/registry.go).
Inspector cũ xem `git show 04fda0f:backend/internal/adapters/terraform/inspect.go`.

## Validation và môi trường

Claude trước review fix: 882 PASS, 0 FAIL, 0 SKIP với REQUIRE_POSTGRES=1.
Sau fix: **884 PASS, 0 FAIL, 0 SKIP**, build/vet/focused tests và registry race
đạt. Evidence ở `/tmp/poc-i0402-registry/fix-go-test-all.json`,
`status.md`, `mutation-check.log` và các logs cùng thư mục.

Codex sau review fix: REQUIRE_POSTGRES=1 + server PostgreSQL 16 riêng,
`go test ./... -count=1 -json`: **884 PASS, 0 FAIL, 0 SKIP**;
`go build ./...` và `go vet ./...` đều exit 0. Evidence:
`/tmp/poc-i0402-review/tests.log`, `build.log`, `vet.log`, `result.json`.
SQL thật kiểm reopen, concurrent conflicting digest/rollback, immutability,
CHECK constraints và quyền Owner/Console/State. Test Git CLI dùng HTTPS server
local thật với CA của test; một số fetcher/CLI/provider là fixture trong unit
tests. Không lấy các tests này làm bằng chứng provision live.

Không chạy frontend checks hoặc Playwright/video vì task chỉ thay backend và
không thực hiện luồng Console. Form Definition còn source shape cũ, phải sửa
ở I04-07; chưa tuyên bố registration qua Console v2 thành công.
Không thay shell script. `REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`
đạt: 334 Markdown files, 286 artifact IDs; `git diff --check` đạt. ERD PNG
đã render lại từ source PlantUML bị ảnh hưởng.

## Cleanup

Codex đã xóa container `poc-i0402-codex-review-pg`, tmpfs database và credential
file riêng; cleanup exit 0, không còn container có label task. Evidence:
`/tmp/poc-i0402-review/cleanup-proof.txt`. Không sửa retained database hoặc
mutate cluster/cloud. Claude đã xóa cả `poc-i0402-pg` và `poc-i0402-pg2`, cùng
credential file; `/tmp/poc-i0402-registry/fix-cleanup-proof.txt` xác minh không
còn container của task. Retained containers được giữ nguyên.

## Ranh giới bàn giao cho task tiếp

- I04-03: durable HTTP state trong executor, private immutable invocation copy
  của binary/module, transitive installation và runtime output type checks.
- I04-03: provider bindings tạo graph edges và resolve scoped target; parsing/
  validation hiện có chưa thay thế executor resolution.
- I04-04/05: Kubernetes/ElastiCache modules và AWS packaged release mới. Pins
  hiện tại: Terraform 1.9.8 linux_amd64, AWS provider 5.80.0. SHA-256 đã pin;
  SHA256SUMS GPG verification không được tuyên bố đã chạy.
- I04-07: form source/mappings/class và Preview tương thích registry v2.
- Git private credentials chưa có vì đợt này dùng nguồn public HTTPS. Không
  tuyên bố hỗ trợ private Git; nếu cần phải dùng platform credential store.
- Inspector hiện hỗ trợ type subset đã chốt; chưa hỗ trợ module calls có
  count/for_each, object inputs hoặc `.tf.json`. Đây là giới hạn implementation,
  không phải thay đổi canonical requirements để hợp thức hóa code.

IMP-021 vẫn mở; live acceptance và gate 2 chưa đạt.
