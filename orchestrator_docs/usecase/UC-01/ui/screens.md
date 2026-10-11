---
id: UC-01-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-11
---

# UC-01 UI screens

Create Application contains Name/Subdomain only. Home and Settings have staging/
production tabs and separate target summaries. Settings includes independently
editable Deployment Connection and Secret Store Connection selectors, safe names
and explicit Save; no permanent lock. Ordinary variables work before store set.

For a deployed Environment, changing execution target opens an explicit transition
form: destination, deploy-new or migrate PostgreSQL, logical resource mapping,
impact and downtime acknowledgement. Preview identifies added/updated/removed
workloads, unchanged workloads to redeploy, and pending configuration changes.
Preview then execute with exact token.
Show persisted stages/source-destination status, recovery errors and retained
source cleanup action. Show the persisted authoritative destination and actual
writer/route compensation outcome, including source quiesce failures after
cutover. Never present a failed/partial transition as successful.

Changing secret store explains that Secrets are copied first and running workloads
move after Preview/Deploy. Secret values are never displayed. On 409 show latest
selection and require review/resubmit; active-operation busy is temporary.

## T07 — Cài đặt môi trường (ENV-01, VI-02)

Trang và dialogs dùng nhãn tiếng Việt theo glossary; tab và giá trị kỹ thuật giữ
`staging`/`production`. Hai selector dùng tên an toàn và key kỹ thuật. Khi chưa
có binding và chỉ có một lựa chọn READY hợp lệ, UI gợi ý lựa chọn trong draft,
ghi rõ chưa lưu; đây không phải Organization default hoặc fallback. Chỉ nút
Lưu gửi thay đổi với expectedVersion. Có nhiều lựa chọn thì người dùng tự chọn;
không có lựa chọn thì hướng dẫn đăng ký đích và cho thử lại. Không đổi target
đã cấu hình. Phạm vi triển khai/region do backend suy ra, chỉ đọc.

Các nhãn tiến trình, chuyển đích, tác động, downtime và recovery dùng tiếng Việt.
Giữ Preview rồi xác nhận execution; migration vẫn cần xác nhận downtime và
recovery vẫn cần xác nhận operation cũ đã dừng. Không thay đổi API semantics.

## T09 — Tạo ứng dụng (APP-01, VI-02)

Form chỉ có Tên ứng dụng và Subdomain; nhãn, hướng dẫn, nút, validation và
aria-label dùng tiếng Việt. Gợi ý subdomain từ tên bằng slug lowercase DNS label
(chuyển dấu tiếng Việt, gồm đ/Đ; gom ký tự phân cách thành dấu gạch ngang,
bỏ gạch ngang đầu/cuối và giới hạn 63 ký tự). Tên không tạo được slug thì để
gợi ý rỗng và yêu cầu nhập subdomain hợp lệ. Gợi ý chỉ hỗ trợ nhập liệu, không
đảm bảo uniqueness; backend vẫn quyết định ID và kiểm tra trùng.

Tên thay đổi cập nhật gợi ý đến khi người dùng sửa subdomain lần đầu, kể cả
xóa trống; sau đó giữ nguyên lựa chọn thủ công. Preview hiển thị endpoints
staging/production theo contract, không provision. Submit chỉ gửi name/subdomain;
tạo thành công mở Application home staging. Không thêm lựa chọn đích/provider.

## T11 — Danh sách ứng dụng (VI-02/04/05)

Trang dùng tiêu đề “Ứng dụng của bạn”, nhãn “Không gian làm việc”, hướng dẫn
và nút “Tạo ứng dụng” bằng tiếng Việt. Cards giữ nguyên tên ứng dụng, endpoint,
thứ tự dữ liệu và điều hướng tới Application home; các nhãn môi trường giữ
`staging`/`production`, nhãn trạng thái hiện có dùng “Sẵn sàng cấu hình”. Đây là
Việt hóa cách trình bày hiện có, không thay cách suy ra trạng thái môi trường.
Nút tạo ứng dụng ở header và trạng thái rỗng mở cùng form tạo hiện có.
Không thêm search/filter hoặc thay API/payload.

## T12 — Trang ứng dụng và triển khai gần đây (VI-02/04/05)

Trang ứng dụng dùng tiếng Việt cho tiêu đề, điều hướng Cài đặt môi trường,
Workload, trạng thái draft, Xem trước thay đổi, Triển khai, Xóa và Hoàn tác;
giữ nguyên tên, IDs, endpoints, `staging`/`production` và payload. Tóm tắt
đích hiển thị Kết nối, Phạm vi triển khai và region an toàn cho đúng môi trường.
Thông báo ứng dụng mới nói rõ chưa triển khai và cần chọn kết nối từng môi trường.

Xem trước chỉ đọc; nút Triển khai vẫn cần thao tác tường minh và token hiện tại.
Kết quả hiển thị trạng thái và hành động tiếng Việt theo từng Workload, link
Deployment ID, thay đổi chưa hoàn tất và yêu cầu xem trước lại để thử lại.
Tài nguyên không còn được tham chiếu không bị diễn giải là đã xóa. Danh sách
triển khai gần đây giữ thứ tự, tối đa năm mục, thời gian và links hiện hành;
nhãn tải/rỗng/lỗi/thử lại và badges dùng tiếng Việt. Không thêm rollback.
