---
id: UC-05-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-28
---

# UC-05 — Validate and Preview Score Changes

## Mục tiêu

Kiểm tra Score và hiển thị thay đổi dự kiến trước khi thực hiện deployment.

## Primary actors

- Developer
- CI/CD System

## Tiền điều kiện

- **PRE-01:** Application và Environment đã tồn tại.
- **PRE-02:** Environment có Deployment Set hiện tại, kể cả Deployment Set rỗng.
- **PRE-03:** Các Resource Type và Resource Definition cần thiết đã được đăng ký.
- **PRE-04:** Internal target được resolve từ matched Definitions và READY Driver Account, không cần Environment Connection/profile (ADR-016).

## Trigger

**TRG-01:** Developer hoặc CI/CD gửi Score và yêu cầu validate/preview trên một Environment.

## Main success scenario

1. **MS-01:** Orchestrator đọc Score và Deployment Set hiện tại.
2. **MS-02:** Orchestrator validate Score theo Score contract và Resource Type schemas, gồm `containers.*.resources.requests/limits` khi được khai báo.
3. **MS-03:** Orchestrator chuyển Score thành workload fragment và giữ nguyên container CPU/memory requests/limits trong module spec.
4. **MS-04:** Orchestrator tạo Humanitec-shaped Deployment Delta và Candidate Deployment Set.
5. **MS-05:** Chọn workload Definition trước, mở namespace/cluster references/provision tới fixed point và match resources; internal không profile/builtin enrichment.
6. **MS-06:** Orchestrator kiểm graph, references, required/typed/bounds inputs
   và module mappings biết được trước execution; lỗi chặn Deploy trước tạo hạ tầng.
7. **MS-07:** Orchestrator tính provision batches và phân loại Active Resources.
8. **MS-08:** Orchestrator trả preview gồm Delta, Candidate Deployment Set, resource changes và provision order mà không thực thi runtime change.

## Hậu điều kiện

- **POST-01:** Không có resource hoặc workload nào được provision và không đổi Deployment Set hiện tại.
- **POST-02:** Preview phản ánh thay đổi dự kiến trên đúng Environment.
- **POST-03:** Preview chứa đủ thông tin planning để đối chiếu với UC-06.

## Quy tắc nghiệp vụ

- **BR-01:** Preview và deploy phải dùng cùng planning pipeline và cùng input snapshot để tránh kết quả khác nhau.
- **BR-02:** Invariant bắt buộc là `current Deployment Set + Delta = Candidate Deployment Set`.
- **BR-03:** Graph edge có chiều `consumer -> provider`; batches luôn đặt provider trước consumer.
- **BR-04:** Preview chỉ đọc, không gọi Resource Executor, Terraform CLI, provider hoặc secret store.
  Module contracts/content lấy từ registry đã kiểm chứng, không tải module.
  Sau khi đóng read snapshot, chỉ được gọi offline preview renderer đã pin
  theo ADR-019; planner không gọi CLI và không persist runtime change.
- **BR-05:** Deployment Delta có shape `modules.add/remove/update` và `shared`; patch trong `modules.update.<id>` relative với module, patch `shared` relative với object shared.
- **BR-06:** Delta deterministic; array được diff theo index chung, remove đuôi từ index lớn xuống nhỏ và add đuôi bằng path `/-`.
- **BR-07:** `containers.*.resources` chỉ nhận `requests`/`limits` với `cpu` và `memory`; mọi field đều optional, nhưng field đã khai báo phải là non-empty string. `null`, number, empty string, branch hoặc resource key khác bị reject. Giá trị hợp lệ phải được bảo toàn nguyên văn từ Score sang Candidate Deployment Set. Validation của planner dừng ở shape này, không kiểm Kubernetes quantity semantics; Kubernetes API kiểm quantity khi workload được apply.
- **BR-08:** Với UC-16 pending changes, Preview bỏ qua draft có Candidate
  Deployment Set tương đương current, renderer không đổi và không dùng key UC-12
  đã đổi revision.
  Nếu key tham chiếu đã đổi revision hoặc Definition/bundle renderer đã đổi
  so với plan lần deploy gần nhất thì vẫn preview update. Preview kiểm tra
  public path trên trạng thái Environment cuối cùng, không từ chối trạng thái
  trung gian của batch chuyển route giữa workload. Khi chỉ còn route sync
  pending, Preview cho retry route-only mà không redeploy workload.

## Standalone Score Preview boundary

Developer có thể mở **Preview Score** từ Application/Environment đang chọn,
nhập Score after và, với update/remove, Score before cùng Workload ID. Đây là
input tạm thời: không Save draft, không tạo Deployment, không có nút Deploy
trong kết quả này. Pending-change Preview của UC-16 vẫn là flow riêng.

Request dùng Score object; UI nhận JSON hoặc YAML và parse trước khi gửi.
Add cần after và không có before; update cần cả hai; remove cần before và
không có after. Workload ID bắt buộc, phải tương ứng Score metadata name.
Run ID là planning context không rỗng, có thể nhập để tái lập phép so sánh;
không phải authorization context hoặc preview token.

Response chứa Application/Environment scope, base Set ID, Environment version,
Run ID, plan hash, Delta, Candidate Set, graph, matches, batches và resource
classification. Snapshot repositories phải được đọc nhất quán trong một
read-only snapshot; không ghép dữ liệu từ các thời điểm khác nhau.

Không resolve configuration secrets hoặc resource outputs khi preview. Chỉ
placeholder và secret reference có thể xuất hiện; secret values, connection
credentials và resolved inputs không được đưa vào response/error/log.
Client-supplied Score không phải API upload secret; UI nhắc không nhập secret
literal và dùng configuration placeholder. Kết quả cũ bị xóa khi sửa input
hoặc đổi scope để tránh hiểu nhầm rằng nó còn phản ánh input mới.

HTTP/UI contract chi tiết ở [UI API mapping](ui/api-mapping.md),
[screens](ui/screens.md) và [states](ui/states.md).

## Luồng nội bộ

```text
UC-05 Validate and Preview
├── Validate Score and schemas
├── Build Delta and Candidate Set
├── Build and validate Resource Graph
├── Match Resource Definitions
├── Calculate provision order
└── Return deployment preview
```

## Ngoài phạm vi happy path

- **OOS-01:** Lưu và quản lý nhiều bản preview.
- **OOS-02:** Approval workflow.
- **OOS-03:** Chi phí dự kiến hoặc policy evaluation.
- **OOS-04:** So sánh preview với runtime state thực tế.
- **OOS-05:** Preview Terraform plan thật.

## Workload renderer selection

- **BR-09:** Chọn đúng một score-k8s Definition/bundle tương thích cho
  mỗi Workload trên cả hai profiles. Không match/tie/bundle thiếu là lỗi.
  Plan hash pin Definition/bundle/binary/provider/module digests, target/
  generation, state identity và versions; thay intent cần Preview mới.
- **BR-10:** Preview hiển thị type/class, Definition/module/revision, target/
  account/region, ownership và create/update/retain intent. Không secret/
  state/credential refs, không khẳng định final manifest hay Terraform plan
  chính xác khi output chưa có; không provision. Ngoại lệ preview-render
  offline/symbolic thuộc VIS-03 và ADR-019, không phải execution render.

See [workload rendering contract](../../architecture/contracts/workload-rendering.md).

## Definition account and target binding

- **BR-11:** All Definition matching follows UC-03 V02-02 without profile
  filtering. AWS pins winning account/region; internal cluster pins winning
  Internal account and consumers pin explicit namespace/cluster references
  under ADR-016. No Environment target is required for internal Preview.
  Account kind/org/READY/version and reference intent join deterministic
  hashes/admission checks; no secret/provider call or mutable target fallback.

## Editable destination and transition consistency (ADR-012)

Preview pins execution/store bindings, target generation, Environment version,
current Set, desired config revision/version and draft version. Deploy validates
this complete snapshot and acquires an Environment operation claim atomically;
stale token or competing operation returns safe 409 before external side effects.
Settings/config/draft writes cannot interleave with admitted execution. No DB
transaction spans network calls. Changing destination forces complete redeploy,
never reuses old-target resource executor state/applied workload identity.
Old resources and historical executions resolve their stored targets for queries,
recovery and cleanup. Explicit PostgreSQL transfer quiesces writers and restores
before destination apps/routes become live. See
[ADR-012](../../architecture/decisions/ADR-012-environment-stores-and-transitions.md).

## Matched internal cluster binding

- **BR-12:** Show matched cluster Definition/account and per-consumer target
  source under ADR-016. Reject missing/ambiguous/mismatched target, cycles and
  V03 multi-cluster Candidate Sets before execution. No system binding/catalog
  admission or write in Preview.

## Ranh giới với matching preview (T19)

UC-03 BR-20–22 chỉ tính matching một context/candidate theo OC-04c; không tạo
Delta, Candidate Set hoặc graph, không thay UC-05 và không phát deploy token.
UC-05/06 vẫn đọc snapshot mới và kiểm toàn bộ planner/contract độc lập.

## T30 pending rename boundary

UC-12 explicit rename opt-in chỉ sửa desired references nguyên tử với revision;
Preview đọc drafts mới, liệt kê workloads bị ảnh hưởng và pin config/draft/current
set versions. Deploy vẫn tường minh; token trước rename bị stale. Rename-only
hoặc delete có thể để reference thiếu và Preview phải chặn Deploy. Không đổi
runtime/current set hoặc Environment khác khi lưu.


## Thay thế tài nguyên

- **V2-04:** Preview phải chỉ ra việc đổi class/module/target cần replacement.
  Deploy thông thường không được delete/replace tài nguyên hiện hữu. Cần
  operation tạo identity/generation mới và xác nhận riêng, hoặc chặn trước
  apply; tài nguyên cũ được giữ, không tự chuyển dữ liệu.


## V03 placement intent

[ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) replaces legacy internal graph enrichment and Environment binding in BR-11. Plan/token include each workload/resource typed cluster and namespace provenance, Definition/account versions and class. UI shows safe resolved or deferred output provenance; Preview never invokes renderer/provider/secrets. Changes invalidate old Preview; pending detection includes target/Definition/reference intent.


## V04–V06 graph and unknown outputs

- **V04-01:** Show graph ordering reasons, provision status and source errors;
  preserve typed/secret output boundaries and full DAG validation under ADR-017.
- **V05-01:** Preview a newly requested cluster as unresolved typed provenance;
  endpoint/credentials/Ready state need not exist. Show per-resource winning
  Definition/account/region and namespace/workload cluster source. Pure Preview
  never invokes provider/auth resolution to replace unknown output values.
- **V06-01:** A managed-resource-only planner graph must not acquire VPC/EKS
  except through authored actual dependencies. This rule does not create a new
  public execution entry without a separately defined contract.

## Visual deployment review — accepted 2026-10-23

- **VIS-01:** Pending changes và standalone Score Preview cung cấp đồ thị thực
  từ cùng planning snapshot. Hiển thị workload/resources, intent tạo/cập nhật/
  giữ nguyên/gỡ tham chiếu, Definition/pins, account/region và vị trí cluster/
  namespace. Internal không tự thêm cloud nodes; managed AWS resources không
  đặt vào EKS chỉ vì cùng môi trường. Placement chưa biết ghi provenance chờ.
- **VIS-02:** Chọn node mở Tổng quan / Thay đổi / Code; safe Terraform source
  đúng verified pin và input-source mappings, không phải Terraform plan thật.
- **VIS-03:** Xem Deployment/Service manifest dự kiến từ đúng installed renderer
  bundle và shared normalization. Unknown output hiện placeholder có nguồn;
  partial/unavailable rõ ràng. Offline preview rendering được phép theo
  [review contract](../../architecture/contracts/deployment-review.md).
- **VIS-04:** Diff source/manifest so sánh baseline thành công cùng identity/
  generation với desired mới; tô đỏ dòng bỏ, xanh dòng thêm, có dấu/số dòng.
  Thiếu baseline không coi là file rỗng; secret bị che không được kết luận bằng
  nhau. Có xem bản sau, copy an toàn và mở rộng code.
- **VIS-05:** Code và graph gắn cùng scope/plan/pins; sửa input/Definition/bundle/
  scope khiến Preview cũ stale theo admission hiện hành. Không hardcode fixture
  prototype vào sản phẩm hoặc dựng resource dựa theo tên hiển thị.
- **VIS-06:** Khả năng xem code không cấp quyền sửa module, apply artifact hay
  bỏ qua Deploy/token. Standalone vẫn không có nút Deploy. Review sử dụng safe
  projection, giới hạn nội dung và fail-closed redaction theo contract chung.
- **VIS-07:** Desktop dùng AppShell, ApplicationHome, stylesheet/component có
  sẵn. Bản mẫu đã duyệt là tham chiếu UI; không có scenario selector/fake timer/
  pause/reset/failure simulation trong luồng sản phẩm.
- **VIS-08:** Sau Deploy từ pending Preview, giữ cùng ngữ cảnh graph và theo dõi
  exact deployments bằng `reviewRequestId` được bind khi admission, theo UC-09 OBS-01..07. Không
  lấy deployment mới nhất làm kết quả của Preview đang xem. Không thêm approval
  workflow hoặc chạy Terraform plan trong Preview.
- **VIS-09:** Mỗi node có tóm tắt thông tin công khai hữu ích theo loại (tên
  database/namespace, port, image/replicas và placement khi biết), cùng thông tin
  đầy đủ hơn ở Tổng quan. Dùng safe projection có nguồn, không suy giá trị bí mật
  hoặc đoán output chưa có. Giãn card/tầng và tách làn đường nối để giữ nội dung
  và dependency dễ đọc; không đổi graph, thứ tự thực thi hoặc runtime state.

## Git rendering intent and Preview — ADR-021

- **RB-01:** Extend BR-09 to pin Git origin/directory/commit/content, engine/binary
  contract and effective render inputs, using scoped verified metadata only.
  Same Score with a changed bundle/input is a pending workload update. Neither
  matching nor pure planning fetches Git or invokes templates/providers.
- **RB-02:** After closing the read snapshot, VIS-03 uses the same pinned cached
  pipeline as execution, with symbolic dependencies. Show source/short commit
  and permitted customization fields in the manifest diff. Preserve unknown and
  redacted paths; never present fabricated values as exact output. An unavailable
  artifact remains visibly unavailable. Preview causes no cluster mutation.
- **RB-03:** Old Preview becomes stale when the selected Definition/bundle/inputs
  change before admission. Same-pin re-inspection/cache repair does not change
  intent. Changing Git branch HEAD alone does not change a saved selection.
  Applied history supplies the before-side across refresh/restart.

Details: [Git contract RB-C06](../../architecture/contracts/git-render-bundles.md#rb-c06--preview-admission-and-history).

## Uploaded ZIP source compatibility — ADR-026

The RB requirements also apply to verified ZIP sources prepared by UC-03
ZIP-01..04. Source preparation/provenance follows
[ADR-026](../../architecture/decisions/ADR-026-zip-source-upload.md);
Preview, rendering, update/reuse and cleanup keep their existing pinned-engine
and cache contracts. No upload or source fetch is introduced into this use case.

## Delivery selection preview — ADR-027

- **CD-01:** Preview pins desired/applied delivery selections, binding versions,
  capabilities, policy, configuration revision and current inventory baseline.
  It remains pure: no Fleet/Git/provider call or Terraform plan. Previously
  verified metadata is read under the same planning snapshot.
- **CD-02:** A delivery-only change produces a Preview action. If ownership must
  move, show every affected workload and Environment route, retained infrastructure,
  source/destination managers and baseline/capability blockers. Do not mix this
  transition with pending workload/config/resource changes.
- **CD-03:** Deploy rechecks those pins plus runtime target identity. Unsupported
  lifecycle kinds or absent credentials cannot silently fall back to direct.
  Drift repair uses the separate UC-08 review, not pure Score Preview.
