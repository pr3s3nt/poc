# Bootstrap platform và acceptance app

Issue #30 mở rộng bootstrap cũ thành luồng từ Console trống đến acceptance app
frontend/backend/worker dùng chung PostgreSQL và Redis. Toàn bộ mutation nghiệp
vụ qua Console; API/native reads xác minh hiện trạng, không dùng fake adapters.

## Chạy

```bash
npm ci --prefix scripts/bootstrap
npx --prefix scripts/bootstrap playwright install chromium
node scripts/bootstrap/run.mjs --config scripts/bootstrap/targets/dev.json
```

Mọi đầu vào nằm trong file; không lấy runtime overrides từ environment. Các file
`targets/dev.json`, `staging.json`, `production.json` có cùng schemaVersion 2.
Hiện chạy thật dev; staging/production là mẫu phải thay endpoint/account/repo/image
trước khi dùng. Credentials chỉ là đường dẫn file 0600/0400 ngoài Git, có hỗ trợ
`~/`; đường dẫn tương đối được resolve từ thư mục config.

`console` và `developer`: URL, username, passwordFile. `platform`: Git scope/tokenFile,
account/context/backend kubeconfig, Types, Git Catalog, Vault scoped token và CD.
`application`: name/subdomain ổn định, Environment đích, variables, secretFiles,
Score paths, image versions, Catalog tag và optional Definition revision IDs.
`upgrade`: enabled (mặc định true), images/tag/revisions đích rõ ràng.
`verification`: URL Tailscale, payload job cố định, timeout; optional local forward.
`runtime`: mode, headless, timeout, host cluster kubeconfig, stateDir, reportDir.

Definitions, Terraform modules và Rendering Templates đến từ Git Catalog đã pin.
Config không chứa nội dung Definition/template. Score acceptance dùng chung từ
`e2e/scenarios/acceptance-app/scores/` để tránh lệch fixture.

## Các chế độ

Đặt `runtime.mode` trong file: `all` (mặc định trong presets), `check` (validate và
prerequisite reads, chỉ tạo auth sessions), `bootstrap` (platform), `acceptance`
(baseline app), `upgrade` (đích khai báo), `verify` (read-only app checks).
Mọi bước bật sẵn trong `all`. `upgrade.enabled=false` giữ pin đang có, không tự
lấy latest. `verification.enabled=false` không được gọi là PASS nghiệp vụ.

Upgrade dùng diff/blockers/CAS/Preview/Deploy của sản phẩm. Dữ liệu/PVC tuân thủ
RETAIN guards, không thực hiện migration/rollback dữ liệu. Không quay về baseline
khi đã ở phiên bản đích; config không phải lệnh downgrade.

## Prerequisites và phạm vi

Console phải chạy với adapter thật và database setup đúng phiên bản. Platform
trống nghĩa là chưa có dữ liệu nghiệp vụ, không cần xóa database/volumes.
Kubernetes, Vault Kubernetes auth, VSO, Fleet, Git read Secret/service account/RBAC (gồm quyền patch đúng release Namespace),
registry/images và network là prerequisites hiện hành. Script kiểm tra và báo
BLOCKED nếu thiếu; không tự cài đè controller hoặc cấp cluster permissions.
`platform.delivery.namespaceAccess` bật sẵn, đọc rulesFile để tạo Role/RoleBinding
chỉ trong namespace do product tạo cho acceptance app. Không tạo Namespace, không
grant ClusterRole/ClusterRoleBinding, không thay RBAC của app khác. Đây là technical
setup được ghi riêng trong report; mọi thao tác nghiệp vụ vẫn qua Console.
Backend phải truy cập Kubernetes/Vault/Git; host cần kubeconfig dùng cho native
reads/port-forward. Dev Compose dùng thêm `deploy/local/compose.kind.yml` và
`deploy/local/compose.bootstrap.yml` (bật CD và giữ delivery Git volume).

Git credential cần scope Catalog và quyền repository CD theo cấu hình sản phẩm.
Secret Store dùng scoped token, không dùng root. Chuẩn bị application secret một
lần trong file riêng; script tính ACCEPTANCE_SECRET_SHA256 từ file đó.

## Chạy lại và resume

Đọc hiện trạng trước ghi; khớp thì REUSED, thiếu thì CREATED, upgrade rõ thì UPDATED.
Bất đồng cấu hình là BLOCKED, không ghi đè. Không đổi credential/secret tự động.
Catalog chỉ refresh khi release yêu cầu chưa có. Không deploy nếu Preview không
có thay đổi. Operation còn chạy được quan sát. `runtime.recoverCompletedDeploy` bật mặc định
nhưng chỉ recovery lượt deploy của chính runner đã nhận response FAILED và xác
minh reviewRequestId trên server. Request mất kết quả không chứng minh executor
đã dừng: trường hợp đó vẫn cần operator recovery qua Console.
Unknown mutation không tự replay trong cùng lượt; lượt sau đọc lại server.

State directory ghi identity receipt, secret hashes và PID forward; không thay
server làm nguồn xác nhận. Dùng cùng stateDir để resume. Muốn dùng Application
đã có từ máy khác, chỉ định `application.expectedKey` và giữ receipt secret phù hợp;
trùng tên không đủ quyền nhận quản lý. Local lock ngăn hai writer cùng stateDir;
product CAS/ownership tiếp tục bảo vệ server. Không sửa pending changes khác.

Report từng bước ở reportDir, không chứa secret; app screenshot không chứa form
credential. Logs/checkpoint/evidence ngoài Git. Forward optional được giữ sau
khi runner đóng browser; PID và log ở stateDir. Không tự xóa app/data/repository.

## Kiểm chứng

```bash
npm --prefix scripts/bootstrap test
node e2e/scenarios/bootstrap-acceptance/run.mjs --config scripts/bootstrap/targets/dev.json
```

Xem [E2E scenario](../../e2e/scenarios/bootstrap-acceptance/README.md) và
[execution flow](EXECUTION_FLOW.md). Cần evidence thật cho fresh, unchanged rerun,
resume, conflict, upgrade bật/tắt, rerun sau upgrade và platform restart; unit tests
không thay runtime. Link bàn giao phải qua [Tailscale](../../orchestrator_docs/operations/tailscale-handoff.md).

## Legacy Catalog-only

Luồng cũ giữ ở `legacy-run.mjs`, helper/presets ở `legacy-targets/`:

```bash
node scripts/bootstrap/run.mjs --target dev --url https://console.example \
  --kubeconfig /private/kubeconfig --git-url https://git.company/catalog.git
```

Luồng legacy chỉ bootstrap Catalog, không deploy acceptance. Dùng config v2 cho
full bootstrap; không dùng legacy environment overrides với config v2.


CD configuration follows issue #31: `platform.delivery.config` only selects
`workspace`, `targetCluster` or an existing `gitConnectionKey`. Internal repository
and Fleet fields are derived by the Platform; presets no longer author them.
Shared Git uses `usePlatformGit=true`. For a separate new Git Connection, set
`usePlatformGit=false`, `writerURL` to its HTTPS account/group scope, plus
`writerUsername` and the private `writerPasswordFile`. Bootstrap registers that
connection through the ordinary inline CD form. Existing infrastructure accounts,
bindings and their applied consumers remain intact.
