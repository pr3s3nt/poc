---
id: UC-12-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-24
related: UC-12
---

# UC-12 UI screens

## Application Settings → Variables & Secrets

```text
Payment / Settings                                      Pending changes
Variables & Secrets

[ Staging  ] [ Production ]

Environment variables                                   [ + Add variable ]
Name                 Value                Used by           Actions
LOG_LEVEL            info                 backend, worker   Edit · Rename · Delete

Secrets                                                 [ + Add secret ]
Name                 Status               Used by           Actions
API_TOKEN            Configured           backend           Update · Rename · Delete
```

Only the selected Environment appears as a tab. Switching Environment changes
both section lists, not just displayed values. A key may be absent in one
Environment. The `Used by` column links to workloads in that Environment and
supports the impact warning.

## Environment variables section

Add/Edit asks for key name and a plain variable value. The value is visible
and editable. Rename asks for a new key name; Delete asks for confirmation.
Changes show a pending label and a path to Environment-scoped Preview changes.

## Secrets section

Add accepts key name and secret value. Existing rows show only `Configured` or
`Pending change`, never the value. Update asks for a new value in a concealed
input; leaving it empty does not reveal the previous value. Rename/Delete use
the same impact-warning flow without displaying secret material.

## Impact warning

For rename/delete of a referenced key, the confirmation names the selected
Environment and lists affected workloads. For delete or rename without opt-in, it states that references remain unchanged
and Preview will reject unresolved references. Rename offers the unchecked
“Đổi tên và cập nhật tham chiếu” checkbox; selecting it explains that only
pending workload references change and runtime requires Preview → Deploy.
`Continue` is available; the warning is not a hard block. Value update shows
affected workloads and explains that runtime changes only after Preview and
Deploy.

## Secret Store Connection

Selected Environment Settings displays the chosen store and editable READY choices.
Unset state allows ordinary Variable edits, but Secret Add asks to select a store.
Change-store action shows copying/verification, success/pending-deploy or safe error.
No raw Secret, store token or backup content appears in the UI.

## T07 — Nhãn và lựa chọn Kho bí mật

Trang dùng “Cài đặt môi trường”, “Biến”, “Bí mật”, “Tên key”, “Giá trị”,
“Workload sử dụng”, “Đổi tên”, “Xóa”, “Lưu thay đổi chờ triển khai” và
“Xem trước”/“Triển khai” theo glossary. Tab vẫn là `staging`/`production`.
Secret input được che và không điền giá trị đã lưu. Dialog đổi tên/xóa nêu
Environment và workload bị ảnh hưởng. Đổi tên có checkbox mặc định bỏ chọn
“Đổi tên và cập nhật tham chiếu”; không chọn hoặc Xóa thì người dùng tự sửa
tham chiếu. Chọn checkbox giữ tên biến trong Container và chỉ sửa drafts.

Khi chưa có kho và chỉ có một READY store hợp lệ, selector gợi ý draft chưa lưu,
không tạo binding hoặc cho phép Secret write trước Lưu. Nhiều kho yêu cầu chọn
tường minh. Kho đã cấu hình không bị thay; Lưu đổi kho vẫn copy/verify trước
commit, runtime chuyển sau Xem trước → Triển khai.

## T30 rename dialog

Checkbox và tên mới được giữ khi lỗi/BUSY/stale; không tự retry mutation.
Reload/review tường minh làm mới versions trước lần xác nhận kế tiếp. Đổi app/env
hoặc mở dialog khác reset opt-in; late reply không chuyển kết quả sang scope mới.
Thành công hiển thị thay đổi chờ triển khai và đường về Xem trước thay đổi trên
trang ứng dụng, không báo đã rollout.


## Tạo key từ workload editor — DEVEX-ENV

UC-12 BR-24–28 cho phép nhúng form tạo Variable/Secret trong UC-16; Settings
vẫn sở hữu sửa/đổi tên/xóa key dùng chung. Layout và trạng thái form con ở
[UC-16 screens](../../UC-16/ui/screens.md) và [states](../../UC-16/ui/states.md).
Create-only, CAS, no-readback, kết quả mutation chưa rõ và ranh giới key/workload
commit theo [OC-12/16](../../../architecture/contracts/operation-contracts.md).
