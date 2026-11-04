# Bootstrap acceptance — issues #30/#33

## Catalog schemaVersion 2

Chạy toàn bộ bootstrap và upgrade theo file dev hai lần, đọc lại trạng thái thật
trên Console/API và đối chiếu Application, Environment, config, workload drafts,
Catalog/Definition selections, resource IDs và deployment count. Business checks
và job/data retention do runner bootstrap xác minh qua browser thật.

Prerequisites: [bootstrap guide](../../../scripts/bootstrap/README.md), dev config
đã review, credential files riêng, Console/schema hiện hành, Kubernetes thật,
Vault/VSO/Fleet có sẵn và images cluster truy cập được. Runner chỉ nhận dev.

```bash
npm ci --prefix scripts/bootstrap
npm ci --prefix e2e
node e2e/scenarios/bootstrap-acceptance/run.mjs --config scripts/bootstrap/targets/dev.json
```

Mutation: cấu hình platform qua Console, GitHub repository CD do platform tạo,
namespace/workloads/PostgreSQL/Redis riêng của acceptance Application. Không sửa
shared controller/Vault, không reset DB, không tự cleanup. Chạy lại dùng cùng
identity/stateDir. Conflict/unknown outcome dừng để quan sát trước retry.

PASS cần cả hai lượt thành công và snapshot server khớp. Report chỉ ghi IDs/counts,
không lưu response có giá trị cấu hình. Evidence/state/credential ngoài Git.
Giữ ứng dụng và forward để người dùng thử. Cleanup là task riêng qua product UI.
Tests parser/reuse/conflict/unknown create nằm trong scripts/bootstrap/lib.
Resume/restart và từng loại Catalog/Definition upgrade phải ghi evidence riêng;
runner hai lượt không tự chứng minh toàn bộ các kịch bản này.

## Direct Definition / Generic Workload — issue #33

Dùng cùng lệnh với config schemaVersion 3 ngoài Git:

```sh
node e2e/scenarios/bootstrap-acceptance/run.mjs --config /private/bootstrap/dev.json
```

Nguồn chuẩn: [ADR-035](../../../orchestrator_docs/architecture/decisions/ADR-035-generic-workload-template-driver.md),
[contract](../../../orchestrator_docs/architecture/contracts/generic-workload.md),
[direct bootstrap](../../../orchestrator_docs/implementation/direct-bootstrap.md).
Cần `npm ci --prefix scripts/bootstrap`, `npm ci --prefix e2e`, Chromium,
Console/schema 8 thật và quyền chạy task-owned resources trên kind đã chỉ định.
Không dùng fake adapters, SQL seed hoặc direct workload apply.

Runner chạy platform-only bằng config loại bỏ application/developer/verification,
resume sang lệnh đầy đủ có acceptance ở cuối, rồi chạy lại cùng input. Helpers
bootstrap là implementation duy nhất; scenario chỉ điều phối và so sánh server
snapshots. PASS yêu cầu app không chọn Catalog, template rendering/CD/Fleet/app
checks đạt, source/Definition pins và platform/app/resource/deployment IDs giữ nguyên
qua unchanged rerun. Platform-only report BOOTSTRAPPED không thay acceptance PASS.

Side effects: đăng ký cấu hình còn thiếu, tạo acceptance Application, Git/CD/Fleet,
namespace PostgreSQL/Redis/PVC và job kiểm chứng; local forward có thể được giữ.
Evidence/config tạm/report/state đều ngoài Git. Không tự chạy staging/production,
không cài controller, không reset/upgrade shared DB, không tự xóa app/data/repo.
App và dữ liệu được giữ để thử/resume. Cleanup khi được yêu cầu phải qua product
remove/Preview/Deploy và Application deletion, kiểm tra đúng receipt app key,
namespace UID và ownership; scoped Role/RoleBinding chỉ xóa bằng UID preconditions.
Không xóa tài nguyên shared hoặc preexisting để làm trống platform.
