---
id: I04-09
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-09 — Redis cho acceptance app hiện có

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Chưa bắt đầu; không suy status này thành task bị bỏ hoặc đã implement.

## Phạm vi

Go frontend/backend/worker job flow: PostgreSQL durable result, worker Redis write-through, backend cache hit/miss/fallback/refill với auth/TLS bindings.

## Nguồn chuẩn

- [operations/terraform-v2-acceptance.md](../../../operations/terraform-v2-acceptance.md)
- [architecture/contracts/terraform-modules.md](../../../architecture/contracts/terraform-modules.md)

## Code impact để giao Claude

- [backend/examples/acceptance-app](../../../../backend/examples/acceptance-app/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

Job submit/completion/reload đúng; Redis outage/auth sai không pass nhờ fallback; no duplicate job, safe diagnostics hiện có; không thêm màn hình kiểm chứng riêng.

## Checks

- `cd backend && go test ./... && go build ./...`
- Redis/PostgreSQL disposable integration cho hit/miss/refill và DB commit/cache-error semantics

## Handoff

Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
