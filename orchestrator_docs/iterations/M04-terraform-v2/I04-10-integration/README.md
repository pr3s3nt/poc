---
id: I04-10
artifact: iteration-plan
status: current
last_reviewed: 2026-10-14
related: [M04, ADR-014, IMP-021]
---

# I04-10 — Tích hợp và runner/evidence/cleanup supervisor

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Current sau I04-09 verified local; cloud session_01HtXzgfyBzoNma2DQ2SzNY5 đang chạy từ55b2fa0; chưa verified.

## Phạm vi

Tích hợp planner->Terraform->outputs/Secret refs->real score-k8s->delivery/readiness; independent cleanup supervisor, evidence phase/video/assertion retention và failure fixtures.

## Nguồn chuẩn

- [usecase/UC-05/specification.md](../../../usecase/UC-05/specification.md)
- [usecase/UC-06/specification.md](../../../usecase/UC-06/specification.md)
- [usecase/UC-08/specification.md](../../../usecase/UC-08/specification.md)
- [operations/terraform-v2-acceptance.md](../../../operations/terraform-v2-acceptance.md)
- [operations/claude-tmux.md](../../../operations/claude-tmux.md)

## Code impact để giao Claude

- [backend/test/integration](../../../../backend/test/integration/)
- [backend/test/e2e](../../../../backend/test/e2e/)
- [frontend/test/e2e](../../../../frontend/test/e2e/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

Product runtime dùng PostgreSQL thật và scoped Vault/Connections; invalid input chặn trước create; fake không thế acceptance; runner failure không làm mất ownership/state hoặc cleanup handles.

## Checks

- `cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...`
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
- Real disposable PostgreSQL full integration/race/reopen, no required SQL tests skipped
- bash -n các scripts thay đổi; module validate và headed recording tooling

## Handoff

Claude cloud viết code/test và push nhánh riêng; Codex review, chạy checks
local không skip, merge và chạy thật/Playwright headed/quay video local.
Lưu session ID/URL/quyền thực tế; fetch mỗi 15 phút (900 giây). Gửi lỗi code về Claude.
Cập nhật verification/Current handoff/Task order, commit/push và xóa nhánh.
Giữ failed evidence và phân biệt adapter thật/fake.
