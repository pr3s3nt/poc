---
id: VERIFY-2026-10-13-I04-04-KUBERNETES
artifact: verification-record
status: evidence
last_reviewed: 2026-10-13
related: [I04-04, I04-05, ADR-014, IMP-021]
---

# I04-04 — Modules Kubernetes và secret binding

## Outcome

Packaged `k8s-namespace`, `redis-k8s`, `postgres-k8s` được pin trong registry
release; Kubernetes provider 2.38.0, Redis/PostgreSQL images dùng digest.
Redis dùng ACL/password, PostgreSQL dùng password authentication; outputs
khớp Type contract và password bí mật. StatefulSet, Service, Secret và PVC
thuộc state; stable names từ executor tag và password từ stable secret ref.
Provider không chờ pod/PVC readiness trong create; executor kiểm sau state ack.

Review phát hiện secret output có thể feed annotation không bí mật. Bản sửa
chặn Type-secret ở registration, Type/module-secret ở planning và thêm runtime
sensitive flag trước lưu inputs/chạy consumer. Tests gồm nested/map/list/chuỗi
ghép, module-only classification, không gọi consumer và không lộ vào metadata.
Nonsecret PostgreSQL host binding vẫn tạo graph fixture cho A01 selected set.
Không đổi behavior của static Kubernetes-driver Definitions.

## Checks và real/fake boundary

Codex độc lập: `REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 go test ./... -count=1
-json` trên PostgreSQL 16 disposable: **1021 PASS, 0 FAIL, 0 SKIP**;
`go build ./...`, `go vet ./...` exit 0. Evidence:
`/tmp/poc-i0404-review/tests.log`, `build.log`, `vet.log`, `result.json`.

Claude vòng sửa 2 cũng báo 1021 PASS, không fail/skip, build/vet đạt. Logs:
`/tmp/poc-i0404-kubernetes-modules/evidence/round2-tests.json`,
`round2-result.txt`, `round2-validate.txt`. Readonly init/validate cả ba modules
đạt; Go tests chạy actual Terraform CLI init/validate/offline plan, registry và
PostgreSQL thật. Provisioning executor/verifier có scripted fake trong tests.
Claude có local Docker image/auth proof, giữ báo cáo trong `status.md`.

Không apply trên cluster/cloud; chưa chứng minh live pod/PVC/storage class hoặc
cleanup backing volume. Không frontend checks/video vì module-only task không
có browser flow. Live headed scenario thuộc I04-11. Không đổi shell/PlantUML
source; docs checker với REQUIRE_PLANTUML=1 và diff check chạy trước commit.

## Cleanup và carryover

Codex container `poc-i0404-codex-review-pg`, tmpfs database và credential file đã
dọn; exit 0, không còn container label task. Evidence:
`/tmp/poc-i0404-review/cleanup-proof.txt`. Claude báo xóa toàn bộ Docker test
containers/volumes/network và password files của task. Evidence/cache public
provider binaries được giữ; không sửa retained DB hoặc k8s/AWS assets.

I04-05 còn AWS modules; I04-07/11 đăng ký Redis Type/Definitions qua UI, tránh
seed trùng criteria cũ. Storage class và readiness timeout 5 phút cần kiểm ở
live scenarios, không coi validate thành provision proof. Gate 2 chưa đạt và
AWS provisioning chưa được phép.
