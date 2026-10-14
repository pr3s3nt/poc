---
id: I04-11
artifact: iteration-plan
status: current
last_reviewed: 2026-10-14
related: [M04, ADR-014, IMP-021]
---

# I04-11 — V2-A01/E01/E02 và dự toán AWS

## Chặng và trạng thái

Chặng 2; xem [M04](../README.md). Current sau I04-10 local checks đạt;
Codex đã chạy A01 lượt đầu `a01-20261014-r1`, chưa có scenario pass.
Đăng ký kho bí mật lỗi422: fixture token thiếu quyền tự kiểm tra Vault.
Namespace/PV đã xác minh vắng, sentinel giữ nguyên; supervisor ban đầu chưa CLEAN.
Phục hồi độc lập đã CLEAN/CLOSED_CLEAN bằng handles gốc; database/ba role,
hai Vault và ảnh bổ trợ đã dọn. Bản sửa Luna qua checks độc lập1352Go/0skip,
622frontend/fullchecks, merge `72a807e`; retry duy nhất `a01-20261014-r2`
đã fail bước xóa workload do chưa trả lời native confirm, dù deploy/job/cache
đạt. Operator dọn workload/VSO/Redis/PG/renderer Secrets, namespace còn vì
default SA thiếu managedFields; chờ quyết định ngoại lệ, giữ DB/hai Vault/
private handles để phục hồi.28 baseline UIDs giữ nguyên. Không mở run mới.
Quyết định2026-10-15 thay giới hạn retry: tiếp tục tìm nguyên nhân/sửa code,
review/checks và chạy lại sau cleanup sạch; không dừng chỉ vì retry fail.
Quyết định ngoại lệ namespace vẫn pending, không bypass guard.
[Biên bản](../../../verification/2026-10-14-i04-11-k8s4f.md).

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

Theo quyết định mới nhất 2026-10-14, code/test chuyển sang Claude local qua
tmux/`clauded` khi cloud hết hạn mức; nếu Claude local tiếp tục hết thì agent
Codex `gpt-6-luna` viết tiếp. Writer không sửa docs/commit/push; Codex review,
chạy checks local không skip, commit/merge/push và chạy thật/headed video.
Lưu cloud head/session và tmux/agent ID, quyền thực tế, bàn giao code dở;
theo dõi mỗi15 phút, kiểm sớm khi lỗi/hoàn tất. Chi tiết
[runbook điều phối](../../../operations/claude-tmux.md).
Cập nhật verification/Current handoff/Task order; giữ failed evidence, cleanup
và điểm duyệt cuối. Không tự ghi đã duyệt.
