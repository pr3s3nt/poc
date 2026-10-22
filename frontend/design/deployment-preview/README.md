# Deployment Preview — bản mẫu review

Bản mẫu độc lập cho review giao diện, sử dụng dữ liệu giả Shopfront. Đây là đề
xuất trải nghiệm, chưa thay đổi specification UC-05 hoặc triển khai sản phẩm.
Không gọi API, resolve secret, chạy Terraform hoặc thay đổi cluster/cloud.
Chỉ review desktop (viewport từ 1280px); không thiết kế mobile theo yêu cầu người dùng.

## Tham chiếu UI hiện có

Bản mẫu nạp trực tiếp [stylesheet Console](../../src/styles/index.css), gồm
tokens và app.css. Shell/nhãn điều hướng theo [AppShell](../../src/app/AppShell.tsx);
tiêu đề ứng dụng, tab môi trường và panel theo
[ApplicationHomePage](../../src/features/applications/ApplicationHomePage.tsx).
Nút dùng class của [Button](../../src/shared/ui/Button.tsx). CSS riêng chỉ định
nghĩa nội dung Preview mới; không thay stylesheet sản phẩm.
Điều hướng và môi trường ngoài staging chỉ là khung minh họa, không chuyển route.

Ảnh review: [Tổng quan](desktop.png) · [Diff code mở rộng](code-diff.png) ·
[Cloud đang cấp phát](cloud-provisioning.png) · [Cloud gặp lỗi](cloud-failure.png).

## Mở bản mẫu

Mở trực tiếp `index.html` trong checkout bằng trình duyệt. Giữ cấu trúc thư mục
để tải stylesheet thật tại `frontend/src/styles/`; không chỉ copy riêng thư mục
bản mẫu. Không cần cài dependency. Hoặc từ repository root:

```bash
node frontend/design/deployment-preview/serve.mjs
```

Mở <http://127.0.0.1:18091/>. Server dùng danh sách đường dẫn cho phép: HTML,
CSS/JS bản mẫu và ba file stylesheet Console, trên loopback. Dùng `PORT` để
đổi cổng. Ctrl+C để dừng.

## Luồng review

1. Chọn **Cluster có sẵn · cập nhật**, **Cluster có sẵn · triển khai mới**,
   hoặc **Cloud · tạo hạ tầng mới**.
2. Bấm node trên bản đồ để xem vị trí, điều kiện sẵn sàng và trạng thái cấp phát.
3. Xem **Tổng quan**, **Thay đổi** trước/sau, **Code** theo từng node.
4. Bấm tài nguyên phụ thuộc trong bảng chi tiết để chuyển node.
5. Chọn kết quả mô phỏng thành công hoặc cache thất bại, rồi bấm **Mô phỏng
   triển khai**. Dùng **Tạm dừng / Tiếp tục** để đọc trạng thái từng node;
   **Đặt lại** dừng/reset. Đổi tình huống cũng hủy mô phỏng cũ.

Code mặc định **Xem diff**: dòng bỏ nền đỏ/dấu `−`, dòng thêm nền xanh/dấu `+`,
số dòng trước/sau và tổng số dòng thay đổi. Diff được tính từ hai fixture code,
không tô màu theo trạng thái node. Backend có image/replica/Redis thay đổi;
Service YAML và PostgreSQL giữ nguyên; Redis mới có toàn bộ dòng thêm.
**Sau thay đổi** hiển thị code sạch. **Sao chép bản sau** luôn sao chép bản sau,
không kèm dòng bị xóa hay số dòng. **Mở rộng / Thu gọn** đổi độ rộng vùng code.

Mũi tên biểu diễn điều kiện sẵn sàng → thành phần phụ thuộc, không phải hướng
lưu lượng request. Các cạnh phụ thuộc trực tiếp đã có đường bắc cầu có thể
được lược khỏi sơ đồ cho dễ đọc; bảng điều kiện ở mỗi node liệt kê đầy đủ.
Đây là cách trình bày UI, không đổi chiều consumer → provider của planner.

## Cấp phát hạ tầng minh họa

- Internal có sáu node: cluster, namespace, PostgreSQL, Redis, Backend, Frontend.
  Cập nhật giữ cluster/namespace/PostgreSQL; triển khai mới giữ cluster và tạo
  năm thành phần còn lại. Không tạo VPC/EKS ở tình huống này.
- Cloud có bảy node: VPC, EKS, namespace, Aurora, ElastiCache, Backend,
  Frontend. Aurora/ElastiCache nằm trong VPC nhưng ngoài EKS. VPC sẵn sàng
  mở ba nhánh EKS/Aurora/ElastiCache song song. Namespace chờ EKS; Backend
  chờ namespace và cả database/cache; Frontend chờ namespace và Backend.
- Mỗi node có trạng thái chưa chạy, chờ phụ thuộc, đang cấp phát, kiểm tra
  sẵn sàng, sẵn sàng, giữ nguyên, thất bại hoặc bị chặn. Tạo xong chưa đồng
  nghĩa sẵn sàng. Bảng chi tiết giải thích bước, thời gian mô phỏng và phụ thuộc.
- Cache thất bại ở bước readiness chặn Backend/Frontend. Nhánh độc lập vẫn
  hoàn tất. Không báo thành công chung hoặc giả định tài nguyên đã cleanup.
- Scheduler dùng thời gian fixture rút gọn trong browser; không phải thời gian
  cấp phát thực của AWS. Tạm dừng đóng băng thời gian mô phỏng; hoàn tất giữ
  thời lượng cố định. Tổng tiến độ là số node sẵn sàng/giữ nguyên trên tổng node,
  không phải ước lượng phần trăm công việc thực tế.
- Intent tạo/cập nhật/giữ nguyên nằm trong cấu hình chi tiết và tổng hợp; trạng
  thái runtime hiển thị độc lập trên node. Địa chỉ/credential cloud không được
  lấy từ provider hoặc điền giả thành output thực.

Code được viết riêng để minh họa UI, không lấy từ source module đã pin, không
phải artifact đã render. Terraform được lược gọn; manifest có placeholder cho
output chưa có. Không dùng code này để apply. Diff chỉ so sánh fixture mong
muốn, không khẳng định kết quả Terraform plan hoặc trạng thái cluster thật.
Trạng thái Sẵn sàng chỉ thuộc mô phỏng. Refresh trang đưa về trạng thái ban đầu.

Khi triển khai sản phẩm, cần cập nhật canonical specification/design trước;
dùng plan snapshot, source/artifact thực và bảo vệ secrets theo contract.
Không đưa fixture hoặc bộ đếm mô phỏng vào luồng deploy thật.

## Kiểm tra bản mẫu

Đã kiểm tra bằng Chrome/Playwright: chọn node, chuyển tài nguyên phụ thuộc,
diff trước/sau, Terraform/inputs, Deployment/Service YAML, hai tình huống,
mô phỏng hoàn tất, hủy/reset và điều hướng tab bằng bàn phím. Mở trực tiếp
qua `file://` hoạt động; không có lỗi JavaScript hoặc request ra ngoài.
Lượt sửa UI/diff đã kiểm tra desktop 1512px/1280px, màu và chiều rộng sidebar,
màu nút từ stylesheet thật; đúng hai dòng bỏ/năm dòng thêm của Backend,
file mới/giữ nguyên, màu diff, clipboard bản sau và mở rộng/thu gọn code.
Đã kiểm tra cú pháp JavaScript, documentation checker và whitespace diff.
Không chạy suite sản phẩm hoặc deploy thật vì thay đổi chỉ là bản mẫu độc lập.

Lượt bổ sung cloud đã kiểm tra bằng Chrome/Playwright với đồng hồ điều khiển:
reuse cluster, namespace gate, ba nhánh song song, chờ readiness, lý do chờ,
pause/resume/reset/đổi tình huống, hoàn tất và lỗi cache lan tới consumers.
Kiểm tra Terraform đúng loại resource minh họa cho từng node cloud, giữ diff
và clipboard, desktop 1512px/1280px và mở file trực tiếp. Không chạy AWS hoặc
Kubernetes thật vì phạm vi chỉ là bản mẫu để review.
