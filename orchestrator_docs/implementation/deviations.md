---
id: IMPLEMENTATION-DEVIATIONS
artifact: design-implementation-deviations
status: current
last_reviewed: 2026-09-22
---

# Known design and implementation deviations

File này ghi khác biệt hiện tại mà AI không được tự suy diễn mất. Hạng mục thuần
roadmap/deferred capability nằm trong [backlog](../backlog/README.md).

| ID | Difference | Required interpretation/action |
|---|---|---|
| IMP-001 | Database architecture mô tả PostgreSQL system of record; baseline hiện dùng in-memory map + JSON snapshot. | Giữ transaction/repository ports; không mô tả snapshot store là production persistence. |
| IMP-002 | UC-01..UC-04 specification đầy đủ hơn seed-backed admin support hiện có. | Treat các UC này là designed, chưa fully implemented. |
| IMP-003 | UC-05 có planner core nhưng chưa có preview system operation/API/UI hoàn chỉnh. | Không coi deploy dry-run nội bộ là UC-05 hoàn tất. |
| IMP-004 | UC-07 planner đã có before/shared rules nhưng update/remove flow và UI chưa hoàn chỉnh. | Không coi conformance cases là full UC-07 delivery. |
| IMP-005 | UC-09 có list/detail và execution artifacts nhưng thiếu history/filter/state comparison. | M02/I06-04 phải hoàn thiện phần còn thiếu trước khi đánh dấu UC-09 complete; iteration được reprioritize sau M01, không bị đóng. |
| IMP-006 | Six rejected challenge fixtures mới chỉ so rejection status, chưa so structured `phase/code/path`. | D02 vẫn deferred; không tuyên bố full rejection-contract conformance. |
| IMP-007 | Terraform inspector hiểu remote source identity nhưng runtime chỉ execute embedded `vpc`/`eks`/`aurora`. | D03 vẫn deferred; không nhận remote module là supported runtime contract. |
| IMP-008 | Canonical planning design yêu cầu immutable `DeploymentDeltaSnapshot` có Humanitec-shaped document với `modules.add/remove/update` và `shared`; implementation đang lưu một RFC 6902 patch phẳng cho toàn Deployment Set và chưa có Snapshot entity. | Không tuyên bố Delta shape/API compatibility từ 33 fixture; sửa planner/domain/persistence trước khi đóng gap. Mutable Humanitec Delta lifecycle là D05 riêng. |
| IMP-009 | UC-05/06 yêu cầu bảo toàn `containers.*.resources.requests/limits`; Score parser hiện reject field này và Kubernetes renderer hard-code requests `10m/32Mi`. | Không mô tả workload resource contract là implemented; bổ sung typed model, conversion, renderer và tests ngoài fixture bundle. |

Humanitec standalone Delta API, asynchronous deploy, whole-set workload apply và
incremental deployment là compatibility scope deferred ở D05. Resource
Definition/Score public boundary được theo dõi ở D06. Đây không phải lỗi code
đối với happy-path API hiện hành.

Khi resolve deviation, cập nhật canonical docs, code/tests, current state và xóa
hoặc sửa dòng tương ứng trong cùng change.
