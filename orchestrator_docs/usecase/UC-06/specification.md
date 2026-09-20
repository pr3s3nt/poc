# UC-06 — Deploy Workload

## Mục tiêu

Triển khai Score workload theo Execution Profile của Application, tự động chuẩn bị hạ tầng cần thiết và đưa workload lên Kubernetes.

## Primary actors

- Developer
- CI/CD System

## Supporting actors

- Resource Driver
- Kubernetes Cluster/Operator

## Tiền điều kiện

- **PRE-01:** Application và Environment đã tồn tại.
- **PRE-02:** Application đã chọn `aws-eks` hoặc `internal-k8s` trong UC-01.
- **PRE-03:** AWS Driver Account hoặc internal cluster connection trạng thái `READY` đã được cấu hình trong UC-04.
- **PRE-04:** Các Resource Type và Resource Definition cần thiết đã được đăng ký.
- **PRE-05:** Score document hợp lệ và mô tả đúng một workload.

## Trigger

**TRG-01:** Developer hoặc CI/CD gửi Score cùng Application ID và Environment ID cần triển khai.

## Main success scenario

1. **MS-01:** Orchestrator tạo Deployment record và đọc Deployment Set hiện tại của Environment.
2. **MS-02:** Orchestrator validate Score và chuyển nó thành workload fragment.
3. **MS-03:** Orchestrator tạo Deployment Delta và Candidate Deployment Set.
4. **MS-04:** Orchestrator đọc Execution Profile và runtime configuration của Application.
5. **MS-05:** Orchestrator enrich graph theo VAR-01 hoặc VAR-02.
6. **MS-06:** Orchestrator thêm namespace có identity theo Environment và resource dependencies từ Score.
7. **MS-07:** Orchestrator match Resource Definitions; `postgres` được chọn thành Aurora hoặc StatefulSet theo profile context.
8. **MS-08:** Orchestrator validate graph/contracts và tính provider-first provision batches.
9. **MS-09:** UC-06 `«include»` UC-08 để provision/reuse resources theo dependency order.
10. **MS-10:** Orchestrator nhận resource outputs và resolve các binding của workload.
11. **MS-11:** Orchestrator render Workload Profile thành Kubernetes manifests và apply lên cluster đã resolve.
12. **MS-12:** Orchestrator lưu Candidate Deployment Set thành current, Active Resources và deployment status `SUCCEEDED`.
13. **MS-13:** Orchestrator trả kết quả deployment thành công cho actor.

## Luồng biến thể trong happy path

- **VAR-01 — `aws-eks`:** tại MS-05, thêm implicit VPC và EKS có Resource Descriptor scope Application; tại MS-09, UC-08 tái sử dụng hoặc provision VPC -> EKS trước Aurora/workload.
- **VAR-02 — `internal-k8s`:** tại MS-05, thêm provider node tham chiếu cluster connection có sẵn; UC-08 không tạo VPC/EKS và provision namespace/PostgreSQL trên cluster đó.

## Hậu điều kiện

- **POST-01:** Resource dependencies đã được provision hoặc tái sử dụng.
- **POST-02:** Cloud Application có đúng một VPC và một EKS theo descriptor được dùng lại cho các Environment/deployment sau.
- **POST-03:** Internal Application sử dụng cluster có sẵn và không tạo VPC/EKS.
- **POST-04:** Environment có Kubernetes namespace riêng.
- **POST-05:** Workload đã được apply lên Kubernetes.
- **POST-06:** Candidate Deployment Set trở thành current Deployment Set của Environment.
- **POST-07:** Resource state và outputs cần thiết đã được lưu.
- **POST-08:** Deployment có trạng thái `SUCCEEDED`.

## Quy tắc nghiệp vụ

- **BR-01:** Planning phải duy trì invariant `current + Delta = Candidate`.
- **BR-02:** Resource Descriptor là identity ổn định; VPC/EKS scope Application, namespace scope Environment, Score dependency dùng declared scope.
- **BR-03:** Execution Profile chỉ enrich graph/context; provider-specific execution được chọn qua Resource Definition và executor adapter.
- **BR-04:** Graph phải là DAG và batches phải đặt mọi provider trước consumer.
- **BR-05:** Chỉ commit Candidate Deployment Set thành current sau khi UC-08 và workload apply hoàn tất thành công.
- **BR-06:** Resource output chỉ được binding nếu thuộc output contract của Resource Type/Definition.

## Luồng nội bộ

```text
UC-06 Deploy Workload
├── UC-06a Plan Deployment
│   ├── Score -> Deployment Delta
│   ├── Candidate Deployment Set
│   ├── Inject implicit infrastructure by Execution Profile
│   ├── Complete Resource Graph
│   ├── Resource Definition Matching
│   └── Provision Schedule
│
└── UC-06b Execute Deployment
    ├── Include UC-08 Provision Resources
    ├── Collect and propagate outputs
    ├── Render Workload Profile
    ├── Apply Kubernetes manifests
    └── Persist deployment state
```

## Implicit Resource Graph

Cloud (`consumer -> provider`):

```text
workload -> k8s-namespace -> eks -> vpc
workload -> postgres-aurora -> vpc
```

Internal Kubernetes:

```text
workload -> k8s-namespace -> existing-k8s-cluster
workload -> postgres-statefulset -> existing-k8s-cluster
```

VPC/EKS có scope theo Application; namespace có scope theo Environment; PostgreSQL có scope theo dependency khai báo trong Score.

## Trạng thái implementation hiện tại

- **UC-06a Plan Deployment:** planner hiện tại đã triển khai phần Score/Delta/Graph cơ bản, nhưng chưa inject implicit VPC, EKS, existing cluster và namespace theo Execution Profile.
- **UC-06b Execute Deployment:** chưa có; cần bổ sung driver executor, output propagation, Kubernetes renderer/deployer và persistent state.

## Ngoài phạm vi happy path

- **OOS-01:** Retry và timeout.
- **OOS-02:** Rollback tự động.
- **OOS-03:** Concurrent deployment.
- **OOS-04:** Partial failure recovery.
- **OOS-05:** Drift reconciliation.
- **OOS-06:** Scheduled deletion và deprovision.
- **OOS-07:** RBAC và approval workflow.
