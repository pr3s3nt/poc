---
id: UC-16-ENVFILE-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-11-11
related: UC-12, UC-16
---

# Bảng biến và nhập file .env — issue #72

Quyết định người dùng 2026-11-11. Extension này thay phần checkbox tạo Secret
ngay của BR-27; các quyền, safe literal provenance và Preview/Deploy giữ nguyên.

## ENVFILE-01 — Một draft mỗi container

Bảng biến mặc định và trình soạn .env cùng chỉnh bindings có thứ tự. Theo quyết
định nhãn mới, bỏ chữ/tab UI; nút Nhập từ file .env mở trình soạn và Quay lại bảng
trở về bảng biến. Chuyển tab không commit.
Thêm/sửa/xóa/đổi nguồn/thứ tự/Bí mật đồng bộ. Text sai được giữ nguyên, báo dòng,
không cập nhật từng phần và chặn Save cùng chỉnh UI cũ cho tới khi sửa text.
Draft chỉ ở memory; không storage/log/telemetry. Secret literal chưa lưu có thể
hiện trong text người dùng đang chỉnh; không coi đây là cơ chế che hoàn toàn.

## ENVFILE-02 — Grammar và round-trip

UTF-8, bỏ BOM đầu file; LF/CRLF là dấu xuống dòng cú pháp. Tên biến theo UC-16.
Cho whitespace quanh tên và dấu bằng. Dòng trống và comment bắt đầu bằng # được
bỏ qua, trừ marker. Unquoted value trim whitespace hai đầu; # bắt đầu comment
chỉ ở đầu value hoặc sau whitespace. Dấu = tiếp theo thuộc value. Single quoted
value giữ nguyên mọi ký tự (kể cả backslash); double quoted value giải mã
\\, \" , \n, \r, \t; escape khác báo lỗi. Cả hai cho multiline, giữ
line endings trong value. Sau quote đóng chỉ cho whitespace/comment.
Serializer dùng double quotes và escape để giữ chính xác whitespace/newline/
backslash/quote/Unicode. Không giữ bố cục/comment thường qua chỉnh UI; giữ value,
nguồn, thứ tự và marker. Không expansion $VAR/${...}, shell hay interpolation.

Giải mã quotes trước, rồi phân loại toàn bộ value:

| Value | Binding |
|---|---|
| @variable.KEY | Variable UC-12 hiện có |
| @secret.KEY | Secret UC-12 hiện có, không readback |
| @service.workload.port | Service của workload khác cùng Environment |
| @alias.output | Output dependency đã khai báo |
| @@text | Literal @text |
| value khác không bắt đầu @ | Literal |

Value bắt đầu @ sai reference không fallback literal. Literal bắt đầu @ được
serializer thêm một @. Reference thiếu/sai loại báo dòng và giữ nguyên draft.
KEY nguồn chưa phân loại phải đợi metadata; không đoán kind khi xuất file.
Service parse giữ alias cũ nếu cùng name/workload/port để không đổi identity.

## ENVFILE-03 — Marker, alias và validation

# @secret (trim whitespace) chỉ áp dụng khai báo ở dòng ngay sau; EOF, blank,
comment, marker khác hoặc khai báo lỗi báo vị trí marker/declaration. Marker chỉ
đánh dấu literal mới; reference phân loại theo metadata, không override nguồn.
Trùng key báo dòng khai báo đầu và dòng trùng. NUL bị từ chối.
Alias secret, variable, service dành riêng, cùng guard env hiện hành. Alias duy
nhất mỗi workload. Dữ liệu cũ xung đột giữ nguyên binding và báo cần đổi alias
rõ ràng; không tự retarget. Rename UI cập nhật RESOURCE bindings và text draft;
không sửa nguồn ngoài phạm vi. Type đổi giữ guard resourceChanged hiện hành.

## ENVFILE-04 — Nhập và tải file

Nhập file là merge có review: mỗi collision mặc định Giữ hiện tại, có Thay thế;
không xóa biến vắng trong file. Thay thế giữ vị trí hiện tại, biến mới append theo
thứ tự file. Text editor là chỉnh trực tiếp toàn draft nên xóa khai báo xóa dòng.
File upload tối đa 1 MiB. Tải mẫu từ dependency/output metadata và tải nội dung hiện tại, không resolve
output/Secret. File lỗi UTF-8 không sửa draft. Reply đọc file muộn bị bỏ khi
scope đổi/unmount hoặc đã có thao tác nhập mới.

## ENVFILE-05 — Secret hàng loạt và boundary

Tick literal chỉ đánh dấu draft; bỏ tick bỏ marker, giữ value. Chọn nhiều dòng
rồi đánh dấu không cần nhập lại. Trước Save phải xác nhận chung tạo Secret đúng
app/env bằng create-only API theo expected config version, tuần tự cập nhật
version. Không lưu secret literal vào workload. Tên Secret mặc định là tên biến,
cho rà soát/chỉnh trước xác nhận. Existing key chỉ reuse sau lựa chọn tường minh,
đúng SECRET kind; không overwrite. Empty/NUL/name validation giữ UC-12.
Mỗi success chuyển đúng row thành reference và xóa literal khỏi draft/text.
Partial success hiển thị từng dòng; các key đã tạo vẫn tồn tại khi hủy workload.
Unknown network outcome chặn tự replay; GET metadata không chứng minh value đúng,
chỉ cho reuse sau review hoặc retry tường minh sau xác minh key chưa tồn tại.
Chưa chọn store dùng initial selection hiện hành, không tự chọn/migrate kho.

## ENVFILE-06 — Acceptance

Focused parser/component/API guards; browser VI/EN và keyboard/focus theo shared
UI. Save → Preview → Deploy → reopen giữ bindings và order. Full checks trên kind,
cleanup namespace riêng, Docker feature stack và browser Tailscale theo AGENTS.
