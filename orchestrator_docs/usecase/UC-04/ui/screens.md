---
id: UC-04-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-17
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
credential in summary/list/notification. Form AWS nhận access key qua ô che; edit/delete Connection ngoài phạm vi.

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

## Đăng ký AWS Connection

Form: name, concealed access key ID/secret access key, region/allowed regions,
explicit “Kiểm tra và lưu”. No Terraform backend input; platform backend is
shared. Safe result shows account/principal/region/check status only, never key
or secret ref. STS verified and provisioning capability checks are distinguished;
not READY solely because identity resolves. Clear credentials after success;
verification error preserves safe metadata and uses concealed re-entry as needed.
No resource creation during Connection registration or host-profile selector.


## V01 — Tài khoản hạ tầng

Navigation presents “Tài khoản hạ tầng”, at `/ui/platform/driver-accounts`.
Legacy `/ui/platform/connections` remains a compatibility entry while existing
consumers remain; prefer shared account UI components rather than copied forms.
List/detail show name/key, AWS/Internal, separate auth method, safe AWS identity
or Internal cluster descriptor, latest successful verification and version. Include
all account statuses with honest labels; an unverified seed is not READY.

Create uses existing concealed AWS/kubeconfig inputs and explicit verify/save.
AWS labels `verificationRegions` as “Vùng kiểm tra”, not deployment scope. Explain
that deployment region will be selected in Driver Inputs. No Environment picker,
applicability selector or account executionProfile control.

Each account offers view, rename, explicit “Kiểm tra kết nối” and a separate
concealed credential-replacement form, with same-target explanation. Never prefill
or reveal stored secrets. Successful save clears submitted secrets and refreshes
metadata/version. Developer cannot open management UI. Account deletion is absent.

## EC2 role [EC2-05]

AWS registration includes “Phương thức xác thực”: Access key / IAM Role của EC2.
EC2 mode asks only expected IAM Role ARN and “Vùng kiểm tra”, explains that
backend/executor must run on EC2 with that role attached, and retains explicit
“Kiểm tra và lưu”. List/detail show the authentication method and canonical role.
Role detail offers rename/reverify and explains automatic temporary credentials;
no access-key replacement form. No pasted metadata token or temporary key.

## Account diagnostics — DIAG-01..14

The Infrastructure Accounts list adds a latest-diagnostic summary with scope,
region/context when relevant and timestamp, separate from connection verification.
The account detail has Information and Diagnose tabs, keeping all existing
management forms/actions in Information. No separate sidebar entry is added.

Diagnose offers **Kiểm tra chung** and **Theo use case**. The account is pinned.
General requires no Application and only conditions mode; AWS can select a region.
Use-case setup selects capabilities, optional Application and Environment, and
conditions/supported execution mode. Missing Application context is explicit;
Application-specific results are UNKNOWN until the context supplies requirements.
These controls change only the diagnostic request, never account configuration.

Reuse the approved account-integrated mock layout, actual shared shell/table/tab/
Button styles and VI/EN catalogs. There is no production mock notice, scenario
selector, fictional identity or fixture timing. Findings/actions use backend DTOs.
Default results prioritize FAIL/BLOCKED/UNKNOWN. The adjacent inspector exposes
observed/expected conditions, affected use cases, remediation and prerequisite
links; safe technical evidence is an expandable disclosure. Export and history
retain exact scope and time. A conditions-only result never implies actual
workload/volume/cleanup readiness. Known stale context is visible.
