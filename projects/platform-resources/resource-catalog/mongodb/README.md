# MongoDB Resource Type và Definition

- `mongodb-type.yaml`: nhập trước tại **Loại tài nguyên → Nhập YAML/JSON**.
- `mongodb-definition.yaml`: nhập sau tại **Cấu hình tài nguyên → Nhập YAML/JSON**.
- Module Git tương ứng: `terraform/mongodb` trong repo phát hành
  `pr3s3nt/orchestrator-workload-templates`.

Mỗi YAML là một body đăng ký độc lập. Thay toàn bộ `<...>` trong Definition
bằng bộ pin từ cùng kết quả **Kiểm tra module** trên Console đích và image
MongoDB đã pin digest. Chọn revision có module MongoDB này. Không tự lấy module
ID/hash từ installation khác hoặc dùng Git URL thay `source_id`.

Mẫu dùng `env_type: staging`, resource class `in-cluster`, cluster class
`bootstrap-dev` và namespace class `default`. Sửa theo các Definitions đang có
trên hệ thống đích; giữ nguyên biểu thức `${resources[...]...}` và `@app/@env`.
StorageClass mặc định phải tồn tại hoặc điền `storage_class` cụ thể. Registry
private cần Secret đã có trong namespace và tên `image_pull_secret_name`.

## Contract

| Input | Kiểu | Bắt buộc | Fallback |
|---|---|---|---|
| `database` | string | Có | Không; workload phải truyền |
| `username` | string | Không | `app` |
| `storage` | string | Không | `10Gi` |

Output bắt buộc: `host` string, `port` number, `database` string, `username`
string, `password` secret string và `connection` secret string. Password do
executor sinh; không điền password hoặc root password vào Definition.
Không xuất PVC/namespace trong contract ứng dụng. Image, storage class, root
username và placement thuộc Definition.

Ứng dụng có thể bind `connection` vào `DATABASE_URL`; chuỗi dùng tài khoản
`readWrite` của database ứng dụng và `authSource` tương ứng. Type mới này dành
cho đăng ký mới; import không thay schema của Type `mongodb` đã có.

Module là một MongoDB instance trên Kubernetes, có PVC và Service nội bộ;
không cung cấp replica set, HA hoặc backup tự động. Đăng ký file chỉ lưu cấu
hình; chưa tạo tài nguyên hay chứng minh kết nối thật. Image phải tương thích
Docker Official Image MongoDB, có `mongosh` và hỗ trợ init script. Không đổi
user/database/password trên volume đã khởi tạo để thay cho migration/rotation.

## Kiểm tra module

Terraform >=1.7 cho `terraform test` với `mock_provider` (module yêu cầu >=1.5).
Trong checkout kiểm tra riêng, chạy:

```sh
terraform -chdir=terraform/mongodb init -backend=false
terraform -chdir=terraform/mongodb fmt -check
terraform -chdir=terraform/mongodb validate
terraform -chdir=terraform/mongodb test
```

Các test dùng mock, không kết nối Kubernetes hoặc MongoDB thật. Dữ liệu trong
test là fixture; không dùng image digest giả hoặc password test khi triển khai.
