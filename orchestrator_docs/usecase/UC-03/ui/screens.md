---
id: UC-03-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-10
related: UC-03
---

# UC-03 UI screens

Trang Cấu hình tài nguyên liệt kê cấu hình thuộc Organization hiện tại, gồm
ID, loại tài nguyên, phạm vi triển khai, driver và số điều kiện áp dụng. Không
hiển thị cấu hình `workload`; các cấu hình này nằm ở trang Mẫu dựng ứng dụng. Cấu hình
`existing-cluster` nếu có chỉ đọc và mang nhãn “Hệ thống”. Không có sửa/xóa;
đăng ký chỉ dành Kỹ sư nền tảng/Quản trị viên.

## Form đăng ký (T02)

Phần cơ bản gồm ID, Loại tài nguyên, cách tạo và Phạm vi triển khai. Form chỉ
cho Terraform/Kubernetes, không tạo `existing-cluster` hoặc `score-k8s`.
`vpc` tự chọn Terraform/module `vpc`; `k8s-cluster` tự chọn Terraform/module
`eks`; `k8s-namespace` tự chọn Kubernetes; `postgres` cho chọn hai cách tạo,
Terraform dùng module `aurora`. Không quảng bá custom type chưa có executor.

Terraform tự điền `aws-eks` và khóa Phạm vi triển khai; Kubernetes cho chọn
“Cluster nội bộ” (`internal-k8s`) hoặc “Dùng chung” (`""`). Dùng chung được xét
ở mọi profile nhưng vẫn phải thỏa điều kiện áp dụng; cảnh báo có thể trùng mức
ưu tiên với cấu hình khác, không tuyên bố đã phát hiện mọi overlap.

Terraform hiện vẫn yêu cầu Kết nối AWS tường minh (T16 mới đổi VPC/EKS).
Dropdown dùng metadata của GET `/connections`, chỉ nhận READY đúng kind trong
Organization của phiên và gửi key kỹ thuật. Kubernetes mặc định không override,
dùng Kết nối của Môi trường; lựa chọn Kết nối riêng nằm trong phần nâng cao.
Không tự chọn một Connection hoặc tạo Connection trong form này. Trợ giúp
Kubernetes nêu rõ Connection riêng phải trùng Connection của Môi trường khi
matching; lựa chọn này không cho phép retarget sang cluster khác.

Điều kiện áp dụng dùng editor chung T02B bên dưới; matching preview chính xác
thuộc T19. Phần nâng cao chứa
Tham số cấu hình và Quy tắc tạo tài nguyên liên quan dưới dạng JSON object;
giữ placeholder/JSON keys và giải thích validation/không nhận credential.
Đổi driver/type cập nhật controls hợp lệ nhưng giữ dữ liệu đã nhập để người
dùng có thể quay lại; collapse không xóa dữ liệu. Chỉ gửi inputs hợp lệ của
cách tạo đang chọn. Không tự xem trước hoặc triển khai.

Nhãn, trợ giúp, aria-label và trạng thái trang dùng tiếng Việt theo glossary;
technical IDs, module, driver, payload và tên người dùng giữ nguyên. Lỗi server
chưa có code mapping đầy đủ dùng thông báo Việt an toàn, không regex dịch raw
message (VI-03 thuộc T20).

## Editor Điều kiện áp dụng dùng chung (T02B)

Bốn chế độ: “Mọi nơi” gửi `[{}]`; “Theo loại môi trường” chỉ gửi
`env_type`; “Theo ứng dụng (+ môi trường)” dùng dropdown ứng dụng và môi
trường thuộc ứng dụng, gửi `app_id`/`env_id` thực; “Tùy chỉnh nâng cao” giữ
năm field chuẩn và nhiều dòng. Chỉ hiện controls của chế độ đang chọn.
Criteria không biểu diễn lossless trong chế độ đơn giản phải được giữ ở nâng
cao; chuyển chế độ cần lựa chọn tường minh trước khi thay dữ liệu đó.

Sao chép điều kiện từ cấu hình khác tạo bản độc lập, không liên kết sống.
Tóm tắt phạm vi phân biệt Phạm vi triển khai (`executionProfile`) với Loại
môi trường (`env_type`). Xem trước phạm vi ở frontend chỉ là trợ giúp: thiếu
`class`/`res_id` phải báo cần thêm context; cảnh báo nguy cơ chồng lấn với
cấu hình cùng type không kết luận ambiguous hoặc Definition thắng. Không gọi
endpoint matching mới, không lưu hoặc provision khi xem trước. Editor có thể
được dùng lại cho Mẫu dựng ứng dụng ở T03.

## Trang Mẫu dựng ứng dụng (T03)

Trang `/ui/platform/rendering-templates` nằm ngang hàng các trang catalog trong
sidebar, chỉ Kỹ sư nền tảng/Quản trị viên truy cập. Danh sách chỉ lấy Definitions
có `resourceType: workload`, hiển thị ID kỹ thuật, bundle ID và điều kiện áp dụng.
Form gồm ID mẫu, ID bundle dựng ứng dụng và editor Điều kiện áp dụng T02B.
ID tuân thủ chữ thường/số/gạch ngang, không có friendly name.

Type `workload`, driver `score-k8s` và profile `internal-k8s` là giá trị ngầm;
không có controls Connection, provision, module hoặc JSON variables tùy ý.
Bundle ID được nhập tường minh theo API hiện tại; trợ giúp nói rõ đây chưa phải
bộ chọn danh sách bundle đã cài (T18). Không suy availability từ ID hard-code.

Mẫu là tùy chọn, chỉ dùng cho cluster nội bộ. Hệ thống tự chọn theo criteria
hiện hành; không có mẫu khớp thì dùng renderer mặc định. Mẫu được chọn render
lỗi phải báo lỗi, không chuyển âm thầm sang renderer mặc định. Không có binding
mới cho Environment hoặc upload template. Text/aria-label/trạng thái dùng Việt;
technical IDs/payload giữ nguyên. Lưu mẫu không tự Preview/Deploy.
