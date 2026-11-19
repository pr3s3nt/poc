---
id: OPS-APPLICATION-DATABASE-PURGE
artifact: operations-runbook
status: current
last_reviewed: 2026-11-19
---

# Xóa một Application thử nghiệm khỏi PostgreSQL

[UC-18 DBP](../usecase/UC-18/specification.md#database-only-application-purge--issue-94)
quy định lệnh quản trị riêng cho issue #94. Lệnh dùng cho một app thử nghiệm, kể
cả app đang DELETING và còn bản ghi recovery. Sau khi purge, Orchestrator không
còn lịch sử, trạng thái Terraform hay khả năng tiếp tục cleanup cho app đó.
Các tài nguyên Fleet, Git, Kubernetes, Vault và storage có vòng đời riêng.
Giữ bản sao lưu PostgreSQL riêng tư và lưu lại báo cáo định danh trước khi
xóa nếu bạn cần đối chiếu hoặc phục hồi về sau.

## Xem trước

Dùng binary của release này trên máy quản trị có đường truy cập PostgreSQL
của installation thử nghiệm. Dùng file URL của login **owner** hiện có, mode
0600 hoặc 0400, bên ngoài Git. Chạy lệnh preview bằng Organization key và app
key chính xác, ví dụ key của app trong đường dẫn Console.

```bash
./orchestrator -purge-application APP_KEY \
  -purge-organization ORG_KEY \
  -database-owner-url-file /private/test-db-owner-url
```

Kết quả JSON ghi installation ID, app ID, tên hiển thị, môi trường,
namespace và các tên Fleet, Git, Terraform đã nhận diện. Đối chiếu installation
và app dự kiến. Mảng rỗng chỉ có nghĩa PostgreSQL thiếu định danh tương ứng.
File URL cùng dữ liệu state, ciphertext và nội dung receipt thô nằm ngoài
báo cáo. Giữ báo cáo riêng tư để hỗ trợ kiểm tra hạ tầng sau purge.

## Xóa khỏi DB

Dừng các writer của installation thử nghiệm, gồm backend và tác vụ đang ghi
PostgreSQL. Xác minh executor cũ của app đã dừng. Một bản ghi thao tác bị gián
đoạn vẫn có thể được purge sau khi đã xác minh tiến trình dừng. Lệnh từ chối
khi còn kết nối ghi. File URL owner chỉ thuộc phiên quản trị này.

```bash
./orchestrator -purge-application APP_KEY \
  -purge-organization ORG_KEY \
  -database-owner-url-file /private/test-db-owner-url \
  -purge-confirm-name 'Tên hiển thị chính xác' \
  -purge-writers-stopped
```

Kết quả `"purged":true` xác nhận PostgreSQL đã commit. Lỗi trong quá trình
xóa làm toàn bộ giao dịch rollback. Một tham chiếu từ app khác có thể chặn
purge để bảo toàn dữ liệu, và kết nối bị gián đoạn lúc commit cần được xác
minh lại bằng preview. Lệnh giữ các ràng buộc khóa ngoại và kiểm tra audit
trigger khôi phục đúng trạng thái trước commit. Thao tác có thể giữ khóa bảng
trong lúc chạy.

Khởi động lại backend sau khi xác minh kết quả. Mở Catalog để xóa Definition
rồi Type nếu đã hết tham chiếu. Đối chiếu các đối tượng bên ngoài theo báo cáo
trước khi tự dọn hoặc trước khi tái sử dụng các tên và endpoint liên quan.
