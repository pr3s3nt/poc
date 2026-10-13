---
id: I04-07
artifact: iteration-plan
status: current
last_reviewed: 2026-10-14
related: [M04, ADR-014, IMP-021]
---

# I04-07 — Definition, Workload class, Preview và renderer

## Chặng và trạng thái

Chặng 2; bắt đầu sau I04-06 đã verified local, đang chuẩn bị giao cloud. Xem [M04](../README.md) và [biên bản I04-06](../../../verification/2026-10-14-i04-06-aws-connection.md).

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

- `cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...`
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
- Headed browser form/Preview recording; real score-k8s CLI contract tests

## Handoff

Claude cloud viết code/test trên nhánh riêng `claude/m04-<task>`, commit cuối
có bàn giao ngắn; không sửa docs hoặc push refactor_ui. Codex lưu session/URL/
quyền thực tế, fetch mỗi 15 phút (900 giây), review và chạy lại checks local không skip.
Codex merge khi đạt, chạy thật và Playwright headed/quay video local; gửi lỗi
code về Claude. Cập nhật verification/Current handoff/Task order, commit/push
và xóa nhánh Claude. Giữ failed evidence và phân biệt adapter thật/fake.
