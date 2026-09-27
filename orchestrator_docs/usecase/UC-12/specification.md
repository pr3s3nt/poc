---
id: UC-12-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-09-27
related: UC-01, UC-05, UC-06, UC-07, UC-16
---

# UC-12 — Quản lý Variables & Secrets

## Mục tiêu

Developer cấu hình variable và secret để các workload trong cùng Application,
cùng Environment có thể tham chiếu và tái sử dụng.

## Primary actor

- Developer

## Tiền điều kiện

- **PRE-01:** Developer đã đăng nhập và có quyền truy cập Application.
- **PRE-02:** Application và hai Environment `staging`, `production` đã tồn tại.

## Trigger

**TRG-01:** Developer mở Application Settings → Variables & Secrets.

## Main success scenario

1. **MS-01:** Hệ thống hiển thị hai tab `Staging` và `Production`. Trong tab
   Environment đang chọn, hệ thống hiển thị cả hai mục `Environment variables`
   và `Secrets` trên cùng một trang.
2. **MS-02:** Developer thêm key mới hoặc chọn key hiện có để cập nhật giá trị.
3. **MS-03:** Hệ thống kiểm tra tên key trong mục và Environment đã chọn, rồi
   lưu thay đổi cấu hình mong muốn.
4. **MS-04:** Hệ thống hiển thị Environment và các workload tham chiếu key bị
   ảnh hưởng; đánh dấu thay đổi chờ Preview → Deploy.

## Luồng biến thể

- **VAR-01 — Đổi tên key:** Developer nhập tên mới. Nếu key đang được workload
  tham chiếu, hệ thống cảnh báo và liệt kê workload bị ảnh hưởng nhưng vẫn cho
  xác nhận. Hệ thống không tự sửa tham chiếu của workload; Developer tự chỉnh
  từng workload trong UC-16.
- **VAR-02 — Xóa key:** Developer yêu cầu xóa. Nếu key đang được workload tham
  chiếu, hệ thống cảnh báo và liệt kê workload bị ảnh hưởng nhưng vẫn cho xác
  nhận. Hệ thống không tự xóa/sửa binding trong workload.
- **VAR-03 — Thay secret:** Developer nhập giá trị mới cho secret. Hệ thống
  không hiển thị lại giá trị cũ hoặc mới sau khi lưu.

## Hậu điều kiện

- **POST-01:** Cấu hình mong muốn của đúng Application, loại và Environment đã
  chọn được cập nhật; workload đang chạy và current Deployment Set không đổi.
- **POST-02:** Thay đổi chờ Preview → Deploy. Nếu đổi tên/xóa làm tham chiếu
  workload không hợp lệ, Preview báo lỗi và không cho Deploy cho tới khi Developer
  chỉnh workload.
- **POST-03:** Giá trị secret không xuất hiện trong danh sách, response đọc,
  thông báo, Preview hoặc cấu hình workload.

## Quy tắc nghiệp vụ

- **BR-01:** Mỗi key thuộc một Application, một Environment và một loại
  `Variable` hoặc `Secret`. Key có thể chỉ tồn tại ở Staging; cùng tên ở
  Production là cấu hình độc lập. Thao tác ở một Environment không sửa key ở
  Environment kia.
- **BR-02:** Tên key là duy nhất trong cùng Application và Environment, kể cả
  giữa Variable và Secret. Thêm hoặc đổi tên trùng bị từ chối.
- **BR-03:** Variable có thể xem/chỉnh giá trị. Secret chỉ cho nhập giá trị mới;
  hệ thống không hiển thị lại giá trị đã lưu, kể cả ở Preview hoặc lỗi.
- **BR-04:** Khi đổi giá trị, hệ thống chỉ tạo thay đổi cấu hình mong muốn và
  thông báo các workload đang tham chiếu. Runtime không đổi cho tới khi thay đổi
  được Preview và Deploy ở đúng Environment.
- **BR-05:** Đổi tên/xóa key đang được tham chiếu phải cảnh báo nhưng không bị
  chặn. Hệ thống không tự cập nhật tham chiếu trong workload. Preview phải chỉ
  rõ tham chiếu thiếu và chặn Deploy cho tới khi workload được chỉnh hợp lệ.
- **BR-06:** UC-16 chỉ được chọn key tồn tại trong đúng Application,
  Environment và loại. Giá trị UC-12 không được sao chép thành literal vào
  cấu hình workload.
- **BR-07:** Resource outputs và Service của workload khác không phải key do
  UC-12 quản lý; UC-16 cho chọn trực tiếp các nguồn đó.
- **BR-08:** Mỗi Application có một provider cấu hình cho Variables & Secrets.
  Cùng một giao diện quản lý và tham chiếu được dùng bất kể provider; provider
  đầu tiên cho môi trường kind là Vault. Staging và Production có vùng dữ liệu
  tách biệt trong provider đã chọn.
- **BR-09:** Lưu thay đổi tạo revision cấu hình mong muốn nhưng không thay
  revision đã áp dụng. Workload đang chạy và Pod được tạo trước Deploy phải tiếp
  tục dùng revision đã áp dụng. Preview xác định workload bị ảnh hưởng; chỉ
  Deploy hợp lệ mới chuyển revision áp dụng và khởi động lại các workload đó.
- **BR-10:** Với provider Vault trên Kubernetes, secret được Vault Agent Injector
  đưa vào Pod dưới dạng file. Script khởi động workload nạp file thành biến môi
  trường trước khi chạy ứng dụng. Giá trị secret không được ghi vào Score,
  Deployment Set, Kubernetes Secret hoặc Pod annotations. Tiến trình đang chạy
  không tự nhận giá trị mới khi file thay đổi.
- **BR-11:** UC-16 dùng resource `env` loại `environment` trong Score; một key
  được tham chiếu bằng `${resources.env.KEY}`. Hệ thống xác định key là Variable
  hay Secret từ cấu hình UC-12 của đúng Application/Environment, không từ tên
  placeholder. Secret phải chiếm toàn bộ binding, không ghép với literal.
- **BR-12:** Preview gắn với đúng revision cấu hình và draft workload đã đọc.
  Nếu một trong hai thay đổi sau Preview, Deploy từ Preview cũ bị từ chối và
  Developer phải Preview lại.
- **BR-13:** Nếu Deploy nhiều workload bị lỗi giữa chừng, hệ thống báo trạng
  thái áp dụng một phần và tên workload thành công/thất bại. Không đánh dấu
  Environment đã áp dụng toàn bộ revision; workload thành công giữ revision đã
  áp dụng của chính nó để lần Deploy sau tiếp tục an toàn.
- **BR-14:** Injector chỉ cấp file chứa các key mà workload tham chiếu. Nội
  dung file được escape an toàn cho shell; script của workload nạp file rồi
  chạy ứng dụng. Mỗi workload chỉ có quyền đọc file/revision được cấp cho nó.

## Ngoài phạm vi

- **OOS-01:** Form cấu hình workload và sửa tham chiếu; thuộc UC-16.
- **OOS-02:** Preview và Deploy; thuộc UC-05, UC-06 và UC-07.
- **OOS-03:** Di chuyển giữa providers và tự động rollback sau Deploy thất bại.

## Trạng thái implementation hiện tại

Đã có API/UI quản lý Application variables/secrets theo Environment, desired
revision, Vault KV v2 adapter, resolution tham chiếu khi Preview/Deploy,
Vault Agent Injector và applied revision theo workload. Fake mode dùng provider
bộ nhớ; Vault-backed kind path đã được kiểm chứng. HA, backup và secret
lifecycle production chưa thuộc MVP.
