---
id: UC-09-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-28
---

# UC-09 — View Deployment Status and Resource Graph

## Mục tiêu

Cho phép Developer xem trạng thái deployment, Resource Graph và thông tin resource của một Environment.

## Primary actor

- Developer

## Supporting actor

- Platform Engineer

## Tiền điều kiện

- **PRE-01:** Application và Environment đã tồn tại.
- **PRE-02:** Có ít nhất một Deployment record.
- **PRE-03:** Actor có quyền xem Environment.

## Trigger

**TRG-01:** Developer mở hoặc truy vấn thông tin một Deployment trong Environment.

## Main success scenario

1. **MS-01:** Developer chọn Application, Environment và Deployment.
2. **MS-02:** Orchestrator đọc Deployment record và Deployment Set tương ứng.
3. **MS-03:** Orchestrator đọc persisted workload/resource status và Active Resources của Deployment.
4. **MS-04:** Orchestrator dựng read model gồm trạng thái tổng thể, thời gian và actor kích hoạt.
5. **MS-05:** Orchestrator thêm Resource Graph, matched Definitions và provision batches vào read model.
6. **MS-06:** Orchestrator lọc secret outputs rồi thêm Active Resources cùng non-secret outputs.
7. **MS-07:** Orchestrator trả deployment view cho Developer.

## Hậu điều kiện

- **POST-01:** Không có trạng thái runtime hoặc persisted state nào bị thay đổi.
- **POST-02:** Developer có đủ thông tin để xác nhận deployment happy path thành công.
- **POST-03:** Thông tin secret không xuất hiện trong response.

## Quy tắc nghiệp vụ

- **BR-01:** UC-09 chỉ đọc snapshot đã persist; không tự gọi provision/apply để refresh runtime.
- **BR-02:** Deployment view phải gắn đúng Application, Environment và Deployment ID.
- **BR-03:** Chỉ output được phân loại non-secret mới được hiển thị.
- **BR-04:** Graph, matched Definition và batches phải là artifact của đúng lần planning đã tạo Deployment đó.
- **BR-05:** Console liệt kê Deployment và detail trong đúng Environment;
  detail gồm graph, module/revision/target, Terraform stage/attempt/status/
  timestamps và failure reason. Poll persisted query khi đang chạy; không
  gọi provider để refresh hoặc thêm raw log/streaming console.
- **BR-06:** Nếu UC-06 thất bại trong planning trước khi tạo Delta Snapshot,
  Deployment view vẫn trả status/failure reason; Delta, graph và batches có thể
  vắng mặt. Không dựng Delta giả hoặc hiển thị plan của lần khác.
- **BR-07:** Lỗi hiển thị safe code/summary đúng node/stage, gồm input/
  permission/transient/interrupted/cleanup. Không raw CLI/state/tfvars/plan
  JSON/credential/secret. History có raw reason cũ được che trong query, không
  viết lại history. FAILED không đồng nghĩa đã cleanup.

## Luồng nội bộ

```text
UC-09 View Deployment Status
├── Load Deployment record
├── Load Deployment Set and Resource Graph
├── Load workload and resource statuses
├── Filter secret information
└── Return deployment view
```

## Ngoài phạm vi happy path

- **OOS-01:** Streaming hoặc real-time status updates.
- **OOS-02:** Driver diagnostic logs; application pod logs are covered by TEL below.
- **OOS-03:** So sánh hai deployments.
- **OOS-04:** Alerting và notifications.
- **OOS-05:** Tracing, business metrics and audit views; basic CPU/memory history is covered by TEL.
- **OOS-06:** Tùy chỉnh quyền xem outputs theo RBAC.


## Chi tiết tài nguyên và tiến trình xóa

- **RES-01:** Resource detail thuộc đúng Organization/Application/Environment/
  generation, gồm tên kỹ thuật (descriptor), trạng thái, module/target/owner,
  safe outputs, consumers và eligibility. Có thể đọc resource UNREFERENCED
  từ lịch sử sau khi Workload đã gỡ; không cần nó còn trong current graph.
- **RES-02:** Nút Xóa tài nguyên chỉ hiện cho PE/Admin. Eligibility phải có
  dữ liệu kiểm chứng; chưa tải/lỗi/missing metadata không được coi là đủ điều
  kiện. Root có consumer, không UNREFERENCED, platform hoặc Environment busy
  thì khóa nút và nêu lý do. Luồng mutation do UC-07 DEL-* sở hữu.
- **RES-03:** Trang detail/progress hiển thị REQUESTED/CHECKING/DESTROYING/
  VERIFYING, từng resource trong phạm vi đã xác nhận, kết quả absence check và
  remaining physical IDs khi lỗi. Poll scoped persisted operation; không báo
  success trước VERIFYING hoàn tất hoặc lấy trạng thái operation khác.

## Visual resource progress — accepted 2026-10-23

- **OBS-01:** Console dùng cùng graph/node identity như Preview và deployment
  đã persist. Nhóm Internal/EKS/VPC/managed resources từ placement provenance;
  hiển thị riêng từng resource kể cả shared/preexisting và từng workload.
- **OBS-02:** Hiển thị chưa chạy/chờ phụ thuộc/đang cấp phát/kiểm tra sẵn sàng/
  sẵn sàng/giữ nguyên/thất bại/bị chặn/chưa xác định theo mapping chuẩn trong
  [review contract](../../architecture/contracts/deployment-review.md).
  Không đổi lifecycle enum; thiếu dữ liệu không tạo trạng thái thành công.
- **OBS-03:** Chọn node thấy safe stage/attempt, timestamp/elapsed, Definition/
  source pin, vị trí và dependency đang chờ. APPLIED chưa đồng nghĩa READY;
  thiếu readiness-stage của workload không được tự suy thành kiểm tra readiness.
- **OBS-04:** Hiển thị đồng thời các nhánh có evidence đang chạy; failure chặn
  consumers theo graph thật và giữ completed peers. FAILED không hàm ý cleanup.
  Không thêm concurrency/retry/rollback hoặc pause execution từ UI.
- **OBS-05:** Poll read-only persisted query, không provider refresh. Lỗi đọc giữ
  dữ liệu cuối với dấu stale; đổi scope bỏ late response; terminal thì dừng poll.
  Timer chỉ tính elapsed từ timestamp, không tạo trạng thái hoặc ETA giả.
- **OBS-06:** Detail hiển thị safe review/source và rendered manifest artifact
  của chính deployment; ghi rõ applied chỉ khi workload READY. Thiếu artifact
  lịch sử báo unavailable; không dựng lại bằng Definition/bundle hiện tại.
- **OBS-07:** List hỗ trợ `reviewRequestId` (UUID riêng cho mỗi lần Deploy) cùng scope để theo dõi pending batch từ
  UC-05. Chỉ assoc đã persist khi admission; không lẫn run người khác hoặc
  deployment cùng workload ở lần khác. Batch chưa có record không là success.

## Native application observability — TEL

Accepted 2026-10-25: [ADR-023](../../architecture/decisions/ADR-023-native-observability.md),
[contract](../../architecture/contracts/observability.md) and the approved
[UI reference](../../../frontend/design/logging-monitoring/README.md).
The earlier deployment-snapshot-only BR rules still apply to Deployment detail;
TEL adds separate application telemetry queries backed by a background collector.

- **TEL-01:** Workloads deployed on internal Kubernetes or EKS are collected
  automatically from their persisted scoped target. Developer Score and app
  containers need no observability setup. Metrics Server is a prerequisite for
  CPU/memory; absence must not block log collection or workload deployment.
- **TEL-02:** Retain received container logs seven days and samples/events fifteen
  days independently of pod/redeploy/remove. Namespace/workload/generation and
  pod UID/container/restart identities prevent mixing or duplication. Report
  gaps/truncation/staleness explicitly; do not claim unretrieved logs survived.
- **TEL-03:** Application home displays errors, unusual events, interval peaks
  and current CPU/memory separately, with notable events and miniature trends.
  Collection health is separate from app health. The basic high indicator is
  80% of that sample's complete positive workload limit; without limits display
  absolute units only. Count each high metric/workload once plus observed
  abnormal events; exact log error counts and charts use the selected interval.
- **TEL-04:** Summary and event clicks preserve environment, interval, workload
  and relevant log level/metric focus. Logs support literal search, recognized
  level, stable older-page navigation, raw-text details and follow/pause of stored
  updates. Metrics show CPU/memory history with units, workload series, true
  timestamps and null gaps. History may include removed pods/workloads.
- **TEL-05:** All GETs authorize the session organization and app/environment,
  read persisted telemetry only and enforce range/page/response limits. Resolve
  collection credentials privately, fail closed on foreign targets and protect
  platform diagnostics. Native API logs are combined stdout/stderr; do not
  fabricate stream-specific filtering or infer ERROR from stderr.
- **TEL-06:** Empty/not-deployed, waiting, missing metrics, stale/gap, storage
  limit and unavailable/error states remain distinct. No sample is not zero;
  successful deployment is not proof telemetry was received. Changing scope
  discards late replies; changing filters clears stale drill context.
- **TEL-07:** Polling/checkpoints survive process restart; fenced collection and
  idempotent records prevent duplicate observations across workers. Bounded
  storage preserves existing history on quota refusal and exposes that limit.
  Reads never mutate or destroy app/provider resources. No alert notifications,
  tracing, CloudWatch, business instrumentation or custom rule editor is added.

## Reconciliation observations — ADR-030

- **REC-01:** Environment status exposes applied/desired delivery, separate sync,
  health and drift dimensions, exact revision, last observed time, freshness and
  safe reason. Historical Deployment status remains immutable; telemetry is
  independent of configuration equivalence.
- **REC-02:** Disconnected, unsupported, not yet observed and partial data remain
  explicit. Ready Pods or a Git commit alone cannot show IN_SYNC. GETs read stored
  observations and never trigger provider calls or mutations.
- **REC-03:** Authorized users can request observation/reconcile of the admitted
  workload revision; PE/Admin can request infrastructure scans and reviewed
  repair. Show persisted operation progress and unknown-outcome recovery, not
  optimistic success. Scope changes discard stale browser replies.
- **REC-04:** Reports redact values and raw provider diagnostics; show bounded
  field paths/actions and coverage. Infrastructure cadence is independent of the
  Environment CD checkbox. No alerting/notification service is introduced.
