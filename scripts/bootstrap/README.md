# Bootstrap platform bằng Playwright

Đăng nhập Console → kiểm tra/đăng ký Internal account bằng upload kubeconfig
→ Loại tài nguyên → nhập schema từng trường → Cấu hình tài nguyên
→ đăng ký cluster/namespace/storage/database Definitions → Mẫu dựng ứng dụng
→ kiểm tra Git bundle → nhập placement/inputs/criteria qua form → đăng ký workload
Definition → mở Thêm Workload và xác minh catalog/volume mount.
Đây là **platform setup**, không phải nghiệm thu deploy ứng dụng.

Entrypoint: `scripts/bootstrap/run.mjs`, có package/config/helpers riêng.
`npm test` kiểm tra config/reuse/pacing và presets.
[Luồng thực thi từng bước](EXECUTION_FLOW.md) ghi điều kiện reuse/create/fail
và vị trí code để đối chiếu khi sửa script.

Bản Terraform modules và render bundle để copy sang Git công ty:
[platform-catalog](../../platform-catalog/README.md).

## Tự cấu hình Truy cập Git nội bộ

Bootstrap thực hiện qua UI: đăng nhập → preflight schema → **Truy cập Git**
→ Infrastructure Account → Types → Definitions → render bundle. Cấu hình Git
được kiểm tra/lưu trước mọi mutation catalog/account. Không tự tạo PAT trên
GitHub, cài Vault, sửa DNS/firewall hoặc chạy deployment.

Trong `targets/staging.json`, sửa `seed.gitAccess` (dev sửa preset dev; với
`--config`, đặt `gitAccess` ở gốc JSON):

```json
"gitAccess": {
  "enabled": true,
  "username": "your-github-username",
  "tokenFile": "~/.orch/bootstrap/staging/git-token"
}
```

Presets để sẵn block `enabled:false` và username mẫu `github-reader`; thay bằng
username thật rồi bật `true`. Khi bỏ block hoặc tắt, giữ nguyên luồng Git public.
`tokenFile` chứa PAT đọc repo, quyền 0600/0400; không đặt token vào JSON hoặc URL.
Script chỉ đọc file này khi tạo mới hoặc có `--update-git-access`, không đọc khi
reuse. CA tùy chọn: thêm `caPemFile` trỏ tới file PEM, tối đa 64 KiB.

- `--git-url` mặc định làm URL/phạm vi và repo kiểm tra; `--git-ref` làm ref kiểm tra.
- Nếu không truyền hai cờ, tự lấy khi các sources có cùng URL/ref rõ ràng.
- Muốn scope theo nhóm/host, thêm `gitAccess.url`; có thể chỉ định
  `repositoryUrl` và `ref` riêng cho kiểm tra. Các catalog sources phải thuộc scope.
- Đường dẫn `~/` được mở rộng theo home; đường dẫn tương đối tính từ working directory.

```bash
node scripts/bootstrap/run.mjs --target staging \
  --url https://platform.company.local \
  --kubeconfig /private/staging.kubeconfig \
  --git-url https://github.company.local/platform/platform-catalog.git \
  --git-ref <full-commit-SHA40>
```

Chưa có setting: điền form → Kiểm tra kết nối → Lưu → đọc lại metadata/version.
Setting đã khớp và có token: kiểm tra token đang lưu, không PUT, không thay token.
Khác metadata/thiếu token hoặc muốn rotate: thêm **`--update-git-access`**, độc lập
với `--update-existing` (chỉ đổi Definition source/pin). Cờ update Git luôn đọc
file token mới, kể cả metadata không đổi. Mỗi POST/PUT từ UI được guard payload
và version trước khi forward; stale/unknown outcome dừng, không tự retry.

Backend phải được cấu hình credential store bền vững để Lưu (ví dụ Vault).
Test dùng draft có thể thành công dù Lưu không thành công; script chỉ tiếp tục
sau khi kiểm tra lưu/reuse đầy đủ. HTTP503 khi Lưu: kiểm tra credential store.
Tắt DEBUG, NODE_DEBUG và PWDEBUG với bước nhập token. Runner không lưu trace,
ảnh, video, browser session; lỗi Git access được rút gọn, không log filled values.
Nếu seed lỗi sau khi Git đã lưu, cấu hình Git vẫn được giữ để lần sau reuse.

## Dùng Git công ty và chuyển nguồn Definition

Copy nội dung `platform-catalog/` làm gốc repo công ty rồi push. Lấy full commit
SHA40 ký tự của repo đó; không dùng SHA của repo GitHub nguồn sau khi copy file.

```bash
node scripts/bootstrap/run.mjs \
  --target staging \
  --url https://platform-cong-ty.example \
  --kubeconfig /private/staging.kubeconfig \
  --git-url https://git-cong-ty.example/team/catalog.git \
  --git-ref <full-commit-SHA40>
```

`--git-url` đổi Git URL cho Terraform sources và render bundle. Các installed
Terraform modules có trong bản phân phối được chuyển sang
`backend/terraform-modules/<tên-module>`; giữ nguyên tên/ID Definition, params,
mappings, provider bindings, criteria và placement. Existing-cluster vẫn trỏ
account đã có. Không biết đường dẫn module thì dừng thay vì tự suy đoán.
Git source đã khai báo giữ nguyên subdirectory. `--git-ref` thay pin Git cho cả
modules và bundle; nhận full commit để các bước inspect dùng cùng snapshot.
Khi chuyển installed modules sang Git, bắt buộc có `--git-ref`. Bỏ cả hai cờ
thì giữ hành vi/config cũ; hỗ trợ cả --target và --config.

Nếu Definition cùng ID đã có nhưng còn dùng source/pin cũ, chạy thêm
`--update-existing`. Cờ này **chỉ** cho phép đổi source Terraform hoặc bundle pin.
Không cho tự đổi account, Type/driver, tên, criteria, mappings,
placement, render inputs hoặc provision. Biến chỉ có trên Console được giữ và cảnh báo;
biến chỉ có trong seed được thêm, biến chung khác giá trị thì dừng. Khác các trường còn lại vẫn dừng để operator
review. Resource Type schema khác vẫn bị chặn. Không reset database/resource.

Update đi qua form Sửa → Lưu thay đổi, dùng expected version CAS. Trước khi
forward PUT, script kiểm payload UI đúng desired và đúng version đã đọc; sai thì
chặn request. Sau khi lưu, đọc lại và kiểm version tăng1/config khớp. HTTP409,
unknown outcome hoặc lỗi khác dừng, không tự retry/reload rồi ghi đè winner.
Seed có thể hoàn tất một phần trước khi lỗi; chạy lại reuse phần đã khớp.

Backend platform phải truy cập HTTPS Git và có CA/auth/source policy phù hợp.
Không nhúng token vào URL. Script không cấu hình Git server, không sửa workload
đã chạy, không Preview/Deploy. Definition mới sẽ được chọn trong Preview/Deploy
sau đó theo criteria; resource instances/state đã tạo không bị reset/migrate bởi
bootstrap. Việc chuyển nguồn resource đang có phải review ở Preview riêng.

## Presets dev và staging

Cấu hình đã điền trong `targets/dev.json` và `targets/staging.json`:

| Target | Console | Login | Storage |
|---|---|---|---|
| dev | `http://localhost:3001` | `platform-engineer` | StorageClass mặc định của kind |
| staging | `https://devportal.stg.exampledevops.com` | `platform-engineer` | `rook-ceph-block` |

Hai target là hai installation platform, khác environment staging/production bên
trong Application. Presets seed cho Application environment staging. Chạy headless,
không delay mặc định. Có thể override bằng các biến ORCH_PLATFORM_* và pacing.
Account name `k8s-4f` giữ theo runbook staging hiện có; preset staging reuse account
READY đã đăng ký theo tên này, không đọc file kubeconfig. Khi account chưa có,
script inspect file qua UI, yêu cầu đúng một context rồi lấy context/endpoint từ
kết quả inspect; không tự suy endpoint hoặc chọn một trong nhiều cluster. Preset
dev kiểm đúng context `kind-k8s-4f` và endpoint Docker `k8s-4f-control-plane:6443`.

Presets dùng sẵn login seed `platform-engineer` / `test-password`, giống tài
khoản mặc định của sản phẩm. Không cần tạo/chép file mật khẩu. Khi đã đổi mật
khẩu, đặt `ORCH_PLATFORM_PASSWORD_FILE` để ghi đè; file được chỉ định mà lỗi thì
script dừng, không âm thầm quay về mật khẩu seed. Chế độ `--config` vẫn yêu cầu
file password như trước. Không ghi password vào log.
Bạn có thể tự nhập URL Console bằng `--url` cùng kubeconfig:

```bash
npm ci --prefix scripts/bootstrap
(cd scripts/bootstrap && npx playwright install chromium)
node scripts/bootstrap/run.mjs --target staging --url https://console-staging.example --kubeconfig /private/staging.kubeconfig
# Dev dùng account hiện có, không cần kubeconfig:
node scripts/bootstrap/run.mjs --target dev
```

`--url` ưu tiên hơn ORCH_PLATFORM_URL và URL preset; nếu bỏ thì dùng cấu hình
mặc định. Áp dụng cả chế độ --target và --config. URL phải là origin Console
(http/https, có thể kèm /ui), không chứa credential/query/hash.

Nếu bỏ `--kubeconfig`, đường dẫn mặc định là
`~/.orch/bootstrap/<target>/kubeconfig`. File phải riêng tư0600/0400, một context,
endpoint backend platform truy cập được; không tự dùng current context máy host.
Không truyền `--config` cùng `--target`. Chế độ `--config` cũ vẫn được giữ.

Preset đăng ký bảy Types, các infrastructure Definitions và standard Git bundle
pin `1182cb5ff275451ebfa8b711bc07effd1e120fcf` có policy PVC Recreate.
Renderer Definition mới chỉ match `env_type=staging, class=pvc`, để không thay
renderer mặc định của các workload đã chạy. Workload mới dùng template catalog
và class `pvc` để chọn bundle này; workload cũ cần thêm execution pin nếu chưa có.
Không tự sửa/di chuyển workload hoặc đổi renderer đang pin. Account/Definitions
cùng ID nhưng cấu hình khác mặc định dừng; chỉ đổi source/pin khi có --update-existing; không tự tạo ID khác để né conflict.
Nếu chưa có Application, seed hoàn tất và ghi rõ bỏ kiểm tra picker; không tạo
Application chỉ để kiểm tra. Nếu config chỉ định verify.applicationKey mà không
có Application tương ứng, script báo lỗi.

Nguồn chuẩn: [UC-04 Infrastructure Account](../../orchestrator_docs/usecase/UC-04/specification.md),
[UC-02 Resource Type](../../orchestrator_docs/usecase/UC-02/specification.md),
[UC-03 RB-01..06](../../orchestrator_docs/usecase/UC-03/specification.md#git-rendering-bundle-authoring--adr-021),
[Git bundle contract](../../orchestrator_docs/architecture/contracts/git-render-bundles.md),
[workload catalog](../../orchestrator_docs/architecture/contracts/workload-catalog.md#wc-c08--concrete-git-owned-catalog-contract),
[hướng dẫn E2E](../../e2e/README.md).

## Chạy trên máy khác

Cần Node.js 22.15+ (hoặc 24+), Console/backend hỗ trợ workload catalog, tài khoản
Platform Engineer/Admin, Chromium và kết nối mạng tới Console. Backend phải truy
cập được HTTPS Git source; browser chạy ở máy bạn không thay backend tải Git.
Console cần có ít nhất một Application với Environment để kiểm tra hộp chọn.
Khi account chưa đăng ký, cần kubeconfig riêng 0600/0400, có context và endpoint
backend truy cập được. Nếu account đã có và khớp, không cần file kubeconfig.
Không cần Docker/kubectl trên máy chạy script nếu file đã được chuẩn bị. Cài từ root repository:

```bash
cd scripts/bootstrap
npm ci
npx playwright install chromium
cd ../..
mkdir -p ~/.orch
cp scripts/bootstrap/config.example.json ~/.orch/catalog.json
```

Sửa `~/.orch/catalog.json`. Đặt mật khẩu vào file riêng `~/.orch/password`, chỉ
chứa mật khẩu, rồi `chmod 600 ~/.orch/password`. Không đưa credential vào JSON,
Git URL hoặc command line. Chạy từ root repository:

```bash
ORCH_PLATFORM_URL=http://localhost:3001 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE="$HOME/.orch/password" \
ORCH_KUBECONFIG_FILE="$HOME/.orch/k8s-4f.kubeconfig" \
ORCH_HEADLESS=1 \
node scripts/bootstrap/run.mjs --config "$HOME/.orch/catalog.json" \
  --type-delay-ms 1000 --definition-delay-ms 1500 --bundle-delay-ms 1000
```

Đổi URL/tài khoản/password-file cho máy đích. Mặc định hiện browser (cần
desktop/display), nghỉ 800 ms trước/sau mỗi thao tác và nhập lần lượt từng ký tự.
`ORCH_STEP_DELAY_MS` nhận 0..10000; tăng lên 2000 nếu muốn xem chậm hơn. Dùng
`ORCH_HEADLESS=1` khi chạy automation không có display. Script dùng helper đăng nhập chung, đặt locale tiếng Việt
trong browser context riêng; không phụ thuộc ngôn ngữ bạn đang chọn trên Console.
Có thể chạy từ `scripts/bootstrap/` bằng `npm run bootstrap -- --config /path/catalog.json`.
Đường dẫn JSON được tính từ working directory; helper/import không phụ thuộc cwd.

### Chọn tốc độ riêng từng phần

Đơn vị mili giây, nghỉ trước/sau mỗi click/select/input; nhập từng ký tự có delay
tối đa 45 ms/ký tự. Chọn tốc độ trước khi chạy, không cần sửa code:

| CLI | Biến môi trường tương đương | Phần thao tác |
|---|---|---|
| `--step-delay-ms` | `ORCH_STEP_DELAY_MS` | Mặc định chung, 800 ms |
| `--account-delay-ms` | `ORCH_ACCOUNT_DELAY_MS` | Upload kubeconfig/đăng ký account |
| `--type-delay-ms` | `ORCH_TYPE_DELAY_MS` | Tạo Resource Types và từng trường schema |
| `--definition-delay-ms` | `ORCH_DEFINITION_DELAY_MS` | Tạo Resource Definitions, mappings, chọn nguồn, criteria |
| `--bundle-delay-ms` | `ORCH_BUNDLE_DELAY_MS` | Git render bundles và kiểm tra hộp chọn |

CLI của từng phần ưu tiên hơn biến môi trường của phần đó, rồi fallback giá trị
chung từ CLI/env. Giá trị hợp lệ 0..10000; `0` dùng cho automation nhanh.

### Kubeconfig và account đã có

`account` trong ví dụ chọn tên `k8s-4f`, context `kind-k8s-4f`, endpoint
`https://k8s-4f-control-plane:6443`. Sửa cả ba cho máy đích. Script đọc danh sách
account trước: đúng tên, INTERNAL/KUBECONFIG, READY và endpoint/context khớp thì
reuse, không đọc file hay upload lại. Cùng tên nhưng khác target hoặc chưa READY
thì FAIL, không thay credentials. Chỉ khi chưa có mới dùng `ORCH_KUBECONFIG_FILE`
(ưu tiên) hoặc `account.kubeconfigFile`, upload qua UI và kiểm tra đúng context/
endpoint **trước** khi bấm Kiểm tra và lưu. Không dán nội dung kubeconfig vào UI.

Nếu cần xuất kubeconfig cho Docker backend trên máy kind đã có:

```bash
(umask 077; kubectl --context kind-k8s-4f config view --minify --flatten --raw > ~/.orch/k8s-4f.kubeconfig)
kubectl --kubeconfig ~/.orch/k8s-4f.kubeconfig config set-cluster kind-k8s-4f \
  --server=https://k8s-4f-control-plane:6443
```

Chỉ file riêng được đổi endpoint; kubeconfig/context mặc định được giữ nguyên.
Backend Docker phải nằm trong network `kind` theo [runbook](../../orchestrator_docs/operations/docker-local.md).
Script không tự sửa network/kubeconfig hoặc tạo cluster. Xác minh account có
Kubernetes API reads; đăng ký metadata này không cho phép script Deploy/PVC.

## Tùy chỉnh catalog

`bundles` nhận một hoặc nhiều entry; mỗi entry đăng ký một rendering Definition.
Một bundle có thể cung cấp nhiều mẫu trong `bundle.yaml.workloadTemplates`.
`resourceTypes` là danh sách tùy chọn; config cũ chỉ có `bundles` vẫn chạy được.
`account` và `resourceDefinitions` cũng tùy chọn; bỏ chúng để chỉ chạy setup cũ.
File JSON chỉ là cấu hình bên ngoài: script **không nhập Score, không mở JSON
nâng cao, không dán JSON vào UI và không dùng API để tạo/sửa dữ liệu**.

| Trường | Ý nghĩa |
|---|---|
| `account` | `{name,context,endpoint,kubeconfigFile?}`; expected target để kiểm tra/reuse, file chỉ đọc khi chưa có account |
| `resourceTypes[].key` | ID loại tài nguyên; không dùng reserved IDs `environment`/`service` |
| `resourceTypes[].inputs` | Danh sách `{name,type,required}`; type là `string`, `number`, `bool` hoặc `any` |
| `resourceTypes[].outputs` | Danh sách `{name,type,required,secret}`; flags bỏ trống mặc định false |
| `key`, `name` | ID duy nhất và tên hiển thị của rendering Definition |
| `source.url`, `path`, `ref` | Git HTTPS repo, thư mục bundle và commit/tag/branch; nên pin full commit để chạy lại ổn định |
| `placement.namespace`, `cluster` | Whole output references phù hợp Resource Definitions của máy đích |
| `renderInputs` | Giá trị string/number/boolean theo inputs schema; script nhập textbox/select/checkbox, ví dụ `{"team":"payments","defaultReplicas":2,"runAsNonRoot":true}` |
| `provision` | Giữ `{}`; form companion Params hiện chỉ có JSON nên script từ chối cấu hình khác trước khi kết nối |
| `criteria` | OR giữa các object, AND giữa các field `env_type`, `app_id`, `env_id`, `res_id`, `class`; mặc định `[{}]` là wildcard |
| `expectedTemplates` | Các `{id, version}` phải xuất hiện trong catalog/picker sau đăng ký; mặc định `[]` chỉ kiểm tra catalog không rỗng |
| `verify.applicationKey` | Application để mở picker; nếu bỏ, chọn Application đầu tiên có Environment được yêu cầu |
| `verify.environment` | `staging` (mặc định) hoặc `production` |
| `verify.volumeMount` | Tùy chọn `{templateId,templateVersion,resourceType}`: chọn mẫu → Thêm volume mount → dialog storage → thêm dependency và mount `/data` trên form local, không Save |

Ví dụ hiện có bảy Types và sáu Infrastructure Definitions: cluster có sẵn,
namespace, persistent-storage, PostgreSQL, Redis và MySQL. Cluster chọn account có tên
`k8s-4f` và công bố class `internal`; các Terraform Definitions bind provider tới
cluster class này. Namespace lấy tên từ deployment context; storage/PostgreSQL/
Redis lấy namespace từ resource output qua UI source chooser. Chỉ metadata được
đăng ký; không tạo cluster, namespace, PVC hoặc database thật.

### Cấu hình Resource Definitions

`resourceDefinitions` được xử lý **theo thứ tự** trong file; đặt cluster trước
namespace rồi các resources cần namespace. Mỗi entry có `key`, `name`,
`resourceType`, `driverType`, `criteria` (cùng năm matching fields đã mô tả).

- `existing-cluster`: chỉ thêm `accountName`; account được resolve theo tên và
  lưu key thực từ server, không suy key từ tên.
- `terraform`: chọn một `module` đã cài (ví dụ `k8s-namespace`, `postgres-k8s`,
  `redis-k8s`) hoặc `source: {url,path,ref}` Git với full commit. Có `clusterClass`,
  `inputMapping` (Type param → module variable), `outputMapping` (Type output →
  module output) và `variables` là nguồn từng giá trị.
- Nguồn variable: `{literal: "2Gi"}` hoặc số/boolean; `{context: "env.namespace"}`
  hoặc `infra.resourceName`; `{resource: {type,class,scope,output}}` với scope
  `environments.@app.@env` hoặc `@infra`. Script chọn qua form nguồn, không nhập
  biểu thức hoặc JSON. Không hỗ trợ literal object/array, AWS hay credentials.

Đã có cùng Definition ID thì so sánh toàn bộ cấu hình, account key, mappings,
variables, criteria và source pin; chỉ reuse khi khớp. Khác thì mặc định FAIL; --update-existing chỉ cho phép source/pin migration qua PUT/CAS.
Installed modules dùng verified capability pin của backend đích. Git modules
phải khớp repo/path/full commit và registry capability; inspect không chạy apply.

Resource Types đã có chỉ được reuse khi inputs/outputs/type/required/secret
khớp cấu hình; thứ tự field, flags false bị API lược bỏ và metadata classes
không gây mismatch. Schema khác thì FAIL trước mọi create/inspect; không sửa
schema đang dùng. Cấu hình có tên/kiểu/flag sai hoặc trùng bị chặn trước đăng nhập.

Để **thêm/đổi chính các mẫu ứng dụng**, chỉnh `workloadTemplates` trong bundle
Git của bạn (ID, version, labels/descriptions, kind, inputsSchema/defaults), giữ
renderer/patches tương thích, push rồi đổi `source` và `expectedTemplates` trong
JSON. Script không tự tạo template từ tên hoặc image và không sửa repo nguồn.
Thay nội dung một template phải tăng version; không publish hai nội dung khác
nhau cùng ID/version trong một Organization.

Để có nhiều policy cho cùng bundle, thêm entry với `key` riêng và `criteria`
khác nhau, ví dụ `[{"class":"payments"}]`. Tránh nhiều wildcard cùng điểm gây
ambiguous matching. `renderInputs` là policy dựng manifest; không phải image,
secret hoặc cấu hình workload cụ thể. Placement mẫu dùng cluster class `internal`
và namespace class `default`; chỉnh nếu platform đích dùng class khác. Đăng ký
catalog không tự tạo Secret Store hoặc deploy infrastructure. Nếu chọn cluster
class khác `internal`, sửa cluster criteria, mọi Terraform `clusterClass` và
`bundles[].placement.cluster` đồng bộ trước khi chạy.

## Assertions, chạy lại và side effects

Các mutation đi qua từng trường UI thật; API GET chỉ đọc kỹ thuật để so sánh schema,
pin, toàn bộ Definition, availability và templates. PASS khi schema/pin/cấu hình
đã lưu khớp file cấu hình, mọi expected template có trên API và hộp chọn hiển thị đúng số mẫu.
Nếu có `verify.volumeMount`, script xác minh nút bấm được khi chưa có dependency,
mở dialog storage tại chỗ, xác nhận dependency và thêm được mount path `/data`.
Không tạo workload/draft, Preview, Deploy, sửa database schema hoặc mutate cluster/cloud.

Chạy lại cùng cấu hình sẽ kiểm tra Git source và reuse Definition cùng ID nếu
toàn bộ cấu hình/pin khớp; không POST Definition lần hai. Khác cấu hình mặc định
FAIL; --update-existing chỉ cho đổi source/pin qua PUT/CAS. Các thay đổi khác
cần review thủ công, không tự delete/reset. Không tự retry POST/PUT khi kết quả chưa rõ.
Script có thể đã lưu bundle/Definition trước khi bước sau thất bại; kiểm tra
Console rồi chạy lại. Không rollback hay xóa tài nguyên đã có.

Account, Resource Types, module/bundle cache/metadata và Definitions được **giữ lại** để người dùng test. Session
do script mở được sign out và browser đóng trong `finally`; không lưu storage
state, screenshot, video hoặc trace. Presets dùng seed login; mật khẩu tùy chỉnh
đọc từ file riêng ngoài Git. Không
có cleanup namespace/PVC vì script không tạo chúng. Nếu chưa có Application,
script báo rõ registration đã xong nhưng verification chưa hoàn tất; tạo
Application qua Console rồi chạy lại.

Kiểm tra local helpers: `cd e2e && npm test`. Kết quả trên một Console không
chứng minh deployment hoặc credentials/placement ở máy khác đã đúng.

## Kiểm tra phần Git migration

`npm test` chạy unit checks cấu hình/override/source-only migration/CAS payload.
Kiểm tra UI riêng trên Console local đã có metadata postgres-k8s và một Git
bundle: `node scripts/bootstrap/test/company-git-ui.mjs`. Script đăng nhập thật,
đọc metadata public rồi intercept mọi inspect/catalog write bằng fixtures;
không ghi Definition vào backend, không truy cập Git công ty hay mutate cluster.
Kiểm tra installed→Git qua form, rerun không PUT, renderer update và HTTP409
không retry. Đây là kiểm tra UI với fixture, không phải nghiệm thu Git staging.

### Private company Git

After deploying the version with platform Git settings, sign in as Platform
Engineer/Admin and open **Cài đặt nền tảng → Truy cập Git**. Save the company
HTTPS URL scope and read username/token; add internal CA PEM when required.
The backend automatically applies this access to Terraform and render bundle
inspection. Then run bootstrap with the existing Git URL/ref options.
No token is placed in bootstrap presets, command arguments or source URLs.
This setting requires the platform's existing credential store to be available;
it does not provision backend DNS, network routing or Vault.

## Chạy Windows, giữ biến Console và lỗi cấu hình

Trên Windows, script không kiểm tra POSIX mode/chmod của file mật khẩu,
kubeconfig hoặc Git token; vẫn yêu cầu file thường, đọc được, không rỗng và
đúng giới hạn nội dung. Linux/macOS vẫn yêu cầu file riêng tư 0600/0400.

Khi chạy `--update-existing` để chuyển nguồn Git, các biến chỉ có trên Console
(ví dụ `image`) được copy nguyên giá trị/kiểu vào desired payload và cảnh báo
bằng tên biến; seed không xóa hoặc đặt lại chúng. Biến chỉ có trong seed sẽ được thêm. Nếu Console và seed cùng khai
báo một biến nhưng giá trị khác nhau thì dừng, nêu Definition, đường dẫn trường,
giá trị Console và seed. Không nới guard schema, account, mapping, criteria hoặc
provision; những thay đổi ngoài source vẫn cần operator review. Readback và CAS
kiểm tra cả biến được giữ. Token/password/credential và URL có credential được
ẩn khỏi thông báo lỗi. Khi cần giữ biến qua thao tác chọn module mới, script dùng
control JSON của form sản phẩm để không làm mất kiểu dữ liệu.

Ví dụ biến đúng trong seed:

```json
"variables": {
  "image": { "literal": "mysql@sha256:<digest>" },
  "storage_class": { "literal": "rook-ceph-block" }
}
```

Không dùng `"image":"mysql:8"` trực tiếp. Lỗi định dạng nêu tên Definition,
`variables["image"]`, kiểu nhận được và ví dụ cú pháp được hỗ trợ; không in
nguyên giá trị nguồn bị lỗi.

## MySQL Definition

Dev/staging/config mẫu bổ sung Type `mysql` và Definition cùng prefix với
PostgreSQL, dùng Git module `backend/terraform-modules/mysql` (không phải installed
module). `--git-url`/`--git-ref` ghi đè nguồn này như các Git modules khác.

MySQL map các resource params `image`, `database`, `username`, `auth_secret_name`,
`root_password_key`, `user_password_key` (bắt buộc) và `storage` (tùy chọn).
`image` cần digest khi module thực thi; seed không tự chọn image hoặc mật khẩu.
Các param Secret là tên Secret và tên key, không phải giá trị mật khẩu. Secret
phải được chuẩn bị bằng cơ chế delivery đã phê duyệt trước khi deploy MySQL.
Bootstrap không tạo Secret hoặc workload. Namespace lấy từ dependency namespace;
storage class staging là `rook-ceph-block`, dev dùng mặc định cluster. Module
mặc định storage `50Gi`; outputs gồm host/port/database/username/claimName/namespace,
không công bố password. Criteria/class kế thừa Definition PostgreSQL của target.
