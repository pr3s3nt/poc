---
id: UC-18-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-28
related: [ADR-025, UC-01, UC-07, UC-08, UC-12]
---

# UC-18 — Xóa Application

## Mục tiêu

Người dùng xóa hẳn một Application: gỡ mọi workload, destroy mọi tài nguyên do
Orchestrator sở hữu ở cả staging và production, xóa secret trong Vault của
Application, rồi đánh dấu Application `DELETED`. Quyết định thiết kế ở
[ADR-025](../../architecture/decisions/ADR-025-application-deletion.md).

## Actors

- **Primary:** Developer, Platform Engineer, Admin của Organization.
- **Supporting:** Resource Driver (Terraform), Kubernetes Cluster, Secret Store (Vault).

## Tiền điều kiện

- **PRE-01:** Người dùng đã đăng nhập (UC-00) với role ADMIN, PLATFORM_ENGINEER
  hoặc DEVELOPER trong Organization sở hữu Application.
- **PRE-02:** Application ở trạng thái `ACTIVE`, hoặc `DELETING` khi Retry.
- **PRE-03:** Không Environment nào của Application có operation đang chạy.
- **PRE-04:** Mọi instance owned có ownership bền vững, state/module/credential
  gốc sẵn sàng, không ở `INTERRUPTED`/`NEEDS_ATTENTION`. Thiếu bằng chứng là
  chưa đủ điều kiện, không đoán quyền.

## Trigger

**TRG-01:** Người dùng chọn **Xóa ứng dụng** trong vùng nguy hiểm của trang
Cài đặt Application.

## Main success scenario

1. **MS-01:** Hệ thống tạo deletion preview chỉ đọc cho cả hai Environment:
   workload đã áp dụng, draft đang chờ, instance owned theo mọi generation và
   dependency, Vault refs, public routes. Liệt kê riêng phần **giữ lại**:
   PREEXISTING cluster, Connection, Secret Store, Definition. Không gọi provider.
2. **MS-02:** Preview trả blockers (PRE-03/PRE-04) và một token pin version
   Application, hai Environment, current Set, draft, configuration và từng instance.
3. **MS-03:** UI hiển thị phạm vi theo Environment, cảnh báo xóa vĩnh viễn dữ
   liệu, không thể hoàn tác. Người dùng gõ đúng tên Application để xác nhận.
4. **MS-04:** Backend kiểm role, token, versions, tên xác nhận và blockers trên
   một lần đọc mới; một compare-and-set theo version Application chuyển sang
   `DELETING`, ghi tiến trình xóa và trả `202` cùng operation ID. Mỗi stage sau
   đó nhận claim Environment qua operation hiện có của nó (DEPLOY, RESOURCE_CLEANUP).
5. **MS-05:** Với mỗi Environment, `REMOVING_WORKLOADS`: áp dụng Deployment Set
   rỗng theo UC-07 VAR-02 (inventory RB-03, VSO-01, public route), ghi một
   Deployment REMOVE vào lịch sử, bỏ draft đang chờ.
6. **MS-06:** `DESTROYING_RESOURCES`: destroy mọi instance trong confirmed scope
   theo thứ tự ngược dependency, dùng target/module/state gốc, owner/fence như
   luồng DEL của UC-07.
7. **MS-07:** `VERIFYING`: kiểm từng tài nguyên vật lý và workload object đã
   hết; persist kết quả từng instance; instance chuyển `DESTROYED`.
8. **MS-08:** `PURGING_SECRETS`: xóa Vault values, delivery bundles và output
   secrets mà Application tham chiếu, theo đúng ref; xác nhận đã xóa.
9. **MS-09:** Khi cả hai Environment `SUCCEEDED`, Application chuyển `DELETED`,
   name/subdomain được giải phóng, Application biến khỏi danh sách.

## Biến thể

- **VAR-01 — Environment trống:** Environment chưa từng deploy bỏ qua MS-05..07
  và chỉ chạy MS-08 nếu có ref.
- **VAR-02 — Retry:** Application `DELETING` với Environment `FAILED`. Preview
  mới chỉ liệt kê phần còn lại; Retry tiếp tục từ stage đã persist, không làm lại
  bước đã xác minh, cần xác nhận tên lần nữa.
- **VAR-03 — Gián đoạn:** process chết giữa chừng thì operation `INTERRUPTED`;
  khôi phục qua recover hiện hành (đổi owner/fence) rồi Retry.

## Hậu điều kiện

- **POST-01:** Không còn workload object, namespace hay tài nguyên owned nào của
  Application trên cluster/cloud; mỗi instance có bằng chứng absence.
- **POST-02:** Vault không còn value/bundle/output secret mà Application tham chiếu.
- **POST-03:** Application `DELETED`, không xuất hiện trong list; scoped read trả 404.
- **POST-04:** Lịch sử Deployment, attempts, terminal state snapshot, kết quả
  cleanup và audit vẫn được giữ trong DB.
- **POST-05:** Tài nguyên platform/PREEXISTING và Application khác không đổi.

## Quy tắc nghiệp vụ

- **BR-01:** Confirmed scope là toàn bộ instance owned của hai Environment tại
  thời điểm admission. Backend tự tính scope; client không chọn hay bỏ mục.
- **BR-02:** Tên xác nhận là `applications.name` từ server; so khớp chính xác,
  không trim/case-fold. Token cũ bị từ chối khi bất kỳ version pin đổi.
  Request lặp cùng token không tạo operation thứ hai.
- **BR-03:** Admission là một compare-and-set; token cũ hoặc Environment đang
  có operation thì không ghi gì. Mỗi stage dùng claim owner/fence của operation
  hiện có. Không giữ DB transaction qua network/provider call.
- **BR-04:** Từ `DELETING`, mọi mutation trên Application/Environment (draft,
  configuration, Settings, Preview, Deploy, Xóa tài nguyên, đổi Secret Store)
  trả 409 `APPLICATION_DELETING`. Đọc vẫn được phép cho tới `DELETED`.
- **BR-05:** Workload removal phải xong và xác minh trước khi destroy resource của
  cùng Environment. Consumer destroy lỗi thì không destroy provider còn cần.
- **BR-06:** Chỉ xóa object/instance có ownership bền vững; PREEXISTING và
  platform objects luôn được giữ. Không suy quyền từ label, prefix, run ID.
- **BR-07:** Vault chỉ xóa theo ref bất biến: Secret value ref (store identity +
  path) của mọi configuration revision và delivery bundle path của mọi Deployment
  của Environment. Không xóa theo prefix hoặc liệt kê mount. Store không truy
  cập được thì stage `FAILED`, không coi là đã xóa.
- **BR-08:** Không rollback, không undo. Lỗi để Application `DELETING`, hiện
  stage và ID còn sót; chỉ báo thành công sau MS-07 và MS-08 xác minh.
- **BR-09:** Name/subdomain chỉ được dùng lại khi `DELETED`. Application mới
  luôn có ID, Environment ID, state key và namespace identity mới.
- **BR-10:** Thu telemetry dừng tại admission. Dữ liệu telemetry đã lưu không
  hiển thị và hết hạn theo retention của ADR-023.
- **BR-11:** Definitions, matching criteria có `app_id`, effectiveness baselines
  và diagnostic reports tham chiếu Application được giữ; preview cảnh báo các
  matching criteria sẽ không còn khớp.

## Ngoài phạm vi

- **OOS-01:** Khôi phục Application hoặc dữ liệu đã xóa.
- **OOS-02:** Xóa một Environment riêng lẻ.
- **OOS-03:** Hủy operation sau admission.
- **OOS-04:** Purge vĩnh viễn tombstone và history (controlled history purge).
- **OOS-05:** Dọn tài nguyên ngoài Terraform state hoặc không rõ ownership (D13).
- **OOS-06:** Xóa Application qua CLI/pipeline (UC-13).

## Delivery state

Implemented; trạng thái và evidence ở [CURRENT_STATE](../../CURRENT_STATE.md).
Giới hạn còn lại ở [IMP-038](../../implementation/deviations.md#imp-038--application-deletion-2026-10-27).

## Scheduled workloads — integration with ADR-028

- BR-13: Before Application deletion admission, every applied CronJob must already
  be explicitly suspended through Save → Preview → Deploy. Deletion preview uses
  the applied Deployment Set, never a pending suspend draft; it reports
  `SCHEDULE_NOT_SUSPENDED` without a token. Otherwise the Application freeze would
  prevent the user from satisfying the CronJob removal precondition afterward.
- BR-14: Application deletion is an explicit confirmed data-deletion operation:
  its preview includes all owned storage and its permanent-data warning. Ordinary
  workload removal still retains storage and ordinary namespace deletion still
  refuses retained data. Full Application cleanup destroys owned claims before
  their namespace using the original dependency/UID/state guards; it does not
  bypass ownership or remove foreign storage.
- BR-15: Suspension does not cancel active Jobs. UID-owned active Jobs still block
  runtime removal; the deletion stays incomplete and can be retried after they
  finish. No implicit cancellation or manual Job trigger is added.

## CD-aware Application deletion — ADR-030

- **CD-01:** Preview includes applied delivery bindings/units, retained infrastructure
  and owned provider metadata. Desired toggle does not replace applied authority.
- **CD-02:** REMOVING_WORKLOADS withdraws the actual CD controller and verifies
  UID-safe workload/route/control-metadata absence before Terraform namespace
  cleanup. Interrupted transition/publish/withdrawal is a blocker, not a reason
  to bypass guards. Preinstalled Fleet/registered clusters/credentials are kept.
- **CD-03:** Retry restores original receipts and checkpoints; provider unavailable
  leaves DELETING with remaining IDs. Do not destroy namespace while Fleet can
  recreate objects, or mark application deleted on Git path removal alone.
