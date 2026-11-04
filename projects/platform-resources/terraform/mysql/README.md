# MySQL trong Kubernetes

Nguồn được đưa từ module MySQL hiện có của Orchestrator vào repo tài nguyên.
Image bắt buộc pin digest; có thể dùng registry công ty kèm port. Cần image
tương thích Docker Official Image MySQL 8.x, không thay bằng MariaDB/Bitnami
nếu chưa review entrypoint và data format. Xem [entrypoint chính thức](https://github.com/docker-library/mysql/blob/master/docker-entrypoint.sh).

Một StatefulSet, Service nội bộ và PVC riêng do state sở hữu. Namespace/cluster
đến từ provider binding; không cài operator hoặc thay StorageClass. Module dùng
Secret có sẵn qua secretKeyRef (root_password_key/user_password_key), không đọc
dữ liệu Secret vào Terraform. Không truyền password thật qua Type params.
Không dùng MYSQL_USER=root. Việc đổi biến mật khẩu/username/database không tự
rotate tài khoản của data directory đã khởi tạo; cần quy trình DB riêng.

Module chỉ cung cấp outputs host, port, database, username, claimName, namespace.
Workload muốn password cần nhận từ hệ thống secret delivery đã duyệt. Bootstrap
đăng ký Definition không có nghĩa đã kiểm chứng kết nối MySQL trên staging.
