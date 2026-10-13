---
id: I04-07
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-07 — Definition, Workload class, Preview và renderer

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Chưa bắt đầu; không suy status này thành task bị bỏ hoặc đã implement.

## Phạm vi

Console chọn source/module/version/mappings/provider ở Definition; Workload chọn type/params/class/shared ID. Planner pin intent; score-k8s required trên hai profiles; safe progress.

## Nguồn chuẩn

- [usecase/UC-03/specification.md](../../../usecase/UC-03/specification.md)
- [usecase/UC-05/specification.md](../../../usecase/UC-05/specification.md)
- [usecase/UC-06/specification.md](../../../usecase/UC-06/specification.md)
- [usecase/UC-16/specification.md](../../../usecase/UC-16/specification.md)
- [architecture/contracts/workload-rendering.md](../../../architecture/contracts/workload-rendering.md)

## Code impact để giao Claude

- [backend/internal/planning](../../../../backend/internal/planning/)
- [backend/internal/adapters/scorek8s](../../../../backend/internal/adapters/scorek8s/)
- [frontend/src/features/platform](../../../../frontend/src/features/platform/)
- [frontend/src/features/workloads](../../../../frontend/src/features/workloads/)
- [frontend/src/features/preview](../../../../frontend/src/features/preview/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

UI không chọn module ở Workload, không fallback class/renderer; typed/advanced roundtrip và stale/busy/metadata errors đúng. Real CLI render có protected identity/secret tests.

## Checks

- `cd backend && go test ./... && go build ./...`
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
- Headed browser form/Preview recording; real score-k8s CLI contract tests

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
