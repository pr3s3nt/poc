# Dùng MySQL, MariaDB và MongoDB

Bộ nguồn có ba Terraform modules và ba Definitions class `in-cluster`.
Bootstrap đăng ký chúng; workload phải khai báo resource mới tạo database.
Đây là single-instance phục vụ môi trường được duyệt, không có HA/backup tự động.

| Type | Port | Image tương thích | Data path | Auth database |
|---|---|---|---|---|
| mysql | 3306 | Docker Official Image MySQL 8.x | /var/lib/mysql | Database ứng dụng |
| mariadb | 3306 | Docker Official Image MariaDB với mariadb client | /var/lib/mysql | Database ứng dụng |
| mongodb | 27017 | Docker Official Image MongoDB với mongosh | /data/db | Database ứng dụng; root ở admin |

## Điền input

Trong `scripts/bootstrap/enterprise/staging.input.json` riêng tư, mỗi `<db>` có:
`<db>Image`, `<db>ModulePath`, `<db>Database`, `<db>Username`, `<db>AuthSecretName`,
`<db>RootPasswordKey`, `<db>UserPasswordKey`, `<db>Storage`, `<db>StorageClass`,
`<db>ImagePullSecretName`. MongoDB có thêm `mongodbRootUsername`.
Giữ module ID/hash là LOCK_WITH_HELPER cho locker tính từ commit đã review.

Image phải pin digest; không nhúng token vào URL. Pull Secret tùy chọn phải có
sẵn trong namespace ứng dụng. Node vẫn cần trust CA và DNS registry công ty.
Cần image tương thích entrypoint chính thức; module không nhận thông số UID/
entrypoint tùy ý. Image chính thức có thể khởi động entrypoint bằng root trước
khi chuyển sang user database; kiểm policy admission của cluster trước deploy.

## Secret

Cấp hai mật khẩu riêng root/app qua Secret có sẵn trong namespace của resource,
ví dụ Secret tên `mysql-auth` với key `root-password` và `user-password`.
Dùng Vault/VSO hoặc quy trình đã được duyệt; Terraform chỉ đặt secretKeyRef,
không đọc Secret và không lưu password vào input/output. Với MariaDB/MongoDB dùng
tên Secret riêng tương ứng. Consumer nhận mật khẩu qua cơ chế secret delivery
của công ty; không lấy password qua Resource Type outputs.

## Resource declaration trong workload

Đây là phần `resources` để ghép vào workload phù hợp của bạn, không phải manifest
độc lập để apply vào Kubernetes. Ví dụ MySQL:

```yaml
resources:
  db:
    type: mysql
    class: in-cluster
    id: app-db
    params:
      image: registry.company.example/platform/mysql@sha256:REPLACE_64_HEX_DIGEST
      database: appdb
      username: app
      auth_secret_name: mysql-auth
      root_password_key: root-password
      user_password_key: user-password
      storage: 10Gi
```

Đổi `type`, `image`, `id`, `auth_secret_name` cho MariaDB hoặc MongoDB.
Các Type params bắt buộc vẫn phải được khai báo; chúng ưu tiên hơn fallback
trong Definition. Chọn cùng image/Secret đã chuẩn bị trong input. Outputs của
cả ba là `host`, `port`, `database`, `username`, `claimName`, `namespace`.
MongoDB khởi tạo app user với role readWrite; kết nối bằng authSource=database
ứng dụng, không dùng tài khoản root cho workload.

## Dữ liệu và lifecycle

Mỗi instance sở hữu StatefulSet, Service nội bộ và PVC riêng. MongoDB có thêm
ConfigMap init code không chứa credential. Init chỉ chạy trên volume trống;
đổi image, Secret/password, user hay database không phải quy trình DB upgrade/
rotation. Không tự xóa PVC/state hoặc dùng MySQL/MariaDB thay thế trực tiếp trên
cùng volume. Thực hiện backup/migration/rotation bằng quy trình được review.

App nghiệm thu mặc định vẫn là PostgreSQL/Redis. Kiểm tra Terraform bằng mock
và loader không chứng minh kết nối, init, retention hay failover trên staging;
AI nội bộ phải nghiệm thu thật theo quyền đã được cấp.

Nguồn contract entrypoint: [MySQL](https://github.com/docker-library/mysql/blob/master/docker-entrypoint.sh),
[MariaDB](https://github.com/MariaDB/mariadb-docker/blob/master/docker-entrypoint.sh),
[MongoDB](https://github.com/docker-library/mongo/blob/master/docker-entrypoint.sh).

## Kiểm tra module offline

Các module có `tests/contract.tftest.hcl` dùng `mock_provider` (cần Terraform >=1.7
để chạy tests; Terraform >=1.5 vẫn là contract của module). Trong checkout kiểm
tra riêng ngoài Git, cài provider Kubernetes từ mirror tin cậy của công ty bằng
`terraform init -backend=false`, rồi `terraform validate` và `terraform test`.
Không dùng `terraform apply`; test mock không kết nối cluster và không xác minh
server database thực tế. Giữ `.terraform/`, lockfile phát sinh và state ngoài
source publish nếu không được review/pin trong quy trình release.
