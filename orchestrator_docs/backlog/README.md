---
id: BACKLOG-INDEX
artifact: backlog-index
status: current
last_reviewed: 2026-10-07
---

# Backlog

Backlog ghi vấn đề/capability đã biết nhưng chưa thuộc current baseline. Các
record này không phải accepted product behavior hoặc architecture.

| ID | Item | State |
|---|---|---|
| [D01](D01-durable-terraform-state.md) | Durable Terraform state và stable physical names | Deferred |
| [D02](D02-structured-planner-errors.md) | Structured comparison cho rejected planner fixtures | Deferred |
| [D03](D03-remote-terraform-runtime.md) | Runtime execution của remote Terraform sources | Deferred |
| [D04](D04-postgres-system-store.md) | PostgreSQL adapter cho orchestrator state | Resolved into normalized adapter |
| [D05](D05-humanitec-deployment-lifecycle.md) | Humanitec external deployment lifecycle compatibility | Deferred |
| [D06](D06-humanitec-score-boundary.md) | Humanitec Resource Definition and Score boundary compatibility | Deferred |
| [D07](D07-planner-catalog-invariant-behavior.md) | Validation boundary cho Resource Definition catalog | Deferred |
| [D08](D08-delta-snapshot-application-id.md) | Ownership của Application identity trên Deployment Delta Snapshot | Resolved into canonical schema |
| [D09](D09-application-cluster-connection-ownership.md) | Chồng trách nhiệm chọn cluster giữa Application và Resource Definition | Deferred |

Các phase tương lai UC-01..UC-07 được theo dõi ở current state/iteration planning;
không tạo backlog record chỉ để sao chép use-case scope đã chấp nhận.
