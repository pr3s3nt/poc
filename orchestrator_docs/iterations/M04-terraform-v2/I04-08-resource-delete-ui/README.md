---
id: I04-08
artifact: iteration-plan
status: current
last_reviewed: 2026-10-14
related: [M04, ADR-014, IMP-021]
---

# I04-08 — Xóa tài nguyên và dependency qua Console

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Current sau I04-07 verified local; đã giao phiên cloud mới, chưa review/verify local.

## Phạm vi

Resource detail/progress read API, PE/Admin delete preview/confirmation/operation; root-only default checkboxes, explicit selected set, token/versions/idempotency và original-target destroy.

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

Developer denied, root UNREFERENCED/no consumer/no busy; exact-name confirmation liệt kê đúng mục đã tick; disabled blockers và bỏ tick kéo theo giải thích; stale/concurrent consumer thay đổi chặn trước destroy; verified absence/remaining IDs.

## Checks

- `cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...`
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
- Real PostgreSQL admission/token replay/selected-set/consumer/version races, rejection of whole invalid set and preservation of unchecked items; headed confirmation/progress video

## Handoff

Claude cloud viết code/test trên nhánh riêng `claude/m04-<task>`, commit cuối
có bàn giao ngắn; không sửa docs hoặc push refactor_ui. Codex lưu session/URL/
quyền thực tế, fetch mỗi 15 phút (900 giây), review và chạy lại checks local không skip.
Codex merge khi đạt, chạy thật và Playwright headed/quay video local; gửi lỗi
code về Claude. Cập nhật verification/Current handoff/Task order, commit/push
và xóa nhánh Claude. Giữ failed evidence và phân biệt adapter thật/fake.
