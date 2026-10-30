# Platform catalog — copy sang Git công ty

Copy **toàn bộ nội dung thư mục này** làm gốc repo Git công ty. Không cần copy
backend Go, frontend, database, kubeconfig hay credential của platform.

```text
<repo-cong-ty>/
├── README.md
├── SOURCE_MAP.json
└── backend/
    ├── terraform-modules/
    │   ├── k8s-namespace/main.tf
    │   ├── persistent-storage/main.tf
    │   ├── postgres-k8s/main.tf
    │   ├── redis-k8s/main.tf
    │   ├── mysql/main.tf
    │   ├── vpc/main.tf
    │   ├── eks/main.tf
    │   ├── aurora/main.tf
    │   └── elasticache/main.tf
    └── render-bundles/
        └── standard/
            ├── bundle.yaml
            ├── defaults.yaml
            ├── inputs.schema.json
            ├── normalize/
            ├── provisioners/
            ├── patches/
            ├── resources/
            └── examples/
```

Ví dụ sau khi copy, thư mục `backend/` phải ở ngay gốc repo công ty. Nếu bạn
commit cả thư mục bao ngoài `platform-catalog/`, phải thêm prefix đó vào các
đường dẫn source trong bootstrap. Giữ layout như trên thì chỉ đổi Git URL/ref;
tên module, đường dẫn con và mappings vẫn giữ nguyên.

## Terraform modules

| Thư mục | Công dụng |
|---|---|
| k8s-namespace | Namespace trên cluster đã có |
| persistent-storage | PVC; có biến storage_class để chọn rook-ceph-block |
| postgres-k8s | PostgreSQL trên Kubernetes |
| redis-k8s | Redis trên Kubernetes |
| mysql | MySQL reference module; yêu cầu image tương thích và Secret refs đã có |
| vpc, eks, aurora, elasticache | Các modules AWS có trong code platform; không dùng trong preset staging Kubernetes |

Mỗi module là một Git subdirectory riêng, ví dụ
`backend/terraform-modules/persistent-storage`. Module không tự cài Ceph/CSI.
Có mã nguồn không đồng nghĩa được quyền chạy Terraform lên Kubernetes/AWS.
Provider mirror, allowlist/source policy và kết nối Git là cấu hình backend
platform; copy repo này không tự cấu hình các thành phần đó.

## Render bundle standard

Source path: `backend/render-bundles/standard`.
Có năm templates: Frontend web, Backend API, Xử lý nền, Ứng dụng có dữ liệu,
Tác vụ theo lịch. Deployment có persistent mount dùng Recreate; những workload
khác giữ controller/rollout policy tương ứng. Không có credential trong bundle.

## Sau khi push repo công ty

1. Ghi lại Git HTTPS URL và commit của **repo công ty**.
2. Dùng URL/commit đó khi inspect Terraform module hoặc render bundle trên
   platform. Backend platform phải truy cập được Git/CA/auth theo cấu hình của nó.
3. Không dùng commit GitHub cũ sau khi chỉ copy file sang repo mới: nội dung giống
   nhau không làm Git commit giống nhau. Bootstrap Terraform Git source hiện cần
   full commit, nên lấy SHA đầy đủ trong repo công ty.
4. Bootstrap hỗ trợ `--git-url <repo-HTTPS> --git-ref <full-commit-SHA40>`.
   Khi có Git URL override, các installed Terraform modules trong preset được
   chuyển sang subdirectory tương ứng ở đây; Git sources giữ path hiện có.
   Mappings/criteria/placement giữ nguyên. Nếu cần chuyển Definition đã tồn tại,
   thêm `--update-existing` để đổi source/pin qua version CAS, không reset dữ liệu.
   Mặc định khác cấu hình vẫn dừng; khác các trường ngoài source/pin vẫn bị chặn
   dù có cờ update. Không tự Deploy hoặc chuyển resource state đã tạo.

Đăng ký/inspect chỉ seed metadata/cache. Apply tài nguyên xảy ra sau Preview →
Deploy; không chạy `terraform apply` hoặc `destroy` chỉ vì vừa copy repo.

## Nguồn và cập nhật snapshot

Đây là bản copy để phân phối theo yêu cầu người dùng, không phải source runtime
mới mà backend tự nạp. `SOURCE_MAP.json` ghi revision nguồn, đường dẫn file gốc,
đường dẫn xuất và SHA-256. Trong repo platform, sửa nguồn chuẩn trước rồi đồng bộ
bản copy này nếu muốn phát hành lại. Không sửa ngầm một bản mà cho rằng bản còn
lại tự thay đổi.

Các file runtime `.tf`, `.tpl`, YAML/JSON và fixtures được copy nguyên nội dung.
README của bundle chỉ đổi link tài liệu sang GitHub để mở độc lập. Vì mọi file
trong bundle tham gia digest, bản copy phải được inspect để lấy pin/digest mới;
không tự gán lại digest hay sửa metadata pin cũ.
