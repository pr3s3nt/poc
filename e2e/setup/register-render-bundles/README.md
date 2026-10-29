# Đăng ký Resource Types và Git render bundles bằng Playwright

Đăng nhập Console → Loại tài nguyên → nhập schema từng trường → Mẫu dựng ứng dụng
→ kiểm tra Git bundle → nhập placement/inputs/criteria qua form → đăng ký workload
Definition → mở Thêm Workload và xác minh catalog/volume mount.
Đây là **platform setup**, không phải nghiệm thu deploy ứng dụng.

Nguồn chuẩn: [UC-02 Resource Type](../../../orchestrator_docs/usecase/UC-02/specification.md),
[UC-03 RB-01..06](../../../orchestrator_docs/usecase/UC-03/specification.md#git-rendering-bundle-authoring--adr-021),
[Git bundle contract](../../../orchestrator_docs/architecture/contracts/git-render-bundles.md),
[workload catalog](../../../orchestrator_docs/architecture/contracts/workload-catalog.md#wc-c08--concrete-git-owned-catalog-contract),
[hướng dẫn E2E](../../README.md).

## Chạy trên máy khác

Cần Node.js 22.15+ (hoặc 24+), Console/backend hỗ trợ workload catalog, tài khoản
Platform Engineer/Admin, Chromium và kết nối mạng tới Console. Backend phải truy
cập được HTTPS Git source; browser chạy ở máy bạn không thay backend tải Git.
Console cần có ít nhất một Application với Environment để kiểm tra hộp chọn.
Không cần kubeconfig hoặc Docker trên máy chạy script. Cài từ root repository:

```bash
cd e2e
npm ci
npx playwright install chromium
cd ..
mkdir -p ~/.orch
cp e2e/setup/register-render-bundles/config.example.json ~/.orch/catalog.json
```

Sửa `~/.orch/catalog.json`. Đặt mật khẩu vào file riêng `~/.orch/password`, chỉ
chứa mật khẩu, rồi `chmod 600 ~/.orch/password`. Không đưa credential vào JSON,
Git URL hoặc command line. Chạy từ root repository:

```bash
ORCH_PLATFORM_URL=http://100.77.224.24:18596 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE="$HOME/.orch/password" \
ORCH_HEADLESS=0 \
ORCH_STEP_DELAY_MS=1000 \
node e2e/setup/register-render-bundles/run.mjs --config "$HOME/.orch/catalog.json"
```

Đổi URL/tài khoản/password-file cho máy đích. Mặc định hiện browser (cần
desktop/display), nghỉ 800 ms trước/sau mỗi thao tác và nhập lần lượt từng ký tự.
`ORCH_STEP_DELAY_MS` nhận 0..10000; tăng lên 2000 nếu muốn xem chậm hơn. Dùng
`ORCH_HEADLESS=1` khi chạy automation không có display. Script dùng helper đăng nhập chung, đặt locale tiếng Việt
trong browser context riêng; không phụ thuộc ngôn ngữ bạn đang chọn trên Console.
Có thể chạy từ `e2e/` bằng `npm run setup:render-bundles -- --config /path/catalog.json`.
Đường dẫn JSON được tính từ working directory; helper/import không phụ thuộc cwd.

## Tùy chỉnh catalog

`bundles` nhận một hoặc nhiều entry; mỗi entry đăng ký một rendering Definition.
Một bundle có thể cung cấp nhiều mẫu trong `bundle.yaml.workloadTemplates`.
`resourceTypes` là danh sách tùy chọn; config cũ chỉ có `bundles` vẫn chạy được.
File JSON chỉ là cấu hình bên ngoài: script **không nhập Score, không mở JSON
nâng cao, không dán JSON vào UI và không dùng API để tạo/sửa dữ liệu**.

| Trường | Ý nghĩa |
|---|---|
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
| `verify.volumeMount` | Tùy chọn `{templateId,templateVersion,resourceType}`: chọn mẫu, thêm dependency và mount `/data` trên form local, không Save |

Ví dụ hiện có đăng ký `persistent-storage`, input `storage` tùy chọn và hai
output string công khai bắt buộc `claimName`/`namespace`. Nhờ đó Backend có thể
chọn dependency này và bật Thêm volume mount. Đây là contract/schema; **để Preview
và Deploy vẫn cần Terraform Resource Definition phù hợp**. Script không tự thêm
Definition hạ tầng, tạo PVC hoặc gắn storage của ứng dụng khác.

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
catalog không tự tạo cluster/namespace Definitions, Secret Store hoặc infrastructure.

## Assertions, chạy lại và side effects

Các mutation đi qua từng trường UI thật; API GET chỉ đọc kỹ thuật để so sánh schema,
pin, toàn bộ Definition, availability và templates. PASS khi schema/pin/cấu hình
đã lưu khớp file cấu hình, mọi expected template có trên API và hộp chọn hiển thị đúng số mẫu.
Nếu có `verify.volumeMount`, script xác minh nút bị khóa trước khi thêm dependency,
bật sau khi chọn storage type và thêm được mount path `/data`.
Không tạo workload/draft, Preview, Deploy, sửa database schema hoặc mutate cluster/cloud.

Chạy lại cùng cấu hình sẽ kiểm tra Git source và reuse Definition cùng ID nếu
toàn bộ cấu hình/pin khớp; không POST Definition lần hai. Khác bất kỳ cấu hình
nào thì FAIL, không update/delete. Với branch/tag đã đổi commit, dùng ID mới hoặc
chỉnh Definition thủ công sau review. Không tự retry POST khi kết quả chưa rõ.
Script có thể đã lưu bundle/Definition trước khi bước sau thất bại; kiểm tra
Console rồi chạy lại. Không rollback hay xóa tài nguyên đã có.

Resource Types, bundle cache/metadata và Definitions được **giữ lại** để người dùng test. Session
do script mở được sign out và browser đóng trong `finally`; không lưu storage
state, screenshot, video hoặc trace. Mật khẩu vẫn ở file riêng ngoài Git. Không
có cleanup namespace/PVC vì script không tạo chúng. Nếu chưa có Application,
script báo rõ registration đã xong nhưng verification chưa hoàn tất; tạo
Application qua Console rồi chạy lại.

Kiểm tra local helpers: `cd e2e && npm test`. Kết quả trên một Console không
chứng minh deployment hoặc credentials/placement ở máy khác đã đúng.
