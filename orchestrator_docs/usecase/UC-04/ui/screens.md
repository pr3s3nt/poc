---
id: UC-04-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-11
---

# UC-04 Connections screen

`Platform / Connections` lists READY Connections owned by the Organization:
name/key, kind, cluster/endpoint and status. Existing unnamed records use key.

Kubernetes registration: Connection name, Upload kubeconfig file or Paste
kubeconfig, inspect action, context selector, selected cluster/endpoint summary,
then **Check and save**. Exactly one context auto-selects; multiple contexts
require a choice. No user-entered key, Cluster ID or endpoint; no host setup.
Switching file/document invalidates prior context/summary and pending results.

Credential textarea supports hiding its content; after success clear it and
file input. Errors retain the submitted form for correction. Never display
credential in summary/list/notification. AWS form and edit/delete are deferred.

## Layout and controls

Keep the registration content in a readable column (approximately 720px maximum),
with consistent field spacing. Present Upload file and Paste content as adjacent
compact choices with clear selected and keyboard-focus states. Radio/checkbox
controls sit beside their labels; text-field width and padding must not apply to
them. Keep the source group accessible and keyboard operable.

Use a file-picker button styled consistently with the console, with selected
filename next to it and an accessible native file input. Place the inspect action
beside or immediately below the file/content area, rather than at the far edge
of the page. Group context selection and the destination summary together below
the source area. Show context, cluster and endpoint as readable labelled values;
long endpoints wrap. The primary Check and save action belongs at the form footer.

Present registered Connections in aligned Name, Type, Destination and Status
columns. Distinguish the friendly name from the secondary key, and use a clear
READY status badge. Do not show empty Kubernetes fields for AWS records. On narrow
screens, stack content or use accessible scrolling without overflowing the page.
Preserve source-switch invalidation, credential masking, single-context automatic
selection, error retention and successful-save clearing behavior.

## Secret stores

Platform navigation adds Secret stores. List displays name/provider/backend and
workload addresses/mount/status. Registration form has name, Vault addresses,
KV/auth mounts, optional trusted CA and concealed token. Check and save verifies
before showing READY; clear token after success. Developers see only safe choices
in Environment Settings, not the privileged registration form.

## Kết nối — T06 (CN-01/02)

Trang `/ui/platform/connections` dùng tiếng Việt cho text, aria-label,
validation và mọi trạng thái; context/cluster/endpoint/key và payload giữ nguyên.
Upload file tự gọi inspect sau khi đọc xong; paste tự inspect sau khi nội dung
ổn định (debounce), không inspect mỗi ký tự. Có nút kiểm tra lại khi inspect lỗi.
Inspect chỉ đọc metadata, không truy cập cluster hoặc lưu Connection.

Một context tự chọn; nhiều context yêu cầu người dùng chọn, không dùng
`current-context` để đoán. Tên được gợi ý từ cluster/context đã chọn khi người
dùng chưa sửa tên; người dùng sửa tên thì gợi ý sau không ghi đè, kể cả khi
đổi document/context. Document đổi lập tức xóa context/summary cũ và vô hiệu
phản hồi inspect/file-read đang chờ. Gợi ý chưa được người dùng sửa đi theo
context mới; không dùng metadata document cũ để gợi ý cho document mới.

Nội dung paste mặc định che; file upload không hiển thị nội dung. Summary/list/
notification chỉ safe metadata. “Kiểm tra và lưu” luôn là thao tác tường minh,
không tự chạy sau inspect; READY hiển thị “Sẵn sàng”, chỉ phản ánh lần xác minh.
Giữ layout, role/org gates, clear credential sau success và phân biệt lỗi
reload list với đăng ký đã thành công. Sidebar Việt hóa thuộc T10.

## Kho bí mật — T05 (SS-01..04)

Trang `/ui/platform/secret-stores` dùng tiếng Việt cho toàn bộ text/aria-label,
validation, thông báo và trạng thái; technical IDs/URL/payload giữ nguyên.
Phần cơ bản gồm tên, địa chỉ backend, địa chỉ workload và token được che.
Checkbox “Dùng cùng địa chỉ backend” mặc định tắt, chỉ đồng bộ địa chỉ trong
payload khi người dùng chọn. Khi tắt, khôi phục địa chỉ workload riêng đã nhập;
không ghi đè draft riêng khi backend đổi hoặc khi bật checkbox.

Mount KV v2, mount xác thực Kubernetes và CA PEM tùy chọn nằm trong phần
“Nâng cao”; đóng phần này không mất dữ liệu. Tóm tắt luôn cho biết mount hiệu
lực (mặc định `kv`/`kubernetes`) và CA tùy chỉnh có/không, TLS luôn xác minh.
Kiểm tra PEM ở frontend chỉ kiểm shape, không chứng minh chứng chỉ hợp lệ,
độ tin cậy hoặc khả năng kết nối.

Danh sách hiển thị tên/key/provider, cả hai địa chỉ, mount và CA, trạng thái kho,
kết quả xác minh backend và xác thực Kubernetes tách riêng. READY hiển thị
“Sẵn sàng”: xác minh backend tại lúc đăng ký, không cam kết deploy-ready;
auth chưa cấu hình phải thể hiện rõ. Thiếu metadata không suy thành công.
Không hiển thị token hoặc credential ref. Không có nhãn/type legacy trong danh sách kho hoặc choices (T23). PE/Admin gates và Developer choices không đổi.
