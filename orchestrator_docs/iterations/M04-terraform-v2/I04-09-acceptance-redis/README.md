---
id: I04-09
artifact: iteration-plan
status: current
last_reviewed: 2026-10-14
related: [M04, ADR-014, IMP-021]
---

# I04-09 — Redis cho acceptance app hiện có

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Current sau I04-08 verified local; phiên cloud session_019GgofsetFjxTDq6V83vv6Q đang chạy từ ee41f1d; chưa verified.

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

- `cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...`
- Redis/PostgreSQL disposable integration cho hit/miss/refill và DB commit/cache-error semantics

## Handoff

Claude cloud viết code/test trên nhánh riêng `claude/m04-<task>`, commit cuối
có bàn giao ngắn; không sửa docs hoặc push refactor_ui. Codex lưu session/URL/
quyền thực tế, fetch mỗi 15 phút (900 giây), review và chạy lại checks local không skip.
Codex merge khi đạt, chạy thật và Playwright headed/quay video local; gửi lỗi
code về Claude. Cập nhật verification/Current handoff/Task order, commit/push
và xóa nhánh Claude. Giữ failed evidence và phân biệt adapter thật/fake.
