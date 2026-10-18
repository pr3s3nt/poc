---
id: UC-03-UI-SCREENS
artifact: use-case-ui-screens
status: current
last_reviewed: 2026-10-17
related: UC-03
---

# UC-03 UI screens

V02 sections below own current authoring/edit/account/matching behavior under
UC-03 V02-01..09. Earlier T02–T31 descriptions remain only for unchanged typed
inputs/interaction details; their profile/Environment AWS/insert-only/client-only
preview requirements are superseded. Predecessor is available at commit96eb602.

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
