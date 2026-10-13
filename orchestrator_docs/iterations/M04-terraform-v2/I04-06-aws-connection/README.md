---
id: I04-06
artifact: iteration-plan
status: current
last_reviewed: 2026-10-14
related: [M04, ADR-014, IMP-021]
---

# I04-06 — AWS Connection qua Console và Vault

## Chặng và trạng thái

Chặng 2; bắt đầu sau I04-05 đã verified. Xem
[evidence modules](../../../verification/2026-10-14-i04-05-aws-modules.md).

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

- `cd backend && REQUIRE_POSTGRES=1 REQUIRE_TERRAFORM=1 go test ./... && go build ./...`
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
- Real isolated PostgreSQL credential-reference/rollback/reopen checks; headed UI video với secret masking

## Handoff

Claude cloud viết code/test trên nhánh riêng `claude/m04-<task>`, commit cuối
có bàn giao ngắn; không sửa docs hoặc push refactor_ui. Codex lưu session/URL/
quyền thực tế, fetch mỗi 240 giây, review và chạy lại checks local không skip.
Codex merge khi đạt, chạy thật và Playwright headed/quay video local; gửi lỗi
code về Claude. Cập nhật verification/Current handoff/Task order, commit/push
và xóa nhánh Claude. Giữ failed evidence và phân biệt adapter thật/fake.
