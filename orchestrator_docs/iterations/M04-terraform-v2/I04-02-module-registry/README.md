---
id: I04-02
artifact: iteration-plan
status: current
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-02 — Registry module bất biến

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Bắt đầu sau I04-01 đã verified; chưa claim registry hoàn tất.

## Phạm vi

Source policy/verified cache/inspector/registry, exact revision/digest và provider pins; typed inputs/output/provider mappings; metadata read-only.

## Nguồn chuẩn

- [architecture/contracts/terraform-modules.md](../../../architecture/contracts/terraform-modules.md)
- [usecase/UC-03/specification.md](../../../usecase/UC-03/specification.md)
- [usecase/UC-03/realization.md](../../../usecase/UC-03/realization.md)

## Code impact để giao Claude

- [backend/internal/application/catalog](../../../../backend/internal/application/catalog/)
- [backend/internal/adapters/terraform](../../../../backend/internal/adapters/terraform/)
- [backend/internal/planning](../../../../backend/internal/planning/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

Allowlist/integrity/transitive pins được kiểm trước insert/execution; source mutable/command/traversal/credential bị chặn; Preview/capability không fetch/run provider.

## Checks

- `cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...`
- Catalog/registry/HTTP tests, immutable bundle SQL reopen/race trên PostgreSQL cô lập

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
