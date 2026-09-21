# Design Gate Review

Review date: 2026-09-20. Implementation synchronization review: 2026-09-21.

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
- Candidate Set/Plan persisted trước execution nhưng chưa là current.
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
- Conformance chạy 33 fixture: accepted artifacts được so sánh; rejected fixtures mới xác nhận rejection status, chưa đối chiếu structured error contract.

## Gate decision

**PASS.** Toàn bộ 27 PlantUML sources parse/render thành công và coverage check tìm thấy đủ 75/75 main-flow step. Không còn gap P0 trong scope happy path. Phase 6 có thể bắt đầu bằng walking skeleton Go cho UC-06/UC-08, React web-console shell và acceptance application frontend/backend/worker/shared database.
