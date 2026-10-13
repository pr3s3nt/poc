---
id: I04-12
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-12 — V2-A02/A03, cleanup và handoff cuối

## Chặng và trạng thái

Chặng 3; xem [M04](../README.md). Chưa bắt đầu; không suy status này thành task bị bỏ hoặc đã implement.

## Phạm vi

Sau I04-11 đạt và video/dự toán đã lưu, chạy tuần tự A02/A03 AWS qua UI với real adapters; cleanup ngay từng run và query sạch; independent review/final checks/commit-push/handoff.

## Nguồn chuẩn

- [operations/aws.md](../../../operations/aws.md)
- [operations/terraform-v2-acceptance.md](../../../operations/terraform-v2-acceptance.md)
- [architecture/decisions/ADR-014-terraform-v2-lifecycle.md](../../../architecture/decisions/ADR-014-terraform-v2-lifecycle.md)
- [traceability/matrix.md](../../../traceability/matrix.md)

## Code impact để giao Claude

- [backend/test/integration](../../../../backend/test/integration/)
- [frontend/test/e2e](../../../../frontend/test/e2e/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Điểm duyệt 2 đã bỏ ngày 2026-10-14; A01/E01/E02 và SQL checks đạt, estimate/cleanup readiness đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS trước I04-11 đạt.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

A02/A03 full job/cache assertions + video; không còn run-owned cloud assets; docs/code/tests/traceability nhất quán. Người dùng duyệt final handoff và cleanup evidence, không tự claim approved.

## Checks

- Real V2-A02/V2-A03; preflight account/region/permissions/pins/estimates và provider absence checks
- `cd backend && REQUIRE_POSTGRES=1 go test ./... && go build ./...`
- `cd frontend && npm run typecheck && npm run lint && npm test && npm run build`
- `REQUIRE_PLANTUML=1 python3 scripts/check_docs.py; git diff --check`

## Handoff

Claude cloud viết code/test và push nhánh riêng; Codex review, tự chạy checks
local không skip, merge và chạy thật/Playwright headed/quay video local.
Lưu session ID/URL/quyền thực tế; fetch mỗi 15 phút (900 giây). Gửi lỗi code về Claude.
Cập nhật verification/Current handoff/Task order, commit/push và xóa nhánh.
Giữ failed evidence và phân biệt adapter thật/fake; bàn giao cuối chờ người dùng.
