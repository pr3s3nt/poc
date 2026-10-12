---
id: BACKLOG-INDEX
artifact: backlog-index
status: current
last_reviewed: 2026-10-13
---

# Backlog

Backlog ghi vấn đề/capability đã biết nhưng chưa thuộc current baseline. Các
record này không phải accepted product behavior hoặc architecture.

| ID | Item | State |
|---|---|---|
| [D01](D01-durable-terraform-state.md) | Durable Terraform state và stable physical names | Selected for v2; implementation pending |
| [D02](D02-structured-planner-errors.md) | Structured comparison cho rejected planner fixtures | Deferred |
| [D03](D03-remote-terraform-runtime.md) | Runtime execution của registered immutable Terraform sources | Selected for v2; implementation pending |
| [D04](D04-postgres-system-store.md) | PostgreSQL adapter cho orchestrator state | Resolved into normalized adapter |
| [D05](D05-humanitec-deployment-lifecycle.md) | Humanitec external deployment lifecycle compatibility | Deferred |
| [D06](D06-humanitec-score-boundary.md) | Humanitec Resource Definition and Score boundary compatibility | Deferred |
| [D07](D07-planner-catalog-invariant-behavior.md) | Validation boundary cho Resource Definition catalog | Deferred |
| [D08](D08-delta-snapshot-application-id.md) | Ownership của Application identity trên Deployment Delta Snapshot | Resolved into canonical schema |
| [D09](D09-application-cluster-connection-ownership.md) | Chồng trách nhiệm chọn cluster giữa Application và Resource Definition | Resolved design via ADR-011; implementation transition IMP-016 |
| [D10](D10-custom-resource-drivers.md) | Custom Resource Drivers | Deferred — sau v2 |
| [D11](D11-existing-external-resources.md) | Echo/External: dùng tài nguyên có sẵn | Deferred — sau v2 |
| [D12](D12-terraform-recovery.md) | Full Terraform recovery/resume sau interruption | Deferred — sau v2 |
| [D13](D13-terraform-orphan-reconciliation.md) | Reconcile v2 objects chưa acknowledgement state | Deferred — sau v2 |
| [D14](D14-platform-state-backup.md) | Platform metadata/state backup, key recovery và restore rehearsal | Deferred — sau v2 |

Các phase tương lai UC-01..UC-07 được theo dõi ở current state/iteration planning;
không tạo backlog record chỉ để sao chép use-case scope đã chấp nhận.

D01/D03 đã có accepted design cho v2; trạng thái này không đồng nghĩa đã
implement. Phần recovery tối thiểu vẫn bắt buộc theo ADR-014; D12–D14 không
cho phép bỏ state/locking/ownership hoặc cleanup acceptance.
