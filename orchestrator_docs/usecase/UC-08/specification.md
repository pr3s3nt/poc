---
id: UC-08-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-28
---

# UC-08 — Provision Infrastructure and Application Resources

## Mục tiêu

Provision implicit infrastructure cùng private/shared resources theo dependency order và cung cấp outputs cho consumer.

## Initiating use cases

- UC-06 — Deploy Workload
- UC-07 — Update or Remove Workload

## Supporting actors

- Resource Driver
- Cloud Provider hoặc Infrastructure Provider
- Secret Store

## Tiền điều kiện

- **PRE-01:** Resource Graph đã được dựng và là DAG.
- **PRE-02:** Mỗi resource có đúng một matched Definition; internal cluster không system binding theo ADR-016.
- **PRE-03:** Driver, Driver Account/connection và credentials cần thiết đã sẵn sàng.
- **PRE-04:** Resource Descriptor và scope Application, Environment hoặc workload đã được xác định.
- **PRE-05:** Provision batches đặt provider trước consumer.

## Trigger

**TRG-01:** UC-06 hoặc UC-07 yêu cầu thực thi provision plan của một Deployment.

## Main success scenario

1. **MS-01:** Orchestrator nhận Resource Graph và provider-first provision batches.
2. **MS-02:** Resolve stable instance/ActiveResource/state theo identity; ghi
   action/module/physical intent/ownership trước provider call dưới owner/fence.
3. **MS-03:** Orchestrator resolve context values, resource inputs và outputs của các provider đã hoàn thành.
4. **MS-04:** Orchestrator chọn executor từ matched Definition hoặc trusted system cluster binding và Execution Profile.
5. **MS-05:** Kiểm verified module/inputs, khóa state và executor CREATE hoặc
   non-destructive UPDATE theo VAR-01/VAR-02 trong deadline.
6. **MS-06:** State được backend xác nhận; kiểm outputs/readiness theo Type và
   lưu secret outputs bằng opaque refs trước khi truyền cho consumer.
7. **MS-07:** Orchestrator lưu Active Resource gồm descriptor, Definition ID, scope, executor state và outputs.
8. **MS-08:** Orchestrator dùng outputs để resolve bindings của consumer ở batch tiếp theo.
9. **MS-09:** Orchestrator lặp MS-02 đến MS-08 cho tới khi hoàn thành mọi batch.
10. **MS-10:** Orchestrator trả provision result thành công cho use case khởi tạo.

## Luồng biến thể trong happy path

- **VAR-01:** `aws-eks`: Terraform quản lý VPC/EKS/namespace và Redis/
  PostgreSQL in-cluster hoặc ElastiCache/Aurora theo class. AWS provider dùng
  cùng Environment account/region; Kubernetes provider bind completed EKS target.
- **VAR-02:** `internal-k8s`: ExistingClusterAdapter kiểm Connection có sẵn;
  Terraform với Kubernetes provider quản lý namespace/Redis/PostgreSQL.

## Hậu điều kiện

- **POST-01:** Mọi desired resource đã được provision/reconcile theo dependency order.
- **POST-02:** Mỗi Cloud Environment/generation có tối đa một VPC và một EKS cho descriptor tương ứng.
- **POST-03:** Internal Application không tạo VPC/EKS và dùng cluster connection có sẵn.
- **POST-04:** Private resource gắn với đúng workload identity; shared resource dùng chung descriptor chỉ có một Active Resource.
- **POST-05:** Redis/PostgreSQL có output contract thống nhất giữa
  implementations. Chỉ outputs/readiness hợp lệ được dùng bởi consumer.
- **POST-06:** Outputs và executor state được lưu để consumer và deployment sau sử dụng.

## Quy tắc nghiệp vụ

- **BR-01:** Edge graph có chiều `consumer -> provider`; chỉ execute một batch khi mọi provider dependency đã hoàn thành.
  Aurora/ElastiCache tham chiếu outputs subnet private của cùng VPC Environment.
  VPC tạo mạng một lần; nhiều database/cache không tạo lại subnet hoặc dải CIDR.
  Xóa một consumer không xóa subnet thuộc VPC; consumer checks giữ nguyên.
- **BR-02:** Logical identity gồm Organization, Environment/generation,
  descriptor/scope; mỗi instance một durable encrypted PostgreSQL state qua
  HTTP backend. Names/password refs giữ ổn định khi redeploy/retry. Run ID
  chỉ là provenance; state/name không đổi theo run hay Definition version.
- **BR-03:** Outputs phải thỏa Type contract và readiness trước consumer.
  Secret bytes lưu trong secret store; ActiveResource/progress chỉ có refs.
  Terraform state là ciphertext hạn chế truy cập riêng, không public output.
- **BR-04:** Node cùng batch không phụ thuộc nhau và có thể execute song song, nhưng MVP có thể chạy tuần tự mà không đổi semantics.
- **BR-05:** Resource hiện hữu vẫn được reconcile bằng UPDATE có kiểm
  state/target. Không delete/replace trong UPDATE; DESTROY chỉ theo operation
  Xóa tài nguyên hoặc cleanup riêng đã kiểm ownership/consumers. Classification
  của planner không tự cấp quyền destroy.
- **BR-06:** Container CPU/memory requests/limits thuộc workload module và được UC-06 Workload Renderer xử lý; chúng không tạo Resource Graph node và không đi qua UC-08 Resource Executor.
- **BR-10:** Mọi loại managed resource của Terraform module đã được registry
  chấp nhận đều dùng được, không cần thêm tên resource vào Go. Sau execution
  thành công, state acknowledgement và output validation, completion của
  provider là điều kiện mặc định. Các kiểm tra readiness bổ sung hiện có cho
  Namespace/Deployment/StatefulSet, VPC/EKS/RDS/ElastiCache vẫn bắt buộc cho
  từng resource tương ứng, kể cả module hỗn hợp. Thiếu bộ kiểm tra bổ sung
  không phải lỗi. State phải có managed resources với address/type đầy đủ;
  ID bắt buộc khi probe tương ứng cần nó, không bắt buộc cho schema provider
  không công bố attribute `id` (ví dụ `kubernetes_manifest`). Không tự tạo ID.
  Kết quả rỗng, deadline hết hoặc query bổ sung thất bại không được coi là
  thành công. Không mở thêm provider, credential hoặc quyền cleanup.

## Luồng nội bộ

```text
UC-08 Provision Resources
├── Load provision batches
├── Find reusable Active Resources by scope
├── Resolve inputs and dependency outputs
├── Execute Terraform modules / resolve existing cluster in batch
├── Collect outputs and resource state
├── Persist Active Resources
└── Continue with dependent batches
```

## Executor mapping

| Profile | Resource | Executor |
|---|---|---|
| aws-eks | VPC/EKS/ElastiCache/Aurora | Terraform với AWS provider |
| internal-k8s / aws-eks | Namespace/Redis/PostgreSQL in-cluster | Terraform với Kubernetes provider |
| internal-k8s | Cluster có sẵn | ExistingClusterAdapter; không tạo/xóa cluster |

## Ngoài phạm vi happy path

- **OOS-01:** Retry không giới hạn hoặc cancellation không dừng process.
  Retry tối đa một lần, timeout và stopped-process checks thuộc V2-06.
- **OOS-02:** Tự động resume sau gián đoạn và reconcile tài nguyên ngoài
  state theo D12/D13. Ghi nhận partial failure/hold và cleanup vẫn bắt buộc.
- **OOS-03:** Incremental provisioning optimization.
- **OOS-04:** Scheduled deletion và detach không kiểm ownership.
  DESTROY có xác nhận/cleanup thuộc UC-07 V2-03 và OC-19.
- **OOS-05:** Credential rotation. Secret outputs và bindings thuộc BR-03.
- **OOS-06:** Drift detection.

## Workload rendering boundary

- **BR-09:** A workload Definition selects UC-06 rendering but is excluded from
  UC-08 batches. UC-08 remains sole owner of infrastructure identity and outputs;
  score-k8s consumes those outputs and cannot provision their resources again.

See [workload rendering contract](../../architecture/contracts/workload-rendering.md).

## Environment connection binding

- **BR-07:** Each AWS node, including Aurora/ElastiCache, uses its winning
  Definition's pinned scoped READY Driver Account and Driver Inputs region
  (UC-03 V02-03/09). Verify kind/identity/version before provision and resolve
  credentials privately per invocation; never substitute Environment/host identity.
  Kubernetes uses inherited cluster dependency target; legacy explicit Environment
  target/system cluster ADR-013 persists until V03. Reuse validates original
  state target/generation/account/region; mismatch stops without fallback.
  Mutable catalog edits never rewrite original execution/cleanup pins.
  Infrastructure scope/ownership/fencing remain; no cross-node account constraint.

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

- **BR-08:** Use matched existing-cluster Definition and pinned READY scoped
  Internal account under ADR-016. Check identity/API/permissions/readiness and
  return a typed opaque target before consumers; no cluster create/destroy,
  builtin catalog admission or Environment/host fallback. Namespace provider
  and workload cluster reference must agree. Historical applied target remains
  authoritative for cleanup, not current catalog selection.

## Locking, retry và outcome chưa rõ

- **V2-04:** Persist intent/ownership trước call. State lock nằm dưới
  Environment operation; mỗi write/unlock kiểm owner/fence. Không giữ DB
  transaction qua Terraform/network call hoặc tự mở khóa vì hết heartbeat.
- **V2-06:** Retry transient tối đa một lần trong deadline sau kiểm state/
  kết quả thực tế. Input/auth/permission hoặc outcome chưa rõ thì dừng, giữ
  progress/state/hold. Không tự resume/import; cleanup độc lập runner vẫn bắt buộc.
- **V2-07:** State thiếu sau prior attempt hoặc provider outcome chưa rõ thì
  chặn CREATE. Giữ recovery artifact riêng tư nếu có và kiểm ownership thủ
  công trước cleanup; không import v1 hoặc rollback resource đã cập nhật.

Protocol/validation đầy đủ ở [state contract](../../architecture/contracts/terraform-state.md).


## V03 typed targets

[ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) supersedes the internal Environment/system binding portions of BR-07/09. Persist target per resource and workload, validate output contract and original target on reuse/restart/cleanup. The existing cluster is PREEXISTING. Workload driver lifecycle remains UC-06/07 after resource-only batches; namespaces are owned resources. No cross-cluster global target inference.


## V04–V06 resource execution replacement

[ADR-017](../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces implicit/profile executor selection in MS-04, VAR-01/02, POST-02/03
and the profile column of the executor table. Match selects the driver, not
Environment policy.

- **V04-01:** Persist instance/status/outputs/original ownership, resolve and
  validate required/type/secret contracts before consumers; block on dependency
  error. Shared descriptors yield one instance across backend and worker.
- **V05-01:** Existing and newly provisioned clusters supply typed targets. EKS
  consumers wait for original-account auth/API/network/node readiness; persist
  opaque identity, never transient token/kubeconfig. Reuse/restart must not
  duplicate infrastructure or substitute current catalog/host credentials.
- **V06-01:** Each node uses its own Definition account/region; managed database/
  cache depend on required network outputs, never a blanket EKS relation.
  Destroy uses reverse actual graph and original state/module/target, verifies
  service APIs and protects shared consumers and PREEXISTING resources.

## Resource progress for visual review

- **REV-01:** Persist safe per-node attempt/stage/status/timestamps tại các ranh
  giới cấp phát và readiness để UC-09 đọc. INIT/PLANNING/APPLYING/APPLIED/OUTPUTS
  không được chiếu thành READY trước khi output validation/readiness hoàn tất.
- **REV-02:** Phân biệt verified retain/preexisting binding với newly provisioned
  READY. Provider failure chặn consumers, giữ committed peers và original state/
  ownership cho cleanup riêng. Query chỉ đọc snapshots, không gọi executor.
- **REV-03:** Internal/cloud cùng contract; VPC/EKS chỉ có nếu authored dependency
  yêu cầu. AWS managed resources có placement ngoài Kubernetes; không tự thêm
  EKS cho managed-only graph hoặc tạo public managed-only execution flow mới.

## Infrastructure drift — ADR-027

INF requirements replace OOS-06 only for acknowledged managed Terraform
instances. Orphan import/recovery D12–D14 remain deferred.

- **INF-01:** Scheduled/on-demand scans compare real provider state with the
  last acknowledged pinned module/inputs/state baseline. No current Definition,
  uncommitted draft or mutable latest secret substitution. Missing baseline is
  unavailable, never clean; failed/partial queries are UNKNOWN.
- **INF-02:** Scans are read-only to infrastructure and canonical state, fenced
  against deployment/cleanup, bounded and restart-observable. Detect attribute
  updates as well as missing resources; separate external drift from repair plan.
- **INF-03:** No unattended Terraform apply. PE/Admin may review and confirm
  exact in-place repair with fresh state/baseline guards. Changed scope/actions,
  replacement/deletion/import, unknown consumer outputs or missing ownership
  block repair; use an appropriate normal deployment/cleanup workflow instead.
- **INF-04:** Successful repair acknowledges state/output contracts under existing
  locks but does not rewrite deployment history or desired drafts. Unknown process
  outcomes retain v2 holds. Secret/state/plan values never appear in reports.

Algorithm and API: [infrastructure drift contract](../../architecture/contracts/infrastructure-drift.md).
