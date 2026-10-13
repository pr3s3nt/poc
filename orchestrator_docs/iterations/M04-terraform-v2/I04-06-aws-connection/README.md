---
id: I04-06
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-06 — AWS Connection qua Console và Vault

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Chưa bắt đầu; không suy status này thành task bị bỏ hoặc đã implement.

## Phạm vi

Implement access-key onboarding đúng quyết định người dùng: concealed UI, scoped Vault ref, identity/region/capability checks và Terraform invocation resolution.

## Nguồn chuẩn

- [usecase/UC-04/specification.md](../../../usecase/UC-04/specification.md)
- [usecase/UC-04/realization.md](../../../usecase/UC-04/realization.md)
- [usecase/UC-04/ui/api-mapping.md](../../../usecase/UC-04/ui/api-mapping.md)
- [architecture/decisions/ADR-009-aws-access-key-storage.md](../../../architecture/decisions/ADR-009-aws-access-key-storage.md)

## Code impact để giao Claude

- [backend/internal/application/connection](../../../../backend/internal/application/connection/)
- [backend/internal/adapters/vault](../../../../backend/internal/adapters/vault/)
- [frontend/src/features/platform](../../../../frontend/src/features/platform/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

STS không giả full READY; failure không lưu/overwrite credential winner; read DTO/evidence không có key. Không dùng host credential trong product execution.

## Checks

- `cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...`
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
- Real isolated PostgreSQL credential-reference/rollback/reopen checks; headed UI video với secret masking

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
