# Luồng thực thi bootstrap platform

Tài liệu mô tả hành vi **đang có trong code**, để đối chiếu khi sửa script.
Khi thay đổi thứ tự, điều kiện dùng lại hoặc dữ liệu seed, cập nhật file này cùng
code. [README](README.md) hướng dẫn cài/chạy; [run.mjs](run.mjs) là entrypoint.
Không dùng tài liệu này thay specification hoặc contract sản phẩm.

## Đầu vào và phạm vi

Ví dụ chạy hiện tại từ root repository:

```bash
node scripts/bootstrap/run.mjs \
  --target staging \
  --url https://platform-cong-ty.example \
  --kubeconfig /private/staging.kubeconfig
```

- Chọn `--target dev|staging` hoặc `--config /path/catalog.json`, không dùng cả hai.
- Presets: [dev.json](targets/dev.json), [staging.json](targets/staging.json).
- `--git-url`/`--git-ref` ghi đè nguồn theo cấu trúc platform-catalog;
  `--update-existing` bật riêng source/pin migration.
- `--url` ưu tiên hơn `ORCH_PLATFORM_URL`, rồi tới URL trong preset.
- `--kubeconfig` ưu tiên hơn `ORCH_KUBECONFIG_FILE`, rồi tới đường dẫn mặc định
  `~/.orch/bootstrap/<target>/kubeconfig` của preset.
- Presets dùng tài khoản seed mặc định; `ORCH_PLATFORM_PASSWORD_FILE` ghi đè mật
  khẩu nếu được chỉ định. Chế độ `--config` vẫn yêu cầu cấu hình đăng nhập ngoài.
- Presets mặc định headless, delay0. Có thể ghi đè headless/pacing bằng env/CLI.
- Target dev/staging chọn installation platform, không chọn Application
  environment. Cả hai preset hiện seed cho Application environment `staging`.
- Script ghi cấu hình qua form Console, dùng API GET để đọc/đối chiếu. Các POST
  inspect/register là request phát sinh khi thao tác UI.

```text
01 Đọc tham số/preset
 → 02 Validate cấu hình
 → 03 Đăng nhập và kiểm tra role
 → 04 Preflight schema Resource Types đã có
 → 04b Thiết lập Truy cập Git (nếu bật)
 → 05 Kiểm tra/đăng ký Infrastructure Account
 → 06 Tạo Resource Types còn thiếu
 → 07 Kiểm tra/đăng ký Infrastructure Definitions
 → 08 Inspect bundle và kiểm tra/đăng ký renderer Definition
 → 09 Xác minh catalog và form volume mount nếu có Application
 → 10 Đăng xuất, đóng browser, trả kết quả
```

## 01 — Đọc tham số và cấu hình

Code: `parseArgs` trong [run.mjs](run.mjs), `loadTarget` trong
[target-config.mjs](lib/target-config.mjs).

1. Parse target/config, URL, kubeconfig và các tham số delay từng phase.
2. `--help` in cách chạy và thoát0; không kết nối Console.
3. Đọc JSON preset hoặc file config do người dùng cung cấp.
4. Ghép các giá trị ghi đè và defaults. Không đọc kubeconfig ở bước này.

`applyGitOverrides` trong [git-overrides.mjs](lib/git-overrides.mjs) chạy trước
validate/browser: đổi URL Git, chuyển installed Terraform modules được biết sang
`backend/terraform-modules/<module>`, giữ subdirectory của Git sources đã có,
đổi ref cho module/bundle khi được chỉ định. Chuyển installed→Git bắt buộc có
full commit SHA40. Không biết module path, URL không HTTPS/nhúng credential,
ref không full commit thì dừng. Không sửa preset/file config gốc trong quá trình
này. Không đổi existing-cluster, mappings, variables, Type/schema hay criteria.

## 02 — Validate cấu hình trước browser

Code: `parseBundleConfig` trong [render-bundle-config.mjs](lib/render-bundle-config.mjs),
`parseInfrastructure`/`readSetupPacing` trong
[infrastructure-config.mjs](lib/infrastructure-config.mjs),
`readConsoleConfig` trong [console.mjs](lib/console.mjs).

- Kiểm tra JSON shape/trường hỗ trợ, ID trùng, tên/kiểu/flags của Type fields.
- Kiểm tra account target cố định hoặc chế độ discovery; module/source,
  mappings, variable sources, criteria, bundle placement và render inputs.
- Git source dùng HTTPS không chứa credential; path tương đối không chứa `..`.
  Terraform Git source hiện yêu cầu full commit; bundle ref nhận chuỗi ref.
- Delay phải là số nguyên0..10000 ms.
- Kiểm URL Console, username và password file nếu người dùng ghi đè.
  File override lỗi thì dừng, không fallback sang mật khẩu seed.

Chưa kiểm Kubernetes/Git có truy cập được hay chưa. Việc đó nằm ở các bước inspect.

## 03 — Đăng nhập và kiểm tra quyền

Code: `openConsole`/`ensureSignedIn` trong [console.mjs](lib/console.mjs), role
check trong [run.mjs](run.mjs).

Mở Chromium và Console, đăng nhập qua form; kiểm session đúng user. Chỉ tiếp tục
với role `PLATFORM_ENGINEER` hoặc `ADMIN`. Context browser riêng dùng locale
Việt để các thao tác UI có label nhất quán.

## 04 — Preflight schema Resource Types đã có

Code: `readTypes` và vòng lặp `existingTypes` trong [run.mjs](run.mjs),
`assertSameResourceType` trong [render-bundle-config.mjs](lib/render-bundle-config.mjs).

1. GET `/resource-types` để lấy danh sách hiện tại.
2. Với từng Type trong seed, tìm theo `key`.
3. Chưa có: để bước06 tạo; bước04 không tạo gì.
4. Đã có: so tên/kiểu/required của inputs và tên/kiểu/required/secret của outputs.
5. Khớp: tiếp tục; khác: báo lỗi và dừng trước các bước tạo mới.

Thứ tự fields không quan trọng. Flags false bị API lược bỏ, empty schema
null/omitted và metadata classes không tạo mismatch giả.

## 04b — Thiết lập Truy cập Git (tùy chọn)

Code: [git-access-config.mjs](lib/git-access-config.mjs),
[git-access-ui.mjs](lib/git-access-ui.mjs). API/behavior:
[GA-01–06](../../orchestrator_docs/usecase/UC-03/git-access.md).

1. Trước browser: validate block `gitAccess`, URL/ref/scope, file CA và đường dẫn
   token. `enabled:false` hoặc không có block thì bỏ qua. Token chưa đọc ở đây.
2. Sau role/schema preflight, GET settings hiện tại của Organization.
3. Version0: tạo. Có metadata/token khớp: reuse, không đọc token file. Khác:
   dừng nếu chưa có `--update-git-access`. Cờ này luôn dùng token file mới.
4. Chỉ create/update mới đọc file token riêng tư 0600/0400, tối đa4096 byte;
   giữ token trong bộ nhớ, không log/trace/video/screenshot.
5. Mở trang settings, điền URL/auth/username/token, CA và repo/ref kiểm tra.
   Form luôn giữ token đã lưu là chuỗi rỗng khi reuse.
6. Guard POST Test và PUT Save phải đúng desired payload và expectedVersion từ
   GET ban đầu. Page đã tải version khác thì chặn trước khi forward. Reuse cấm PUT.
7. Bấm Kiểm tra kết nối; failure dừng trước Save/account/catalog mutations.
8. Create/update bấm Lưu; reuse không lưu. GET lại xác minh metadata/tokenConfigured
   và version: reuse không tăng; Save tăng đúng1. Không replay mutation khi lỗi.
9. Xóa token khỏi input sau thao tác; tiếp tục account/types/Definitions như cũ.

Không cài Vault/credential store, không tạo GitHub token hay sửa network.
Save503/409, timeout hoặc readback không xác minh được thì dừng an toàn;
Git setting đã commit không tự rollback khi phase sau thất bại.

## 05 — Kiểm tra/đăng ký Infrastructure Account

Code: `ensureInfrastructureAccount` trong [infrastructure-ui.mjs](lib/infrastructure-ui.mjs),
`assertAccountTarget`/`discoverAccountTarget` trong
[infrastructure-config.mjs](lib/infrastructure-config.mjs).

1. GET `/driver-accounts`, tìm theo tên account trong seed (presets: `k8s-4f`).
2. Có nhiều account cùng tên: dừng.
3. Đã có: yêu cầu INTERNAL, KUBECONFIG, READY và metadata target hợp lệ.
   Target cố định phải khớp context/endpoint; discovery reuse target đã cấu hình.
   **Không đọc/upload kubeconfig khi dùng lại account.**
4. Chưa có: đọc file kubeconfig riêng tư0600/0400, mở form đăng ký và upload.
5. Chờ inspect qua UI. Discovery staging yêu cầu đúng một context; lấy
   context/endpoint từ metadata inspect. Không tự chọn một trong nhiều cluster.
6. Target cố định phải có đúng context/endpoint trong kết quả inspect.
7. Bấm Kiểm tra và lưu, đọc lại account để xác nhận trạng thái/target.

Không tự sửa credential hoặc endpoint của account đã tồn tại. Không tạo cluster.

## 06 — Tạo Resource Types còn thiếu

Code: vòng lặp `phase = 'type'` trong [run.mjs](run.mjs).

Dùng kết quả preflight ở bước04. Type đã có thì bỏ qua. Type thiếu thì mở trang
Loại tài nguyên, nhập ID và từng input/output, chọn kiểu/required/secret rồi
đăng ký. Đọc lại API và kiểm schema cùng thông báo thành công trên UI.

Preset có sáu Types: persistent-storage, k8s-cluster, k8s-namespace, postgres,
redis, workload. Thứ tự thực tế theo mảng `resourceTypes` trong JSON.

## 07 — Kiểm tra/đăng ký Infrastructure Definitions

Code: `ensureResourceDefinitions`, `chooseSource`, `applyCriteria` trong
[infrastructure-ui.mjs](lib/infrastructure-ui.mjs).

Với từng Definition, theo thứ tự trong `resourceDefinitions`:

1. Đọc accounts và Definition hiện có theo key.
2. `existing-cluster`: chọn account INTERNAL/READY theo tên đã cấu hình.
3. Installed module: tìm đúng một verified capability phù hợp type/driver/module.
   Nếu Definition đã có, so toàn bộ cấu hình; khớp thì reuse, khác thì dừng.
4. Git module: mở form mới, điền ID/name/type, URL/path/ref rồi bấm Kiểm tra
   module. Kiểm URL/path/revision trả về và capability Kubernetes, không AWS.
   **Git module vẫn được inspect khi Definition đã có**, rồi mới so/reuse.
5. Definition chưa có: điền input/output mappings, provider cluster class,
   các variable sources literal/context/resource và criteria.
6. Đăng ký; đọc lại và so cấu hình cùng source pin/mappings.

Nếu Git Definition đã có và --update-existing bật: mở form Sửa, inspect pin mới,
so policy source-only trước khi Save; điền lại mappings/variable sources của
desired đã kiểm tra, giữ criteria hiện có. Khác ngoài source bị chặn. Save qua
PUT version CAS với payload guard, kiểm GET sau update và version tăng1.
Không có flag thì khác cấu hình vẫn dừng. Account/Type không tự cập nhật.

Preset có cluster, namespace, storage, PostgreSQL và Redis Definitions.
Storage staging đặt module variable `storage_class=rook-ceph-block`.
Đăng ký Definition chưa chạy Terraform, chưa tạo namespace/PVC/database thật.

## 08 — Inspect bundle và đăng ký renderer Definition

Code: vòng lặp `phase = 'bundle'` trong [run.mjs](run.mjs),
`desiredDefinition`/`assertSameDefinition` trong
[render-bundle-config.mjs](lib/render-bundle-config.mjs).

Với mỗi bundle entry:

1. Mở Mẫu dựng ứng dụng; nhập ID, tên hiển thị, Git URL, thư mục bundle và ref.
2. Bấm Kiểm tra bundle; backend inspect/cache Git source.
3. Yêu cầu AVAILABLE, đúng URL/path và revision40 ký tự hex. Nếu input là
   full commit thì revision phải khớp. Ghi nhận bundle ID đã kiểm tra.
4. GET `/resource-definitions`, tìm renderer Definition theo key.
5. Đã có: so name/type/driver/account/inputs/provision/criteria, bao gồm bundle
   pin và placement. Khớp thì reuse; khác mặc định dừng. Khi --update-existing bật, chỉ bundle pin
   được đổi nếu các trường còn lại khớp: mở Sửa mẫu, chọn bundle đã xác minh,
   PUT qua form với version CAS/payload guard, GET xác nhận tăng version1.
   Không tạo Definition khác để né conflict. **Inspect luôn đi trước
   kiểm tra Definition tồn tại**, nên rerun vẫn cần backend truy cập Git.
6. Chưa có: điền namespace reference, cluster reference và render inputs qua
   control đúng kiểu; nhập criteria rồi đăng ký.
7. Đọc lại Definition và đối chiếu, kiểm mục đã lưu xuất hiện trên UI.

Bundle chứa template/patch; renderer Definition quy định khi nào dùng bundle,
placement và inputs. Presets hiện pin standard bundle ở commit
`1182cb5ff275451ebfa8b711bc07effd1e120fcf`; renderer mới match
`env_type=staging AND class=pvc`. Catalog có template không có nghĩa mọi workload
hiện có sẽ tự chọn renderer mới. Workload class pvc cần execution/template pin
phù hợp; bootstrap không tự chuyển workload cũ.

## 09 — Xác minh catalog và form volume mount

Code: phần sau vòng lặp bundle trong [run.mjs](run.mjs).

1. GET `/applications`. Dùng `verify.applicationKey` nếu đã chỉ định, nếu không
   chọn app đầu tiên có environment cần kiểm tra.
2. Chỉ định app nhưng không tìm thấy: dừng báo lỗi.
3. Không chỉ định app và chưa có app phù hợp: báo seed hoàn tất, bỏ kiểm tra
   picker/mount. Không tạo Application. Bước kiểm cache bên dưới cũng bị bỏ qua.
4. Có app: đọc workload templates, kiểm ID/version mong đợi và catalog không rỗng.
5. Đọc lại các bundle ID đã inspect, yêu cầu cache AVAILABLE.
6. Mở Thêm Workload; kiểm label/version và số card khớp metadata catalog.
7. Nếu có `verify.volumeMount`: chọn template, kiểm storage Type có outputs
   claimName/namespace string công khai, mở Thêm volume mount và thêm dependency
   vào form local; xác nhận mount path `/data`.

Không bấm Save draft, Preview hoặc Deploy. Đóng browser bỏ phần form local.

## 10 — Kết quả, lỗi và cleanup

Code: `catch`/`finally` trong [run.mjs](run.mjs), `closeConsole` trong
[console.mjs](lib/console.mjs).

- Hoàn tất bình thường: exit0; lỗi trong luồng chính: in FAIL và action cuối,
  đặt exit code1; không in raw Playwright error chứa giá trị đã điền.
- Khi có session, gọi đăng xuất và đóng browser trong finally.
- Update dùng [definition-action.mjs](lib/definition-action.mjs) để chặn thay đổi
  ngoài source/pin và [definition-update-ui.mjs](lib/definition-update-ui.mjs)
  để chặn PUT sai payload/version trước khi gửi backend. HTTP409 không tự retry.
- Không tự retry POST tạo mới, không rollback/xóa các mục đã đăng ký.
- Fail có thể để lại một phần seed hoặc metadata/cache từ inspect. Sửa nguyên
  nhân rồi chạy lại; mục khớp được dùng lại, mục khác cấu hình vẫn bị chặn.
- Không lưu browser session, trace, screenshot hay video trong runner này.
- Không cài Ceph/CSI/Vault/operator, tạo Application hoặc Deploy workload.

## Sửa phần nào trong code?

| Muốn thay đổi | Nơi sửa |
|---|---|
| URL mặc định, user seed, dữ liệu dev/staging | [targets/](targets/) và [target-config.mjs](lib/target-config.mjs) |
| Git URL/ref và chuyển installed module sang Git | [git-overrides.mjs](lib/git-overrides.mjs) |
| Update policy và PUT payload/CAS guard | [definition-action.mjs](lib/definition-action.mjs), [definition-update-ui.mjs](lib/definition-update-ui.mjs) |
| CLI hoặc thứ tự thực thi | [run.mjs](run.mjs) |
| Validate Type/bundle, so renderer Definition | [render-bundle-config.mjs](lib/render-bundle-config.mjs) |
| Validate account/infra, dựng Definition, pacing | [infrastructure-config.mjs](lib/infrastructure-config.mjs) |
| Thao tác account/Definition/source chooser | [infrastructure-ui.mjs](lib/infrastructure-ui.mjs) |
| Đăng nhập, file credential, browser/session | [console.mjs](lib/console.mjs) |
| Regression checks | Các file `lib/*.test.mjs`; chạy `npm test` tại thư mục này |

Sau khi sửa, cập nhật luồng này và README nếu behavior/cách chạy đổi. Chạy checks
đúng phần bị ảnh hưởng; không cần chạy Deploy ứng dụng để kiểm một thay đổi tài liệu.
