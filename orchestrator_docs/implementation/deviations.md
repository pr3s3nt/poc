---
id: IMPLEMENTATION-DEVIATIONS
artifact: design-implementation-deviations
status: current
last_reviewed: 2026-09-23
---

# Known design and implementation deviations

File này ghi khác biệt hiện tại mà AI không được tự suy diễn mất. Hạng mục thuần
roadmap/deferred capability nằm trong [backlog](../backlog/README.md).

| ID | Difference | Required interpretation/action |
|---|---|---|
| IMP-012 | UC-00 yêu cầu internal account, opaque session và authenticated Organization/role context; baseline hiện không có authentication hoặc authorization boundary. | Không coi request-provided identity/Organization là trusted; implement UC-00 before exposing Developer self-service flows. |
| IMP-001 | Database architecture mô tả PostgreSQL system of record; baseline hiện dùng in-memory map + JSON snapshot. | Giữ transaction/repository ports; không mô tả snapshot store là production persistence. |
| IMP-002 | UC-01..UC-04 specification đầy đủ hơn seed-backed admin support hiện có; riêng UC-01 nay yêu cầu Developer self-service, generated Application ID, fixed `staging`/`production` Environments và desired endpoints. | Treat các UC này là designed, chưa fully implemented; không coi seeded `dev` Environment là UC-01 complete. |
| IMP-003 | UC-05 có planner core nhưng chưa có preview system operation/API/UI hoàn chỉnh. | Không coi deploy dry-run nội bộ là UC-05 hoàn tất. |
| IMP-004 | UC-07 planner đã có before/shared rules nhưng update/remove flow và UI chưa hoàn chỉnh. | Không coi conformance cases là full UC-07 delivery. |
| IMP-005 | UC-09 có list/detail và execution artifacts nhưng thiếu history/filter/state comparison. | M02/I06-04 phải hoàn thiện phần còn thiếu trước khi đánh dấu UC-09 complete; iteration deferred sau M00-a, không bị đóng. |
| IMP-006 | Six rejected challenge fixtures mới chỉ so rejection status, chưa so structured `phase/code/path`. | D02 vẫn deferred; không tuyên bố full rejection-contract conformance. |
| IMP-007 | Terraform inspector hiểu remote source identity nhưng runtime chỉ execute embedded `vpc`/`eks`/`aurora`. | D03 vẫn deferred; không nhận remote module là supported runtime contract. |
| IMP-013 | Canonical schema dùng UUID surrogate IDs cho Organization, Connection, Environment và các FK, còn domain/local snapshot dùng business keys (`acme`, `internal-cluster`, `staging`) ở nhiều quan hệ; seeded Application `acceptance` cũng không phải UUID. | PostgreSQL adapter phải ánh xạ key ↔ surrogate UUID ổn định, không ép business key vào UUID column hoặc đổi schema để chứa snapshot. Cần migration/repository test cho legacy seed và generated UUID Application trước khi chuyển bootstrap. |
| IMP-014 | The self-hosted Orchestrator on kind receives the host's admin kubeconfig and the scoped Vault token as UC-12 secrets injected as environment variables, and keeps its state in memory. UC-04 designs a durable, least-privilege cluster credential store and IMP-001 a PostgreSQL system of record. | Treat the self-hosted instance as a kind demonstration only, not a production deployment model. Do not reuse the admin-kubeconfig delivery outside kind; replace it with a scoped ServiceAccount or the UC-04 credential store before any shared use. |

Humanitec standalone Delta API, asynchronous deploy, whole-set workload apply và
incremental deployment là compatibility scope deferred ở D05. Resource
Definition/Score public boundary được theo dõi ở D06. Đây không phải lỗi code
đối với happy-path API hiện hành.

Khi resolve deviation, cập nhật canonical docs, code/tests, current state và xóa
hoặc sửa dòng tương ứng trong cùng change.
