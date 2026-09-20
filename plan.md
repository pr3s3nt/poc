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
- [ ] Code sản phẩm — Phase 6 chưa bắt đầu.

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

#### Verify internal path bằng kind

- [ ] Phát hiện cụm bằng `kind get clusters`, xác nhận đúng Kubernetes context và ghi nhận version/capacity trước khi test.
- [ ] Không tạo cluster mới nếu cụm hiện có đáp ứng test; không thay đổi context hoặc workload ngoài phạm vi test.
- [ ] Tạo namespace riêng có run ID cho orchestrator verification.
- [ ] Chạy UC-06/UC-08 internal happy path: provision PostgreSQL StatefulSet, thu outputs, inject vào workload, apply workload và kiểm tra Deployment status.
- [ ] Kiểm tra readiness của frontend/backend/worker, Service/StatefulSet, frontend -> backend, backend/worker -> PostgreSQL và trạng thái đã persist.
- [ ] Gửi ít nhất một job qua backend; xác nhận worker xử lý và kết quả đọc lại được qua backend/frontend.
- [ ] Thu log/test evidence cần thiết, sau đó xóa namespace và mọi cluster-scoped test object đã tạo; xác nhận không còn resource mang run ID.

#### Verify cloud path bằng cloud identity cục bộ

- [ ] Dùng cloud profile/identity đã cấu hình trên máy; xác nhận account, principal và region trước khi tạo tài nguyên. Không đọc, copy, ghi log hoặc commit access key/secret/session token.
- [ ] Dùng một run ID và tag bắt buộc cho mọi tài nguyên, tối thiểu gồm `project`, `environment`, `owner`, `run-id` và `expires-at`.
- [ ] Dùng Terraform state riêng cho lần verify; ghi lại toàn bộ resource ID để có thể cleanup kể cả khi orchestration dừng giữa chừng.
- [ ] Chọn region và cấu hình rẻ nhất vẫn kiểm chứng đúng kiến trúc: VPC tối thiểu, EKS một node group dung lượng thấp nhất phù hợp, Aurora cấu hình nhỏ nhất được hỗ trợ, không bật thành phần tùy chọn tốn phí.
- [ ] Trước `apply`, kiểm tra quota, giá hiện hành, Terraform plan và ước lượng giới hạn chi phí; xác nhận lại đúng cloud account/region và ngân sách tối đa cho lần chạy.
- [ ] Chạy UC-06/UC-08 cloud happy path: tạo VPC/EKS, provision Aurora, truyền outputs, deploy frontend/backend/worker lên EKS và kiểm tra cùng luồng job end-to-end.
- [ ] Ngay sau khi thu đủ evidence — hoặc khi test lỗi — chạy teardown bằng đúng state/run ID. Thứ tự cleanup phải xử lý workload và load balancer trước, sau đó Aurora, EKS/node group và VPC.
- [ ] Xác minh cleanup bằng cả Terraform state và cloud API/tag query; không coi `terraform destroy` thành công là bằng chứng duy nhất.
- [ ] Chỉ giữ lại log đã loại bỏ dữ liệu nhạy cảm, test result và chi phí thực tế. Không để credential, kubeconfig tạm hoặc Terraform state chứa secret trong repository.

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

## 7. Bước đầu tiên của phiên chat mới

1. Đọc `AGENT.md`, file này và `orchestrator_docs/INDEX.md`.
2. Kiểm tra cấu trúc file và không sửa các thư mục reference.
3. Đọc `orchestrator_docs/implementation/uc06-planner-reference.md` trước khi thiết kế UC-06/UC-08.
4. Đọc `orchestrator_docs/traceability/design-gate.md` và `orchestrator_docs/implementation/package-layout.md`.
5. Bắt đầu **Phase 6 bước 1**: Go walking skeleton UC-06 + UC-08 với acceptance frontend/backend/worker/shared database fixtures và fake adapters; đồng thời dựng React web-console shell cùng Deploy/Deployment details fixture views.
6. Viết Go tests theo traceability matrix và frontend tests cho reducer/API/page states trước hoặc cùng implementation; chưa truy cập kind/cloud ở bước walking skeleton.
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

### 2026-09-20 — Bổ sung web console vào Phase 6

- Chốt làm Orchestrator Web Console ngay trong Phase 6 bằng React + TypeScript strict + Vite, tham khảo `final_idp/idp/frontend`.
- Tách rõ web console quản trị với acceptance application frontend được triển khai như Kubernetes workload.
- Ánh xạ màn hình web console vào UC-01 đến UC-09; UC-08 chỉ được quan sát qua Deploy/Deployment details.
- Chốt same-origin `/api/v1/` + `/ui/`, feature-oriented source layout, typed API client và Vitest/Testing Library.
- Bước kế tiếp vẫn là Phase 6 bước 1, nhưng bao gồm cả React shell và deploy/status fixture views song song với Go walking skeleton.
