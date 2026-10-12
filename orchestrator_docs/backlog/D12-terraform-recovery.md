---
id: D12
artifact: backlog-item
status: deferred
last_reviewed: 2026-10-13
related: [ADR-014, UC-06, UC-08]
---

# D12 — Full Terraform execution recovery

Người dùng hoãn automatic resume sau backend/runner interruption. V2 vẫn phải
persist intent/stage/state/ownership, giữ khóa khi outcome chưa rõ, kiểm process
đã dừng trước manual intervention và có cleanup acceptance độc lập browser.
Contract tối thiểu ở [state contract](../architecture/contracts/terraform-state.md).

## Phạm vi tương lai

- Reconcile persisted stage với provider/state sau restart, không replay apply mù.
- Xác minh/terminate old execution, lấy ownership mới có fencing và resume batch.
- Bounded retry/wait/cancellation, hiển thị recovery progress và safe result.
- Không rollback/xóa resource preexisting hoặc UPDATED_BY_RUN tự động.

## Điều kiện hoàn thành

Thiết kế recovery được chốt trước code; fault-injection crash ở từng boundary
chứng minh không chạy hai owners, không tạo trùng, không commit success giả,
tiếp tục đúng dependencies và cleanup được sau interruption. Có real state/
provider evidence trong môi trường được cho phép, kể cả failed attempts.
Không mở rộng thành v1 import hoặc data migration.
