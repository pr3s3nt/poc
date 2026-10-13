---
id: I04-03
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-03 — Executor tạo, cập nhật và xóa

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Chưa bắt đầu; không suy status này thành task bị bỏ hoặc đã implement.

## Phạm vi

Registered module execution với AWS/Kubernetes providers; stable state/name/password refs, write-ahead stages, bounded retry/cancellation, outputs/readiness và destroy engine.

## Nguồn chuẩn

- [usecase/UC-08/specification.md](../../../usecase/UC-08/specification.md)
- [architecture/contracts/terraform-modules.md](../../../architecture/contracts/terraform-modules.md)
- [architecture/contracts/terraform-state.md](../../../architecture/contracts/terraform-state.md)

## Code impact để giao Claude

- [backend/internal/adapters/terraform](../../../../backend/internal/adapters/terraform/)
- [backend/internal/application/provisioning](../../../../backend/internal/application/provisioning/)
- [backend/internal/ports/execution](../../../../backend/internal/ports/execution/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

CREATE/UPDATE/DESTROY contract có tests; UPDATE destructive plan bị chặn; unknown outcome không tạo trùng; cleanup giữ foreign/platform/preexisting/updated assets theo authorization mode.

## Checks

- `cd backend && go test ./... && go build ./...`
- Executor cancellation/process-stop, retry/plan policy/output/secret tests và real HTTP-state PostgreSQL tests

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
