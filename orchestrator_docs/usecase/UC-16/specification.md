---
id: UC-16-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-09-24
related: UC-01, UC-05, UC-06, UC-07, UC-12
---

# UC-16 — Quản lý cấu hình workload

## Mục tiêu

Developer thêm, sửa hoặc xóa cấu hình workload trong một Environment của
Application. UC-16 lưu cấu hình mong muốn, không tự preview hoặc deploy.

## Primary actor

- Developer

## Tiền điều kiện

- **PRE-01:** Developer đã đăng nhập và có quyền truy cập Application.
- **PRE-02:** Application và Environment `staging` hoặc `production` đã tồn tại.

## Trigger

**TRG-01:** Developer chọn Add, Edit hoặc Delete workload trên Application home
của Environment đang chọn.

## Main success scenario

1. **MS-01:** Hệ thống hiển thị workload và thay đổi cấu hình đang chờ trong
   Environment đã chọn.
2. **MS-02:** Developer chọn Add hoặc Edit. Hệ thống mở form cấu hình mặc định;
   Developer cũng có thể nạp một file Score.
3. **MS-03:** Developer khai báo container, image, Service/cổng, tài nguyên và
   resource dependencies cần dùng.
4. **MS-04:** Với mỗi biến môi trường của container, Developer chọn một nguồn:
   variable của Application đã cấu hình cho Environment này trong UC-12; output
   không bí mật của resource dependency; hoặc Service/cổng của workload khác
   trong cùng Environment.
5. **MS-05:** Với mỗi secret của container, Developer chọn secret của Application
   đã cấu hình cho Environment này trong UC-12 hoặc output bí mật của resource
   dependency.
6. **MS-06:** Hệ thống kiểm tra cấu hình và tham chiếu. Nếu nạp Score, hệ thống
   hiển thị nội dung đã đọc để Developer kiểm tra trước khi lưu.
7. **MS-07:** Developer lưu. Hệ thống lưu cấu hình mong muốn cho đúng Environment,
   hiển thị thay đổi đang chờ và cho phép yêu cầu Preview changes theo UC-05.

## Luồng biến thể

- **VAR-01 — Nạp Score:** tại MS-02, Developer chọn file Score mô tả một
  workload. Cấu hình nhập từ file chịu cùng quy tắc với form; nạp file không tự
  lưu hoặc deploy.
- **VAR-02 — Xóa workload:** tại TRG-01, Developer chọn Delete và xác nhận.
  Hệ thống đánh dấu workload chờ xóa trong Environment; Developer có thể hoàn
  tác trước khi deploy. Workload đang chạy chưa bị xóa.

## Hậu điều kiện

- **POST-01:** Cấu hình mong muốn của Environment được lưu hoặc workload được
  đánh dấu chờ xóa; thay đổi sẵn sàng để preview.
- **POST-02:** Current Deployment Set, resource và workload đang chạy không đổi.
- **POST-03:** Giá trị secret không xuất hiện trong cấu hình workload hay màn
  hình đọc lại.

## Quy tắc nghiệp vụ

- **BR-01:** Cấu hình workload thuộc đúng một Environment; thay đổi ở `staging`
  không sửa cấu hình `production`, và ngược lại. Tên workload là duy nhất trong
  Environment.
- **BR-02:** Biến môi trường và secret của container chỉ được lưu dưới dạng
  tham chiếu tới các nguồn ở MS-04/MS-05. Form không cho nhập giá trị trực tiếp;
  file Score nạp qua UC-16 chứa literal ở các binding này bị từ chối với lỗi chỉ
  rõ trường vi phạm.
- **BR-03:** Tham chiếu UC-12 phải thuộc đúng Application và được cấu hình cho
  Environment đang chọn. Workload không sao chép giá trị variable/secret đó.
- **BR-04:** Resource output chỉ được chọn từ dependency khai báo trong workload
  và phải có trong output contract. Output được phân loại secret chỉ được chọn
  trong mục Secrets; giá trị secret không hiển thị ở form, lỗi hoặc preview.
- **BR-05:** Tham chiếu workload-to-workload luôn đi qua Service, không trỏ
  trực tiếp tới Pod. Workload đích phải thuộc cùng Environment, có Service/cổng
  đã khai báo và không đang chờ xóa. Hệ thống tạo địa chỉ nội bộ từ Service/cổng
  đã chọn; Developer không phải nhập URL trực tiếp.
- **BR-06:** BR-02 chỉ giới hạn giá trị biến môi trường và secret của container,
  không cấm nhập các trường khác như image, port, CPU/memory hoặc tham số resource.
  Quy tắc này áp dụng cho luồng UC-16, không ngầm sửa Score API/CI hiện có.
- **BR-07:** Lưu/sửa/xóa cấu hình không tự provision resource, apply Kubernetes
  manifest hay thay current Deployment Set. UC-05 sở hữu preview; UC-06/UC-07
  sở hữu việc áp dụng thay đổi.
- **BR-08:** Form và file Score chịu cùng validation của UC-16; nạp file không
  được bỏ qua BR-02–BR-05.

## Ngoài phạm vi

- **OOS-01:** Quản lý variable/secret của Application; thuộc UC-12.
- **OOS-02:** Preview, approval hoặc deploy; thuộc UC-05, UC-06 và UC-07.
- **OOS-03:** Cấp public URL cho frontend chạy trong trình duyệt; Service
  reference ở BR-05 là địa chỉ nội bộ cho giao tiếp giữa các workload.
- **OOS-04:** Cú pháp lưu tham chiếu UC-12/Service trong Score, resolution tại
  deploy và persistence draft; cần thiết kế contract trước implementation.

## Trạng thái implementation hiện tại

UC-16 chưa được triển khai. Score/planner hiện hỗ trợ resource output binding
và giá trị literal; chưa có Application variable/secret reference hoặc Service
reference giữa workload. UI affordance ở UC-01 chưa có luồng UC-16 hoạt động.
