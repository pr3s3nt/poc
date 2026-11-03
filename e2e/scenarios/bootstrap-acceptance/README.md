# Bootstrap acceptance — issue #30

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
