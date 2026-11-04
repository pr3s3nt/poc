# Bootstrap staging với GitHub Enterprise

Mục tiêu: AI nội bộ điền dữ liệu môi trường trong input, chuẩn bị bản chụp input,
rồi chạy cùng script. Không sửa code để thay hostname, tên môi trường hay image.
Bắt đầu từ [bàn giao cho AI](AI_HANDOFF.md), [chú thích input](INPUT_REFERENCE.md),
[root workflow](../../../AGENTS.md) và [scope](../../../orchestrator_docs/implementation/direct-bootstrap.md).

## Bộ file

| File | Vai trò |
|---|---|
| `staging.input.json` | Mẫu input schema 3; thay các giá trị công ty và commit, không chứa secret |
| `../../../projects/platform-resources/` | Resource Types, chín Definitions, ba Workloads và policy RBAC dùng chung, lưu trong Git nội bộ |
| `../prepare-enterprise-input.mjs` | Chuẩn bị local snapshot/pins không cần Git CLI; dùng cho staging Console |
| `../lock-enterprise-input.mjs` | Tùy chọn cho quy trình dev/prepare có Git CLI, xác minh origin và exact Git objects |
| `INPUT_REFERENCE.md` | Giải thích trường cấu hình, giá trị nào cần thay |
| `AI_HANDOFF.md` | Trình tự setup, kiểm tra, chạy, xử lý lỗi và bàn giao |

Các Definition/Workload dùng `${bootstrap.NAME}` để nhận giá trị nonsecret từ
`parameters`. Chỉ thay dữ liệu đã parse, không chèn chuỗi vào JSON/YAML thô.
Các placeholder của sản phẩm như `${context...}`, `${resources...}` và
`${templateSources...}` vẫn giữ nguyên ý nghĩa. SHA-256 xác minh file Git gốc
trước khi áp dụng tham số. Không dùng tham số cho mật khẩu/token.

## Luồng staging qua Console (không cần git/kubectl/gh CLI)

`runtime.profile=console` là chế độ mặc định cho schema 3. Máy chạy cần Node 24,
Playwright/Chromium và input đã chuẩn bị. Giữ các file credential riêng tư để điền
qua UI; kubeconfig của platform account dùng cho onboarding qua Console, không
phải quyền để runner gọi Kubernetes trực tiếp.

1. AI nội bộ chuẩn bị nội dung repo tài nguyên đã review (pull/export bằng quy
   trình công ty). Root nguồn có `terraform/`, `templates/`, `bootstrap/`.
2. Copy `staging.input.json` ra thư mục riêng tư, ví dụ
   `/var/lib/orch-bootstrap/staging/`. Điền endpoint, account, credential file,
   registry/image, Vault, URL nguồn và ba full commits theo INPUT_REFERENCE.
   Có thể dùng cùng một repo/commit cho ba nhóm nguồn.
3. Cài `npm ci --prefix scripts/bootstrap` và
   `npm exec --prefix scripts/bootstrap -- playwright install chromium` từ
   checkout công cụ đã có sẵn. Dùng registry/mirror nội bộ theo chính sách công ty.
4. Chuẩn bị input từ các file đã có, **không gọi Git, Kubernetes hoặc mạng**:

```sh
node scripts/bootstrap/prepare-enterprise-input.mjs \
  --input /var/lib/orch-bootstrap/staging/staging.input.json \
  --output /var/lib/orch-bootstrap/staging/staging.prepared.json \
  --resources /opt/company/platform-resources
```

Nếu tách repo nguồn, dùng `--inputs-dir`, `--modules-dir`, `--templates-dir`
thay cho `--resources`. Input/output phải cùng thư mục riêng tư, output mới chưa
có. Helper tạo file input 0600 và thư mục `prepared-inputs-*` chứa bản chụp business
documents; giữ cùng bộ khi chuyển máy. Không copy credential vào snapshot.

Helper tính hash của bytes local và pins tương thích release, không xác minh
checkout đó thuộc remote/commit đã khai báo. AI phải bảo đảm snapshot đã review;
backend sẽ lấy module/template từ Git và xác minh commit/hash khi inspect qua UI.
Business documents được kiểm SHA-256 local; report ghi `gitCommitVerified=false`,
không báo là đã kiểm chứng remote Git. Working-tree chỉnh khác module/template
của commit sẽ bị backend chặn bằng digest. Không điền hash thủ công.

5. Giữ `runtime.mode=check`, chạy qua Playwright:

```sh
node scripts/bootstrap/run.mjs --config /var/lib/orch-bootstrap/staging/staging.prepared.json
```

6. Khi kiểm tra qua Console đạt và target đã được cấp quyền, đổi **chỉ** mode
   thành `bootstrap` để cấu hình platform; `all` thêm deploy acceptance app và
   browser business/file checks. Chạy cùng lệnh và giữ state khi rerun.

Chế độ Console không tạo Role/RoleBinding, không port-forward, không đọc cluster
trực tiếp. Fleet/VSO/RBAC, registry pull access và route tới workload là prerequisites
của platform. Thiếu quyền thì xử lý theo quy trình vận hành; không bật native setup
ngầm để chạy qua. Report chỉ xác nhận Console/deployment/readiness do backend báo
và browser checks; không tuyên bố đã đọc Fleet commit/PVC UID hoặc tái tạo Pod.
`check` chỉ đọc qua Console và kiểm input, không phải kiểm chứng cluster độc lập.

## Luồng test dev riêng

Các preset `git-targets/` và `direct-targets/` opt-in
`runtime.profile=dev-test`. Chế độ này giữ CLI Git/native cluster checks, scoped
namespace RBAC và optional port-forward cho dev đã được cấp quyền. Công cụ
`lock-enterprise-input.mjs` vẫn dành cho người muốn xác minh object Git chính xác
trong bước chuẩn bị có Git CLI; không bắt buộc cho staging Console.

Không dùng chung state giữa hai profile: report/evidence và quyền khác nhau.
Không đổi environment thành dev hoặc xóa state để vượt guard của scenario PVC.

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

Input schema-3 dev cũ không có profile cần thêm `runtime.profile=dev-test` để
giữ luồng native. Không chuyển state/evidence dev sang console bằng cách đổi
profile hoặc xóa receipt. Dùng thư mục state/report riêng cho Console staging.
