---
id: I04-11
artifact: iteration-plan
status: deferred
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-11 — V2-A01/E01/E02 và dự toán AWS

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Chưa bắt đầu; không suy status này thành task bị bỏ hoặc đã implement.

## Phạm vi

Chạy real adapters qua headed Console/app UI trên namespace orch-v2-test-run; A01 job/cache + user deletion, E01 no-create và E02 controlled readiness failure. Chuẩn bị estimate AWS.

## Nguồn chuẩn

- [operations/terraform-v2-acceptance.md](../../../operations/terraform-v2-acceptance.md)
- [usecase/UC-07/specification.md](../../../usecase/UC-07/specification.md)
- [operations/k8s4f-dev.md](../../../operations/k8s4f-dev.md)

## Code impact để giao Claude

- [backend/test/integration](../../../../backend/test/integration/)
- [frontend/test/e2e](../../../../frontend/test/e2e/)

New state/cleanup ports/adapters được đặt theo dependency rule của project;
không copy code từ orchestrator_reference. Exact new paths cập nhật code map
sau implement, không tạo link tới file chưa có.

## Dependencies và quyền

Sau gate 1 và các task trước trong bảng M04; task registry/executor/modules có thể ghép batch khi dependency đã đủ.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS trước I04-11 đạt.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

Ba scenario pass có video/phụ đề Việt/phase marks/assertions; A01 UI delete verified, failed evidence giữ; owned cleanup sạch và retained k8s-4f nguyên vẹn. Lưu video A01 + dự toán; khi cả ba scenario pass và k8s-4f sạch, tự sang I04-12.

## Checks

- Real V2-A01/V2-E01/V2-E02 assertions và provider absence queries
- Video decode/frame/subtitle/secret review; bash -n runner scripts
- AWS cost table có nguồn giá/ngày/sizing/số lượng/deadline và theo từng A02/A03; chỉ read-only preparation

## Handoff

Claude cloud viết code/test và push nhánh riêng; Codex review, tự chạy checks
local không skip, merge và chạy thật/Playwright headed/quay video local.
Lưu session ID/URL/quyền thực tế; fetch mỗi 15 phút (900 giây). Gửi lỗi code về Claude.
Cập nhật verification/Current handoff/Task order, commit/push và xóa nhánh.
Giữ failed evidence và phân biệt adapter thật/fake; bàn giao cuối chờ người dùng.
