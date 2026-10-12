---
id: UC-16-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-13
related: UC-16
---

# UC-16 UI screens

## Application home — selected Environment

The existing UC-01 Application home keeps visible `staging`/`production` tabs.
The selected Environment shows its workload list, pending-change labels,
`+ Add workload`, Edit/Delete actions and `Preview changes`. A link to the
Application-level Variables & Secrets settings opens UC-12. Preview is scoped
to the selected Environment, never to both at once. The pending Preview/Deploy
panel displays the persisted selected Environment connection key/profile (UC-01 BR-08);
each Environment displays its own independently configured target (UC-01 BR-04/BR-05).

## Add/Edit workload

One page shows the selected Environment and a two-way input choice: **Enter on
form** (default) or **Import Score**. The form groups basic information,
containers and compute resources, Service/health checks, resource dependencies,
environment variables, then secrets. Save/Cancel remain visible at the bottom.
Edit uses the same form and shows the current workload name.

| Group | Input |
|---|---|
| Application variables | Per-container checklist of UC-12 Variable key names in this Environment. Tick to use the same name; expand "Use a different container name" only for aliases. No value field or repeated Application-source selection. |
| Application secrets | Separate checklist of Secret key names with the same optional alias control. Values are never shown or requested. |
| Other sources | Explicit container name and resource-output or workload-Service picker. No Application variable/secret options in this row editor; Application keys are selected above. |
| Resource output picker | Select declared dependency, then eligible output from its contract; secret classification determines which group can use it. |
| Resource dependency inputs | After selecting Resource Type, show its declared input fields with required indicators and type-appropriate controls. PostgreSQL exposes required `database` and `username`; editing preserves saved params. |
| Workload Service picker | Select a non-deleted workload in the same Environment that declares a Service, then select a declared port; show the resulting internal endpoint as read-only explanation. |
| Public access | Repeatable Public path + Service port rows; for example `/` to frontend and `/api` to backend. Paths must be unique within the Environment, ports declared on their workload. Show the intended shared host without claiming DNS/TLS is ready. |

The variable and secret groups link to Application Variables & Secrets (UC-12).
Returning from settings preserves the unsaved workload form and refreshes the
available reference names.

## Import Score

Choose one Score file for one workload. After parsing, show the same form for
review, including resolved reference types and validation messages. Reject
direct literal values in container variable/secret bindings. Import does not
save or deploy by itself.

## Delete confirmation

The confirmation names workload and Environment and explains that deletion is
pending until deployment. After confirmation the row reads `Pending deletion`
and offers Undo. It remains visible in Preview changes.

## Pending changes

Saving or marking deletion returns to Application home. A visible pending
indicator leads to UC-05 Preview changes. No Deploy action is silently
performed by this screen.

## Existing-key selection behavior

- Render keyboard-accessible labelled checkboxes under Variables and Secrets
  for each container. Newly added containers start unchecked. Display the key
  name and selected container name; do not show catalog values in this picker.
- For `DATABASE_PASSWORD`, a checked key defaults to container name
  `DATABASE_PASSWORD`. Expanding "Use a different container name" reveals an
  input where the user can enter `PGPASSWORD`. Removing the override resets
  the name to `DATABASE_PASSWORD`.
- Edit/import preselect existing keys and reveal differing aliases. Multiple
  names already mapped to the same key remain visible and editable. Unchecking
  removes those mappings from that container only.
- Unavailable selected keys appear explicitly with their original names and
  removal affordance. Empty Variables/Secrets sections explain that keys are
  created in Application Settings; API failure has Retry, not an empty state.
- All selected/added names must be complete and unique within their container.
  Conflicts with Application/resource/Service mappings keep the form and block
  Save; identical names in another container are allowed.
- Freeze key-selection and alias controls during a pending Save. A stale Save
  preserves selection/aliases and uses the existing reload/review flow.

## T08 — Form cơ bản và nâng cao, nhãn tiếng Việt

Trang editor dùng nhãn tiếng Việt theo [glossary](../../../GLOSSARY.md#nhãn-web-console-vi-01),
bao gồm validation frontend, aria-label, trạng thái tải/rỗng/lỗi và key picker.
Giữ `Workload`, `Container`, `Service`, `staging`/`production`, technical IDs,
image, Score keys và payload. Mapping lỗi backend theo contract code T20.

- Phần cơ bản giữ tên Workload, Container/image, key picker, resource alias/type
  và các input có kiểu, Service port và đường dẫn công khai.
- Mỗi Container có phần nâng cao cho CPU/memory request và limit. Tóm tắt luôn
  hiện request có hiệu lực: request tường minh, nếu thiếu thì limit cùng loại,
  nếu cả hai thiếu thì CPU `10m`, memory `32Mi`. Limit thiếu hiển thị chưa đặt.
  Đây là giải thích defaults hiện hành, không tự thêm capacity vào Score.
- Resource class nằm trong phần nâng cao từng dependency. Class rỗng có hiệu
  lực `default`; giữ quy tắc bỏ field class rỗng trong Score.
- Target port override nằm trong phần nâng cao từng Service port. Khi rỗng,
  target port có hiệu lực bằng Service port và serialization giữ fallback số
  hiện hành. Không tự thêm public route.
- Sau thao tác “Thêm đường dẫn công khai”, chỉ tự chọn đích khi có đúng một
  Service port hợp lệ (tên không rỗng, số nguyên 1–65535, tên duy nhất).
  Với không hoặc nhiều cổng hợp lệ, yêu cầu chọn tường minh. Thay đổi/xóa cổng
  khiến đích route không hợp lệ phải hiển thị lỗi và chặn lưu; không retarget.
- Đóng phần nâng cao chỉ đổi hiển thị; imported/edited overrides và alias vẫn
  giữ nguyên. Hiện tóm tắt override ngay cả khi đóng, để cấu hình không bị ẩn.
- Container đầu mặc định `main` và key-name defaults giữ như BR-15–BR-18;
  không tự chọn Secret hoặc key mới. Lưu vẫn chỉ tạo draft cho đúng môi trường.

## T12 — Entry points từ trang ứng dụng

Trang ứng dụng giữ scope môi trường đã chọn, với entry points “Thêm Workload”,
“Sửa”, “Xóa”, “Hoàn tác”, “Xem trước Score” và “Xem trước thay đổi”.
Lưu draft quay về danh sách có nhãn Chờ thay đổi; không triển khai ngầm.
Tab và payload giữ `staging`/`production`; missing key chỉ có metadata an toàn,
không hiện giá trị. Chuyển môi trường không nhận phản hồi của scope cũ.

## T25 — Import đường dẫn công khai

Form chỉ serialize `service.publicRoutes`. Import có `service.publicPort` báo
lỗi tiếng Việt theo `INVALID_SCORE`, giữ nội dung để sửa, không tạo route `/`
và không tự lưu. Draft/current projection cũ chứa field này bị từ chối rõ;
không mở form với routes bị bỏ mất. Valid import/edit giữ đầy đủ nhiều routes,
Service ports và overrides; Lưu → Xem trước vẫn là các bước tường minh.

## T30 pending rename boundary

UC-12 explicit rename opt-in chỉ sửa desired references nguyên tử với revision;
Preview đọc drafts mới, liệt kê workloads bị ảnh hưởng và pin config/draft/current
set versions. Deploy vẫn tường minh; token trước rename bị stale. Rename-only
hoặc delete có thể để reference thiếu và Preview phải chặn Deploy. Không đổi
runtime/current set hoặc Environment khác khi lưu.

## Terraform v2 Workload form

Keep current groups and Save draft → Xem trước thay đổi → Triển khai flow.
Each Redis/PostgreSQL dependency displays “Cách cung cấp / Class” beside its
alias/type/typed params using authoritative metadata: “Trong cluster”
(`in-cluster`) and “Dịch vụ managed AWS” (`cloud-managed`). This replaces hiding
these v2 choices solely under generic advanced class text. Redis in-cluster
on EKS is valid. Developer never chooses Terraform module or provider credential.
Keep advanced/custom/imported class lossless; do not silently rewrite default.
Shared ID control keeps existing semantics for backend/worker dependencies.
Show target incompatibility or missing metadata/Definition as actionable error,
not automatic class switch. Secret outputs remain bindings only; no values.
