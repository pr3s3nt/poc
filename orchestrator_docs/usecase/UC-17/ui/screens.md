---
id: UC-17-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-23
---

# UC-17 screens

Route `/ui/platform/effectiveness?application=&workload=&days=` (`days` 7, 30
hoặc 90; mặc định 30, bỏ khỏi URL). Query giữ phạm vi/kỳ khi reload hoặc chia
sẻ link; Console tính `from/to` theo ngày hiện tại ở giờ Việt Nam. Sidebar: mục “Hiệu quả platform”
sau “Kho bí mật”, chỉ PE/Admin; Developer không thấy mục và deep link không
render trang. Mục “Lịch sử triển khai” vẫn disabled.

## Trang “Hiệu quả platform”

Một trang, không tab. Từ trên xuống:

1. **Header:** `h1` “Hiệu quả platform”. Actions: “Mốc so sánh”, “Xuất CSV”
   (primary). Nhãn “Dữ liệu kiểm thử” cạnh tiêu đề khi
   `dataOrigin=TEST_INSTALLATION`.
2. **Bộ lọc:** đường dẫn phạm vi “Toàn tổ chức › <Ứng dụng> › <Workload>”
   (mỗi cấp cha là nút quay lên); select “Ứng dụng” (“Tất cả ứng dụng” +
   Application của Organization); select “Workload” khi đã chọn Application;
   select “Thời gian” 7/30/90 ngày; dòng phụ hiển thị khoảng ngày và “giờ Việt Nam”.
3. **Giao phần mềm (DORA):** 5 thẻ cùng hàng. “Tần suất triển khai” có số lượt
   production và lượt/tuần. Bốn thẻ còn lại hiển thị “Chưa thu thập”, màu muted.
   Nút “Cách đọc chỉ số” mở trợ giúp DORA.
4. **Mức sử dụng:** 5 thẻ: Ứng dụng hoạt động (x/y), Người dùng hoạt động (phụ:
   lượt tự động nếu có), Công việc hoàn thành (phụ: đang mở), Lượt chạy (phụ: lần
   thử/công việc, thất bại), Thời gian thực hiện (trung vị; phụ: p90, độ phủ).
5. **Hai cột:** trái “Triển khai production” (biểu đồ cột theo ngày/tuần, có số
   trên cột và aria-label); phải “Thời gian rút ngắn” (baseline − trung vị, ghi
   “Thời gian quy trình, không phải giờ công”, phiên bản/nguồn mốc; chưa có mốc
   thì “Chưa có mốc so sánh” và nút “Thêm mốc so sánh”).
6. **Phân bố:** bảng “Theo ứng dụng” (cấp tổ chức) hoặc “Theo workload” (cấp
   ứng dụng). Cột: tên, Triển khai production, Tần suất, Công việc hoàn thành,
   Lượt chạy (thất bại), Thời gian thực hiện (trung vị), Người dùng. Bấm dòng để
   đi sâu. Ở cấp tổ chức, trên bảng có dòng phân bố nhóm tần suất (“Hằng ngày
   n · Hằng tuần n · Hằng tháng n · Thưa hơn n · Không có n”). Không có ở cấp
   workload.
7. **Chưa thu thập:** một panel gọn liệt kê thời gian yêu cầu → hoàn thành,
   tự phục vụ, công sức Ops, onboarding, giờ công tiết kiệm; mỗi mục mở chi tiết.

Thẻ chỉ số là `button`, accessible name “<Tên>: <giá trị>. Xem chi tiết”.
Mỗi chỉ số hiển thị một lần. Trạng thái nguồn không khả dụng chỉ báo trong chi
tiết chỉ số liên quan và CSV.

## Dialog chi tiết chỉ số

Tiêu đề là tên chỉ số. Nội dung: công thức một câu; dòng phạm vi · kỳ · đơn
vị; tử số/mẫu số; độ phủ và cảnh báo thiếu dữ liệu nếu có. `NOT_COLLECTED`:
“Chưa thu thập” và nguồn cần có, không có bảng. Chỉ số có dữ liệu: bảng bản
ghi (Hoàn thành/Bắt đầu, Ứng dụng · môi trường, Workload/tài nguyên, Loại,
Lần thử, Thời gian, Người thực hiện, liên kết “Mở”). “Mở” đi tới chi tiết
Deployment (UC-09) hoặc trang tài nguyên gốc của cleanup. Định nghĩa dài và
link tham khảo DORA/Humanitec nằm trong `details` “Định nghĩa”. Đóng bằng nút ×,
Escape hoặc click nền.

## Dialog “Mốc so sánh”

Phạm vi là cấp Toàn tổ chức hoặc Application đang xem; ở cấp Workload dialog
nêu “Mốc so sánh áp dụng cho tổ chức hoặc ứng dụng” và khóa form. Hiển thị
phiên bản đang áp dụng (giá trị, nguồn, ngày áp dụng, người tạo). Form: Thời
gian thực hiện trung vị (phút), Ngày áp dụng, Nguồn đo, Số mẫu (tuỳ chọn), Kỳ
đo từ/đến (tuỳ chọn). Actions “Lưu phiên bản mới”, “Ngừng áp dụng” (khi đang có
mốc). `details` “Lịch sử phiên bản” liệt kê mọi phiên bản.

## CSV

Tên `hieu-qua-platform-<phạm vi>-<from>-<to>.csv`, UTF-8 BOM. Phần đầu: phạm
vi, kỳ, timezone, nguồn dữ liệu, thời điểm tạo, nguồn khả dụng. Phần chỉ số: ID,
tên, trạng thái, giá trị, đơn vị, tử số, mẫu số, độ phủ, nguồn cần có. Phần bản
ghi: các công việc logic của records không lọc metric.
