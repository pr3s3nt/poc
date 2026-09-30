---
id: IMPLEMENTATION-DEVIATIONS
artifact: design-implementation-deviations
status: current
last_reviewed: 2026-09-30
---

# Known design and implementation deviations

File này ghi khác biệt hiện tại mà AI không được tự suy diễn mất. Hạng mục thuần
roadmap/deferred capability nằm trong [backlog](../backlog/README.md).

| ID | Difference | Required interpretation/action |
|---|---|---|
| IMP-002 | UC-01 self-service flow đã hoàn tất; UC-02..UC-04 vẫn thiếu một phần production lifecycle/management so với specification, gồm AWS Connection registration và production-grade authorization/credential storage. | Không dùng seed hoặc registration baseline để tuyên bố toàn bộ UC-02..UC-04 complete; UC-01 không còn thuộc deviation này. |
| IMP-003 | UC-05 có planner core nhưng chưa có preview system operation/API/UI hoàn chỉnh. | Không coi deploy dry-run nội bộ là UC-05 hoàn tất. |
| IMP-004 | UC-07 planner đã có before/shared rules nhưng update/remove flow và UI chưa hoàn chỉnh. | Không coi conformance cases là full UC-07 delivery. |
| IMP-005 | UC-09 có list/detail và execution artifacts nhưng thiếu history/filter/state comparison. | M02/I06-04 phải hoàn thiện phần còn thiếu trước khi đánh dấu UC-09 complete; iteration deferred sau M00-a, không bị đóng. |
| IMP-006 | Six rejected challenge fixtures mới chỉ so rejection status, chưa so structured `phase/code/path`. | D02 vẫn deferred; không tuyên bố full rejection-contract conformance. |
| IMP-007 | Terraform inspector hiểu remote source identity nhưng runtime chỉ execute embedded `vpc`/`eks`/`aurora`. | D03 vẫn deferred; không nhận remote module là supported runtime contract. |
| IMP-014 | The self-hosted Orchestrator on kind receives the host's admin kubeconfig and the scoped Vault token as UC-12 secrets injected as environment variables. Logical state is now durable in PostgreSQL, but cluster credential delivery remains over-privileged. | Treat the self-hosted instance as a kind demonstration only, not a production deployment model. Do not reuse the admin-kubeconfig delivery outside kind; replace it with a scoped ServiceAccount or the UC-04 credential store before any shared use. |

Humanitec standalone Delta API, asynchronous deploy, whole-set workload apply và
incremental deployment là compatibility scope deferred ở D05. Resource
Definition/Score public boundary được theo dõi ở D06. Đây không phải lỗi code
đối với happy-path API hiện hành.

Khi resolve deviation, cập nhật canonical docs, code/tests, current state và xóa
hoặc sửa dòng tương ứng trong cùng change.
