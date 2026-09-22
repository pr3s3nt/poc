---
id: DESIGN-GATE
artifact: design-gate-review
status: current
last_reviewed: 2026-09-22
---

# Design Gate Review

Review date: 2026-09-20. Implementation synchronization reviews: 2026-09-21,
Humanitec gap reconciliation 2026-09-22 and I06-05 conformance catalog
semantics 2026-09-22.

## Scope

UC-01 đến UC-09, happy path cho `aws-eks` và `internal-k8s`. Gate ban đầu được duyệt trước khi code; artifact được đồng bộ lại sau Phase 6 bước 3c để phản ánh executable baseline.

## Gate checklist

| Check | Result | Evidence |
|---|---|---|
| 9 specifications có stable IDs | PASS | `usecase/UC-01..UC-09/specification.md` |
| Mỗi UC có realization, sequence và VOPC PlantUML | PASS | `usecase/UC-xx/` |
| UC-06 có cloud/internal sequence độc lập | PASS | `sequence-cloud.puml`, `sequence-internal.puml` |
| Planner reference phân biệt reusable/harness/gaps | PASS | `implementation/uc06-planner-reference.md` |
| Consolidated class/component/domain design | PASS | `architecture/domain`, `architecture/components` |
| Database schema và ERD | PASS | `architecture/database` |
| Operation contracts và state machines | PASS | `architecture/contracts`, `architecture/state-machines` |
| UC step -> operation -> method -> data/state -> test | PASS | `traceability/matrix.md` |
| Go package ownership/dependency direction | PASS | `implementation/package-layout.md` |
| Web console boundary, API integration và delivery | PASS | `architecture/decisions/ADR-004-react-web-console.md`, component diagram, package layout |
| PlantUML source/render validation | PASS | 27/27 `.puml` parse và render PNG thành công bằng PlantUML |
| Main-flow traceability coverage | PASS | UC-01..UC-09: 75/75 `MS-nn` xuất hiện trong matrix |

## Gap review

### Method ownership

- Planning side effects thuộc `PlanningService` và components; no executor call.
- Resource execution thuộc `ResourceProvisioningService`; workload node không được execute như resource.
- Workload render/apply thuộc `DeploymentService` qua ports sau UC-08.
- Query UC-09 chỉ đọc persisted snapshots.

Result: không còn operation P0 trùng owner.

### Transaction review

- Không giữ transaction qua external call.
- Immutable Delta Snapshot/Candidate Set/Plan persisted trước execution nhưng Candidate Set chưa là current.
- Resource progress persisted per node.
- Final optimistic transaction commit current set/workloads/deployment success.

Result: transaction boundary rõ cho happy path; retry/resume/rollback vẫn OOS.

### Data ownership review

- Mọi aggregate/lifecycle có table hoặc external owner rõ.
- Terraform state và secret values không bị đưa vào database như plaintext/state file.
- Graph/matches/batches được lưu trong immutable DeploymentPlan cho UC-09.
- Logical resource reuse có unique identity + scope.

Result: không còn dữ liệu P0 không có owner/persistence location.

### UC-06/UC-08 end-to-end review

- Cloud: Application VPC -> EKS; VPC -> Aurora; outputs -> renderer -> frontend/backend/worker on EKS.
- Internal: existing cluster -> namespace/PostgreSQL StatefulSet; outputs -> renderer -> frontend/backend/worker.
- Shared `postgres` contract cung cấp cùng logical outputs cho backend/worker.
- Final commit chỉ sau Kubernetes readiness.

Result: hai happy path đầy đủ ở mức thiết kế.

### Implementation synchronization 2026-09-21

- UC-06/UC-08 đã chạy qua HTTP API với fake adapters, kind và AWS; một request vẫn xử lý đúng một Score/workload.
- Matching Criteria dùng đúng năm field `env_type`, `app_id`, `env_id`, `res_id`, `class`; không có field profile riêng.
- UC-07 planning đã validate before shared contribution, từ chối shared conflict và giữ shared entry khi workload khác còn tham chiếu; runtime update/remove vẫn thuộc Phase 6 bước 7.
- Terraform runtime của MVP chỉ execute module nhúng; remote source inspection trong conformance harness không được xem là runtime support.
- Conformance chạy 33 fixture: accepted Candidate Set/graph/matching/batches/classification/Terraform artifacts được so sánh; Delta không được assert và rejected fixtures mới xác nhận rejection status, chưa đối chiếu structured error contract.

### Humanitec gap reconciliation 2026-09-22

- Delta Snapshot design dùng `modules.add/remove/update` và `shared`, relative JSON Patch,
  deterministic array diff và immutable persisted artifact; current code vẫn là
  flat whole-document patch (IMP-008).
- Score subset giữ typed `containers.*.resources.requests/limits` đến workload
  renderer; current parser rejects field và renderer hard-code requests
  (IMP-009).
- Container resources thuộc UC-06 workload path, không làm thay đổi UC-08
  resource-only execution boundary.
- 33-fixture harness không assert Delta và fixture bundle không có container
  resources, nên cần product contract tests riêng.
- Standalone Delta API, async/full-set/incremental lifecycle và content-addressed
  Set IDs vẫn deferred tại D05.
- Resource Definition/Score boundary differences được phân loại tập trung trong
  compatibility matrix; lifecycle/mapping tương thích được deferred tại D06.
- Conformance loader empty-criteria normalization trái product registration
  invariant và challenge adapter semantics, được ghi là IMP-010; 33-fixture
  pass không bao phủ nhánh này.

### Delta Snapshot implementation 2026-09-22

- I06-06 hiện thực `DeploymentDeltaSnapshot`, `ModuleDelta`,
  `JSONPatchOperation` và `DeltaBuilder.BuildHumanitecDelta`; flat
  whole-document patch đã bị xóa khỏi plan/persistence/query path (IMP-008
  đã đóng).
- Conformance so Delta của 27 accepted fixtures với `expected/delta.yaml` và
  kiểm `base + delta = candidate`; array diff, shared remove và persistence có
  product tests riêng.
- Schema `deployments.delta_snapshot_id NOT NULL` chưa được enforce cho
  Deployment `PLANNING`/planning-`FAILED` (IMP-011).

### Conformance catalog semantics 2026-09-22

- I06-05 đóng IMP-010: conformance adapter bỏ Definition thiếu criteria hoặc
  `criteria: []` khỏi challenge catalog và giữ `{}` thành wildcard điểm 0.
- Loader unit tests phân biệt ba input shape; product planner vẫn từ chối
  catalog vi phạm UC-03 BR-07; 33/33 fixture pass
  ([evidence](../verification/2026-09-22-imp010-conformance-catalog.md)).

## Gate decision

**PASS cho design coverage.** Toàn bộ 27 PlantUML sources parse/render thành
công và coverage check tìm thấy đủ 75/75 main-flow step. IMP-008 (Delta
Snapshot) đã đóng ở I06-06; IMP-009 là implementation gap đã được trace đầy đủ
và phải đóng trước khi tuyên bố UC-05/06 conform với container-resource
contract. Empty-criteria adapter semantics đã được đóng ở I06-05. Executable
kind/AWS baseline lịch sử vẫn có giá trị trong phạm vi behavior đã kiểm chứng,
không phải bằng chứng cho Delta Snapshot hoặc IMP-009.
