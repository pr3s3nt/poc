---
id: UC-03-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-24
related: UC-03
---

# UC-03 UI screens

Current authoring/edit/account/matching follows the V02, V03 and V04–V06
replacement sections below and UC-03 specification. Earlier T02–T31 descriptions
are historical wherever they name profiles, an Environment target, insert-only
editing, builtin cluster restrictions or client-only matching preview. Only
unchanged typed inputs/interaction details survive. Predecessor: commit96eb602.

Trang Cấu hình tài nguyên liệt kê cấu hình thuộc Organization hiện tại, gồm
ID, loại tài nguyên, phạm vi triển khai, driver và số điều kiện áp dụng. Không
hiển thị cấu hình `workload`; các cấu hình này nằm ở trang Mẫu dựng ứng dụng. Cấu hình
`existing-cluster` nếu có chỉ đọc và mang nhãn “Hệ thống”. Không có sửa/xóa;
đăng ký chỉ dành Kỹ sư nền tảng/Quản trị viên.

## Form đăng ký

PE/Admin chọn ID, Resource Type, cách tạo Terraform và profile phù hợp module.
Form chọn module từ nguồn Admin cho phép, immutable version/revision/digest,
input/output mapping và symbolic provider bindings. Nhãn “Terraform” áp dụng
cho cả cloud và Kubernetes; không khóa vào aws-eks hoặc tự chọn module theo Type.

Hiển thị schema đã inspect: types/defaults/required, Type outputs và secret
flags. Inputs typed hoặc JSON nâng cao giữ dữ liệu/kiểu khi đổi cặp/module.
Output mapping không được hạ secret flag. AWS binding luôn là “Kết nối AWS
của môi trường”, kể cả Aurora/ElastiCache; không có selector account riêng.
Kubernetes bind target Environment hoặc output target của EKS dependency.
Đăng ký không cần Environment cụ thể và không provision hạ tầng.

Loading/error/empty registry là các trạng thái khác nhau, đều chặn submit chưa
kiểm chứng; retry giữ edits. Source không hợp lệ, drift hoặc schema mismatch
có hướng dẫn safe theo code. Không fields provider credential trong Definition.
Giữ editor criteria, provision rules, matching preview và advanced JSON lossless.
Public existing-cluster/reserved keys không được đăng ký; Workload rendering
ở trang Mẫu dựng ứng dụng riêng.

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
cấu hình cùng type không kết luận ambiguous hoặc Definition thắng. Trợ giúp
này không gọi endpoint; không lưu hoặc provision khi xem trước. Editor có thể
được dùng lại cho Mẫu dựng ứng dụng ở T03.

## Trang Mẫu dựng ứng dụng (T03)

Trang `/ui/platform/rendering-templates` nằm ngang hàng các trang catalog trong
sidebar, chỉ Kỹ sư nền tảng/Quản trị viên truy cập. Danh sách chỉ lấy Definitions
có `resourceType: workload`, hiển thị ID kỹ thuật, bundle ID và điều kiện áp dụng.
Form gồm ID mẫu, bộ chọn Bundle dựng ứng dụng và editor Điều kiện áp dụng T02B.
ID tuân thủ chữ thường/số/gạch ngang, không có friendly name.

Type `workload` và driver `score-k8s` cố định; profile chọn `internal-k8s`
hoặc `aws-eks` theo bundle tương thích;
không có controls Connection, provision, module hoặc JSON variables tùy ý.
Bundle lấy từ GET `/api/v1/render-bundles`, hiển thị ID/version. Một bundle tự
chọn khi chưa có lựa chọn; nhiều bundle cần chọn tường minh. Không có bundle
thì khóa đăng ký và hướng dẫn liên hệ Kỹ sư nền tảng. Không hard-code bundle ID.

Mọi Workload cần đúng một mẫu score-k8s khớp criteria cho profile đang dùng.
Không có match/tie/bundle thiếu hoặc render lỗi đều chặn triển khai. Không có binding
mới cho Environment hoặc upload template. Text/aria-label/trạng thái dùng Việt;
technical IDs/payload giữ nguyên. Lưu mẫu không tự Preview/Deploy.

## Điều hướng từ Loại tài nguyên (T04)

CTA từ UC-02 mở `/ui/platform/resource-definitions?resourceType=<key>`.
Chỉ chọn sẵn type khi catalog tải thành công xác nhận key có cặp hỗ trợ trong metadata T31; query thiếu/không hỗ trợ không tạo lựa chọn hợp lệ. Prefill không
gửi POST hoặc ghi đè dữ liệu người dùng khi reload/response đến muộn.

## Matching preview chính xác (T19)

Hai form Cấu hình tài nguyên/Mẫu dựng ứng dụng có thao tác tường minh “Kiểm tra
matching” trong editor chung, dùng draft Definition đầy đủ đang nhập và context
ứng dụng/môi trường/ID tài nguyên/Class mẫu. Không tự save hoặc full graph Preview.
Hiển thị eligibility, criterion/score, winner/ties hoặc conflict bằng tiếng Việt;
INCOMPLETE yêu cầu bổ sung context, transport failure có Thử lại riêng.
Hiển thị marker/version snapshot và nhắc “Kết quả theo snapshot; hệ thống vẫn
kiểm tra lại khi đăng ký và triển khai”. Giữ cảnh báo nguy cơ frontend riêng.

## Tham số typed và provision editor (T31)

Form lấy capabilities OC-04d; PE chọn module/revision đã kiểm chứng, không dùng
whitelist hoặc module ngầm ở frontend. Tham số cấu hình có typed controls string/number/bool/list/map theo
metadata, ghi nguồn param/context/default và required. Không điền params workload
vào Definition; param thực của workload ưu tiên hơn giá trị fallback trong
Definition. Default omitted không thành override; người dùng chọn override
hoặc preset tường minh, giữ whole/interpolated placeholders và escaped literals.
Namespace preset và VPC refs @infra chỉ theo BR-25, không đoán capacity/engine.

Chuyển typed/JSON nâng cao giữ giá trị/kiểu; JSON không biểu diễn được an toàn
ở typed giữ ở advanced với hướng dẫn Việt. Chuyển type/driver giữ draft riêng
của cặp; response metadata/retry không ghi đè edits. Executor-owned/credentials
không có controls. JSON hợp lệ vẫn được server kiểm tra strict.
Provision editor gồm descriptor, params và hai flags, giải thích consumer/provider
và chiều consumer → provider (is_dependent đảo vai trò parent/companion,
match_dependents nối consumers tới companion); advanced JSON giữ cases phức tạp.
Không tự Preview/provision khi đổi form. CTA/prefill chỉ được bật khi metadata
và catalog cùng xác nhận cặp supported.

## V02 — authoritative replacement of legacy authoring controls

UC-03 V02-01..09 owns behavior. List/form show Definition ID, Name, Resource
Type, supported Driver Type, Driver Inputs, Driver Account and five-field
criteria. No executionProfile selector, client-only scope preview, account
applicability or Environment AWS inheritance. Label Definition ID separately
from Resource ID (res_id) criteria. Explain AND within row, OR across rows,
missing criteria versus explicit wildcard. Only server preview declares winner.

AWS driver shows scoped compatible READY account selector and region in typed
Driver Inputs. Kubernetes dependency-bound driver shows inherited cluster source,
not another account selector; retained Environment target is visibly legacy.
Keep typed/advanced variables, mappings, verified module pins, references and
provision flags losslessly. Backend capability metadata is authoritative.

Edit opens current Name/config/version; ID/type read-only. Save is explicit;
no resource mutation. Success reloads catalog, failure keeps edits; 409 prompts
reload/review without retry mutation. Explain subsequent Preview/Deploy uses new
config while existing deployed snapshot stays unchanged. Rendering templates
consume same Name/version/profile-free Definition contract; no Generic Workload
lifecycle UI in V02. Preview shows winner/reason/criterion/score, account/region
or inherited cluster source, distinct no-match/ties/conflict; all edits stale it.


## V03 UI/API replacement

[ADR-016](../../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) defines current internal placement. Remove deployment Connection selection after replacement pass; keep Secret Store/configuration/version/error/operation state. Definition authoring supports existing-cluster account choice and workload namespace/cluster references, Type params and provision flags; consumers inherit account. Score import/edit/export preserves metadata.class. Preview shows each workload/resource Definition/account/cluster/namespace and provider-first graph/batches without secrets, and blocks missing/ambiguous/mismatched/unready inputs as appropriate. No Environment target is required. Retired deployment-Connection writes fail explicitly; safe matching and stale-token gates remain. UI multi-cluster V03 unsupported error occurs before execution. Progress/readiness/errors/update/remove use per-workload targets; failure never shown as success.


## V04–V06 current target/dependency behavior

[ADR-017](../../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces remaining profile/Environment-target requirements above for the
replacement path.

Show Definition/driver/account/region per resource and the explicit cluster
source per namespace/workload. Graph shows real consumer→provider edges and
ordering reasons separately from selection-only relations. Unknown EKS output
values show waiting/provenance, never fabricated Ready/endpoint values. Reuse
existing authoring, Preview and progress views; no new managed-only execution
entry or multi-cluster/patch-authoring screen.


## Git module form — issue #7

UC-03 GIT-01..06 thay giới hạn selector-only trước đây. Chọn module verified
hoặc Thêm từ Git (HTTPS URL, thư mục, tag/commit); nút Kiểm tra module gọi
POST `/api/v1/terraform-modules/inspect` với `{resourceType,url,path,ref}`.
200 trả `{capability}`; 400 invalid/unavailable module, 401 session, 403 role,
413 oversized; lỗi nội bộ dùng generic 500, không raw Git stderr/URL credential.
Loading/error giữ draft; thay source/type/mode hoặc unmount vô hiệu late reply.
Save chặn tới verified selection; inspect không lưu Definition hay provision.
Form giữ typed mappings/variables/provision/criteria và CAS hiện hành. Selector
không hiện digest; Chi tiết kỹ thuật đóng mặc định chứa full pin. Public HTTPS
không cần credential; chưa có private Git auth/SSH. New Type có thể chọn trước
khi module đầu tiên được inspect. Giữ create/edit/reload và installed module flow.

## Resource Definition v3 — issue #8 (current replacement)

UI3-01..04 thay bốn chế độ criteria ở form Resource Definition bằng rule editor.
Hai tab Cấu hình / Điều kiện áp dụng có keyboard navigation, active gạch cam
và cùng draft. Tab Cấu hình có module verified/Git, tài khoản/region/variables;
Thiết lập nâng cao đóng mặc định chứa mapping/provider/provision, Form/JSON
hiện hành. Xem bản đang nhập cho phép review payload đầy đủ trước Save.
Tab Điều kiện áp dụng có trường hợp AND, OR giữa trường hợp, selector năm
field chuẩn và giá trị; tóm tắt phạm vi luôn có. Copy, matching, help đóng mặc
định. Không xuất hiện missing-context message cho tới khi mở matching.
Validation/actionable errors và overlap warning vẫn hiện trong tác vụ tương
ứng. Mẫu dựng ứng dụng giữ projection cũ; shared guard/converter không đổi.
Không có Usage, dữ liệu minh họa hoặc thay đổi theme/navigation.

### V3 visual fidelity correction — 2026-10-21

Catalog and Definition editor are separate views. Create/Edit opens a focused
Definition view with breadcrumb, display name/driver/version header and two tabs.
Back to catalog preserves the mounted draft; explicit new/cancel actions retain
the existing reset semantics. Type CTA opens creation directly. Configuration
uses numbered module and account/parameters cards, an actual metadata overview
on desktop, and a collapsed advanced card. Source modes use adjacent buttons.
Input/output mapping rows align Type name → module selector; types/required/
secret classification remain visible, detailed explanations are disclosures.
Matching rows are compact with the scope summary alongside them. Save actions
stay visible in a bottom bar. Responsive layouts keep all tasks accessible.
No synthetic defaults, inspector/matching results or backend changes.

The advanced card has one shared Form/JSON toolbar for mappings and provision,
with separate labeled JSON objects under the existing contract. Switching is
atomic: either both existing converters succeed or both drafts stay in their
current representation with actionable errors. Variables retain explicit opt-in
overrides in a compact unboxed grid; read-only fields show metadata defaults or
inherited sources without inserting values into the submitted draft. Scope summary groups actual conditions by
case; full-payload review remains a quiet disclosure.

Nguồn module mặc định khi tạo mới/reset là Git (UC-03 GIT-03). Đặt Git
trước lựa chọn Module đã đăng ký; edit giữ nguyên pin hiện có.

## Resource Definition module forms — issue #11

UI11-01..05 thay phần 2 “Tài khoản & thông số” và “Tài nguyên đi kèm” theo
[mock đã duyệt](module-forms-prototype.md); sidebar, tabs, font, spacing, nút và
các phần khác của form giữ nguyên. Chỉ desktop.

Phần 2 mở đầu bằng thẻ “Kết nối AWS” (Tài khoản READY + Vùng) hoặc “Kết nối
Kubernetes” (Nhóm cluster/class + ghi chú dùng thông tin truy
cập từ cluster) cho từng provider theo metadata. Tiếp theo là “Thông số tài
nguyên”: thẻ cho thông số cần cung cấp/ánh xạ (viền cảnh báo khi thiếu), nhóm
thu gọn “Thông số có mặc định” và “Thông số tùy chọn”, nhóm “Hệ thống tự cung
cấp” chỉ đọc. Mỗi thẻ có tên, kiểu, nguồn hiện tại, giá trị tóm tắt, nút
“Cung cấp giá trị”/“Thay đổi” và ghi chú ưu tiên khi có param ánh xạ.

Hộp thoại “Chọn nguồn giá trị”: Nhập giá trị / Dùng mặc định hoặc param ứng
dụng (nếu hợp lệ) / Lấy từ ngữ cảnh triển khai / Lấy từ tài nguyên khác. Nhánh
cuối chọn Loại tài nguyên, Nhóm cấu hình (class), Phạm vi, Output cần lấy và hiện
biểu thức sinh trong phần kỹ thuật thu gọn. Hủy không đổi draft; Áp dụng ghi
một lần vào draft.

“Tài nguyên đi kèm” nằm trong Thiết lập nâng cao: thẻ tài nguyên đang cấu hình,
một thẻ cho mỗi tài nguyên đi kèm (Loại tài nguyên, class, phạm vi, params theo
schema Type, hai cờ bằng tên cụ thể) và “Thêm tài nguyên đi kèm”. “Cấu hình kỹ
thuật” giữ descriptor và params JSON. Không có “Thứ tự thực hiện”/sơ đồ live.

## Unified cluster authoring — 2026-10-21

UI3-05 replaces the separate existing-cluster section: one catalog and Create/Edit view. k8s-cluster offers server-supported existing-cluster and Terraform methods; existing-cluster shows Internal READY account plus shared identity, criteria tabs and payload review, without module/region/variables. Accounts remain separate.

## Git render-bundle source — ADR-021

Extend Mẫu dựng ứng dụng at the existing route, PE/Admin gate and shell. Reuse
Resource Definition's Git source/inspect and verified-source patterns, shared
Button/input/help/technical-details components. Do not redesign navigation,
criteria, placement or companion-resource controls.

New source section has “Thêm từ Git” (default) and “Bundle đã xác minh”. Git fields:
“Git HTTPS URL”, “Thư mục bundle” (default `.`), “Tag hoặc commit”, action
“Kiểm tra bundle”. Explain credential-free HTTPS limitation in help. Successful
inspection shows name, short commit and “Đã xác minh”, then schema-driven input
controls. Boolean checkbox, enum select, string/number input; nested object/array
use validated JSON editor with field errors. Keep advanced JSON lossless. Explicit
false/0/empty and omitted values stay distinct; show effective default as a hint,
not a silently saved user value. All labels/errors/help use VI/EN catalog keys.

Verified selector includes legacy installed entries and same-org Git records.
Edit opens saved source/input/version without reinspection. Full URL/path/commit/
digest are in “Chi tiết kỹ thuật”; renderer pin is not editable. “Kiểm tra lại”
on an unavailable saved pin uses its exact full commit. Selecting a new ref is
an explicit source edit. No registered installed bundle must not block Git authoring.

Source/schema/inputs share draft lifetime with existing matching/provision/target
fields. Source change blocks Save until verified and shows incompatible inputs;
never erase unrelated draft fields. Save remains explicit and does not deploy.
Preview/Deployment overview shows source name and short commit; manifest view
shows permitted rendered defaults/patches with existing before/after/unknown UI.

Implemented 2026-10-24; visual comparison at 1512px/1280px with Resource
Definitions → Create Git source is recorded in the
[verification](../../../verification/2026-10-24-git-render-bundles.md#ui-comparison).
Reference inspected on 2026-10-24: actual Rendering Templates and Resource
Definitions → Create in Chromium, localhost:13003. Keep source controls compact;
move repeated explanations and full pins to help/details. Future handoff must
record visual comparison at 1512px/1280px; no new product screenshot was made in
this documentation-only design task.
