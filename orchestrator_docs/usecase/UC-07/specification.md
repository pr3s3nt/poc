---
id: UC-07-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-24
---

# UC-07 — Update or Remove Workload

## Mục tiêu

Cập nhật hoặc xóa một workload trong Environment mà vẫn giữ nguyên các workload và resource không bị ảnh hưởng.

## Primary actors

- Developer
- CI/CD System
- Platform Engineer/Admin: chỉ luồng Xóa tài nguyên riêng.

## Supporting actors

- Resource Driver
- Kubernetes Cluster/Operator

## Tiền điều kiện

- **PRE-01:** Application, Environment và workload đã tồn tại.
- **PRE-02:** Score `before` khớp workload trong current Deployment Set.
- **PRE-03:** Environment đã được kết nối với deployment target.
- **PRE-04:** Active Resource state của deployment trước có thể truy vấn được.

## Trigger

**TRG-01:** Developer hoặc CI/CD gửi Score mới để cập nhật workload, hoặc yêu cầu xóa workload hiện có.

## Main success scenario

1. **MS-01:** Orchestrator đọc current Deployment Set và cấu hình workload trước/sau thay đổi.
2. **MS-02:** Orchestrator xác nhận trạng thái `before` khớp contribution hiện tại của workload.
3. **MS-03:** Orchestrator tạo Humanitec-shaped Delta chỉ thay đổi contribution của workload mục tiêu và dựng Candidate Deployment Set.
4. **MS-04:** Orchestrator dựng lại desired Resource Graph và phân loại Active Resources.
5. **MS-05:** UC-07 `«include»` UC-08 để provision/reconcile resource còn được yêu cầu theo dependency order.
6. **MS-06:** Orchestrator cập nhật hoặc xóa Kubernetes workload manifests theo VAR-01/VAR-02.
7. **MS-07:** Orchestrator đánh dấu resource không còn được graph tham chiếu là `unreferenced`.
8. **MS-08:** Orchestrator lưu Candidate Deployment Set thành current và deployment status `SUCCEEDED`.

## Luồng biến thể trong happy path

- **VAR-01 — Update:** `after Score` tồn tại; workload manifests được render/apply lại từ Candidate Deployment Set.
- **VAR-02 — Remove:** `after Score` là `null`; workload manifests bị xóa và resource chỉ được đánh dấu `unreferenced`, không tự destroy.
- **VAR-03 — Fleet GitRepo:** khi platform dùng GitOps workload delivery,
  Update ghi lại directory của workload và chờ Fleet Ready; Remove xóa đúng
  directory đó rồi chờ Fleet gỡ Deployment/Service. UC-08 resources không bị
  xóa theo directory này.

## Hậu điều kiện

- **POST-01:** Workload được cập nhật hoặc không còn trong Environment.
- **POST-02:** Các workload khác không thay đổi.
- **POST-03:** Shared resource vẫn còn nếu vẫn được workload khác sử dụng.
- **POST-04:** Resource không còn được tham chiếu ở trạng thái `unreferenced` và chưa bị destroy.
- **POST-05:** Candidate Deployment Set trở thành current Deployment Set và deployment có trạng thái `SUCCEEDED`.

## Quy tắc nghiệp vụ

- **BR-01:** `before Score` phải deep-equal contribution hiện tại của workload trước khi tạo Delta, gồm cả module contribution lẫn từng shared entry mà `before Score` khai báo.
- **BR-02:** Delta không được thay đổi module/shared contribution ngoài phạm vi workload, trừ shared entry do chính workload thêm/bỏ và không xung đột consumer khác.
- **BR-03:** Bỏ dependency hoặc Workload không tự xóa hạ tầng/dữ liệu.
  Chỉ đánh dấu UNREFERENCED khi không còn consumer trong successful final
  transaction. Xóa tài nguyên cần thao tác riêng theo V2-03.
- **BR-04:** Resource có cùng descriptor được xem xét tái sử dụng qua UC-08.
- **BR-05:** Nếu workload khai báo một shared ID đã tồn tại với type/class/params khác, và chính workload đó chưa khai báo shared ID này trong `before Score`, planning phải từ chối vì xung đột thay vì ghi đè.
- **BR-06:** Khi workload thôi khai báo một shared resource, entry chỉ rời Deployment Set nếu không còn module nào tham chiếu nó. Đây là phần mở rộng so với planner challenge, nơi entry bị xóa ngay theo khai báo của workload.
- **BR-07:** Add/remove/update module phải nằm lần lượt trong `modules.add`, `modules.remove`, `modules.update`; update patch relative với module và shared patch relative với shared object, gồm array diff deterministic theo UC-05 BR-05/BR-06.
- **BR-08:** Candidate Environment không được giữ Service reference tới
  workload/cổng đã bị loại. Pending batch kiểm tra trạng thái cuối cùng, cho
  phép xóa consumer và provider cùng batch; không từ chối chỉ vì trạng thái
  trung gian. Quy tắc này hiện thực yêu cầu Service validity của UC-16/OC-17.

## Console/API delivery boundary

Console dùng luồng UC-16 đang có: Edit và Save pending draft; Delete có xác
nhận nêu Workload/Environment và có Undo; Preview changes rồi người dùng bấm
Deploy. Save/Delete/Undo không thay đổi runtime. Không thêm một luồng Deploy
ngầm hoặc một API public update/remove song song chỉ vì tên operation thiết kế.
Application service vẫn thực hiện semantics `UpdateWorkload`/`RemoveWorkload`
qua orchestration chung của UC-06. Before lấy từ current deployed contribution,
không dùng pending draft làm before.

Xem [screens](ui/screens.md), [states](ui/states.md) và
[HTTP mapping](ui/api-mapping.md). Những artifact này bổ sung mapping delivery,
không mở scope sang rollback/recovery/concurrent workload updates.

## Luồng nội bộ

```text
UC-07 Update or Remove Workload
├── Verify current workload state
├── Build scoped Deployment Delta
├── Rebuild desired Resource Graph
├── Reconcile required resources
├── Update Kubernetes workload
├── Classify unreferenced resources
└── Persist new Deployment Set
```

## Ngoài phạm vi happy path

- **OOS-01:** Concurrent workload updates.
- **OOS-02:** Automatic rollback.
- **OOS-03:** Scheduled deletion và deprovision ngầm theo Remove.
  Xóa tài nguyên có xác nhận riêng thuộc luồng DEL dưới đây.
- **OOS-04:** Recovery khi update Kubernetes thất bại.
- **OOS-05:** Canary, blue/green hoặc progressive delivery.

## Internal applied-target restoration

Update/remove/retry restores each workload's persisted applied target and
namespace under [ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md).
Catalog edits cannot redirect deletion. Replacement target changes still require
UC-05 V2-04 explicit operation or rejection; no automatic migration/rebinding.
Old-data migration is not required in V03. Ownership/current consumers and
original provider state guard separately confirmed resource cleanup.

## T30 pending rename boundary

UC-12 explicit rename opt-in chỉ sửa desired references nguyên tử với revision;
Preview đọc drafts mới, liệt kê workloads bị ảnh hưởng và pin config/draft/current
set versions. Deploy vẫn tường minh; token trước rename bị stale. Rename-only
hoặc delete có thể để reference thiếu và Preview phải chặn Deploy. Không đổi
runtime/current set hoặc Environment khác khi lưu.


## Ownership và Xóa tài nguyên

- **V2-02:** Redis/PostgreSQL private thuộc Workload; shared thuộc Environment/
  target generation. Shared còn consumer được giữ; type/class/params conflict
  bị chặn. Platform resource không thuộc quyền xóa của Environment.
- **V2-03:** PE/Admin có nút Xóa tài nguyên tại trang chi tiết tài nguyên trong
  Environment. Root phải UNREFERENCED, không còn consumer và không có
  Environment operation đang chạy. Developer không có quyền, kể cả gọi API.
  Hộp thoại cảnh báo mất dữ liệu, yêu cầu gõ tên root và liệt kê đúng
  các tài nguyên người dùng đã tick. Backend kiểm lại snapshot/ownership/consumer/
  dependency trong giao dịch nhận operation; không xóa ngoài tập đã xác nhận.
- **V2-04:** Failed provision không cấp quyền xóa toàn bộ dependency. Cleanup
  tự động của run chỉ xóa CREATED_BY_RUN được giao; PREEXISTING/UPDATED_BY_RUN
  được giữ. Xóa do PE/Admin xác nhận dựa trên ownership bền vững, dùng target/
  module/state gốc. Không suy quyền xóa từ run ID, UNREFERENCED hay tiền tố tên.

### Luồng riêng: Xóa tài nguyên

- **DEL-PRE-01:** PE/Admin có quyền trong Organization/Environment; root do
  Orchestrator quản lý, UNREFERENCED và không thuộc platform.
- **DEL-PRE-02:** Không có operation đang chạy; state/module/credential gốc
  sẵn sàng. Không consumer trong current graph hoặc pending desired graph.
  Thiếu bằng chứng là chưa đủ điều kiện, không đoán quyền.

1. **DEL-MS-01:** Người dùng mở resource detail và chọn Xóa tài nguyên.
2. **DEL-MS-02:** Hệ thống hiển thị root và các dependency trong cùng
   Environment/generation. Chỉ root được tick mặc định; dependency để trống.
   Không hiển thị tài nguyên platform như k8s-4f hoặc Connection. Chưa gọi
   provider hoặc nhận khóa dài.
3. **DEL-MS-03:** Người dùng tự tick dependency muốn xóa thêm. Ô không đủ
   điều kiện bị khóa và nêu lý do. Hộp xác nhận liệt kê đúng các mục đã tick,
   cảnh báo mất dữ liệu vĩnh viễn; người dùng gõ đúng tên root để xác nhận.
4. **DEL-MS-04:** Backend kiểm toàn bộ tập đã tick: token/versions, quyền,
   UNREFERENCED, ownership, consumer và operation trống trong một giao dịch, nhận operation rồi ghi REQUESTED.
   Có thay đổi thì chặn trước destroy và yêu cầu review phạm vi mới.
5. **DEL-MS-05:** CHECKING kiểm ownership/state/target; DESTROYING khóa state
   và xóa consumer trước provider theo tập đã xác nhận.
6. **DEL-MS-06:** VERIFYING kiểm tài nguyên vật lý đã hết. Persist kết quả
   từng instance, giữ lịch sử/state cuối; không xóa history của Deployment.
7. **DEL-MS-07:** UI hiển thị thành công đã xác minh hoặc lỗi/ID còn lại.
   Operation chưa sạch không được báo thành công; tiến trình chưa rõ thì giữ hold.

- **DEL-BR-01:** Chỉ xóa tập người dùng đã tick, bắt buộc có root. Mọi mục
  phải cùng Environment/generation, thuộc quyền sở hữu, UNREFERENCED và không
  thuộc platform. Chỉ cho tick dependency khi mọi consumer còn tồn tại nằm
  trong tập đã tick; kể cả Workload, current graph và pending desired graph.
  Khi bỏ tick một mục, tự bỏ tick các mục đang chọn trở nên không hợp lệ
  vì có consumer ngoài tập; lặp đến khi tập hợp lệ và giải thích các thay đổi.
  Không tự tick thêm bất kỳ mục nào. Backend kiểm lại toàn bộ tập trong giao
  dịch nhận operation; tập không hợp lệ bị từ chối toàn bộ, không xóa một phần
  hoặc tự thêm dependency. Các mục không tick phải được giữ nguyên.
- **DEL-BR-02:** Tên xác nhận do server lấy từ persisted descriptor, cũng là
  tên kỹ thuật hiển thị trên resource detail. So khớp chính xác; không trim/
  case-fold hoặc tin tên do client chọn. Xác nhận cũ không dùng lại sau scope/
  version thay đổi. Request lặp cùng token không tạo operation thứ hai.
- **DEL-BR-03:** Tập xóa và versions được pin; operation claim/locks kiểm owner/
  fencing. Không có transaction DB kéo dài qua provider call. Consumer xóa
  lỗi thì không xóa provider còn cần; chỉ tiếp tục nhánh độc lập an toàn.
  Lỗi không rollback dữ liệu và không tự gửi lại mutation từ browser.
- **DEL-POST-01:** Chỉ khi tất cả tài nguyên đã xác nhận không còn, cleanup
  SUCCEEDED và resource chuyển DESTROYED. Dữ liệu bị xóa không được coi là
  có thể Undo. Tài nguyên giữ lại, current Set và history không bị sửa ngoài ý định.


## V03 target restoration

[ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) replaces the internal ADR-013 compatibility section. Update/remove/retry uses each persisted applied workload target/namespace/identity; catalog edits cannot redirect deletion. Remove marks unused resources UNREFERENCED; separate confirmed cleanup removes only owned namespace/resources and preserves PREEXISTING cluster. A class/target change requiring replacement is blocked under UC-05 V2-04, never silently retargeted.

## Review diff and update/remove continuity

- **REV-01:** Update/remove Preview dùng UC-05 VIS-*; baseline chỉ từ applied
  snapshot thành công cùng identity/generation, không latest failed attempt.
  Gỡ workload không diễn giải unreferenced resource thành destroy. Đổi module/
  class/target vẫn giữ replacement guard và không tự chuyển dữ liệu.
- **REV-02:** Pending execution giữ `review_request_id` và artifact/status capture
  như UC-06 REV-*. Retry cần Preview mới; không replay deploy khi poll lỗi.

## Git bundle updates and removal — ADR-021

- **RB-01:** Updating only a rendering Definition's Git pin or typed inputs produces
  a workload update without a Score draft. Use the same application/environment,
  original resource identities and Terraform reuse rules. Re-select an older pin
  through explicit edit + Preview + Deploy; never rewrite applied history or
  execute an old mutable Git ref.
- **RB-02:** Removal continues to use original applied target/identity/ownership,
  independent of the current Definition or Git reachability. No new object kinds
  or Terraform cleanup behavior. Cache/history retain prior pins; backend restart
  must not lose the applied manifest before-side. "No new object kinds" is
  superseded by RB-03 (ADR-024).
- **RB-03:** Removing a workload deletes every object of its persisted
  inventory (ownership and UID verified), then the fixed workload objects;
  objects of other workloads or pre-existing objects are never deleted.

## Secret delivery objects on workload removal — user decision 2026-10-24

Issue #14 acceptance showed that a removed workload left its VSO delivery
objects, so the namespace failed confirmed cleanup (unaccounted objects). The
user chose to extend removal for this case.

- **VSO-01:** Removing a workload (VAR-02) also removes the VSO objects the
  platform created for that workload: VaultStaticSecrets below the Application
  Environment's own Vault value prefix whose VaultAuth binds the workload's
  deterministic delivery ServiceAccount, their destination Secrets and
  VaultAuths, that ServiceAccount once no remaining VaultAuth uses it, and a
  VaultConnection no remaining VaultAuth references. Objects of other workloads,
  Applications or stores are kept. Deletion is by exact name from those
  references, never by label selector, prefix or bulk delete.
- **VSO-02:** A failed or unknown delivery-object removal keeps the instance
  FAILED like a failed workload delete; retry is safe and idempotent. Vault KV
  bundles, policies and roles are unchanged (out of scope). Legacy Vault Agent
  delivery keeps its existing behavior.
