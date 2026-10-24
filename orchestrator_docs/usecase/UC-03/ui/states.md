---
id: UC-03-UI-STATES
artifact: use-case-ui-states
status: current
last_reviewed: 2026-10-24
related: UC-03
---

# UC-03 UI states

V02 sections below own current authoring/edit/account/matching behavior under
UC-03 V02-01..09. Earlier T02–T31 descriptions remain only for unchanged typed
inputs/interaction details; their profile/Environment AWS/insert-only/client-only
preview requirements are superseded. Predecessor is available at commit96eb602.

- Loading: đang tải Resource Types và Definitions.
- Empty: chưa có Definition; form vẫn dùng được nếu có Resource Type.
- Ready: list và form hiển thị.
- Saving: khóa submit và chỉnh form đang gửi; không để response thành công xóa
  input mới chưa được submit.
- Error: giữ nguyên form khi JSON/contract/connection không hợp lệ.
- Created: nạp lại danh sách, reset form.
- Created/load error: giữ thông báo registration đã commit; reset submitted
  form, tách lỗi reload khỏi lỗi POST và cho Retry tải list, không tự đăng ký
  lại. Lỗi tải ban đầu có Retry và không giả danh empty catalog.

## Trạng thái form T02

- Loading/error của Loại tài nguyên hoặc Kết nối không ghi đè metadata đang
  nhập; có nút Thử lại. Không giả lỗi tải thành catalog rỗng.
- Provider metadata đã kiểm chứng quyết định AWS Environment binding hoặc
  Kubernetes Environment/EKS target. Không có Aurora account selector.
  Registry thiếu/lỗi hoặc symbolic binding không hợp lệ thì chặn submit,
  không dùng host/default credential.
- ID rỗng/không hợp lệ có lỗi Việt rõ; duplicate 409 giữ toàn bộ form để sửa
  ID và thử lại. Không chuẩn hóa ID âm thầm.
- Submit lỗi giữ metadata, criteria, JSON và lựa chọn; không tự replay POST.
- Đổi type/driver hoặc mở/đóng nâng cao không mất JSON/criteria hoặc lựa chọn
  trước đó; controls và payload chỉ dùng driver/type đang có hiệu lực.
- Created/reload error phân biệt đăng ký thành công với lỗi tải danh sách;
  Thử lại chỉ tải list, không đăng ký lại.

## Trạng thái editor T02B

- Tải/lỗi danh sách ứng dụng có thông báo Việt và Thử lại; không sửa criteria
  đang nhập. Môi trường chỉ lấy từ ứng dụng tương ứng; response đến muộn hoặc
  refresh không ghi đè edits/lựa chọn mới.
- Criteria nâng cao nhiều dòng hoặc chứa field không biểu diễn được trong chế
  độ đơn giản được giữ lossless; không âm thầm bỏ field khi chuyển chế độ.
- Copy chỉ áp dụng khi người dùng thao tác tường minh, clone các dòng; thay
  đổi nguồn hoặc tải lại catalog không cập nhật bản đã sao chép.
- Context xem trước chưa đủ có trạng thái cần thêm context. Cảnh báo chồng
  lấn chỉ nêu nguy cơ; không coi trợ giúp frontend là matching authoritative.
- Submit lỗi giữ chế độ, criteria và metadata; submit thành công reset wildcard.

## Trạng thái Mẫu dựng ứng dụng (T03)

- Loading/empty/error/ready của list phân biệt rõ; lỗi có Thử lại, không giả empty.
- ID rỗng/sai shape hoặc chưa chọn bundle hợp lệ có validation Việt trước POST.
- Saving khóa form đã gửi. Duplicate, bundle không hợp lệ/không có sẵn hoặc lỗi
  server giữ ID, bundle và criteria; thông báo Việt an toàn, không lộ raw error.
- Created reset form/criteria và báo ID đã đăng ký; reload lỗi giữ thông báo đã
  lưu, Thử lại chỉ tải list, không gửi lại POST.
- Editor dùng các trạng thái T02B; loading/late replies không ghi đè edits.
- Developer không thấy navigation và không render form khi mở deep link.

## Điều hướng từ Loại tài nguyên (T04)

CTA từ UC-02 mở `/ui/platform/resource-definitions?resourceType=<key>`.
Chỉ chọn sẵn type khi catalog tải thành công xác nhận key có cặp hỗ trợ trong metadata T31; query thiếu/không hỗ trợ không tạo lựa chọn hợp lệ. Prefill không
gửi POST hoặc ghi đè dữ liệu người dùng khi reload/response đến muộn.

## Bundle selector (T18)

Loading/error/empty của bundle list tách khỏi list mẫu; khóa submit cho đến khi
có lựa chọn AVAILABLE trong list tải thành công. Lỗi có Thử lại, giữ form;
refresh/late replies không đổi lựa chọn đã sửa. Lựa chọn đã mất availability
được giữ để giải thích nhưng không submit. Sau Created, draft mới áp dụng lại
quy tắc một bundle tự chọn/nhiều bundle chọn tường minh. POST unavailable giữ
ID/bundle/criteria và thông báo Việt; retry GET không tự replay POST.

## Matching preview states (T19)

Idle/incomplete/loading/result/transport-error tách biệt. Input candidate (kể cả
ID, JSON, bundle, Connection, criteria), context, scope/session đổi thì invalidate
result và pending reply; refresh context metadata không khiến result cũ còn đúng.
Không xóa edits khi lỗi/retry; không cho late request A thay result của request B.
Status AMBIGUOUS chỉ từ winning ties server; conflict không có fallback candidate.
Snapshot marker/version hiện cùng lời nhắc kiểm lại ở registration/planner.
INCOMPLETE ghi ứng viên đủ eligibility là chưa đánh giá do thiếu context, không
diễn giải matched=false thành không khớp; SYSTEM_BINDING ghi không xét criteria
của authored Definitions vì node dùng binding hệ thống.

## Lỗi theo code — T20

Áp dụng [public HTTP error contract](../../../architecture/contracts/operation-contracts.md#public-http-error-contract--t20--vi-03)
và [shared error presentation](../../../architecture/ui/README.md#trình-bày-lỗi-theo-code--t20--vi-03).
Known code chọn hướng dẫn Việt cụ thể; unknown/missing code dùng fallback Việt
an toàn, không hiển thị/parse raw `error`, submitted values hoặc provider cause.
Status/scoping và guards hiện hành giữ nguyên. Giữ draft/confirmation theo flow;
BUSY không mất edits, stale cần reload/review hoặc Preview mới. Retry tường minh
không tự replay mutation; lỗi tải lại không phủ nhận kết quả đã commit.

## Capability/typed states (T31)

Metadata loading/error khóa typed registration và CTA, phân biệt với không có
cặp supported; có Thử lại, giữ draft. Late replies/scope changes không ghi đè
variables/provision hoặc chọn cặp cũ. Default/param omitted, preset và override
phân biệt; type/placeholder/null errors có hướng dẫn Việt, không echo credentials.
Advanced round-trip giữ typed literals/collections/flags/refs; không chuyển sang
mode làm mất fields. Submit/metadata failures và collapse giữ edits; Created
reset đúng submitted draft. Retry metadata không replay POST/Preview.


## Metadata và tiến trình tài nguyên

Module/class metadata đang tải, lỗi hoặc unavailable chặn thao tác chưa kiểm
chứng; retry giữ edits, không thay bằng default hay replay mutation. Known
input/provider/integrity error dùng code/field an toàn. BUSY giữ dữ liệu đang
sửa; stale yêu cầu review/Preview mới. Progress lấy persisted stage/attempt,
không raw CLI/state/secret; interruption không được coi là cleanup thành công.

## V02 replacements

Apply UC-03 V02-01..09: loading/error/empty capabilities and account lists are
distinct; no invented default driver/account. Account/type/region errors keep
draft. Edit starts from loaded version; ID/type immutable; stale409 preserves
local draft and requires explicit reload/review. Unknown mutation outcome does
not auto-resubmit. Saved/reload-error remains saved and Retry only GETs catalog.
Matching results/pending replies invalidate on Name/driver/account/region/inputs/
criteria/provision/context/session/metadata change; no client-side winner.


## V03 UI/API replacement

[ADR-016](../../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) defines current internal placement. Remove deployment Connection selection after replacement pass; keep Secret Store/configuration/version/error/operation state. Definition authoring supports existing-cluster account choice and workload namespace/cluster references, Type params and provision flags; consumers inherit account. Score import/edit/export preserves metadata.class. Preview shows each workload/resource Definition/account/cluster/namespace and provider-first graph/batches without secrets, and blocks missing/ambiguous/mismatched/unready inputs as appropriate. No Environment target is required. Retired deployment-Connection writes fail explicitly; safe matching and stale-token gates remain. UI multi-cluster V03 unsupported error occurs before execution. Progress/readiness/errors/update/remove use per-workload targets; failure never shown as success.


## V04–V06 current target/dependency behavior

[ADR-017](../../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces remaining profile/Environment-target requirements above for the
replacement path.

Definition/account/region/reference edits invalidate prior Preview/pending
results. Unresolved typed EKS outputs are a valid planned state; consuming node
waits for successful producing provision and API/auth/network/node readiness.
Missing/type/dependency/auth/network failure stays a source error and blocks
consumer; no fallback account/cluster or false success. Maintain loading/error/
retry/stale/BUSY and secret-redaction behavior.


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

## Resource Definition v3 interaction states — issue #8

- Tab switch: giữ mounted panels và toàn bộ draft/version, không mutate API.
- Advanced/copy/matching/help: đóng mặc định; mở/đóng không reset editor.
- New condition: chọn thuộc tính chưa có trong trường hợp, giá trị rỗng là
  incomplete; chặn submit tới khi nhập hoặc xóa điều kiện tường minh.
- No conditions in a case: wildcard; no cases: invalid empty criteria.
- JSON invalid/unrepresentable: giữ text và mode, hiện hướng dẫn, không discard.
- Hidden matching: thay draft/context/metadata/session vẫn invalidate và bỏ
  late response; mở lại chỉ hiện kết quả còn thuộc đúng signature hiện tại.
- Save validation: thông báo lỗi ngoài tab, mở vùng liên quan để sửa; CAS/
  unknown-outcome giữ explicit reload/review hiện hành, không replay mutation.

### V3 catalog/detail navigation

Catalog là view mặc định; Tạo cấu hình hoặc Sửa mở focused editor. Breadcrumb
quay lại catalog giữ mounted draft, pin và review hold; mở lại bản nháp tạo mới
không mất JSON đang sửa. Tạo mới khi đang edit hoặc Hủy sửa là reset tường minh
như trước. Save bar bị khóa theo cùng saving/review state; không cho chuyển view
trong write. ID/Name errors mở tab Cấu hình; actionable validation không bị mất
trong disclosure. Details/help chỉ ẩn lời giải thích, không nới validation.

Tạo mới, reset và sau đăng ký thành công mặc định ở Git; chưa inspect thì
không chọn pin đóng gói và không Save. Module đã đăng ký cần chuyển mode
tường minh. Edit giữ pin đã lưu, không tự chuyển sang Git.

## Resource Definition module forms states — issue #11

- Kết nối: account loading/error/empty/stale (không còn READY/đúng loại) giữ
  lựa chọn và có Thử lại; vùng không literal chặn Save; JSON nâng cao không
  biểu diễn được làm vùng kết nối chỉ đọc, không ghi đè JSON.
- Nguồn giá trị: Hủy giữ nguyên; Áp dụng thiếu trường (Loại/class/phạm vi/
  output/giá trị sai kiểu) bị khóa kèm lý do; chọn “mặc định/param” bỏ ghi đè.
  Reference mà Type/output không còn trong metadata hiện là nguồn nâng cao, giữ
  chuỗi, không âm thầm xóa hoặc gửi giá trị cũ khác.
- Đổi module/metadata: draft theo pin được giữ; thông số không còn trong module
  chặn Save với hướng dẫn thay vì bị bỏ.
- Companion: descriptor hoặc params không biểu diễn được hiện chỉ ở “Cấu hình kỹ
  thuật” với cảnh báo; chuyển Form/JSON/tab không mất dữ liệu.

## Unified cluster authoring — 2026-10-21

UI3-05: Switching cluster methods retains each driver draft. Existing-cluster shares catalog/detail navigation, criteria validation, metadata/account loading and error/retry states, CAS stale/unknown-outcome holds and explicit reload with Terraform. ID/Type stay fixed during Edit.

## Git bundle states — ADR-021

| State | Required behavior |
|---|---|
| New/uninspected | Git fields editable; inspect enabled when valid; Save blocked |
| Inspecting | Show progress; no duplicate inspect; source edits invalidate request signature |
| Verified | Pin/name/schema and typed inputs shown; Save only with valid full Definition |
| Source changed | Previous result stale, retain drafts per pin; discard late response |
| Invalid source/schema/input | Safe field error, preserve draft; no implicit fallback |
| Fetch/cache/runtime unavailable | Explicit retry/reinspect; distinguish empty registry from error |
| Existing verified edit | Saved immutable pin/inputs restored without fetch; legacy still selectable |
| CAS conflict/uncertain save | Existing reload/review flow, no automatic replay |
| Saved | Refresh catalog; new create/reset draft defaults to Git |

Developer cannot inspect/edit through UI or deep link; existing session-expired
behavior applies. Unknown backend errors use localized fallback, no raw template
or Git stderr. Full error codes follow RB-C07.
