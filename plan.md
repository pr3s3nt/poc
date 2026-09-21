# Orchestrator UP Development Plan

## 1. Mục tiêu

Hoàn thành Use Case Realization và thiết kế dùng chung cho toàn bộ UC-01 đến UC-09 theo Unified Process, vượt qua design gate, rồi mới code MVP. UC-06/UC-08 là architectural baseline được hiện thực đầu tiên sau gate.

## 2. Trạng thái hiện tại

- [x] Chốt phạm vi happy path UC-01 đến UC-09.
- [x] Đặc tả ngắn gọn cho cả 9 use case.
- [x] Chốt hai Execution Profile: `aws-eks` và `internal-k8s`.
- [x] Chốt cloud Application có VPC/EKS riêng; internal dùng cluster có sẵn.
- [x] Refactor specification vào `orchestrator_docs/usecase/UC-xx/specification.md`.
- [x] Copy planner challenge vào thư mục reference chỉ đọc.
- [x] Thiết lập workflow cho phiên làm việc mới.
- [x] Chốt chiến lược verify: internal path trên cụm `kind` có sẵn; cloud path bằng cloud identity cục bộ với tài nguyên chi phí tối thiểu và teardown ngay sau test.
- [x] Chốt ngôn ngữ implementation: lõi orchestrator/API/executor/CLI dùng Go; web console dùng React + TypeScript; không dùng Python cho code sản phẩm.
- [x] Chốt chuẩn diagram: toàn bộ diagram dùng PlantUML và lưu source `.puml`.
- [x] Hoàn thành Phase 1: chuẩn hóa 9 specification với ID ổn định, thuật ngữ, business rules và quan hệ use case.
- [x] Hoàn thành Phase 2: phân tích planner challenge, chạy baseline và ánh xạ vào UC-06/UC-08.
- [x] Hoàn thành Phase 3–5: realization, shared architecture, traceability và design gate.
- [x] Khởi tạo root Git repository trên branch `main`; local reference repositories/source duplicates được loại bằng `.gitignore`.
- [x] Phase 6 bước 1: local walking skeleton UC-06/UC-08/UC-09 với fake adapters và Orchestrator Web Console.
- [x] Phase 6 bước 2: internal happy path trên cụm kind thật, đã verify cleanup.
- [x] Phase 6 bước 3: cloud happy path trên AWS thật, đã verify cleanup.

## 3. Nguồn làm việc

- Yêu cầu chuẩn: [use case index](orchestrator_docs/usecase/README.md).
- Workflow bắt buộc: [AGENT.md](AGENT.md).
- UC-06 technical reference: `orchestrator_reference/humanitec-planner-challenge-v4/`.
- UP structure reference: `final_idp/`, branch `uc03-impl`; chỉ đọc.

## 4. Kế hoạch thực hiện

### Phase 1 — Chuẩn hóa specification — hoàn thành 2026-09-20

- [x] Rà soát UC-01 đến UC-09, không thêm use case ngoài phạm vi.
- [x] Gán ID ổn định cho precondition, trigger, main-flow step, happy-path variant, business rule, postcondition và out-of-scope exception.
- [x] Chuẩn hóa thuật ngữ Application, Environment, Execution Profile, Deployment, Resource Definition, Active Resource và scope.
- [x] Ghi rõ quan hệ include/trigger giữa UC-06, UC-07 và UC-08.
- [x] Kiểm tra cloud/internal path và các quyết định đã duyệt không mâu thuẫn.

Deliverable hoàn thành: 9 file `specification.md` và glossary/traceability convention trong `orchestrator_docs/usecase/README.md`.

### Phase 2 — Phân tích planner challenge cho UC-06 — hoàn thành 2026-09-20

- [x] Đọc `PROBLEM.md`, `SOURCES.md`, `TESTCASES.md` và `VALIDATION.md`.
- [x] Đọc toàn bộ source trong `starter-go/planner/`, entrypoint, invariant tests và grader.
- [x] Lần theo các fixture đại diện cho delta, matching, reference, co-provision, graph, Terraform contract, Active Resource và lỗi planning.
- [x] Chạy fixture validator, invariant/source tests và grader; ghi lại baseline.
- [x] Viết `orchestrator_docs/implementation/uc06-planner-reference.md` gồm reusable, challenge-only và missing for product.
- [x] Ánh xạ thuật toán vào từng bước UC-06/UC-08 mà chưa cố định class của realization.

Deliverable hoàn thành: `orchestrator_docs/implementation/uc06-planner-reference.md`.

Baseline đã kiểm chứng:

- `python3 validate_fixtures.py`: pass, 33 cases.
- `go test ./...` trong `starter-go`: pass.
- Build planner executable: pass.
- Grader chạy executable trên toàn bộ 33 cases: pass.
- Bản reference không bị thay đổi sau kiểm chứng (`diff -qr` sạch).

### Phase 3 — Use Case Realization cho UC-01 đến UC-09 — hoàn thành 2026-09-20

Thực hiện cho từng use case dựa trên specification hiện có:

- [x] Tạo `realization.md`: boundary/control/entity/repository/gateway, system operation và method signature.
- [x] Tạo `sequence.puml`: message, transaction boundary và external interaction bám step ID.
- [x] Tạo `vopc.puml`: class tham gia, trách nhiệm, method và dependency.
- [x] Review trace từ từng main-flow step sang operation và class.

Thứ tự thiết kế: UC-01 -> UC-02 -> UC-03 -> UC-04 -> UC-05 -> UC-06 -> UC-07 -> UC-08 -> UC-09. Đây là thứ tự hoàn thiện tài liệu, không phải thứ tự code.

UC-06 bắt buộc có hai sequence:

- Cloud: enrich VPC/EKS -> provision application infrastructure -> provision Aurora/resources -> propagate outputs -> render/apply workload lên EKS -> persist status.
- Internal: resolve registered cluster/namespace -> provision PostgreSQL StatefulSet/resources -> propagate outputs -> render/apply workload -> persist status.

Deliverable hoàn thành: mỗi UC có realization, sequence và VOPC nhất quán; UC-06 có thêm `sequence-cloud.puml` và `sequence-internal.puml`.

### Phase 4 — Shared architecture — hoàn thành 2026-09-20

- [x] Hợp nhất VOPC thành consolidated design class diagram.
- [x] Phân loại domain object và persistence object; định nghĩa aggregate boundary.
- [x] Xác định component/interface và dependency direction.
- [x] Thiết kế database schema, migration ordering/transaction model, constraints và ERD.
- [x] Viết operation contracts cho operation quan trọng.
- [x] Viết state machine cho Deployment, Active Resource, Workload Instance và Connection.
- [x] Ghi architectural decisions cho application-scoped EKS/VPC, transaction boundary và planner/executor separation.

Deliverable hoàn thành: shared baseline trong `orchestrator_docs/architecture/` và Go package layout trong `orchestrator_docs/implementation/package-layout.md`.

### Phase 5 — Traceability và design gate — hoàn thành 2026-09-20

- [x] Tạo ma trận `UC step -> system operation -> sequence -> class.method -> table/contract/state -> planned test`.
- [x] Rà gap, method trùng nghĩa, owner không rõ, transaction mơ hồ và dữ liệu không có nơi lưu.
- [x] Parse/render toàn bộ PlantUML và sửa link lỗi: 27/27 diagrams pass.
- [x] Chốt package/module layout dự kiến từ design class, không từ repository tham khảo.
- [x] Duyệt design gate: không còn gap P0; UC-06 cloud/internal và UC-08 có đường đi end-to-end.

Deliverable hoàn thành: `traceability/matrix.md`, `traceability/design-gate.md`; coverage 75/75 main-flow steps, design gate PASS.

### Phase 6 — Code MVP sau design gate

Phase này có **hai frontend khác nhau** và phải giữ ranh giới rõ:

1. **Orchestrator Web Console**: giao diện quản trị do người dùng platform sử dụng, nằm trong `frontend/`, gọi Go API.
2. **Acceptance application frontend**: workload mẫu được orchestrator triển khai lên Kubernetes để kiểm chứng UC-06/UC-08; đây không phải giao diện quản trị.

#### Orchestrator Web Console

Web console được làm ngay trong Phase 6, tham khảo cấu trúc và trải nghiệm của `final_idp/idp/frontend` nhưng không copy domain, API contract hoặc code nguyên trạng.

- Công nghệ: React + TypeScript strict + Vite; Vitest + Testing Library cho unit/component test.
- Ranh giới: source nằm trong `frontend/`; Go backend sở hữu `/api/v1/`, phục vụ production bundle dưới `/ui/`; Vite development server proxy `/api` về Go backend.
- Cấu trúc: `src/app/` cho shell/router, `src/features/<use-case>/` cho API client/draft/page/component, `src/shared/` cho HTTP transport/UI dùng chung và `src/styles/` cho design tokens.
- Giữ dependency ban đầu tối thiểu: History API router, React state/reducer và typed API client; chưa thêm router library, global state library hoặc UI framework nếu chưa có nhu cầu được chứng minh.
- Mọi màn hình phải có loading, empty, validation, success và API-error state phù hợp; backend vẫn là nơi kiểm tra business rule cuối cùng.
- Không làm login, RBAC hoặc secret-management UI trong MVP vì nằm ngoài scope đã chốt.
- Không commit `node_modules/`, `dist/` hoặc coverage output. Go server không cần Node.js khi chạy; Node chỉ dùng ở frontend build/test stage.

Phạm vi màn hình theo use case:

| Web console area | Use case | Chức năng MVP |
|---|---|---|
| Applications & Environments | UC-01 | List/create Application, Environment và chọn Execution Profile |
| Resource Types | UC-02 | List/create Resource Type và output contract |
| Resource Definitions | UC-03 | List/create Definition, criteria và driver inputs |
| Connections | UC-04 | Đăng ký/verify AWS identity hoặc internal Kubernetes target, không hiển thị secret |
| Preview | UC-05 | Nhập/upload Score, xem delta, matches, graph và batches trước deploy |
| Deploy | UC-06, UC-08 | Khởi chạy deployment và theo dõi resource/workload execution |
| Update/Remove | UC-07 | Tạo deployment mới từ thay đổi workload hoặc remove request |
| Deployment details | UC-09 | Xem status, graph, batches, Active Resources, workloads và redacted outputs |

UC-08 không có màn hình thao tác riêng; tiến trình provision được trình bày trong Deploy/Deployment details. JSON API contract là ranh giới duy nhất giữa web console và Go backend.

#### Acceptance application

Acceptance application của MVP gồm:

- `frontend`: Go Kubernetes workload phục vụ giao diện mẫu và gọi `backend` qua Service nội bộ.
- `backend`: Go Kubernetes workload cung cấp API, đọc/ghi database và cung cấp health endpoint.
- `worker`: Go Kubernetes workload xử lý job nền và đọc/ghi cùng database với backend.
- `database`: shared Score resource có Resource Type `postgres`; dùng PostgreSQL StatefulSet/Service trên `internal-k8s` và Aurora PostgreSQL trên `aws-eks`.

Frontend, backend và worker là ba workload độc lập trong cùng Application/Environment. Tên `frontend` ở đoạn này luôn chỉ **acceptance workload**, không phải Orchestrator Web Console. Backend và worker cùng tham chiếu database bằng một shared resource ID ổn định. Happy-path verification phải chứng minh frontend gọi được backend, backend kết nối được database và worker xử lý được ít nhất một job có kết quả quan sát được qua backend/frontend. Để giảm chi phí cloud, không tạo public load balancer nếu port-forward hoặc Kubernetes API proxy đã đủ cho verification.

Thứ tự code theo rủi ro kiến trúc:

1. UC-06 + UC-08 walking skeleton với fixture/seed cho acceptance frontend, backend, worker, shared database và fake adapter. Đồng thời tạo React shell, typed API transport và trang Deploy/Deployment details chạy bằng fixture/mock API.
2. UC-06 + UC-08 internal happy path với ba acceptance workload trên Kubernetes thật và PostgreSQL StatefulSet; nối trang Deploy/Deployment details vào Go API thật.
3. UC-06 + UC-08 cloud happy path với VPC, EKS, Aurora và ba acceptance workload được triển khai lên EKS; tái sử dụng cùng UI, chỉ khác Execution Profile và dữ liệu trạng thái.
4. UC-09 hoàn thiện truy vấn và giao diện quan sát deployment/resource state, graph, batches và redacted outputs.
5. UC-01 đến UC-04 thay fixture bằng dữ liệu quản trị thật và bổ sung lần lượt các màn hình Applications/Environments, Resource Types, Resource Definitions và Connections.
6. UC-05 preview dùng cùng planner pipeline và có màn hình Preview hiển thị kết quả trước deploy.
7. UC-07 update/remove, reuse resource state và bổ sung thao tác tương ứng trong Deployment details.

Mỗi bước code phải có Go test bám traceability matrix, frontend unit/component test cho trạng thái UI liên quan và cập nhật tài liệu nếu phát hiện giả định thiết kế sai. Mỗi increment chỉ hoàn thành khi `go test ./...`, frontend typecheck/lint/test/build và test tích hợp tương ứng đều pass.

#### Cấu trúc code sản phẩm

Toàn bộ code sản phẩm nằm trong `implementation/` tại repository root; Go module tên `orchestrator`.
`orchestrator_docs/implementation/` chỉ là tài liệu thiết kế. Layout chi tiết nằm trong
[package-layout.md](orchestrator_docs/implementation/package-layout.md).

#### Phase 6 bước 1 — Local walking skeleton — hoàn thành 2026-09-21

- [x] Go walking skeleton: HTTP API -> DeploymentService -> Planner -> ResourceProvisioningService -> Fake ResourceExecutor -> outputs -> WorkloadRenderer -> Fake WorkloadDeployer -> repository -> status API.
- [x] Seed/fixture cho Organization, Application, Environment, Resource Type, Resource Definition và Connection (`implementation/internal/seed`).
- [x] Fixture Score cho acceptance frontend/backend/worker và shared PostgreSQL `acceptance-db`.
- [x] Planner sinh resource graph, matches và provider-first batches; workload node không nằm trong resource batches.
- [x] Fake ResourceExecutor trả database outputs đúng contract `postgres`.
- [x] Database outputs được inject vào backend và worker manifests; password đi qua Kubernetes Secret, không inline trong Deployment.
- [x] Fake WorkloadDeployer ghi nhận frontend, backend và worker.
- [x] Deployment, DeploymentPlan, Active Resources và Workload Instances được persist; status cuối `SUCCEEDED`.
- [x] API trả graph, batches, resources, workloads và redacted outputs.
- [x] Orchestrator Web Console: React + TypeScript strict + Vite, shell + History API router, typed same-origin client `/api/v1/`, trang Deploy và Deployment Details với loading/empty/validation/success/API-error states.
- [x] Go backend phục vụ production bundle dưới `/ui/` và browser fallback route.

**Code đã thực hiện (bước 1)**

| Vùng | Package |
|---|---|
| Domain | `internal/domain/{application,environment,resource,deployment}` |
| Planning | `internal/planning` + `jsonpatch`, `placeholder`, `score` |
| UC-06/UC-09 | `internal/application/deployment` |
| UC-08 | `internal/application/provisioning` |
| Ports | `internal/ports/{persistence,execution}` |
| Adapters | `internal/adapters/{store,fake,kubernetes,secrets}` |
| Delivery | `internal/delivery/http`, `cmd/orchestrator` |
| Seed | `internal/seed` |
| Web Console | `frontend/src/{app,shared,features,styles,test}` |

**Phần còn dùng fake sau bước 1**

- `internal/adapters/fake`: ResourceExecutor và WorkloadDeployer.
- `internal/adapters/terraform` và phần Kubernetes executor/deployer chưa được wire (`bootstrap.realAdapters` trả lỗi có chủ đích).
- State store là in-memory + JSON snapshot (`internal/adapters/store`); adapter PostgreSQL hoãn sang bước 5.

**Lệnh test đã chạy và kết quả (bước 1)**

| Lệnh | Kết quả |
|---|---|
| `cd implementation && go build ./... && go vet ./...` | pass |
| `cd implementation && go test ./...` | pass, 10 package có test |
| `cd implementation/frontend && npm ci` (lần đầu `npm install`) | pass |
| `npm run typecheck` | pass |
| `npm run lint` | pass |
| `npm test` | pass, 23 test + 1 live test skip |
| `npm run build` | pass, `dist/` 236 kB JS |
| Live HTTP smoke (`orchestrator -adapters fake`) | pass |
| `ORCHESTRATOR_LIVE_URL=... npx vitest run src/test/live-console.test.tsx` | pass |

**Runtime evidence (bước 1)**

- Ba deployment thật qua HTTP: `backend`, `worker`, `frontend`, tất cả `SUCCEEDED`.
- Batches: `[k8s-cluster.internal#internal-cluster]`, `[k8s-namespace.default#acceptance-dev]`, `[postgres.default#acceptance-db]`.
- Deployment Set cuối chứa ba module; `backend` và `worker` cùng tham chiếu shared `acceptance-db`.
- Workload instances `backend`, `worker`, `frontend` đều `READY`.
- `postgres` outputs hiển thị `password = ***redacted***`, các output khác hiển thị bình thường.
- `/ui/` trả 200, asset trả 200, `/ui/deployments/<id>` trả `index.html` (browser fallback).
- Web Console chạy trên jsdom gọi Go API thật, submit deployment và hiển thị Deployment Details.
- Process orchestrator đã dừng; binary và log nằm trong thư mục tạm ngoài repository.

**Thay đổi tài liệu thiết kế phát sinh từ implementation (bước 1)**

1. `usecase/UC-06/specification.md`: ghi rõ Resource Definition được phép thêm edge `postgres-statefulset -> k8s-namespace` (internal) và `postgres-aurora -> vpc` (cloud) qua resource reference; StatefulSet không thể apply trước namespace.
2. Quyết định tạm thời khi làm bước 1 từng thêm `execution_profile` vào `matching_criteria`; quyết định này đã bị thay thế ở bước 3b bằng năm field chuẩn Humanitec và dùng `app_id` để phân biệt hai Application/profile.
3. `implementation/package-layout.md`: cập nhật theo thư mục `implementation/`, thêm `internal/adapters/store`, `internal/seed`, `internal/bootstrap` và ghi rõ transport `kubectl`/`terraform` CLI, Score contract dạng JSON.

#### Phase 6 bước 2 — Internal Kubernetes bằng kind — hoàn thành 2026-09-21

- [x] Phát hiện cụm bằng `kind get clusters`: `idp-internal`, context `kind-idp-internal`, Kubernetes v1.36.1, một node control-plane.
- [x] Không tạo cluster mới; không sửa namespace hoặc workload ngoài phạm vi test.
- [x] Namespace riêng theo run ID: `acceptance-kind-20260920172929-16103`.
- [x] Build ba image acceptance (static Go binary + scratch) và `kind load docker-image`; không dùng registry ngoài.
- [x] Kubernetes adapters thật: `k8s-namespace` executor, PostgreSQL StatefulSet + Service executor, existing-cluster adapter, workload renderer, apply và readiness observation qua `kubectl`.
- [x] Chạy UC-06/UC-08 internal happy path cho ba workload; mỗi deployment `SUCCEEDED`.
- [x] Job end-to-end: frontend -> backend -> PostgreSQL -> worker -> backend -> frontend.
- [x] Web Console hiển thị deployment thành công cùng resources và workloads.
- [x] Cleanup namespace theo run ID và xác minh không còn object mang run ID.

**Code đã thực hiện (bước 2)**

| Vùng | Nội dung |
|---|---|
| `internal/adapters/kubernetes/kubectl.go` | transport `kubectl` dùng chung cho kind và EKS |
| `internal/adapters/kubernetes/executor.go` | namespace + PostgreSQL StatefulSet/Service; password sinh một lần rồi tái sử dụng từ Secret |
| `internal/adapters/kubernetes/existing.go` | existing-cluster adapter, verify reachability và trả cluster target |
| `internal/adapters/kubernetes/deployer.go` | apply manifests và `rollout status` |
| `internal/bootstrap/adapters.go` | executor registry theo Driver Type |
| `examples/acceptance-app/` | frontend, backend, worker và `internal/jobstore` dùng chung PostgreSQL |
| `test/integration/kind_test.go`, `build-images.sh`, `kind-verify.sh` | integration test và driver script |

**Lệnh test đã chạy và kết quả (bước 2)**

| Lệnh | Kết quả |
|---|---|
| `go build ./... && go vet ./...` | pass |
| `go test ./...` | pass |
| `test/integration/build-images.sh` | pass, ba image scratch |
| `kind load docker-image` cho ba image | pass |
| `go test -tags integration ./test/integration/` | pass, `TestKindInternalHappyPath` 31.70s |
| `npx vitest run src/test/live-console.test.tsx` (read-only, deployment thật) | pass |

**Runtime evidence (bước 2)**

- kind context `kind-idp-internal`, cluster `idp-internal`, namespace `acceptance-kind-20260920172929-16103`.
- Pods: `acceptance-db-0`, `backend-*`, `frontend-*`, `worker-*` đều `Running` và `1/1`.
- `statefulset.apps/acceptance-db` 1/1; Services `acceptance-db`, `backend`, `frontend`; PVC `data-acceptance-db-0` 1Gi `Bound`.
- Worker log: `processed job 1`; kết quả job `processed:KIND-1789925419797733062` đọc lại được qua backend và hiển thị trên trang frontend.
- Secrets trong namespace: `acceptance-db-credentials`, `backend-env`, `worker-env`; Deployment `backend` không chứa `PGPASSWORD` dạng plaintext (test assert).
- Deployment view API trả `password = ***redacted***`; `/ui/deployments/<id>` trả 200.

**Lỗi và cách xử lý (bước 2)**

1. `kind load docker-image postgres:16-alpine` lỗi với image multi-platform (`content digest ... not found`). Xử lý: load bằng `docker save` + `kind load image-archive`, và nếu vẫn lỗi thì để kubelet tự pull.
2. Acceptance frontend panic khi khởi động: `pattern "/api/" conflicts with pattern "GET /"`. Xử lý: đổi route gốc thành `GET /{$}` rồi build lại; đã smoke binary cục bộ trước khi chạy lại kind.

**Cleanup evidence (bước 2)**

- `kubectl delete namespace acceptance-kind-20260920172929-16103` chạy trong trap `EXIT`.
- `kubectl get ns -o name | grep acceptance` -> không còn.
- `kubectl get all,pvc,secret,ns -A -l orchestrator.io/run-id` -> rỗng.
- PersistentVolume còn lại thuộc namespace `idp` của người dùng (tạo 2026-09-16), không thuộc test.
- Đã xóa image `acceptance-*` mang tag run ID khỏi Docker local.
- Không xóa kind cluster; các namespace khác giữ nguyên.

#### Phase 6 bước 3 — AWS cloud happy path — hoàn thành 2026-09-21

- [x] Chọn identity tự động: không có `AWS_PROFILE`, dùng default credential chain; region lấy từ profile (`us-east-1`).
- [x] Ghi nhận account ID, principal ARN và region; không in hoặc lưu credential.
- [x] Tra giá bằng AWS Pricing API và Spot price history trước `apply`; chọn cấu hình rẻ nhất vẫn chạy được ba workload.
- [x] Terraform execution với state riêng theo run ID trong thư mục tạm ngoài repository.
- [x] Automated plan review: mỗi module chỉ được tạo resource type nằm trong allowlist và không vượt giới hạn số lượng.
- [x] VPC application-scoped, EKS application-scoped, một managed node group, Aurora PostgreSQL Serverless v2.
- [x] ECR tạm theo run ID; build và push image `linux/arm64` cho node Graviton.
- [x] Orchestrator tự bổ sung implicit VPC/EKS; batches provider-first.
- [x] Inject Aurora outputs vào backend/worker; password chỉ đi qua Kubernetes Secret.
- [x] Deploy frontend/backend/worker lên EKS và chờ ready.
- [x] Không tạo public load balancer; verify bằng `kubectl port-forward`.
- [x] Job end-to-end qua Aurora; Web Console hiển thị deployment, graph, batches, Active Resources, workloads và redacted outputs.
- [x] Cleanup ngay sau khi đủ evidence; xác minh bằng AWS API theo tag/run ID.

**Code đã thực hiện (bước 3)**

| Vùng | Nội dung |
|---|---|
| `internal/adapters/terraform/executor.go` | Terraform executor: workspace/state theo descriptor, tfvars typed, plan review tự động, apply plan đã review, Spot -> On-Demand fallback, `aws eks update-kubeconfig` |
| `internal/adapters/terraform/modules/vpc` | VPC, IGW, 2 public subnet, route table; không NAT Gateway |
| `internal/adapters/terraform/modules/eks` | IAM role cluster/node, EKS cluster, một managed node group |
| `internal/adapters/terraform/modules/aurora` | DB subnet group, security group, Aurora cluster Serverless v2, một writer |
| `internal/seed` | `CloudConfig` cho version/instance/capacity/ACU; definitions `vpc-aws`, `cluster-aws-eks`, `postgres-aws-aurora` |
| `test/integration/costreport` | tra giá Pricing API + Spot, chọn instance/capacity và sinh cấu hình |
| `test/integration/aws_test.go` | `TestAWSCloudHappyPath` |
| `test/integration/aws-verify.sh`, `aws-cleanup.sh` | driver và cleanup có trap |

**Cấu hình cloud đã chọn**

| Hạng mục | Giá trị | Lý do |
|---|---|---|
| Region | `us-east-1` | region của profile hiện có |
| EKS version | `1.34` | đang ở standard support (hết 2026-12-02) |
| Control plane | EKS thường | không dùng Auto Mode, không Provisioned Control Plane |
| Node group | 1 group, 1 node `t4g.small`, `SPOT` | rẻ nhất vẫn đủ pod limit (11 pod/node) cho ba workload + CoreDNS |
| Node AMI | `AL2023_ARM_64_STANDARD` | node Graviton; image acceptance build `linux/arm64` |
| Node disk | 20 GiB gp3 | nhỏ nhất đáp ứng AMI |
| Aurora | Serverless v2, min 0 ACU, max 1 ACU, Aurora Standard, engine 16.14 | không read replica, không I/O-Optimized |
| Network | 2 public subnet + IGW | không NAT Gateway, không load balancer, không bastion |
| Verify | `kubectl port-forward` | không tạo public endpoint |

**Cost estimate (AWS Pricing API, 2026-09-20)**

| Thành phần | Đơn giá | Số lượng | USD/giờ |
|---|---|---|---|
| EKS control plane | $0.1000/giờ | 1 | $0.1000 |
| Node `t4g.small` SPOT | $0.0053/giờ | 1 | $0.0053 |
| EBS gp3 20 GiB | $0.0800/GB-tháng | 20 | $0.0022 |
| Aurora Serverless v2 | $0.1200/ACU-giờ | 0.5 ACU | $0.0600 |
| Public IPv4 | $0.0050/giờ | 1 | $0.0050 |
| **Tổng** | | | **$0.1725** |

Quota: account chưa có EKS/RDS/EC2 nào trước khi chạy, và `terraform plan` + `apply` cho một cluster, một node và một Aurora writer đều thành công, nên không chạm giới hạn service quota; không gọi thêm API quota riêng.

Giả định: min capacity là 0 ACU nhưng ước lượng tính 0.5 ACU vì writer phục vụ traffic trong lúc test.
Ước lượng 2 giờ: $0.34. Thời gian tài nguyên thực tế tồn tại khoảng 45 phút (apply 17:45:34Z -> cleanup xong ~18:30Z),
nên chi phí thực tế ước tính khoảng **$0.13**.

**Runtime evidence (bước 3)**

- Run ID `aws-20260920174531`; local AWS identity đã được xác minh nhưng account/principal cụ thể không được commit; region `us-east-1`.
- Tags trên mọi tài nguyên: `project`, `owner`, `environment`, `run-id`, `expires-at=2026-09-20T19:45:34Z`, `managed-by=orchestrator-verification`.
- Batches: `[vpc.default#acceptance]`, `[k8s-cluster.eks#acceptance, postgres.default#acceptance-db]`, `[k8s-namespace.default#acceptance-aws-20260920174531]`.
- Resource IDs không nhạy cảm: VPC `vpc-0a36a99fcca16de97`; subnets `subnet-0a1bf86ee2ff9d174`, `subnet-08b8d957f47a6fdfa`; EKS `acceptance-aws-20260920174531` (v1.34, ACTIVE); node group `acceptance-aws-20260920174531-ng`; Aurora cluster `acceptance-aws-20260920174531` (engine 16.14, min 0 / max 1 ACU, auto-pause 3600s).
- Node: `ip-10-42-17-67.ec2.internal`, `v1.34.11-eks-a887778`, Amazon Linux 2023 aarch64.
- Thời gian deployment: backend 21m9s (gồm tạo VPC/EKS/Aurora), worker 1m56s, frontend 1m55s; tổng test 1513s.
- Job flow: `processed:AWS-1789928029900686801`; worker log `processed job 1` với host Aurora `...cluster-ck9e28c6chk7.us-east-1.rds.amazonaws.com`.
- Deployment view: status `SUCCEEDED`, profile `aws-eks`, `postgres` outputs có `password = ***redacted***`.
- Web Console: `/ui/deployments/<id>` trả 200; live console test đọc deployment thật và assert resources/workloads/batches.
- Không có Service kiểu LoadBalancer (test assert).

**Cleanup evidence (bước 3)**

Thứ tự: dừng port-forward -> xóa namespace Kubernetes -> kiểm tra không có Service LoadBalancer -> xóa 3 ECR repository (kèm image) -> `terraform destroy` theo thứ tự ngược `aurora -> eks -> vpc` bằng đúng state của run.

Truy vấn xác minh sau destroy (tất cả rỗng): vpc, subnets, security-groups, nat-gateways, internet-gateways, ec2-instances, ebs-volumes, elastic-ips, route-tables, eks-clusters, rds-clusters, rds-instances, db-subnet-groups, ecr-repositories, load-balancers (v2 và classic), iam-roles.

Kiểm tra độc lập toàn region sau cleanup: `eks list-clusters` rỗng, `rds describe-db-clusters`/`describe-db-instances` rỗng, `ecr describe-repositories` rỗng, không còn EC2 instance/EBS volume/Elastic IP/load balancer; chỉ còn default VPC `vpc-074a751c57b841c62` có sẵn từ trước. `ap-southeast-1` cũng không còn EKS/RDS. Image local và ECR login đã xóa.

**Lệnh test đã chạy và kết quả (bước 3)**

| Lệnh | Kết quả |
|---|---|
| `go run ./test/integration/costreport` | pass, $0.1725/giờ |
| `terraform init -backend=false && terraform validate` cho vpc/eks/aurora | pass |
| `terraform plan` + plan review tự động cho từng module | pass, không có resource ngoài allowlist |
| `go test -tags integration -run TestAWSCloudHappyPath ./test/integration/` | pass, 1513.05s |
| `npx vitest run src/test/live-console.test.tsx` (deployment cloud thật) | pass |
| `bash test/integration/aws-cleanup.sh` | pass, 17/17 truy vấn sạch |

**Lỗi và cách xử lý (bước 3)**

1. Tên image ECR ban đầu không khớp tên repository có run ID. Xử lý: truyền `FRONTEND_IMAGE`/`BACKEND_IMAGE`/`WORKER_IMAGE` tường minh cho integration test.
2. `performance_insights_enabled` đặt ở `aws_rds_cluster` có rủi ro không hợp lệ. Xử lý: chỉ giữ ở `aws_rds_cluster_instance`.

**Phần còn dùng fake sau bước 3**

- `internal/adapters/fake` chỉ còn dùng cho walking skeleton và unit test.
- State store vẫn là in-memory + JSON snapshot; adapter PostgreSQL và UC-01..UC-05, UC-07, UC-09 UI đầy đủ vẫn thuộc các bước sau.

#### Phase 6 bước 3b — Sửa theo review contract Humanitec — hoàn thành 2026-09-21

Review sau bước 3 chỉ ra ba vấn đề; cả ba đã được kiểm chứng lại trong bản reference
`orchestrator_reference/humanitec-planner-challenge-v4/` trước khi sửa.

**1. Web Console/API chưa deploy được lên AWS**

- Nguyên nhân: `cmd/orchestrator` nhận `-region` nhưng không truyền `Region`, `Tags`, `KubectlPath`
  và `TerraformPluginCache` vào `bootstrap.Options`, nên `-adapters aws` chết ở
  `terraform: an AWS region is required`.
- Sửa: wire đủ các trường, thêm `-owner`, `-ttl`, `-cloud-namespace`, `-terraform`, `-kubectl`,
  `-terraform-plugin-cache`; `-adapters aws` bắt buộc có `-region`, `-terraform-root` và `-run-id`.
- Thêm `test/integration/deployctl`: gửi deployment qua `POST /api/v1/deployments`. Cả kind và AWS verification giờ đi qua HTTP thay vì gọi thẳng service.

**2. Resource ID chưa giống Humanitec**

Descriptor giờ dùng đường dẫn trong Deployment Set:

| Node | Trước | Sau |
|---|---|---|
| Workload | `workload.default#backend` | `workload.default#modules.backend` |
| Private dependency | `postgres.default#acceptance-dev-backend-db` | `postgres.default#modules.backend.externals.db` |
| Shared dependency | `postgres.default#acceptance-db` | `postgres.default#shared.acceptance-db` |
| VPC implicit | `vpc.default#acceptance` | `vpc.default#applications.acceptance-cloud` |
| EKS implicit | `k8s-cluster.eks#acceptance` | `k8s-cluster.eks#applications.acceptance-cloud` |
| Cluster đã đăng ký | `k8s-cluster.internal#internal-cluster` | `k8s-cluster.internal#connections.internal-cluster` |
| Namespace | `k8s-namespace.default#acceptance-dev` | `k8s-namespace.default#environments.acceptance.dev` |

Deployment Set cũng đổi sang shape Humanitec: `modules.<id>.{profile,spec,externals}` và `shared.<id>`;
placeholder trong `spec` là `${externals.<name>.<output>}` và `${shared.<id>.<output>}`. Scope của
Active Resource được suy ra từ chính đường dẫn identity, không còn map thủ công.

**3. Matching Criteria chưa giống Humanitec**

- Trước: sáu field riêng (`executionProfile`, `environmentType`, `resourceClass`, `applicationKey`,
  `environmentKey`, `resourceId`) với trọng số 1/2/4/8/16/32 — thứ tự ưu tiên khác Humanitec.
- Sau: đúng năm field chuẩn `env_type=1`, `app_id=2`, `env_id=4`, `res_id=8`, `class=16`. Bỏ field
  riêng cho Execution Profile; profile gắn cố định vào Application nên `app_id` phân biệt được
  Aurora với StatefulSet. Seed đăng ký hai Application (`acceptance` internal-k8s và
  `acceptance-cloud` aws-eks) nên một process phục vụ được cả hai và Web Console liệt kê cả hai.

**Bổ sung từ planner challenge (mục 3 của review)**

| Khả năng | Hiện thực |
|---|---|
| Score `params` | Validate theo `inputs` của Resource Type, vào Deployment Set entry rồi thành resource input của node; `params` có thể tham chiếu resource khác và tạo edge |
| Resource Reference trong driver inputs | `${resources['TYPE[.CLASS][#ID]'].outputs.NAME}` và dạng rút gọn, kế thừa `@` cùng token mở rộng `@app`/`@env`/`@connection` |
| Co-provision | `provision` rules của Definition tạo resource đi kèm, kèm `is_dependent` |
| `match_dependents` | Nối mọi consumer của node cha sang resource đi kèm, áp lại sau mỗi vòng expansion |
| Fixed-point expansion | Vòng match/expand tới khi không còn node hoặc edge mới, giới hạn 32 vòng |
| Terraform contract tại planning | `internal/adapters/terraform/inspect.go` parse module nhúng bằng `hashicorp/hcl/v2`; kiểm tra variable thừa/thiếu và output theo Resource Type |
| Terraform source fingerprint | `sha256` trên nội dung module đã sort, lưu trong plan snapshot |
| Output validation | Reference tới Terraform node kiểm tra theo output của module; node khác kiểm tra theo Resource Type |

Terraform module output được đổi tên theo đúng output contract (`id`/`cidr`/`subnetIds`,
`name`/`endpoint`, `host`/`port`/`database`/`username`/`password`), nên executor truyền thẳng
variable và output, không còn map tay — đúng thứ planner đã kiểm tra.

**Chưa làm (người dùng quyết định hoãn)**

- Terraform state vẫn nằm trong thư mục tạm theo run ID, chưa có backend bền vững.
- Tên VPC/EKS/Aurora vẫn chứa run ID (`${context.app.id}-${context.run.id}`). Identity logic đã ổn định
  theo Application (`applications.<app>`), nhưng tên vật lý còn gắn run ID nên mỗi lần verify tạo
  hạ tầng mới. Hai việc này đi cùng nhau ở bước sau.

**Test đã chạy lại sau khi sửa**

| Lệnh | Kết quả |
|---|---|
| `go build ./... && go vet ./...` | pass |
| `go test ./...` | pass, 11 package có test |
| `npm run typecheck && npm run lint && npm test && npm run build` | pass, 23 test |
| `bash test/integration/kind-verify.sh` | pass, run `kind-20260921051906-1105` |
| `bash test/integration/aws-verify.sh` | pass, run `aws-20260921052038` |

**Runtime evidence sau khi sửa**

- kind: deployment qua HTTP, `shared-acceptance-db` StatefulSet/Service, job `processed:KIND-...`,
  namespace `acceptance-kind-20260921051906-1105` đã xóa và xác minh sạch.
- AWS: ba deployment qua `POST /api/v1/deployments` trên binary chạy `-adapters aws` —
  backend 21m4s, worker 2m0s, frontend 1m54s, tất cả `SUCCEEDED`.
- EKS `acceptance-cloud-aws-20260921052038` v1.34 ACTIVE, node `t4g.small` SPOT arm64,
  Aurora `acceptance-cloud-aws-20260921052038` engine 16.14, namespace `acceptance-aws-20260921052038`.
- Job flow: `processed:AWS-1789969672664387632`, worker log `processed job 1` với host Aurora.
- Web Console: `/ui/deployments/<id>` 200, live console test đọc deployment cloud thật.
- Cleanup: 17/17 truy vấn theo run ID sạch; kiểm tra độc lập toàn region không còn EKS/RDS/ECR/EC2/EBS/VPC.

#### Phase 6 bước 3c — Sửa theo review lần hai — hoàn thành 2026-09-21

**1. Web Console không gửi `runId`**

Đúng: `CreateDeploymentInput` không có `runId`, `DeployPage` không gửi, API truyền thẳng `req.RunID`
nên tên cloud resource thành `acceptance-cloud-` thiếu hậu tố. Sửa: API tự dùng `s.seedOptions.RunID`
khi request bỏ trống, và `deployctl` **bỏ** cờ `-run-id` để body gửi đi giống hệt body của Deploy page.
Process sở hữu run ID qua `-run-id`, console không cần biết. Thêm test
`TestHTTPUsesTheConfiguredRunID`: deploy cloud application bằng body không có `runId` rồi assert
`resolvedInputs.name` của node `k8s-cluster` kết thúc bằng run ID đã cấu hình.

Câu mô tả cũ trong mục 3b đã được sửa lại cho đúng.

**2. Hai quy tắc planner còn thiếu**

- Before-state chỉ so module. Sửa: so thêm từng shared entry mà `before Score` khai báo với
  `shared` hiện tại (UC-07 BR-01 mở rộng).
- Shared resource trùng ID nhưng khác nội dung bị ghi đè. Sửa: báo lỗi xung đột, trừ khi chính
  workload đó đã khai báo shared ID này trong `before Score` (UC-07 BR-05).
- Bổ sung: workload thôi khai báo shared resource thì entry bị bỏ, **trừ khi** module khác còn
  tham chiếu (UC-07 BR-06). Đây là chỗ sản phẩm cố ý khác challenge, vì challenge xóa ngay và
  như vậy vi phạm UC-07 BR-03.

Bốn test mới trong `internal/planning`: before-state shared lệch, shared conflict, shared bị bỏ khi
không còn ai tham chiếu, shared được giữ khi worker còn tham chiếu.

**3. Chạy 33 fixture trên planner sản phẩm**

Thêm `implementation/test/conformance`: load từng fixture của
`orchestrator_reference/humanitec-planner-challenge-v4` vào `planning.Request` của sản phẩm rồi so
với expected. **33/33 case pass.** So sánh gồm:

- Candidate Deployment Set so khớp nguyên văn với `expected/deployment-set.yaml`.
- Resource graph nodes (descriptor, origins, resourceInputs) và edges (from/to/reason/path).
- Matched Definitions (definitionId + score) cho resource node.
- Thứ tự batch tôn trọng mọi cạnh của expected.
- Active Resource classification.
- Terraform contract: fingerprint, outputs, và từng input với `source`/`type`/`value`.

Hai khác biệt có chủ đích được lọc ra và ghi rõ trong package:

1. Workload node không được match Definition và không nằm trong provision batch, vì UC-08 chỉ execute
   resource node (quyết định đã duyệt trong AGENT.md mục 5).
2. Orchestrator thêm namespace và cluster implicit theo Execution Profile; challenge không có khái niệm này.

Để chạy được fixture, planner sản phẩm đã được chỉnh cho sát contract hơn:

- Container trong Deployment Set mang `id` trùng key; `replicas` chỉ xuất hiện khi Score khai báo.
- `is_dependent` tạo edge không có `path`, giống reference.
- Driver variables chỉ resolve `${context.*}`; Resource Reference giữ nguyên văn bản gốc.
- Terraform contract ghi `value` của từng input, kể cả default lấy từ module.
- Module key suy ra từ `source.module`, hoặc `url[@rev][/path]` cho source từ xa.

`gopkg.in/yaml.v3` chỉ được dùng trong harness conformance, không nằm trong đường chạy sản phẩm.

**4. `implementation/` chưa được Git theo dõi**

Đúng, và là cố ý: yêu cầu ban đầu ghi rõ "Không commit code khi kết thúc; dừng để người dùng review".
Khi commit, nhớ `git add implementation` vì toàn bộ cây còn untracked.

**Test đã chạy lại**

| Lệnh | Kết quả |
|---|---|
| `go build ./... && go vet ./...` | pass |
| `go test ./...` | pass; conformance chạy đủ 33 case (27 accepted so artifact, 6 rejected so rejection status) |
| `npm run typecheck && npm run lint && npm test && npm run build` | pass |
| `bash test/integration/kind-verify.sh` | pass, run `kind-20260921071211-11863`, cleanup verified |

**Chưa chạy lại**

Cloud happy path chưa chạy lại sau các thay đổi này; bằng chứng AWS mới nhất vẫn là run
`aws-20260921052038`. Thay đổi kể từ đó: run ID mặc định phía API (đã phủ bằng
`TestHTTPUsesTheConfiguredRunID` trên cloud application với fake adapters), `id`/`replicas` trong
module (đã phủ bằng kind), và các thay đổi thuần planning (đã phủ bằng 33 fixture). Nên chạy lại
`aws-verify.sh` trước khi coi là release-ready.

#### Phase 6 bước 3d — Đồng bộ tài liệu với executable baseline — hoàn thành 2026-09-21

- Đồng bộ trạng thái implementation của UC-01 đến UC-09 với code hiện có và phần còn hoãn theo đúng thứ tự Phase 6.
- Sửa schema/ERD Matching Criteria về năm field `env_type`, `app_id`, `env_id`, `res_id`, `class`; xóa mô tả `execution_profile` mâu thuẫn.
- Sửa UC-06 descriptor convention thành ba contract chung + bốn implicit extension; cloud/internal sequence thể hiện một request deploy đúng một Score/workload, acceptance harness gọi ba request tuần tự.
- Đồng bộ UC-07 specification, realization, sequence, VOPC, operation contracts và traceability cho before shared validation, shared conflict và last-reference preservation.
- Ghi rõ giới hạn hiện tại: Terraform runtime chỉ execute module nhúng; conformance so artifact cho 27 accepted fixture, còn 6 rejected fixture mới kiểm tra rejection status.
- Verification sau đồng bộ: 27/27 PlantUML parse pass; `go build ./...`, `go vet ./...`, `go test ./...` và race test cho planner/HTTP E2E/conformance đều pass; frontend typecheck, lint, 23 test và production build đều pass.
- Cloud không chạy lại trong bước tài liệu này; release gate vẫn cần `aws-verify.sh` vì lần AWS gần nhất là `aws-20260921052038`.

#### Bước tiếp theo sau Phase 6 bước 3

1. Terraform state backend bền vững và bỏ run ID khỏi tên VPC/EKS/Aurora (đã hoãn theo yêu cầu).
2. Bước 4: hoàn thiện UC-09 (history, filter, so sánh trạng thái) trên nền read model đã có.
3. Bước 5: thay seed bằng dữ liệu quản trị thật cho UC-01..UC-04 và bổ sung adapter PostgreSQL cho state store.
4. Bước 6: UC-05 preview dùng chung planner pipeline.
5. Bước 7: UC-07 update/remove và reuse resource state.


#### Verify internal path bằng kind

- [x] Phát hiện cụm bằng `kind get clusters`, xác nhận đúng Kubernetes context và ghi nhận version/capacity trước khi test.
- [x] Không tạo cluster mới nếu cụm hiện có đáp ứng test; không thay đổi context hoặc workload ngoài phạm vi test.
- [x] Tạo namespace riêng có run ID cho orchestrator verification.
- [x] Chạy UC-06/UC-08 internal happy path: provision PostgreSQL StatefulSet, thu outputs, inject vào workload, apply workload và kiểm tra Deployment status.
- [x] Kiểm tra readiness của frontend/backend/worker, Service/StatefulSet, frontend -> backend, backend/worker -> PostgreSQL và trạng thái đã persist.
- [x] Gửi ít nhất một job qua backend; xác nhận worker xử lý và kết quả đọc lại được qua backend/frontend.
- [x] Thu log/test evidence cần thiết, sau đó xóa namespace và mọi cluster-scoped test object đã tạo; xác nhận không còn resource mang run ID.

#### Verify cloud path bằng cloud identity cục bộ

- [x] Dùng cloud profile/identity đã cấu hình trên máy; xác nhận account, principal và region trước khi tạo tài nguyên. Không đọc, copy, ghi log hoặc commit access key/secret/session token.
- [x] Dùng một run ID và tag bắt buộc cho mọi tài nguyên, tối thiểu gồm `project`, `environment`, `owner`, `run-id` và `expires-at`.
- [x] Dùng Terraform state riêng cho lần verify; ghi lại toàn bộ resource ID để có thể cleanup kể cả khi orchestration dừng giữa chừng.
- [x] Chọn region và cấu hình rẻ nhất vẫn kiểm chứng đúng kiến trúc: VPC tối thiểu, EKS một node group dung lượng thấp nhất phù hợp, Aurora cấu hình nhỏ nhất được hỗ trợ, không bật thành phần tùy chọn tốn phí.
- [x] Trước `apply`, kiểm tra quota, giá hiện hành, Terraform plan và ước lượng giới hạn chi phí; xác nhận lại đúng cloud account/region và ngân sách tối đa cho lần chạy.
- [x] Chạy UC-06/UC-08 cloud happy path: tạo VPC/EKS, provision Aurora, truyền outputs, deploy frontend/backend/worker lên EKS và kiểm tra cùng luồng job end-to-end.
- [x] Ngay sau khi thu đủ evidence — hoặc khi test lỗi — chạy teardown bằng đúng state/run ID. Thứ tự cleanup phải xử lý workload và load balancer trước, sau đó Aurora, EKS/node group và VPC.
- [x] Xác minh cleanup bằng cả Terraform state và cloud API/tag query; không coi `terraform destroy` thành công là bằng chứng duy nhất.
- [x] Chỉ giữ lại log đã loại bỏ dữ liệu nhạy cảm, test result và chi phí thực tế. Không để credential, kubeconfig tạm hoặc Terraform state chứa secret trong repository.

#### Quy tắc cleanup sau verification

- Phải xóa toàn bộ **tài nguyên test** trên kind và cloud sau khi verify; không để EKS, Aurora, node, load balancer, volume hoặc public IP tiếp tục phát sinh chi phí.
- Giữ lại code sản phẩm và automated test có giá trị tái sử dụng. Chỉ xóa script, manifest, binary, kubeconfig và scaffolding dùng một lần; file tạm cần thiết cho cleanup chỉ được xóa sau khi đã chứng minh không còn resource.
- Không dùng lệnh xóa theo wildcard, account-wide hoặc region-wide. Mọi target phải được resolve bằng state, run ID và tag của lần test.
- Nếu cleanup chưa được xác minh thì hạng mục cloud verification chưa hoàn thành, dù happy path đã chạy thành công.

## 5. Design gate — PASS 2026-09-20

Chỉ được bắt đầu Phase 6 khi tất cả điều kiện sau đúng:

- UC-01 đến UC-09 có specification, realization, sequence và VOPC đã review.
- Planner challenge đã được phân tích và phân biệt rõ thuật toán với harness.
- UC-06 có hai end-to-end sequence; UC-08 thể hiện dependency order và output propagation.
- Design class, database/ERD, operation contracts và state machines nhất quán.
- Traceability matrix không còn gap P0.
- Tên operation/method và transaction boundary đủ rõ để viết test và code.

Cloud verification chỉ được chạy sau design gate và sau khi internal path trên kind đã thành công. Paid `apply` phải có preflight ghi nhận account, region, plan, TTL/tag và giới hạn chi phí; teardown verification là một phần bắt buộc của test.

## 6. Quyết định đã chốt

- Cloud mặc định dùng EKS; mỗi Application có một EKS và VPC application-scoped.
- Environment trên Kubernetes được cô lập bằng namespace.
- Internal profile dùng Kubernetes cluster đã đăng ký.
- Score PostgreSQL dependency được hiện thực bằng Aurora trên cloud hoặc StatefulSet trong internal Kubernetes thông qua Resource Definition.
- UC-08 được UC-06/UC-07 gọi để provision resource.
- Planner challenge là input kỹ thuật cho UC-06, không phải specification hay kiến trúc sản phẩm.
- Toàn bộ planner, service, executor, CLI/API và automated test phía orchestrator backend được viết bằng Go; Orchestrator Web Console dùng React + TypeScript với Vitest/Testing Library. Python validator trong reference chỉ dùng để kiểm tra fixture và không thuộc sản phẩm.
- Web console tham khảo cách chia `app/features/shared/styles`, same-origin API và delivery model của `final_idp/idp/frontend`; không copy nguyên code/domain và không đưa authentication/RBAC vào MVP.
- Orchestrator Web Console và acceptance application frontend là hai artifact độc lập.
- Mọi sequence, VOPC, class, component, ERD và state-machine diagram dùng PlantUML; source `.puml` là artifact chuẩn.
- Acceptance application gồm frontend, backend, worker và shared `postgres` database; implementation database phụ thuộc Execution Profile.
- Internal integration test dùng cụm kind có sẵn trên máy và namespace riêng theo run ID.
- Cloud integration test dùng identity đã lưu cục bộ, cấu hình chi phí tối thiểu và tài nguyên có tag/TTL; toàn bộ tài nguyên phải được teardown ngay sau verify.
- MVP hiện chỉ xét happy path; failure recovery, rollback, secret lifecycle, RBAC và audit nằm ngoài scope.
- Matching Criteria chỉ dùng năm field chuẩn; Execution Profile gắn với Application và không phải criterion riêng.
- Terraform executor của MVP chỉ chạy module nhúng `vpc`, `eks`, `aurora`; remote source execution chưa thuộc happy path.

## 7. Bước đầu tiên của phiên chat mới

1. Đọc `AGENT.md`, file này và `orchestrator_docs/INDEX.md`.
2. Kiểm tra cấu trúc file và không sửa các thư mục reference.
3. Đọc `orchestrator_docs/implementation/package-layout.md` để nắm cây code trong `implementation/`.
4. Chạy lại baseline: `cd implementation && go test ./...` và `cd implementation/frontend && npm ci && npm run typecheck && npm run lint && npm test && npm run build`.
5. Bắt đầu **Phase 6 bước 4**: hoàn thiện UC-09 (history, filter, quan sát trạng thái) trên read model đã có.
6. Giữ nguyên ranh giới đã chốt: planner side-effect free, UC-08 chỉ execute resource node, commit current Deployment Set chỉ sau readiness.
7. Cập nhật checkbox và ghi chú quyết định vào file này trước khi kết thúc phiên.

## 8. Nhật ký thực hiện

### 2026-09-20 — Phase 1 và Phase 2

- Hoàn thành chuẩn hóa 9 specification; mỗi UC có `PRE`, `TRG`, `MS`, `BR`, `POST`, `OOS` và `VAR` khi có nhánh happy path.
- Thêm glossary chung và quan hệ UC-06/UC-07 `«include»` UC-08.
- Xác nhận planner challenge là planning-only: chưa có implicit profile infrastructure, runtime output resolution, Terraform apply/state, Kubernetes apply hoặc persistence.
- Chốt ranh giới cho realization: UC-08 chỉ execute resource nodes và trả outputs; UC-06 dùng outputs để render/apply workload.
- Chốt Go là ngôn ngữ duy nhất cho code sản phẩm và automated test; không đưa Python validator của challenge vào build/runtime.
- Bước kế tiếp khi đó: Phase 3, bắt đầu realization UC-01.

### 2026-09-20 — Phase 3 đến Phase 5

- Hoàn thành realization, PlantUML sequence và VOPC cho UC-01 đến UC-09; UC-06 có cloud/internal diagrams riêng.
- Hoàn thành consolidated class, component, database ERD/schema, operation contracts, state machines và ba ADR.
- Chốt planner side-effect free; UC-08 chỉ execute resources; UC-06 render/apply workload sau output propagation.
- Chốt transaction A persist plan, short transaction per resource và final optimistic transaction commit current Deployment Set.
- Parse và render thành công 27/27 PlantUML sources; kiểm tra trực quan consolidated class, ERD và UC-06 cloud sequence.
- Traceability coverage đạt 75/75 `MS-nn`; design gate PASS, không còn gap P0 trong happy path.
- Acceptance application Phase 6 gồm frontend, backend, worker và shared `postgres` database.
- Khởi tạo Git repository tại workspace root với branch `main`; chưa tạo commit.
- Bước kế tiếp: Phase 6 bước 1, Go walking skeleton UC-06/UC-08 với fake adapters.

### 2026-09-21 — Phase 6 bước 1 đến bước 3

- Tạo `implementation/` làm cây code sản phẩm; Go module tên `orchestrator`.
- Bước 1 pass: walking skeleton UC-06/UC-08/UC-09 với fake adapters, Web Console React/TypeScript/Vite, local HTTP smoke và production bundle dưới `/ui/`.
- Bước 2 pass: internal happy path trên kind `idp-internal`, namespace theo run ID, cleanup đã xác minh.
- Bước 3 pass: cloud happy path trên AWS `us-east-1` với VPC/EKS/Aurora, ECR tạm, chi phí ước tính $0.1725/giờ, cleanup đã xác minh bằng AWS API.
- Ba thay đổi tài liệu thiết kế phát sinh từ implementation: definition-driven edge trong UC-06, matching theo Application bằng năm field chuẩn và package layout theo `implementation/`.
- Chưa tạo commit; dừng để người dùng review.

### 2026-09-21 — Review fixes và đồng bộ tài liệu

- Sửa API fallback về process run ID cho request Web Console, bổ sung shared before/conflict/reference rules và chạy planner sản phẩm qua 33 fixture.
- Đồng bộ UC-01..UC-09 implementation status, UC-05/UC-07 planning collaboration, UC-06 single-workload sequence, database ERD/schema, operation contracts, traceability và design-gate addendum.
- Ghi rõ hai giới hạn không chặn happy path: rejected conformance case chưa so structured error và Terraform source từ xa chưa execute ở runtime.
- Bước kế tiếp sau commit: Phase 6 bước 4 (UC-09); trước release phải chạy lại AWS happy path.

### 2026-09-20 — Bổ sung web console vào Phase 6

- Chốt làm Orchestrator Web Console ngay trong Phase 6 bằng React + TypeScript strict + Vite, tham khảo `final_idp/idp/frontend`.
- Tách rõ web console quản trị với acceptance application frontend được triển khai như Kubernetes workload.
- Ánh xạ màn hình web console vào UC-01 đến UC-09; UC-08 chỉ được quan sát qua Deploy/Deployment details.
- Chốt same-origin `/api/v1/` + `/ui/`, feature-oriented source layout, typed API client và Vitest/Testing Library.
- Bước kế tiếp vẫn là Phase 6 bước 1, nhưng bao gồm cả React shell và deploy/status fixture views song song với Go walking skeleton.
