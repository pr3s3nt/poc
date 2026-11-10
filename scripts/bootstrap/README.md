# Bootstrap platform và acceptance app

## Chỉ seed Resource Definitions lần đầu — issue #62

Input hiện hỗ trợ seed cả Resource Types trước Definitions (issue #65).
Kubernetes Driver Account và Git settings phải có sẵn. Copy
[input mẫu](definitions.example.yaml) ra ngoài Git, `chmod 600` và điền Console,
file password, Organization, `parameters.driverAccountKey`, tên và điều kiện
matching. Không cần khai báo lại Git credential hoặc các prerequisites này.

```sh
npm ci --prefix scripts/bootstrap
node scripts/bootstrap/seed-definitions.mjs --config /private/definitions.yaml
```

Input mẫu dùng `pr3s3nt/orchestrator-workload-templates` tại exact commit và tạo
tám Types cùng tám Definitions: cluster, namespace, PostgreSQL, Redis, MySQL,
storage, workload, endpoint. `resourceTypes` khai báo `key`, `inputs`, `outputs`; mỗi field
có `name`, `type` (`string`, `number`, `bool`, `any`), optional `required` và
`secret` cho outputs chứa bí mật. Xóa entry không cần. Bỏ `resourceTypes` để
dùng input cũ với Types đã có. `repository.url` là URL Git
duy nhất: khi mirror nguyên repo/commit sang Git nội bộ, chỉ thay URL đó. Nếu
commit của bản nội bộ khác, sửa thêm `repository.ref`. Backend dùng Git settings
hiện có để xác thực và trust CA; script không yêu cầu GitHub CLI/token riêng,
không sửa Git settings và không tắt TLS verification.

`modulePath`/`templatePath` nằm cạnh các trường Definition JSON thông thường.
Script inspect qua backend, lấy source/module IDs, exact revision, digest hoặc
template ID rồi điền vào Definition; không tự tính hoặc hardcode pins. Chỉ
`${seed.NAME}` được thay bằng `parameters.NAME`; expressions của sản phẩm giữ
nguyên. Inputs/outputs, biến và provider bindings còn lại khai báo trong YAML.
MySQL lấy `image` từ resource param; không đưa password vào input. Có thể thêm
output PostgreSQL `asyncUri` vào mapping nếu Type đã khai báo output đó.

Endpoint dùng driver có sẵn `endpoint`, không có module/template Git hoặc
`driverAccount` riêng. Điền `parameters.endpointRootDomain`, `endpointScheme`,
`gatewayName`, `gatewayNamespace`, `gatewaySectionName` theo Gateway/listener
thực tế của Traefik. Ví dụ mẫu là dữ liệu cần thay, không xác nhận Gateway đó
đã có. Muốn bỏ listener tùy chọn, xóa `gatewaySectionName` trong variables của
Definition; có thể thêm `reservedNames: [www, api]` để bảo vệ tên ngoài platform.
Type `endpoint` nhận `name`/`port`/optional `path`, xuất `hostname`/`url`.
Gateway API, Gateway cho phép HTTPRoute từ namespace app, wildcard DNS/TLS là
prerequisites. Seed chỉ lưu Type/policy; HTTPRoute được backend sinh khi deploy
workload có endpoint. Không tự cài/sửa Traefik, Gateway, DNS/TLS hoặc tạo route
trong bước seed. Xem [endpoint contract](../../orchestrator_docs/architecture/contracts/endpoint.md).

Đây là seed lần đầu qua API, không ghi SQL trực tiếp: tạo mục thiếu, dùng lại
mục khớp, khác thì CONFLICT. Kiểm tra mọi Type conflict và mapping trước khi tạo
Type; tạo và đọc lại Types trước khi inspect. Tất cả conflicts trên Definitions
được kiểm trước khi tạo Definition mới. Types đã tạo và metadata/cache từ inspect
được giữ ngay cả khi phase Definition bị chặn; các bước đã thành công không tự
rollback. Type có contract cũ không tự đổi sang contract mới. Không tự retry POST khi
kết quả không rõ, không sửa/promote revision hoặc deploy ứng dụng. Chạy một
importer cho mỗi target. Thành công báo `Definitions SEEDED`, không phải nghiệm
thu workload.

## Local API seed (không browser)

Cho Console chạy trong `kind-k8s-4f`, dùng [runbook](../../orchestrator_docs/operations/platform-seed.md)
và [YAML mẫu](api-dev.example.yaml). Copy input/credentials ra ngoài Git, đặt quyền
0600, rồi chạy từ checkout này:

```sh
node scripts/bootstrap/seed-api.mjs --config /private/dev.yaml
```

Lệnh đọc file local, gọi API backend, tạo mục thiếu và thoát. Không tạo seed Job
hay upload input thành ConfigMap/Secret; credential vẫn được backend lưu vào Vault
để sử dụng sau đó. Chạy lại tái sử dụng dữ liệu khớp, khác thì CONFLICT, không tự
ghi đè hoặc rotate. API seed platform không tạo app/deploy workload. AWS cần file
credential riêng; không dùng EC2 instance role trên kind. Các workflow Console và
acceptance bên dưới giữ nguyên.

Luồng [Definitions trực tiếp — issue #33](#definitions-trực-tiếp--issue-33-schemaversion-3)
nhận config schemaVersion 3 theo môi trường, có tính lũy đẳng và deploy acceptance
app ở cuối. Dùng `direct-targets/dev.json` làm mẫu; input thực tế nằm ngoài Git.

Phần Catalog schemaVersion 2 được giữ tương thích dưới đây. Issue #30 mở rộng bootstrap cũ thành luồng từ Console trống đến acceptance app
frontend/backend/worker dùng chung PostgreSQL và Redis. Toàn bộ mutation nghiệp
vụ qua Console; API/native reads xác minh hiện trạng, không dùng fake adapters.

## Chạy Catalog schemaVersion 2

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
`application`: name ổn định, Environment đích, variables, secretFiles,
Workload paths, image versions, Catalog tag và optional Definition revision IDs.
`upgrade`: enabled (mặc định true), images/tag/revisions đích rõ ràng.
`verification`: URL Tailscale, payload job cố định, timeout; optional local forward.
`runtime`: mode, headless, timeout, host cluster kubeconfig, stateDir, reportDir.

Definitions, Terraform modules và Rendering Templates đến từ Git Catalog đã pin.
Config không chứa nội dung Definition/template. Workload acceptance dùng chung từ
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

## Staging qua Console

Schema 3 mặc định `runtime.profile=console`. Dùng
[enterprise setup kit](enterprise/README.md) và `prepare-enterprise-input.mjs`
để chuẩn bị local documents, sau đó Playwright thao tác qua Console. Runner
không cần git/kubectl/gh CLI, không cấp RBAC hay port-forward. Backend vẫn lấy
module/template từ Git và truy cập Kubernetes. State/report của Console và
`dev-test` phải riêng; các native checks bên dưới chỉ dành dev-test.

## Prerequisites và phạm vi

Console phải chạy với adapter thật và database setup đúng phiên bản. Platform
trống nghĩa là chưa có dữ liệu nghiệp vụ, không cần xóa database/volumes.
Kubernetes, Vault Kubernetes auth, VSO, Fleet, Git read Secret/service account/RBAC (gồm quyền patch đúng release Namespace),
registry/images và network là prerequisites hiện hành. Script kiểm tra và báo
BLOCKED nếu thiếu; không tự cài đè controller hoặc cấp cluster permissions.
Trong explicit `runtime.profile=dev-test`, `platform.delivery.namespaceAccess`
đọc rulesFile để tạo Role/RoleBinding
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

## Definitions trực tiếp — issue #33 (schemaVersion 3)

```sh
node scripts/bootstrap/run.mjs --config /private/bootstrap/dev.json
```

Bắt đầu từ [dev input](direct-targets/dev.json) hoặc
[staging input](direct-targets/staging.json); copy ra ngoài Git và sửa endpoint,
account, credential file references, Definition files, source pins, images và URL
Tailscale cho môi trường của bạn. Hai ví dụ dùng Console/cluster riêng và stateDir
riêng. `environment` là identity của lượt bootstrap, không thay thế Environment
`staging`/`production` của sản phẩm. Script không tự cấp quyền chạy staging.

`runtime.mode=all` chạy **platform → acceptance app → xác minh CD/Fleet và app/job**.
Acceptance là phase bắt buộc ở cuối; chỉ báo SUCCEEDED khi toàn luồng đạt.
`bootstrap` chỉ chuẩn bị platform và báo BOOTSTRAPPED; không cần `application`,
`developer`, `upgrade`, `verification`, image, Workload hay Catalog input.
`check` chỉ validate/prerequisite reads và auth sessions, không bootstrap.
V3 không chạy upgrade matrix hoặc chọn upgrade revisions; config v2 và legacy CLI
vẫn được giữ.

V3 bỏ `platform.catalog` và `application.catalogTag`; dùng:

- `platform.templateSources`: tên và nguồn `git`, `zip`, `inline` hoặc `installed`,
  với `pin.revision` và `pin.contentSha256`. Git dùng full commit trong `ref`;
  ZIP có `file`, `archiveSha256`, optional `path`; Inline dùng `filesFile` chứa
  map tên file → content. Installed phải tồn tại và AVAILABLE.
- `platform.definitions`: mỗi entry trỏ `file` JSON của Resource Definition.
  Terraform entry có `source` gồm `kind` và full `pin` (sourceId, moduleId,
  revision, subdirectory, contentSha256); pin phải khớp `driverInputs.values.source`.
  Các nguồn Git/ZIP được inspect/prepare qua Console. Installed pin được kiểm tra
  từ capabilities trước mutation. Ví dụ kind dùng verified packaged Terraform
  modules cho namespace, PostgreSQL và Redis; storage tùy input, không tự thêm.
- `${account.key}` trong `driverAccount` được resolve thành đúng account vừa
  xác minh; `${templateSources.acceptance}` trong `template_source` trỏ source
  đã pin. Resource expressions của sản phẩm được giữ nguyên.
- `application.workloads`: ba Generic Workload files backend/worker/frontend,
  cùng image pins, variables, secretFiles, name và Environment đích.
  Workload fixtures hiện hành được dùng chung, không có bước Score/render bundle.

Inline hỗ trợ đúng bộ file Console cho phép sửa: `deployment.tpl`, `service.tpl`,
optional các `.tpl` thêm, `inputs.schema.json`, `defaults.yaml` và `bundle.yaml`
của editor `custom-workload`. Manifest khác hoặc có normalization/patches dùng
Git/ZIP. Nội dung file/template nằm ngoài runner; local files được kiểm tra pin.
Typed template inputs theo schema, criteria, references, provider/account mappings
và companion resources đi qua form Definition hiện hành; review JSON phải khớp
input trước khi lưu. Product giữ precedence của explicit Workload values và
bảo vệ Secret references; không truyền secret bytes qua template inputs.

Chạy lại cùng input giữ nguyên source IDs, Definition revisions, account/CD,
acceptance Application, resource/PVC UIDs, deployment IDs và Fleet commits.
Receipt pin cả active revision ID/generation và semantic acceptance input (gồm
hash nội dung secret, không lưu secret bytes). Khác input hoặc hiện trạng thì
BLOCKED, không publish/promote revision, rotate
credential hay ghi đè pending changes. Platform-only rồi chuyển `all` dùng cùng
stateDir để resume. Dùng stateDir/reportDir riêng cho mỗi target; không dùng
receipt của môi trường khác. Request mất kết quả phải đối chiếu server; chưa
chứng minh kết quả thì dừng, không replay mutation.

Backend phải khởi động thành công trên schema 8 và công bố Generic Workload/template
capabilities; preflight đọc chúng và template-source API trước mutation. Xác minh
schema installation trước runtime; upgrade installation có sẵn là thao tác riêng
[ADR-027](../../orchestrator_docs/architecture/decisions/ADR-027-postgres-upgrades.md).
Không reset database/chỉnh migration marker. Shared Fleet/VSO/Vault/RBAC vẫn là
prerequisites; chỉ Role/RoleBinding của namespace acceptance do sản phẩm tạo mới
được setup khi config cho phép.

Nghiệm thu dev/kind bằng [bootstrap acceptance](../../e2e/scenarios/bootstrap-acceptance/README.md).

## Git-only inputs and backend PVC

The current Git-only presets are [dev](git-targets/dev.json) and
[staging](git-targets/staging.json). Copy the environment config outside Git and
review endpoints/accounts/credential-file references/images/access URLs before
running the ordinary command. The staging preset does not authorize a staging run.
The older `direct-targets/` presets retain their original local-document/installed
module compatibility; they are not the Git-only path.

`gitOnly=true` (schemaVersion 3) requires Resource Types, every Definition document,
Workload documents and namespace-access rules to be Git file references:

```json
{
  "git": {
    "url": "https://github.com/company/platform.git",
    "ref": "0123456789012345678901234567890123456789",
    "path": "definitions/backend.json",
    "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  }
}
```

Definition entries use `git` directly alongside their Terraform `source` metadata.
Terraform/template sources must have `kind=git`, full commit and content digest.
The bootstrap obtains regular input files from the exact Git objects, verifies
file SHA-256 and caches them outside the repository (`runtime.gitCacheDir`, default
`<stateDir>/git-inputs`). Missing/wrong commits, file/digest drift, symlink/executable
files and local/inline/installed fallback are blocked. It never loads input from
the current checkout or resolves latest. Product Git inspection separately pins
Terraform modules/templates; Definition registration remains through Console,
without an Application Catalog selection. Reports include safe Git origin/commit/
file-digest receipts. Config and credential files remain local environment inputs.

The host input reader uses the declared platform Git credential only within its
configured HTTPS scope, through a private askpass helper; no token in URLs/CLI
arguments/logs. Git global credential helpers/hooks and interactive prompts are
disabled. Public Git inputs can omit credentials when the platform/CD profile
also permits that. Explicit `gitAccess.repositoryUrl/ref` selects the connectivity
test when module/template sources use several pinned commits.

The Git presets register a persistent-storage Definition and use the committed
backend Workload with a managed `/data` mount and `ACCEPTANCE_STORAGE_DIR=/data`.
`verification.storage` enables browser write/read of a bounded public test `value`,
checks the actual mounted PVC is owned/Bound and preserves its UID/file hash on
rerun. It never overwrites an existing different value. Bootstrap itself does not
recreate Pods; the separate
[Git/PVC scenario](../../e2e/scenarios/bootstrap-git-pvc/README.md) proves file/hash
retention after one ownership/UID-checked backend Pod recreation, then reruns
bootstrap to verify source/Definition/deployment identity preservation.

### Video acceptance

Set `runtime.recordVideo: true` to record public acceptance-app checks with
Playwright. Videos are saved only after successful checks; failed recordings
are deleted. The latest successful video for each phase replaces its previous
file under `runtime.reportDir/videos/`, alongside a JSON status receipt.
Console sign-in and credential-entry forms are excluded; no browser storage
state is saved. For one final Git/PVC acceptance video, use the dedicated
`e2e/scenarios/bootstrap-git-pvc/run.mjs` runner: intermediate bootstrap runs
have recording disabled, and only the successful complete scenario publishes
`videos/backend-pvc-retention.webm`. CLI/Kubernetes operations are not screen
recordings; the video shows the real app before and after Pod recreation.

## Company staging / GitHub Enterprise

Use the [Enterprise staging setup kit](enterprise/README.md),
[field-by-field input reference](enterprise/INPUT_REFERENCE.md) and
[internal-AI handoff](enterprise/AI_HANDOFF.md). The preset is data-only and
uses Git-owned portable Definitions/Workloads with explicit nonsecret parameters.
The offline lock helper computes pins from exact local Git commits; platform
preflight/inspection/deployment remain separate. No company endpoint, credential
or staging execution is supplied or implied by the example.

## Cấu hình Organization khi cài đặt — issue #47

API seed input supports `organization.key` and `organization.name`. On a new
installation prepare its nonsecret identity file before creating the first admin:

```sh
node scripts/bootstrap/seed-api.mjs --config /private/dev.yaml \
  --prepare-organization /private/organization.json
```

Mount that file read-only and configure `ORCHESTRATOR_ORGANIZATION_FILE` for both
backend startup and the first-admin CLI. It contains only key/name, not the seed
credentials. Start the backend/create the admin against that identity, then run
`node scripts/bootstrap/seed-api.mjs --config /private/dev.yaml` normally.
Importer checks the session's Organization key and name before writing platform
data. Reusing the same config preserves identities; changing an existing name
reports a conflict. It is not a rename tool or an API for creating Organizations.
Legacy `console.expectedOrganization` remains a key-only compatibility check.
See the [installation procedure](../../orchestrator_docs/operations/platform-seed.md#configurable-organization--issue-47).
