---
id: UC-08-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-07
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
- **PRE-02:** Mỗi resource node đã match đúng một Resource Definition.
- **PRE-03:** Driver, Driver Account/connection và credentials cần thiết đã sẵn sàng.
- **PRE-04:** Resource Descriptor và scope Application, Environment hoặc workload đã được xác định.
- **PRE-05:** Provision batches đặt provider trước consumer.

## Trigger

**TRG-01:** UC-06 hoặc UC-07 yêu cầu thực thi provision plan của một Deployment.

## Main success scenario

1. **MS-01:** Orchestrator nhận Resource Graph và provider-first provision batches.
2. **MS-02:** Với từng node trong batch hiện tại, Orchestrator tìm Active Resource theo descriptor/scope để resolve state có thể tái sử dụng.
3. **MS-03:** Orchestrator resolve context values, resource inputs và outputs của các provider đã hoàn thành.
4. **MS-04:** Orchestrator chọn executor từ matched Resource Definition và Execution Profile.
5. **MS-05:** Executor provision/reconcile resource theo VAR-01 hoặc VAR-02.
6. **MS-06:** Executor trả resource state và outputs theo Resource Type contract.
7. **MS-07:** Orchestrator lưu Active Resource gồm descriptor, Definition ID, scope, executor state và outputs.
8. **MS-08:** Orchestrator dùng outputs để resolve bindings của consumer ở batch tiếp theo.
9. **MS-09:** Orchestrator lặp MS-02 đến MS-08 cho tới khi hoàn thành mọi batch.
10. **MS-10:** Orchestrator trả provision result thành công cho use case khởi tạo.

## Luồng biến thể trong happy path

- **VAR-01 — `aws-eks`:** Terraform Executor provision/reconcile VPC, EKS và Aurora; Active Resource/driver state quyết định create hay reuse.
- **VAR-02 — `internal-k8s`:** connection adapter trả outputs của cluster có sẵn; Kubernetes Executor apply namespace, PostgreSQL StatefulSet và Service.

## Hậu điều kiện

- **POST-01:** Mọi desired resource đã được provision/reconcile theo dependency order.
- **POST-02:** Mỗi Cloud Application có tối đa một VPC và một EKS cho descriptor tương ứng.
- **POST-03:** Internal Application không tạo VPC/EKS và dùng cluster connection có sẵn.
- **POST-04:** Private resource gắn với đúng workload identity; shared resource dùng chung descriptor chỉ có một Active Resource.
- **POST-05:** `postgres` trả cùng output contract dù implementation là Aurora hay StatefulSet.
- **POST-06:** Outputs và executor state được lưu để consumer và deployment sau sử dụng.

## Quy tắc nghiệp vụ

- **BR-01:** Edge graph có chiều `consumer -> provider`; chỉ execute một batch khi mọi provider dependency đã hoàn thành.
- **BR-02:** Resource Descriptor cộng scope là identity logic dùng để tìm Active Resource; executor state là identity vật lý của provider.
- **BR-03:** Mỗi output phải thỏa Resource Type contract trước khi persist hoặc truyền cho consumer.
- **BR-04:** Node cùng batch không phụ thuộc nhau và có thể execute song song, nhưng MVP có thể chạy tuần tự mà không đổi semantics.
- **BR-05:** Existing resource vẫn đi qua reconcile/provision contract của driver; planner classification không tự tạo action `reuse` hay `destroy`.
- **BR-06:** Container CPU/memory requests/limits thuộc workload module và được UC-06 Workload Renderer xử lý; chúng không tạo Resource Graph node và không đi qua UC-08 Resource Executor.

## Luồng nội bộ

```text
UC-08 Provision Resources
├── Load provision batches
├── Find reusable Active Resources by scope
├── Resolve inputs and dependency outputs
├── Execute Terraform or Kubernetes drivers in batch
├── Collect outputs and resource state
├── Persist Active Resources
└── Continue with dependent batches
```

## Executor mapping

| Execution Profile | Resources | Executor |
|---|---|---|
| `aws-eks` | VPC, EKS, Aurora | Terraform Executor |
| `internal-k8s` | Existing cluster | Connection/Echo adapter |
| `internal-k8s` | Namespace, PostgreSQL StatefulSet/Service | Kubernetes Executor |

## Trạng thái implementation hiện tại

- Planner đã dựng private/shared/implicit resource nodes, match Definition và tạo provider-first batches; new VPC/EKS and namespace use environment scope; legacy AWS identity remains application-scoped (ADR-011).
- `ResourceProvisioningService` đã chọn fake, existing-cluster, Kubernetes hoặc Terraform executor theo matched Definition; outputs được validate, truyền sang node phụ thuộc/workload và lưu vào Active Resource/deployment-resource state.
- Internal path đã provision namespace/PostgreSQL StatefulSet trên kind; cloud path đã provision VPC/EKS/Aurora bằng Terraform trên AWS.
- Normalized PostgreSQL repository đã có cho logical orchestration state; in-memory/JSON vẫn dùng cho local/test. Terraform state vẫn nằm trong local run directory; remote state/locking và lifecycle reconcile đầy đủ chưa có.

## Ngoài phạm vi happy path

- **OOS-01:** Retry, timeout và cancellation.
- **OOS-02:** Partial failure và resume.
- **OOS-03:** Incremental provisioning optimization.
- **OOS-04:** Scheduled deletion, detach và deprovision.
- **OOS-05:** Secret outputs và credential rotation.
- **OOS-06:** Drift detection.

## Workload rendering boundary

- **BR-07:** A workload Definition selects UC-06 rendering but is excluded from
  UC-08 batches. UC-08 remains sole owner of infrastructure identity and outputs;
  score-k8s consumes those outputs and cannot provision their resources again.

See [workload rendering contract](../../architecture/contracts/workload-rendering.md).

## Environment connection binding

- **BR-07:** Internal Kubernetes and AWS VPC/EKS execution must use pinned Environment connection; mismatch rejected before executor. External resource Driver Account semantics remain. Reuse validates persisted resource connection identity; do not execute against another Environment target. New AWS infrastructure uses Environment scope; legacy AWS retains old Application descriptors/state under ADR-011.

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
