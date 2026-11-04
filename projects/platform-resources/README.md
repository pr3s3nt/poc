# Platform resources — nguồn Git cho Orchestrator

Đưa **nội dung thư mục này** lên root một repo GitHub Enterprise riêng:

```
terraform/             # 9 Terraform modules
templates/standard/  # bộ template chuẩn
bootstrap/             # Resource Types, Definitions, Workloads, policy
README.md
AGENTS.md
```

Không tạo thêm tầng `platform-resources/` trong repo mới. Definitions nằm ở
`bootstrap/definitions/`; chúng tham chiếu module trong `terraform/` qua các
tham số input. Preset staging dùng bốn module namespace/postgres/redis/storage;
MySQL có thêm Definition riêng và yêu cầu Secret có sẵn trong namespace;
các module AWS chỉ là mã nguồn bổ sung, không phải quyền triển khai AWS.

## Chuyển sang Git công ty

1. Giải nén, đưa nội dung thư mục lên repo nội bộ theo quy trình công ty và commit.
2. Lấy HTTPS clone URL và full commit SHA đã review. Không nhúng token vào URL.
3. Trong bản `staging.input.json` riêng tư, thay `parameters.inputsRepository`,
   `parameters.modulesRepository`, `parameters.templatesRepository` bằng URL mới. Có thể dùng
   cùng một URL cho cả ba. Thay ba tham số commit tương ứng bằng full SHA.
4. Giữ các đường dẫn mặc định `bootstrap/...`, `terraform/...`,
   `templates/standard`. Nếu đổi cấu trúc repo, cập nhật input tương ứng.
5. Checkout commit từ đúng HTTPS origin rồi chạy `lock-enterprise-input.mjs`
   trong repo Orchestrator. Tool tính lại source ID, module ID và SHA; không điền
   hash thủ công. Dùng cùng checkout cho ba tùy chọn nếu chung repo.
6. Điền các dữ liệu staging còn lại theo hướng dẫn; chạy mode `check`, rồi
   `bootstrap`/`all` khi target đã được cấp quyền.

Bản ZIP bàn giao có thư mục `setup/` chứa input mẫu, chú thích và hướng dẫn AI.
Trong source Orchestrator, các file chuẩn ở `scripts/bootstrap/enterprise/`.
Đây là repo tài nguyên, không chứa chương trình bootstrap hoặc ứng dụng
Orchestrator; chạy công cụ từ checkout Orchestrator đúng release.

## Quy tắc duy trì

Không commit credential, kubeconfig, state, reports, video hoặc cache Terraform.
Không dùng branch/latest thay exact commit. Thay module phải review contract và
khóa lại input. Module không tự tạo image: vẫn cần image registry nội bộ phù hợp.

Các bản module/template embed trong backend Orchestrator được giữ để tương thích
release đã phát hành. Thư mục này là bộ nguồn để publish Git; không sửa ngược
các snapshot embed bất biến khi tùy chỉnh repo công ty.

Image hạ tầng được điền tại `parameters.postgresImage`, `parameters.redisImage`,
`parameters.mysqlImage` trong input riêng tư, dạng digest-pinned. Image của
backend/worker/frontend tiếp tục nằm tại `application.images`. Với MySQL, xem
`terraform/mysql/README.md` và các tham số Secret/storage trong INPUT_REFERENCE.
