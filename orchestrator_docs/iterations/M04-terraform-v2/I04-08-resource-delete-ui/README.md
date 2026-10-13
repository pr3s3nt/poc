---
id: I04-08
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-08 — Xóa tài nguyên và dependency qua Console

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Chưa bắt đầu; không suy status này thành task bị bỏ hoặc đã implement.

## Phạm vi

Resource detail/progress read API, PE/Admin delete preview/confirmation/operation; eligible dependency closure, token/versions/idempotency và original-target destroy.

## Nguồn chuẩn

- [usecase/UC-07/specification.md](../../../usecase/UC-07/specification.md)
- [usecase/UC-07/realization.md](../../../usecase/UC-07/realization.md)
- [usecase/UC-07/ui/api-mapping.md](../../../usecase/UC-07/ui/api-mapping.md)
- [usecase/UC-09/specification.md](../../../usecase/UC-09/specification.md)
- [architecture/contracts/operation-contracts.md](../../../architecture/contracts/operation-contracts.md)

## Code impact để giao Claude

- [backend/internal/application/provisioning](../../../../backend/internal/application/provisioning/)
- [backend/internal/delivery/http](../../../../backend/internal/delivery/http/)
- [frontend/src/features/deployments](../../../../frontend/src/features/deployments/)
- [frontend/src/features/applications](../../../../frontend/src/features/applications/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

Developer denied, root UNREFERENCED/no consumer/no busy; exact-name confirmation liệt kê toàn bộ cascade; stale/concurrent consumer thay đổi chặn trước destroy; verified absence/remaining IDs.

## Checks

- `cd backend && go test ./... && go build ./...`
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
- Real PostgreSQL admission/token replay/closure-version races; headed confirmation/progress video

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
