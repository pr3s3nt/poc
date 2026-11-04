# Bootstrap staging với GitHub Enterprise

Mục tiêu: AI nội bộ điền dữ liệu môi trường trong input, khóa phiên bản nguồn,
rồi chạy cùng script. Không sửa code để thay hostname, tên môi trường hay image.
Bắt đầu từ [bàn giao cho AI](AI_HANDOFF.md), [chú thích input](INPUT_REFERENCE.md),
[root workflow](../../../AGENTS.md) và [scope](../../../orchestrator_docs/implementation/direct-bootstrap.md).

## Bộ file

| File | Vai trò |
|---|---|
| `staging.input.json` | Mẫu input schema 3; thay các giá trị công ty và commit, không chứa secret |
| `../../../projects/platform-resources/` | Resource Types, bảy Definitions, ba Workloads và policy RBAC dùng chung, lưu trong Git nội bộ |
| `../lock-enterprise-input.mjs` | Tính pin/hash từ đúng commit đã có trong checkout, xuất file input riêng tư |
| `INPUT_REFERENCE.md` | Giải thích trường cấu hình, giá trị nào cần thay |
| `AI_HANDOFF.md` | Trình tự setup, kiểm tra, chạy, xử lý lỗi và bàn giao |

Các Definition/Workload dùng `${bootstrap.NAME}` để nhận giá trị nonsecret từ
`parameters`. Chỉ thay dữ liệu đã parse, không chèn chuỗi vào JSON/YAML thô.
Các placeholder của sản phẩm như `${context...}`, `${resources...}` và
`${templateSources...}` vẫn giữ nguyên ý nghĩa. SHA-256 xác minh file Git gốc
trước khi áp dụng tham số. Không dùng tham số cho mật khẩu/token.

## Luồng chạy

1. Đưa **nội dung** `projects/platform-resources/` vào root repo GitHub Enterprise riêng:
   root có `terraform/`, `templates/`, `bootstrap/`. Ba URL nguồn có thể cùng repo.
   Dùng preset tương thích release công cụ, chọn commit đã review. Đây là công việc publish
   có quyền riêng; công cụ lock không tự push.
2. Copy `staging.input.json` ra thư mục riêng tư, ví dụ `/var/lib/orch-bootstrap/staging/`.
   Điền URL, context/endpoint, đường dẫn credential, registry, Vault, access URL,
   Git repo và full commits. Điền `parameters.*ModulePath` nếu dùng đường dẫn khác.
3. Có checkout local đúng HTTPS origin của từng repo; fetch commit bằng quy trình
   credential nội bộ, không nhúng token vào URL. Có thể dùng cùng một checkout
   cho ba nhóm nguồn nếu là một monorepo.
4. Cài dependencies: Node 24, Git, `npm ci --prefix scripts/bootstrap`,
   `npm exec --prefix scripts/bootstrap -- playwright install chromium`. Khi chạy
   kịch bản E2E cài thêm `npm ci --prefix e2e`. Cần `kubectl` tương thích cluster.
5. Khóa input từ root checkout công cụ:

```sh
node scripts/bootstrap/lock-enterprise-input.mjs \
  --input /var/lib/orch-bootstrap/staging/staging.input.json \
  --output /var/lib/orch-bootstrap/staging/staging.locked.json \
  --inputs-checkout /opt/company/platform-sources \
  --modules-checkout /opt/company/platform-sources \
  --templates-checkout /opt/company/platform-sources
```

Input và output phải ở cùng thư mục riêng tư để đường dẫn tương đối không đổi.
Output mới được tạo với quyền 0600; không ghi đè file đã có. Đọc commit từ Git,
không hash working tree đang sửa. Helper từ chối origin khác, ref không phải full
commit, file symlink/executable và path thiếu. Hash không thay cho contract validation.

6. Giữ `runtime.mode=check`, chạy:

```sh
node scripts/bootstrap/run.mjs --config /var/lib/orch-bootstrap/staging/staging.locked.json
```

7. Sau khi kiểm tra và có quyền target, đổi **chỉ** `runtime.mode` sang `bootstrap`
   để cấu hình platform, hoặc `all` để cấu hình rồi deploy acceptance app. Dùng
   cùng lệnh. Chạy lại cùng input/state để xác minh lũy đẳng; không xóa state.

`check` vẫn cần credential, kết nối Console/cluster và đọc Git inputs platform;
đây không phải kiểm tra offline. Workload files được xác minh khi dùng mode `all`,
trước mutation của lượt đó. `bootstrap` không nghiệm thu deploy. `all` chỉ thành công khi
app, job, storage và CD/Fleet checks đều pass. Ví dụ staging không tự cấp quyền
chạy kịch bản Pod recreation: runner Git/PVC hiện giới hạn dev; chỉ mở scope staging
sau khi được duyệt riêng. Không đổi environment thành dev để vượt guard.

## GitHub Enterprise và CA

Preset dùng platform Git chung, cùng một Organization chứa source repos và các
repo manifest do Orchestrator tạo. Host Git, owner, REST API `/api/v3` và CA được
backend suy ra từ platform Git settings; không nhập các trường CD nội bộ/read-only
để né discovery. Repo nguồn là Git-only, không Catalog/inline fallback.

GHES API base là `https://HOSTNAME/api/v3`, theo
[GitHub documentation](https://docs.github.com/en/enterprise-server@3.18/rest/using-the-rest-api/getting-started-with-the-rest-api).
Token cần quyền đọc source và tạo/ghi private application repositories trong org;
chỉ quyền pull source chưa đủ cho CD. CA được dùng cho Git fetch, Git publication,
Fleet đọc repo và HTTPS API. Browser/Node/OS truy cập Console cũng cần trust chain
của Console; cấu hình Git CA không tự cài trust vào toàn bộ máy.

Không có quyền tạo repo thì dừng Verify/CD và nhờ owner cấp đúng quyền; không tự
chuyển sang repo dùng chung hoặc disable ownership checks. Preset không bao quát
multi-org credentials, Fleet management/target ở các cluster khác nhau hoặc module
contract khác acceptance contract nếu chưa cấu hình và kiểm chứng riêng.
