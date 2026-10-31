# Catalog upgrade

Mục tiêu: kiểm tra một Environment staging nâng từ catalog v1 lên v2 qua Console,
Save chỉ lưu desired, Preview pin đúng release, Deploy toàn bộ thành công,
production giữ v1 và resource IDs cũ được giữ lại. Theo
[CR-01..06](../../../orchestrator_docs/usecase/UC-03/catalog-releases.md),
[contract](../../../orchestrator_docs/architecture/contracts/catalog-releases.md)
và [upgrade runbook](../../../orchestrator_docs/operations/catalog-releases.md).

Prerequisites: Console VI, account có quyền Application, nguồn đã đăng ký và hai
release ready, staging và production của Application riêng cho acceptance đã
apply bản nguồn, staging có workload/resource đang chạy; không có pending draft
khác. Cài dependencies và cấu hình auth theo [E2E guide](../../README.md).
Chạy chỉ trên môi trường mà người dùng đã cho phép mutation:

```bash
ORCH_HEADLESS=1 ORCH_CATALOG_APP=<app-key> ORCH_CATALOG_SOURCE=<source-id> \
ORCH_CATALOG_FROM=v1.0.0 ORCH_CATALOG_TO=v1.1.0 ORCH_CATALOG_DEPLOY=1 \
node e2e/scenarios/catalog-upgrade/run.mjs
```

Side effects: đổi desired/applied của staging và triển khai graph đã Preview;
giữ ứng dụng sau chạy. Runner không retry deploy thất bại hoặc rollback tự động.
Cleanup thuộc explicit Application/resource deletion hiện có với ownership/UID
và retained-storage guards; không có source/release deletion API. Không xóa app
production/shared để cleanup acceptance. Credential/session/evidence ở ngoài Git.

Assertions ở đây kiểm catalog và logical resource IDs; bổ sung application-level
reads/writes, PostgreSQL/Redis/PVC identities và jobs để nghiệm thu data/runtime.
Script tồn tại không chứng minh đã chạy cluster acceptance. Trường hợp moved tag,
manifest invalid, partial outcome, stale selection và migration dùng focused
unit/integration suites; run live chỉ khi external verification được cấp quyền.
