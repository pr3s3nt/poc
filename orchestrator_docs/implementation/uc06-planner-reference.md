---
id: UC-06-PLANNER-REFERENCE
artifact: technical-reference-analysis
status: current
last_reviewed: 2026-09-22
---

# UC-06 Planner Reference Analysis

## 1. Mục đích

Tài liệu này phân tích bản copy chỉ đọc `orchestrator_reference/humanitec-planner-challenge-v4/` trước khi realization UC-06. Nó là technical input cho UC-05, UC-06, UC-07 và UC-08; specification trong `orchestrator_docs/usecase/` vẫn là nguồn yêu cầu chuẩn.

## 2. Phạm vi đã đọc và kiểm chứng

Đã đọc:

- `PROBLEM.md`, `SOURCES.md`, `TESTCASES.md`, `VALIDATION.md`, `README.md` và `testcases.yaml`.
- Entry point, toàn bộ source và invariant tests trong `starter-go/`.
- Grader source và hướng dẫn chạy.
- Fixtures đại diện cho Score conversion/delta, private/shared resource, preservation, matching, definition reference, inherited descriptor, co-provision, chain/diamond topology, Terraform default/contract, Active Resource, add/remove workload, rejected planning, combined flow, Score param reference và context placeholder.

Baseline ngày 2026-09-20:

| Kiểm tra | Kết quả |
|---|---|
| `python3 validate_fixtures.py` | Pass, `validated 33 cases` |
| `go test ./...` trong `starter-go` | Pass; invariant, JSON Patch và 33 expected-result tests đều đạt |
| Build planner executable | Pass |
| Grader chạy executable trên toàn bộ fixtures | Pass, 33/33 cases |

Bundle có 27 accepted cases và 6 rejected cases. Kết quả baseline chỉ xác nhận implementation đáp ứng challenge subset, không xác nhận nó đã là orchestrator runtime.

## 3. Pipeline thực tế của challenge

`Run` thực hiện tuần tự:

```text
Load fixture documents
-> validate envelope and before/after workload identity
-> convert Score before/after to workload fragments
-> prove before fragment matches current Deployment Set
-> build Candidate Deployment Set and deterministic Delta
-> build initial Resource Graph
-> match Definitions and expand references/provision rules to fixed point
-> validate referenced outputs
-> inspect Terraform contracts
-> classify Active Resources
-> topological batches
-> render challenge result
```

Các invariant quan trọng:

- `current Deployment Set + Deployment Delta = Candidate Deployment Set`.
- Một Score document mô tả đúng một workload; các workload khác phải được giữ nguyên.
- Resource Descriptor có dạng `type.class#res_id`.
- Graph edge có chiều `consumer -> provider`.
- Kahn batches đưa provider vào batch sớm hơn consumer.
- Bỏ dependency chỉ tạo `unreferenced`; không suy ra `destroy`.
- Kết quả được sort ổn định để cùng input luôn sinh cùng plan.

## 4. Thành phần có thể tái sử dụng về mặt thiết kế

Không copy nguyên source; tái sử dụng semantics và thuật toán sau trong realization:

| Thành phần | Giá trị cho orchestrator |
|---|---|
| Score converter | Chuyển một Score workload thành module/private/shared contribution, bảo toàn container requests/limits và rewrite placeholder. |
| Before-state check | Chống lập kế hoạch từ snapshot cũ hoặc sai workload. |
| Deterministic Delta | Sinh RFC 6902 patch và giữ invariant current + delta = candidate. |
| Resource Descriptor | Identity chung cho private/shared/implicit resource và Active Resource lookup. |
| Initial graph builder | Tạo node/edge từ workload, dependencies và placeholders. |
| Weighted matching | Chọn Definition đặc hiệu nhất; từ chối no-match và equal-score ambiguity. |
| Fixed-point expansion | Tiếp tục match/expand Resource References và co-provision rules tới khi không còn node mới. |
| Contract validation | Kiểm tra Score output, Definition reference, Terraform inputs/outputs trước execution. |
| Terraform fingerprint | Fingerprint deterministic của source để audit plan và state. |
| Topological batching | Tạo provider-first execution batches và phát hiện cycle. |
| Active classification | Cơ sở để nhận biết desired existing/new/unreferenced, nhưng cần mở rộng state cho sản phẩm. |
| Phase ordering | Cho error precedence ổn định và dễ trace/test. |

Các chi tiết implementation đáng giữ:

- Map traversal và descriptor/path được sort lexical để plan deterministic.
- JSON Pointer phải escape `~` thành `~0` và `/` thành `~1`.
- Humanitec-shaped Delta tách `modules.add/remove/update` và `shared`; module/shared patches dùng relative root, array diff theo index và append bằng `/-`.
- `$${...}` là escaped literal, không tạo graph edge.
- Reference có class/ID thiếu hoặc `@` kế thừa node hiện tại.
- `match_dependents` phải được áp lại sau mỗi vòng expansion vì consumer mới có thể xuất hiện muộn.
- Terraform source được parse bằng HCL parser, không bằng regex.

## 5. Thành phần chỉ thuộc challenge harness

Không đưa các thành phần sau vào domain/API sản phẩm:

- `PlannerCase` và các path file trong `case.yaml`.
- `terraformSourceMap` ánh xạ URL/revision vào checkout local.
- Projection `active-resources.yaml` rút gọn.
- Output envelope `status/delta/deploymentSet/challengePlan` của grader.
- `challengePlan` như một schema API chính thức.
- CLI `--case`, quy tắc chỉ một JSON trên stdout và timeout 10 giây của grader.
- Fixture account, fake URLs và local Terraform checkouts.
- Workload Definition dùng Echo Driver chỉ để planner match node `workload`.

`map[string]any` (`Document`) thuận tiện cho bài chấm nhưng không nên là domain model chính. Product cần typed value objects ở boundary quan trọng và chỉ giữ document linh hoạt tại contract boundary.

## 6. Khoảng trống phải bổ sung cho sản phẩm

### 6.1 Planning

- `ExecutionProfileResourceEnricher` để thêm VPC/EKS theo Application cho `aws-eks`, existing cluster cho `internal-k8s`, và namespace theo Environment.
- Scope-aware descriptor/identity cho Application, Environment, workload và shared resource.
- Registry/repository thật cho Application, Environment, Resource Type, Resource Definition, connection và Active Resource.
- JSON Schema validation đầy đủ; challenge chỉ chặn unknown top-level input keys, chưa kiểm tra required/type/nested constraints đầy đủ.
- Durable Plan/Deployment snapshot và optimistic version check để plan không execute trên current Deployment Set đã đổi.
- Quy tắc ownership/reference cho shared resource. Challenge chưa đủ để đảm bảo shared entry do workload bỏ vẫn được giữ khi workload khác còn phụ thuộc như BR-03 của UC-07.
- Persist `DeploymentDeltaSnapshot` như immutable entity khi deploy/update/remove; preview chỉ dùng transient typed Delta document. Mutable standalone Humanitec Delta API/lifecycle vẫn là compatibility scope riêng.

### 6.2 Terraform và driver execution

- Source resolver thật, revision pinning, path confinement, integrity policy và cache an toàn.
- Driver Account/credential resolution; field `driver_account` bị challenge bỏ qua.
- Terraform backend, workspace/state identity, locking, init/plan/apply và output collection.
- Idempotency/reconcile contract cho Active Resource đã tồn tại.
- Executor selection theo matched Definition thay vì chỉ scan Terraform source.
- Runtime binding resolver: challenge giữ `${resources[...].outputs.X}` trong input, chưa thay bằng output thực của provider.
- Persist executor state, source fingerprint, Definition revision và non-secret/secret outputs.

### 6.3 Kubernetes workload execution

- Workload Profile renderer cho Kubernetes manifests.
- Typed `containers.*.resources.requests/limits` trong workload module và mapping nguyên vẹn sang Kubernetes container resources.
- Cluster/namespace resolver và Kubernetes apply client.
- Readiness verification cho namespace, StatefulSet/Service và workload Deployment.
- PostgreSQL output adapter chung để Aurora và StatefulSet cùng trả contract `postgres`.
- Tách resource provisioning khỏi workload deployment: UC-08 trả resource outputs; UC-06 mới render/apply workload. Không đưa Echo workload node vào Terraform/Kubernetes resource executor chỉ vì challenge có node này.

### 6.4 Persistence và orchestration

- Deployment lifecycle/status, plan snapshot, Resource Graph, matched Definition và provision batches.
- Transaction boundary giữa việc persist progress, external apply và commit current Deployment Set.
- Active Resource lookup bằng logical identity cùng executor state bằng physical identity.
- Status/read model cho UC-09.
- Secret classification/redaction và audit metadata tối thiểu dù secret lifecycle đầy đủ nằm ngoài MVP.

Retry, rollback, partial-failure resume, drift và deprovision vẫn nằm ngoài happy path theo specification; realization không được vô tình coi chúng đã được giải quyết bởi challenge.

## 7. Ánh xạ vào UC-06

| UC-06 step | Input từ challenge | Phần sản phẩm phải bổ sung |
|---|---|---|
| MS-01 | Load current Deployment Set từ fixture | Deployment record/repository và versioned Environment snapshot. |
| MS-02 | `ConvertScoreToWorkloadFragment`, schema/output checks, container resources in the declared subset | Full Score boundary, typed container resource values và typed errors. |
| MS-03 | `ValidateBeforeFragment`, Candidate Set, Humanitec-shaped `BuildDeploymentDelta` | Immutable Delta Snapshot for execution, durable plan snapshot và optimistic version. |
| MS-04 | Fixture context có app/env/env_type | Load Execution Profile, connection và runtime config từ repositories. |
| MS-05 | Không hỗ trợ implicit cluster/namespace | Profile-driven implicit resource enricher. |
| MS-06 | Initial graph từ workload dependencies/placeholders | Thêm implicit namespace và scope-aware identities. |
| MS-07 | Weighted match + fixed-point expansion | Definition registry thật và profile context. |
| MS-08 | Output validation, Terraform scan, Active classification, Kahn batches | Plan artifact chính thức, executor-ready bindings và state-aware reuse. |
| MS-09 | Không execute | Gọi UC-08 với resource-only batches. |
| MS-10 | Chỉ giữ output reference dạng string | Resolve outputs thực từ provision result. |
| MS-11 | Echo workload node, không render/apply | Workload renderer including declared requests/limits, Kubernetes apply và readiness check. |
| MS-12 | Không persistence/lifecycle | Commit current set, Active Resources và `SUCCEEDED` theo transaction design. |
| MS-13 | Render JSON cho grader | API/use-case response của sản phẩm. |

## 8. Ánh xạ vào UC-08

| UC-08 step | Input từ challenge | Phần sản phẩm phải bổ sung |
|---|---|---|
| MS-01 | Resource Graph và topological batches | Loại workload node khỏi resource execution schedule hoặc đánh dấu node kind rõ ràng. |
| MS-02 | `existing/new/unreferenced` theo descriptor | Load Active Resource và driver state thật; không biến classification thành action `reuse/destroy`. |
| MS-03 | Resolve `context.*`; output reference vẫn raw | Runtime value/binding resolver từ provider outputs đã persist trong run. |
| MS-04 | Definition có `driver_type` | Executor registry và account/connection resolution. |
| MS-05 | Không execute | Terraform Executor, Kubernetes Executor và existing-cluster adapter. |
| MS-06 | Static output contract scan | Thu output thật, validate type/secret classification. |
| MS-07 | Không persist | Active Resource repository và executor state ownership. |
| MS-08 | Graph dependency biểu diễn binding | Resolve output vào consumer inputs trước batch tiếp theo. |
| MS-09 | Kahn batches | Batch runner; chạy tuần tự là đủ cho MVP semantics. |
| MS-10 | Không có provision result | Typed result trả cho UC-06/UC-07. |

## 9. Quyết định cho realization sắp tới

1. Giữ planning pipeline thành một application service độc lập với executor.
2. Đưa implicit infrastructure enrichment vào giữa Candidate Deployment Set và Definition matching.
3. Resource Graph có thể chứa workload node để biểu diễn dependency, nhưng UC-08 chỉ execute resource node; workload được UC-06 triển khai sau khi có outputs.
4. Plan phải chứa typed output bindings, không chỉ string placeholder; binding chỉ resolve khi provider batch hoàn thành.
5. Definition matching và graph expansion phải deterministic, fixed-point và có giới hạn node/edge.
6. Active Resource classification là planning evidence, còn create/reconcile behavior thuộc executor/driver contract.
7. Terraform contract scanner được dùng ở planning; Terraform apply/state/output collection thuộc UC-08 execution.
8. Hai profile dùng cùng planner core; khác biệt nằm ở implicit enricher, matched Definitions, connection và executors.

## 10. Fixture-to-requirement evidence

Bundle fixture là evidence không đầy đủ theo field coverage: không testcase nào
khai báo `containers.*.resources`, dù field này thuộc Score subset ở §6. Product
conformance harness hiện cũng không đọc/so expected Delta và chỉ kiểm rejection
status cho sáu rejected cases. Vì vậy 33/33 không chứng minh Delta shape, array
diff, structured error hay container resource preservation.

| Khả năng | Fixtures tiêu biểu | Use case/rule nhận evidence |
|---|---|---|
| Candidate Set và giữ module khác; reference Delta có trong expected artifacts nhưng product harness chưa assert | 01–04, 11, 23, 24 | UC-05 BR-02/BR-05/BR-06; UC-07 BR-02/BR-07 |
| Private/shared identity | 05–13 | UC-06 MS-06; UC-07 BR-03 |
| Matching specificity/ambiguity | 14, 22, 25, 26 | UC-03 BR-01/BR-02; UC-06 MS-07 |
| Reference/inheritance/co-provision | 15–17 | UC-03 BR-04; UC-06 MS-07 |
| Provider-first DAG | 18, 19, 30 | UC-05 BR-03; UC-08 BR-01 |
| Terraform contract/default/fingerprint | 20, 28, 29, 31 | UC-06 MS-08; UC-08 MS-04–MS-06 |
| Active classification | 07, 10, 21, 24 | UC-07 MS-07; UC-08 MS-02 |
| Resource input binding | 32 | UC-06 BR-06; UC-08 MS-03/MS-08 |
| Context resolution | 33 | UC-08 MS-03 |
| Container requests/limits | Không có fixture | UC-05 BR-07; UC-06 BR-11; cần product contract test riêng |

## 11. Kết luận

Planner challenge giải quyết tốt phần deterministic desired-state planning: Score -> Humanitec-shaped Delta/Candidate Set -> expanded Resource Graph -> Definition match -> Terraform contract -> provider-first batches. Nó chưa giải quyết phần làm nên orchestrator chạy thật: implicit profile infrastructure, stateful driver execution, runtime output propagation, Kubernetes deployment và persistence/transaction boundary. Fixture coverage cũng không thay thế contract review cho field không xuất hiện trong testcase. Realization UC-06/UC-08 phải dùng pipeline trên làm lõi planning nhưng thiết kế rõ các ranh giới còn thiếu này.
