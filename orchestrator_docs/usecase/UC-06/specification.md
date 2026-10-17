---
id: UC-06-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-17
---

# UC-06 — Deploy Workload

## Mục tiêu

Triển khai Score workload theo Execution Profile của Environment, tự động chuẩn bị hạ tầng cần thiết và đưa workload lên Kubernetes.

## Primary actors

- Developer
- CI/CD System

## Supporting actors

- Resource Driver
- Kubernetes Cluster/Operator

## Tiền điều kiện

- **PRE-01:** Application và Environment đã tồn tại.
- **PRE-02:** Environment đã cấu hình Connection; profile `aws-eks` hoặc `internal-k8s` được suy ra từ Connection theo UC-01, không đọc từ Application.
- **PRE-03:** AWS Driver Account hoặc internal cluster connection trạng thái `READY` đã được cấu hình trong UC-04.
- **PRE-04:** Các Resource Type và Resource Definition cần thiết đã được đăng ký.
- **PRE-05:** Score document hợp lệ, mô tả đúng một workload và mọi container resource requests/limits khai báo đều đúng Score subset.

## Trigger

**TRG-01:** Developer hoặc CI/CD gửi Score cùng Application ID và Environment ID cần triển khai.

## Main success scenario

1. **MS-01:** Orchestrator tạo Deployment record và đọc Deployment Set hiện tại của Environment.
2. **MS-02:** Orchestrator validate Score và chuyển nó thành workload fragment, bảo toàn container CPU/memory requests/limits.
3. **MS-03:** Orchestrator tạo Humanitec-shaped Deployment Delta và Candidate Deployment Set.
4. **MS-04:** Orchestrator resolve và pin Execution Profile, Connection, region và runtime configuration của Environment; Application chỉ cung cấp identity/current metadata.
5. **MS-05:** Orchestrator enrich graph theo VAR-01 hoặc VAR-02.
6. **MS-06:** Orchestrator thêm namespace có identity theo Environment và resource dependencies từ Score.
7. **MS-07:** Orchestrator bind internal cluster ngầm từ Environment Connection, match Resource Definitions cho các resource còn lại rồi expand Resource References và provision rules tới fixed point; Redis/PostgreSQL được chọn theo type/class/profile và criteria, không tự đổi
   class khi matching thất bại.
8. **MS-08:** Orchestrator validate graph/contracts và tính provider-first provision batches.
9. **MS-09:** Sau kiểm inputs/artifacts và nhận Environment operation dưới
   CAS/owner fencing, UC-06 `«include»` UC-08 để provision theo dependency order.
10. **MS-10:** Orchestrator nhận resource outputs và resolve các binding của workload.
11. **MS-11:** score-k8s sinh manifest từ Score đã chuẩn hóa và outputs/Secret
    refs. Giữ container requests/limits, kiểm manifest, Apply và chờ Ready.
12. **MS-12:** Orchestrator lưu Candidate Deployment Set thành current, Active Resources và deployment status `SUCCEEDED`.
13. **MS-13:** Orchestrator trả kết quả deployment thành công cho actor.

## Luồng biến thể trong happy path

- **VAR-01:** `aws-eks`: thêm VPC/EKS thuộc Environment/generation;
  Terraform tạo hạ tầng theo dependency. Redis/PostgreSQL có thể in-cluster
  trên EKS hoặc ElastiCache/Aurora theo class, cùng AWS account/region.
- **VAR-02:** `internal-k8s`: cluster provider tham chiếu Connection có sẵn;
  Terraform tạo namespace/Redis/PostgreSQL trên target đó, không tạo VPC/EKS.
- **VAR-03 — Fleet GitRepo trên internal kind:** tại MS-11, khi platform cấu
  hình mode này, Orchestrator ghi workload manifests không bí mật vào repo
  GitOps theo Application/Environment/workload và Ingress vào bundle
  `Application/Environment/_routes`, push commit và chờ Fleet áp
  dụng revision lên namespace của Environment. Image đã có trong Harbor; UC-06
  không clone mã nguồn hoặc build image. UC-08 vẫn sở hữu namespace/resources.

## Hậu điều kiện

- **POST-01:** Resource dependencies đã được provision hoặc tái sử dụng.
- **POST-02:** AWS Environment có VPC/EKS theo descriptor/scope/generation đã pin, được dùng lại cho deployment cùng đích; không lấy target từ Application.
- **POST-03:** Internal Environment sử dụng cluster có sẵn và không tạo VPC/EKS.
- **POST-04:** Environment có Kubernetes namespace riêng.
- **POST-05:** Workload đã được apply lên Kubernetes với container requests/limits đúng như Score khi có khai báo.
- **POST-06:** Candidate Deployment Set trở thành current Deployment Set của Environment.
- **POST-07:** Resource state và outputs cần thiết đã được lưu.
- **POST-08:** Deployment có trạng thái `SUCCEEDED`.

## Quy tắc nghiệp vụ

- **BR-01:** Planning phải duy trì invariant `current + Delta = Candidate`.
- **BR-02:** Resource Descriptor là identity ổn định; VPC/EKS scope Environment theo ADR-011/T27, namespace scope Environment, Score dependency dùng declared scope.
- **BR-03:** Execution Profile chỉ enrich graph/context; provider-specific execution được chọn qua Resource Definition và executor adapter; internal cluster dùng trusted system binding theo ADR-013, không matching catalog.
- **BR-04:** Graph phải là DAG và batches phải đặt mọi provider trước consumer.
- **BR-05:** Chỉ commit Candidate thành current sau khi UC-08, manifest
  validation, delivery và readiness đều thành công. Lỗi không giả success.
- **BR-06:** Resource output chỉ được binding nếu thuộc output contract của Resource Type/Definition.
- **BR-07:** Score `params` của một resource được validate theo `inputs` của Resource Type, đi vào Deployment Set entry và trở thành resource input của node tương ứng.
- **BR-08:** Provision rule của Definition tạo resource đi kèm; `is_dependent` đảo chiều phụ thuộc sang node cha, `match_dependents` nối mọi consumer của node cha sang resource đi kèm.
- **BR-09:** Planner kiểm inputs/module mappings biết trước execution từ
  verified registry, pin module/provider/binary digests. Deploy kiểm lại
  artifacts trước provision; runtime outputs được type-check trước từng node.
- **BR-10:** Deployment Delta dùng shape Humanitec `modules.add/remove/update` và `shared`; module update patch relative với module, shared patch relative với shared object, array diff theo index và `/-`.
- **BR-11:** `containers.*.resources.requests/limits` chỉ gồm `cpu`/`memory`, phải đi nguyên vẹn qua Score fragment, Candidate Deployment Set và workload renderer; renderer không được thay giá trị đã khai báo bằng giá trị hard-code. Request của từng field (`cpu`, `memory`) được chọn theo thứ tự: request đã khai báo; nếu request thiếu thì limit đã khai báo của cùng field; nếu thiếu cả hai thì platform default `cpu: 10m` hoặc `memory: 32Mi`. Limits chỉ chứa các field được khai báo và không có default. Giá trị suy ra không được ghi ngược vào Score fragment hoặc Candidate Deployment Set.
- **BR-12:** Khi parse Resource Reference, token `@app`, `@env`, `@infra` và
  `@connection` trong phần Resource ID được thay bằng key của Application,
  Environment và Connection hiện tại trước khi tạo descriptor. Đây là product
  extension; ký hiệu `@` dùng để kế thừa class/ID hiện tại vẫn giữ semantics
  Resource Reference chuẩn.
- **BR-13:** Trong Fleet GitRepo mode, Deployment chỉ `SUCCEEDED` khi Git
  revision được Fleet quan sát và workload revision tương ứng Ready. Không ghi
  raw secret hoặc Docker registry credential vào GitOps repository.
- **BR-14:** Trên internal kind, các public path của Environment cùng nằm
  trong một Traefik Ingress; mỗi path trỏ đến Service/cổng đã khai báo. Sau
  khi mọi workload bị ảnh hưởng trong batch Deploy Ready, UC-06 reconcile
  toàn bộ Ingress từ Deployment Set đích. Direct mode dùng Kubernetes API;
  Fleet mode ghi bundle `_routes`, chờ GitRepo quan sát commit và Ingress có
  đúng revision trước khi báo thành công. Gỡ hết path sẽ xóa Ingress.
- **BR-16:** Deployment có thể chưa có Delta Snapshot khi `PLANNING`; nếu
  planning thất bại thì record `FAILED` có thể tiếp tục không có Snapshot.
  Trước khi chuyển sang `PROVISIONING`, Snapshot bất biến phải được lưu và gắn
  vào Deployment. Mọi trạng thái `PROVISIONING`, `DEPLOYING`, `SUCCEEDED` phải
  có Snapshot; một association đã thiết lập không được xóa hoặc đổi.
  Host staging là
  `staging.<subdomain>.<base-domain>`, production là
  `<subdomain>.<base-domain>`. Route không được coi là truy cập được từ bên ngoài cho tới khi DNS và
  đường vào Ingress Controller được cấu hình; TLS/cloud ingress nằm ngoài MVP.
  `service.publicRoutes` là extension của orchestrator trên Score subset hiện tại,
  không phải khẳng định field chuẩn của Score. `service.publicPort` không được nhận
  ở Score hoặc public Deployment Set projection, không rewrite hay fallback sang route `/`.
- **BR-15:** Nếu workload trong batch đã apply nhưng route reconcile lỗi,
  Environment giữ trạng thái `publicRoutesPending`. Preview kế tiếp cho phép
  retry riêng route từ current Deployment Set, không redeploy/restart workload;
  chỉ xóa trạng thái pending sau khi route được quan sát thành công.

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
workload -> postgres-aurora / redis-elasticache -> vpc
workload -> redis/postgres-in-cluster -> namespace -> eks -> vpc
```

Internal Kubernetes:

```text
workload -> k8s-namespace -> existing-k8s-cluster
workload -> postgres-in-cluster -> namespace -> existing-k8s-cluster
workload -> redis-in-cluster -> namespace -> existing-k8s-cluster
```

VPC/EKS có scope theo Environment và target generation; namespace có scope theo Environment; Redis/PostgreSQL có private/shared scope theo dependency trong Score.

Hai sơ đồ trên là hình dạng tối thiểu sinh ra từ Score và Execution Profile. Resource Definition được
phép bổ sung edge cho chính resource của nó bằng Resource Reference trong driver inputs
(`${resources['TYPE[.CLASS][#ID]'].outputs.NAME}`, UC-03 BR-04) và bằng provision rule; graph expansion
hợp nhất các edge đó vào cùng DAG. Hai edge bắt buộc theo cách này:

- `postgres-statefulset -> k8s-namespace` trên `internal-k8s`, vì StatefulSet/Service chỉ apply được
  sau khi namespace của Environment tồn tại.
- `postgres-aurora -> vpc` trên `aws-eks`, vì Aurora cần subnet group của VPC đã provision.

Edge do Definition khai báo không được đổi chiều `consumer -> provider` và vẫn phải giữ DAG (BR-04).

## Resource Descriptor convention

Resource ID là đường dẫn trong Deployment Set, không phải tên tự đặt:

| Node | Descriptor |
|---|---|
| Workload | `workload.default#modules.<workload-id>` |
| Private dependency của workload | `<type>.<class>#modules.<workload-id>.externals.<name>` |
| Shared dependency | `<type>.<class>#shared.<shared-id>` |
| VPC implicit (`aws-eks`) | `vpc.default#environments.<app-id>.<env-id>` |
| EKS implicit (`aws-eks`) | `k8s-cluster.eks#environments.<app-id>.<env-id>` |
| Cluster đã đăng ký (`internal-k8s`) | `k8s-cluster.internal#connections.<connection-id>` |
| Namespace của Environment | `k8s-namespace.default#environments.<app-id>.<env-id>` |

Ba dòng đầu là contract chung với Humanitec. Bốn dòng implicit là phần mở rộng của orchestrator và
dùng cùng quy ước `<scope>.<path>` để identity luôn suy ra được từ scope.

Theo BR-12, trong Resource Reference, ngoài `@` kế thừa class/ID của node hiện tại, orchestrator cho phép các token
`@app`, `@env`, `@connection` và `@infra` bên trong phần ID. Chúng được thay bằng Application key, Environment key
và Connection key đang xử lý; @infra là AWS infrastructure path theo ADR-011, nhờ vậy một Resource Definition tham chiếu hạ tầng implicit mà không phải
hard-code Application hay Environment.

## Ngoài phạm vi happy path

- **OOS-01:** Retry không giới hạn và timeout vô hạn. Retry transient có
  giới hạn, deadline và cancellation thuộc V2-05.
- **OOS-02:** Rollback tự động.
- **OOS-03:** Concurrent deployment.
- **OOS-04:** Partial failure recovery.
- **OOS-05:** Drift reconciliation.
- **OOS-06:** Scheduled deletion hoặc destroy ngầm trong Deploy. Xóa
  resource có xác nhận riêng thuộc UC-07 V2-03.
- **OOS-07:** RBAC chi tiết và approval workflow chung. Role gate PE/Admin
  cho Xóa tài nguyên và các điểm duyệt M04 vẫn bắt buộc.
- **OOS-08:** Humanitec-compatible standalone Delta API, asynchronous deployment và incremental deployment mode.

## Definition-selected rendering

- **BR-17:** Mỗi Workload cần một score-k8s Definition và bundle trên cả
  hai profiles. Pin lựa chọn trong Plan, kiểm runtime trước UC-08, rồi render
  sau outputs. Thiếu hoặc lỗi renderer thì dừng; không có renderer mặc định thay thế.
- **BR-18:** score-k8s receives normalized Score plus resolved output-only
  bindings and Secret references. Disable default provisioners; no database,
  PVC, namespace or cloud provisioning occurs in generation. Existing names,
  selectors, Secret delivery, probes, ports, replica and BR-11 semantics remain.
- **BR-19:** A selected renderer failure has no automatic fallback. Validate
  generated objects before apply; existing readiness/current-set/Fleet/route
  rules continue. Workspace state is disposable, private and never published.

See [workload rendering contract](../../architecture/contracts/workload-rendering.md).

## Environment connection binding

- **BR-20:** AWS execution pins account/region from each winning Definition
  under UC-03 V02-03/09, never from Environment or host. Same weighted matcher
  as server matching preview, without profile eligibility. Kubernetes consumer
  inherits scoped dependency target; explicit legacy Environment binding and
  system cluster ADR-013 remain until V03. Account/Definition edits stale prior
  Preview and never rewrite historical plan/resource/cleanup pins. Wrong winning
  account, stale identity or persisted state target mismatch fails before call,
  without another winner/fallback. Scope/generation/ownership/fencing remain.
  No invariant requires all nodes to share account/region/cluster.

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

## Implicit internal cluster binding

- **BR-21:** Internal-k8s creates an implicit existing-cluster provider from the pinned Environment Connection, bypasses user Definition matching for that node, and executes it through UC-08 before downstream Kubernetes consumers. Namespace/PostgreSQL and AWS VPC/EKS/Aurora retain matching.

See [ADR-013](../../architecture/decisions/ADR-013-implicit-existing-cluster.md).

Public `existing-cluster` registration is rejected before insert under UC-03
BR-08/BR-17. This does not remove the trusted internal driver, executor or
system Definition admission required by ADR-013. No retained records are
deleted by this API change.


## Retry, gián đoạn và kiểm chứng

- **V2-05:** Retry lỗi tạm thời tối đa một lần sau kiểm state/kết quả, trong
  deadline theo [state contract](../../architecture/contracts/terraform-state.md).
  Lỗi input/auth/permission hoặc chưa rõ kết quả thì dừng. Giữ stage/state/
  ownership và khóa khi chưa xác minh tiến trình cũ đã dừng; không tự resume/rollback.
- **V2-06:** Redeploy giữ identity/dữ liệu. Replacement cần Preview và xác nhận
  riêng theo UC-05 V2-04. Acceptance phải hoàn tất luồng job trên UI ứng dụng
  và chứng minh đọc/ghi Redis/PostgreSQL, ngoài việc Declare/Preview/Deploy qua
  form Console; không chỉ chứng minh Pod Ready. V2-A01 có bước xóa qua UI.
