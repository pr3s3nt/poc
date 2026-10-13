---
id: I04-01
artifact: iteration-plan
status: current
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-01 — State backend, schema và ownership graph

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Được giao bắt đầu sau gate 1; chưa có bằng chứng implementation/checks hoàn thành.

## Phạm vi

Implement HTTP state/capabilities, ciphertext/key boundary, instance/attempt/dependency persistence và hai lớp locking/CAS/fencing; không automatic recovery/import/backup.

## Nguồn chuẩn

- [architecture/database/schema.md](../../../architecture/database/schema.md)
- [architecture/contracts/terraform-state.md](../../../architecture/contracts/terraform-state.md)
- [architecture/domain/domain-objects.md](../../../architecture/domain/domain-objects.md)
- [architecture/contracts/operation-contracts.md](../../../architecture/contracts/operation-contracts.md)

## Code impact để giao Claude

- [backend/internal/adapters/postgres](../../../../backend/internal/adapters/postgres/)
- [backend/internal/ports](../../../../backend/internal/ports/)
- [backend/internal/delivery/http](../../../../backend/internal/delivery/http/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

State tồn tại sau restart; hai tiến trình không ghi chồng; stale fence/unlock/decryption/serial/scope bị chặn; write failure rollback; dependency graph đủ cho cleanup.

## Checks

- `cd backend && go test ./... && go build ./...`
- Real disposable PostgreSQL: full opt-in persistence suite, race/reopen/rollback, zero skipped tests của contract mới

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.

## Handoff đang chờ quyết định DB privilege — 2026-10-13

Claude đã implement state backend và contract tests; Codex đã review sớm và
chạy độc lập toàn bộ Go tests trên PostgreSQL disposable. Chưa hoàn thành:
DB role hiện vẫn sở hữu mọi bảng, chưa đáp ứng tách quyền state/public query.
Người dùng đang yêu cầu giải thích trước khi chọn separate roles/connections
hoặc SQL SECURITY DEFINER; chưa chọn option nào. Không tự giảm yêu cầu.

Code hiện chưa commit, được giữ nguyên trong workspace; evidence ở
`/tmp/poc-m04-i04-01-20261013/` và `/tmp/poc-i0401-codex-review/`.
Cả hai PostgreSQL container task đã dọn. Internal loopback handler có tests,
nhưng mounting cho executor production vẫn thuộc I04-03. Chưa chạy browser,
k8s-4f hoặc AWS. Sau quyết định, giao Claude tiếp phần DB privilege rồi review
và verify trước I04-02. Chi tiết [verification](../../../verification/2026-10-13-i04-01-state-backend-partial.md).
