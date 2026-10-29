# Đăng ký Git render bundles bằng Playwright

Đăng nhập Console → Mẫu dựng ứng dụng → kiểm tra Git bundle → đăng ký workload
Definition → kiểm tra lại cấu hình đã lưu → mở Thêm Workload và xác minh catalog.
Đây là **platform setup**, không phải nghiệm thu deploy ứng dụng.

Nguồn chuẩn: [UC-03 RB-01..06](../../../orchestrator_docs/usecase/UC-03/specification.md#git-rendering-bundle-authoring--adr-021),
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
ORCH_HEADLESS=1 \
node e2e/setup/register-render-bundles/run.mjs --config "$HOME/.orch/catalog.json"
```

Đổi URL/tài khoản/password-file cho máy đích. Bỏ `ORCH_HEADLESS=1` để xem browser
(cần desktop/display). Script dùng helper đăng nhập chung, đặt locale tiếng Việt
trong browser context riêng; không phụ thuộc ngôn ngữ bạn đang chọn trên Console.
Có thể chạy từ `e2e/` bằng `npm run setup:render-bundles -- --config /path/catalog.json`.
Đường dẫn JSON được tính từ working directory; helper/import không phụ thuộc cwd.

## Tùy chỉnh catalog

`bundles` nhận một hoặc nhiều entry; mỗi entry đăng ký một rendering Definition.
Một bundle có thể cung cấp nhiều mẫu trong `bundle.yaml.workloadTemplates`.

| Trường | Ý nghĩa |
|---|---|
| `key`, `name` | ID duy nhất và tên hiển thị của rendering Definition |
| `source.url`, `path`, `ref` | Git HTTPS repo, thư mục bundle và commit/tag/branch; nên pin full commit để chạy lại ổn định |
| `placement.namespace`, `cluster` | Whole output references phù hợp Resource Definitions của máy đích |
| `renderInputs` | Object literal theo inputs schema của bundle, ví dụ `{"team":"payments","defaultReplicas":2,"runAsNonRoot":true}` với standard bundle |
| `provision` | Companion rules theo contract hiện hành; mặc định `{}` |
| `criteria` | OR giữa các object, AND giữa các field `env_type`, `app_id`, `env_id`, `res_id`, `class`; mặc định `[{}]` là wildcard |
| `expectedTemplates` | Các `{id, version}` phải xuất hiện trong catalog/picker sau đăng ký; mặc định `[]` chỉ kiểm tra catalog không rỗng |
| `verify.applicationKey` | Application để mở picker; nếu bỏ, chọn Application đầu tiên có Environment được yêu cầu |
| `verify.environment` | `staging` (mặc định) hoặc `production` |

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

Các mutation đi qua UI thật; API GET chỉ đọc kỹ thuật để so sánh pin, toàn bộ
Definition, availability và templates. PASS khi pin hợp lệ, cấu hình đã lưu
khớp JSON, mọi expected template có trên API và hộp chọn hiển thị đúng số mẫu.
Không tạo workload/draft, Preview, Deploy, sửa database schema hoặc mutate cluster/cloud.

Chạy lại cùng cấu hình sẽ kiểm tra Git source và reuse Definition cùng ID nếu
toàn bộ cấu hình/pin khớp; không POST Definition lần hai. Khác bất kỳ cấu hình
nào thì FAIL, không update/delete. Với branch/tag đã đổi commit, dùng ID mới hoặc
chỉnh Definition thủ công sau review. Không tự retry POST khi kết quả chưa rõ.
Script có thể đã lưu bundle/Definition trước khi bước sau thất bại; kiểm tra
Console rồi chạy lại. Không rollback hay xóa tài nguyên đã có.

Bundle cache/metadata và Definitions được **giữ lại** để người dùng test. Session
do script mở được sign out và browser đóng trong `finally`; không lưu storage
state, screenshot, video hoặc trace. Mật khẩu vẫn ở file riêng ngoài Git. Không
có cleanup namespace/PVC vì script không tạo chúng. Nếu chưa có Application,
script báo rõ registration đã xong nhưng verification chưa hoàn tất; tạo
Application qua Console rồi chạy lại.

Kiểm tra local helpers: `cd e2e && npm test`. Kết quả trên một Console không
chứng minh deployment hoặc credentials/placement ở máy khác đã đúng.
