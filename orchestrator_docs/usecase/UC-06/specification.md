---
id: UC-06-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-10-24
---

# UC-06 — Deploy Workload

## Mục tiêu

Triển khai Score workload qua matched Definition và references, chuẩn bị dependency theo graph và đưa workload lên đúng Kubernetes target đã pin. V03 internal contract theo [ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md).

## Primary actors

- Developer
- CI/CD System

## Supporting actors

- Resource Driver
- Kubernetes Cluster/Operator

## Tiền điều kiện

- **PRE-01:** Application và Environment đã tồn tại.
- **PRE-02:** Internal Preview/Deploy không cần Environment Connection/profile; workload, namespace và cluster Definitions xác định đích theo ADR-016. Legacy AWS binding còn đến V05–V06.
- **PRE-03:** AWS Driver Account hoặc internal cluster connection trạng thái `READY` đã được cấu hình trong UC-04.
- **PRE-04:** Các Resource Type và Resource Definition cần thiết đã được đăng ký.
- **PRE-05:** Score document hợp lệ, mô tả đúng một workload và mọi container resource requests/limits khai báo đều đúng Score subset.

## Trigger

**TRG-01:** Developer hoặc CI/CD gửi Score cùng Application ID và Environment ID cần triển khai.

## Main success scenario

1. **MS-01:** Orchestrator tạo Deployment record và đọc Deployment Set hiện tại của Environment.
2. **MS-02:** Orchestrator validate Score và chuyển nó thành workload fragment, bảo toàn container CPU/memory requests/limits.
3. **MS-03:** Orchestrator tạo Humanitec-shaped Deployment Delta và Candidate Deployment Set.
4. **MS-04:** Orchestrator chọn/pin workload Definition theo class trước graph expansion; Application/Environment cung cấp scope/version/configuration, không chọn internal account.
5. **MS-05:** Orchestrator mở references và provision rules của workload Definition, bao gồm namespace/cluster; legacy AWS enrichment theo VAR-01 chỉ còn đến V05.
6. **MS-06:** Orchestrator thêm namespace có identity theo Environment và resource dependencies từ Score.
7. **MS-07:** Orchestrator match mọi resource kể cả existing cluster bằng criteria chung, expand references/provision rules tới fixed point và pin đích từng resource/workload. Không đổi class, dùng builtin hoặc fallback khi matching thất bại.
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
- **VAR-02:** Internal: matched cluster Definition chọn Internal Driver Account; namespace/workload kế thừa typed cluster reference. V03 frontend không yêu cầu PostgreSQL/Redis; các workload dependency đó nghiệm thu tại V04.
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
- **BR-03:** Internal placement do Definition/reference quyết định theo ADR-016; không profile enrichment/system cluster. Legacy AWS enrichment được giữ tới V05, không trở thành matching filter.
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
- **BR-11:** `containers.*.resources.requests/limits` chỉ gồm `cpu`/`memory`, phải đi nguyên vẹn qua Score fragment, Candidate Deployment Set và workload renderer; renderer không được thay giá trị đã khai báo bằng giá trị hard-code. Request của từng field (`cpu`, `memory`) được chọn theo thứ tự: request đã khai báo; nếu request thiếu thì limit đã khai báo của cùng field; nếu thiếu cả hai thì default của bundle đã pin (legacy installed/reference Git bundle: `cpu: 10m` hoặc `memory: 32Mi`; Git bundle có thể khai báo default khác theo ADR-021). Limits chỉ chứa các field được khai báo và không có default. Giá trị suy ra không được ghi ngược vào Score fragment hoặc Candidate Deployment Set.
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

## Definition-selected internal Resource Graph and legacy AWS graph

Internal (`consumer -> provider`) follows authored references, not profile:

```text
workload Definition -> namespace -> matched existing-k8s-cluster
workload Definition -> companion resources / Score dependencies
```

Workload is selected first. Namespace provider binding and workload cluster
reference point to the same explicitly described cluster. The cluster winner
selects its own Internal account; no implicit connection-addressed node.
Scope and explicit descriptor identity remain deterministic. Output declarations
alone create no dependency; Resource References and provision flags do.

V04 adds real PostgreSQL/Redis consumers and business coverage on this graph.
For legacy AWS until V05–V06, enrichment still supplies environment/generation
VPC/EKS and dependencies use their authored mappings/Definition accounts:

```text
workload -> k8s-namespace -> eks -> vpc
workload -> postgres-aurora / redis-elasticache -> vpc
workload -> redis/postgres-in-cluster -> namespace -> eks -> vpc
```

Same DAG/provider-first, output validation, state/ownership/fencing and secret
rules apply. V03 does not execute this AWS path.

## Resource Descriptor convention

Resource ID là đường dẫn trong Deployment Set, không phải tên tự đặt:

| Node | Descriptor |
|---|---|
| Workload | `workload.default#modules.<workload-id>` |
| Private dependency của workload | `<type>.<class>#modules.<workload-id>.externals.<name>` |
| Shared dependency | `<type>.<class>#shared.<shared-id>` |
| VPC implicit (`aws-eks`) | `vpc.default#environments.<app-id>.<env-id>` |
| EKS implicit (`aws-eks`) | `k8s-cluster.eks#environments.<app-id>.<env-id>` |
| Matched internal cluster | `k8s-cluster.internal#environments.<app-id>.<env-id>` (hoặc explicit scoped descriptor trong Definition reference) |
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

## Definition account and target binding

- **BR-20:** AWS nodes pin winning Definition account/region under UC-03
  V02-03/09; internal clusters pin their matched existing-cluster Definition's
  Internal Driver Account. Consumers inherit their explicitly referenced
  cluster. Validate kind/org/READY/version and original persisted target before
  calls; no Environment/host fallback or alternative winner. Catalog edits stale
  Preview, never rewrite original execution/cleanup pins. No global same-account
  or same-region invariant. V03 same-cluster capability rejects multi-target
  Candidate Sets while preserving per-resource placement data.

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

- **BR-21:** Internal cluster uses a public existing-cluster Definition through
  normal five-field matching and its own pinned Internal Driver Account under
  ADR-016. It is PREEXISTING, never created/destroyed. Namespace/workload typed
  targets must agree; missing/ambiguous targets, cycles and V03 multiple-cluster
  Candidate Sets fail before execution. No builtin Definition admission.
  Reserved historical builtin IDs stay protected. Existing snapshots are not
  silently rewritten; no old-data migration is required.

## Retry, gián đoạn và kiểm chứng

- **V2-05:** Retry lỗi tạm thời tối đa một lần sau kiểm state/kết quả, trong
  deadline theo [state contract](../../architecture/contracts/terraform-state.md).
  Lỗi input/auth/permission hoặc chưa rõ kết quả thì dừng. Giữ stage/state/
  ownership và khóa khi chưa xác minh tiến trình cũ đã dừng; không tự resume/rollback.
- **V2-06:** Redeploy giữ identity/dữ liệu. Replacement cần Preview và xác nhận
  riêng theo UC-05 V2-04. Acceptance phải hoàn tất luồng job trên UI ứng dụng
  và chứng minh đọc/ghi Redis/PostgreSQL, ngoài việc Declare/Preview/Deploy qua
  form Console; không chỉ chứng minh Pod Ready. V2-A01 có bước xóa qua UI.


## V03 Generic Workload lifecycle

- **V03-01:** [ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) owns input shape, class, typed namespace/cluster references, safe outputs and lifecycle. Workload Definition chọn trước graph fixed point; resource params/references/companion flags giữ đầy đủ semantics.
- **V03-02:** Render sau dependency outputs, deploy/readiness trên đúng target; persisted target/namespace/Definition/account/bundle pins dùng lại sau restart và khi Remove. Không lấy cluster cuối graph.
- **V03-03:** Preview cho từng workload/resource Definition/account/cluster/namespace, edges/batches và lỗi; no secret/credential refs. Wrong account/permission/unready/ambiguous target không false success hoặc deploy sang cụm khác.
- **V03-04:** Replacement real happy path pass trước V03.10 bỏ Connection selector/internal legacy path. No migration, không reset tự động. Frontend-only acceptance và deferred V04 coverage theo bàn giao V03; không fake backend để claim pass.


A post-workload resource required by the executing workload must not be silently
deferred while reporting SUCCEEDED. If another workload provider is not Ready
in the correct target generation/Candidate context, fail safely and preserve
progress/current-set invariants. An old-generation READY instance is insufficient.
Workload lifecycle metadata/readiness is queried through existing status views;
V03 does not supply arbitrary workload Type output bindings (Type outputs empty).


## V04–V06 Definition-driven execution replacement

[ADR-017](../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces remaining profile enrichment and same-Environment-account assumptions
in PRE-02, MS-05, VAR-01, POST-02/03 and BR-03 as V05 is implemented.

- **V04-01:** Expand references/provision/provider edges to fixed point; retain
  flags and schedule providers before consumers. Real shared PostgreSQL/Redis
  outputs and Secret bindings precede render. Entire acceptance app must prove
  browser proxy/job submission→worker result, persistence over reload/redeploy/
  restart, Redis worker write/hit/miss/TTL/DB refill, config/secret/auth/errors.
- **V05-01:** Infrastructure exists in the plan only because of matched
  Definitions/references; same-execution EKS outputs may be unknown at Preview.
  Use typed completed cluster identity per consumer, original AWS account/region
  and fresh invocation authentication; API/auth/network/Ready node capacity
  must pass before namespace/workload. No global last-cluster inference.
- **V06-01:** Managed resources follow actual network dependencies and their own
  account/region. A02 then verified cleanup then A03 prove real app business
  behavior, isolation, lifecycle and owned cleanup; cross-account/region
  contract tests need not create more live accounts or regions.
- **V06-02:** Apply legacy AWS-profile removal only after V05 replacement PASS,
  final executionProfile removal only after corresponding V06 coverage PASS.
  Validate affected final source and preserve original state/cleanup pins.

## Review artifacts and execution observation

- **REV-01:** Admission tiếp tục so token/snapshot/pins trước side effects. Lưu
  safe desired review vào immutable Plan và admission-bound `review_request_id`
  trên Deployment; review failure không cho phép bỏ qua existing preflight.
- **REV-02:** Sau render/validation và trước Apply, persist sanitized manifest
  review đúng workload/deployment, không đổi bytes/digest dùng cho execution.
  Chỉ workload READY mới dùng artifact đó làm applied baseline. Failed/partial
  không thay successful baseline của workload khác. Theo ADR-019/review contract.
- **REV-03:** Tiến độ lấy từ persisted resource/attempt/workload records. Không
  tạo nhánh hoặc tăng concurrency chỉ để khớp hình mô phỏng; graph/admission/
  ownership/readiness/current-set guards hiện hành vẫn là nguồn execution.

## Git-owned manifest generation — ADR-021

- **RB-01:** Infrastructure matching, dependencies, Terraform/state/targets and
  provisioning stay unchanged. Before UC-08, validate cached render pin/engine
  and typed inputs. After outputs, execute Git-owned defaults/normalization,
  output-only provisioners and patches; no Git download during deployment.
- **RB-02:** Git-selected rendering must not call a native renderer to generate
  an authoritative hidden Deployment/Service or replace author changes. Git files
  own transformation, missing-value defaults and labels/patch policy. Backend
  supplies neutral input and validates explicit Score intent, approved references,
  identity, namespace, ownership and current supported object envelope.
- **RB-03:** Secret resolution/delivery remains platform-owned; templates only see
  authorized name/key references and safe delivery metadata. Same explicit Score
  resource requests/limits, probes, replicas, image, command/args and ports remain
  protected. BR-11 missing-value fallbacks are bundle-configurable. Original
  Score/Candidate are not rewritten. Existing Vault Agent/VSO behavior remains.
- **RB-04:** Render failure blocks Apply with no fallback. Successful validated
  result is persisted as safe review and passed unchanged to existing delivery,
  readiness and current-set commit. No new kinds, route owner, resource lifecycle
  or cleanup scope. Restart and in-flight executions use original immutable pins.

Exact stages and validation: [Git contract](../../architecture/contracts/git-render-bundles.md).
