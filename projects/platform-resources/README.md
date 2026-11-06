# Platform resources — nguồn Git cho Orchestrator

Đưa **nội dung thư mục này** lên root một repo GitHub Enterprise riêng:

```
terraform/             # 11 Terraform modules
templates/standard/  # bộ template chuẩn
bootstrap/             # Resource Types, Definitions, Workloads, policy
README.md
AGENTS.md
```

Không tạo thêm tầng `platform-resources/` trong repo mới. Definitions nằm ở
`bootstrap/definitions/`; chúng tham chiếu module trong `terraform/` qua các
tham số input. Preset staging đăng ký namespace/postgres/redis/storage và mysql/mariadb/mongodb;
các module AWS chỉ là mã nguồn bổ sung, không phải quyền triển khai AWS.

## Chuyển sang Git công ty

1. Pull nhánh `main` repo Orchestrator, lấy nội dung `projects/platform-resources/`
   đưa lên repo nội bộ theo quy trình công ty và commit.
2. Lấy HTTPS clone URL và full commit SHA đã review. Không nhúng token vào URL.
3. Trong bản `staging.input.json` riêng tư, thay `parameters.inputsRepository`,
   `parameters.modulesRepository`, `parameters.templatesRepository` bằng URL mới. Có thể dùng
   cùng một URL cho cả ba. Thay ba tham số commit tương ứng bằng full SHA.
4. Giữ các đường dẫn mặc định `bootstrap/...`, `terraform/...`,
   `templates/standard`. Nếu đổi cấu trúc repo, cập nhật input tương ứng.
5. Từ bộ nguồn đã có, chạy `prepare-enterprise-input.mjs` trong repo Orchestrator
   để tạo private snapshot/pins cho Console profile (không cần git/kubectl CLI).
   Helper hash bytes local; backend kiểm module/template với Git commit thật.
   Xem setup kit để phân biệt local snapshot với proof remote commit.
6. Điền các dữ liệu staging còn lại theo hướng dẫn; chạy mode `check`, rồi
   `bootstrap`/`all` khi target đã được cấp quyền.

Input mẫu, chú thích và hướng dẫn AI chuẩn ở `scripts/bootstrap/enterprise/`
trong repo Orchestrator.
Đây là repo tài nguyên, không chứa chương trình bootstrap hoặc ứng dụng
Orchestrator; chạy công cụ từ checkout Orchestrator đúng release.

## Quy tắc duy trì

Không commit credential, kubeconfig, state, reports, video hoặc cache Terraform.
Không dùng branch/latest thay exact commit. Thay module phải review contract và
khóa lại input. Module không tự tạo image: vẫn cần image registry nội bộ phù hợp.

Các bản module/template embed trong backend Orchestrator được giữ để tương thích
release đã phát hành. Thư mục này là bộ nguồn để publish Git; không sửa ngược
các snapshot embed bất biến khi tùy chỉnh repo công ty.

Image PostgreSQL và Redis được điền tại `parameters.postgresImage` và
`parameters.redisImage` trong input riêng tư, dạng digest-pinned. Image của
backend/worker/frontend tiếp tục nằm tại `application.images`.

MySQL, MariaDB và MongoDB cũng có `<db>Image`, `<db>AuthSecretName`, tên key,
storage và pull Secret trong input. Xem [hướng dẫn database](bootstrap/DATABASES.md).

HTTPRoute nằm trong workload template, cùng manifest Git/Fleet với Service.
Điền nhóm `parameters.httprouteGateway*` và `endpoint*`; frontend mẫu khai báo
resource `endpoint`, Deploy giữ chỗ hostname và route trỏ Service của chính workload. Không tạo Gateway/controller/state Terraform.
Xem [HTTPRoute setup](bootstrap/HTTPROUTE.md).
