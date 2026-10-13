---
id: I04-00
artifact: iteration-plan
status: historical
last_reviewed: 2026-10-13
related: [M04, ADR-014, IMP-021]
---

# I04-00 — Duyệt tài liệu A–D và plan M04

## Chặng và trạng thái

Chặng 1; xem [M04](../README.md). A–D đã được chuẩn bị và validated; người dùng đã duyệt gate 1 sau sửa selected-set và coverage I06-10.

## Phạm vi

Sửa trực tiếp luật, chuyển implementation commentary, bổ sung Xóa tài nguyên và chọn tập xóa qua ô tick; đồng bộ diagrams/UI/contracts/schema/runbook.

## Nguồn chuẩn

- [architecture/decisions/ADR-014-terraform-v2-lifecycle.md](../../../architecture/decisions/ADR-014-terraform-v2-lifecycle.md)
- [implementation/v2-rule-reconciliation.md](../../../implementation/v2-rule-reconciliation.md)
- [traceability/matrix.md](../../../traceability/matrix.md)

## Dependencies và quyền

Không có code dependency; chỉ làm tài liệu.
Chỉ các môi trường và mutation trong chặng hiện tại được giao. Không reset
retained database, sửa cluster-wide component hoặc tạo AWS ngoài gate.
Module/provider pins, deadline và small sizing ghi trước execution.

## Acceptance criteria

A–D validated, commit/push; người dùng duyệt bộ tài liệu/plan trước code. Gate 1 đã được người dùng duyệt có điều kiện hoàn tất sửa đổi trong lượt này.

## Checks

- `REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`
- `git diff --check`

## Handoff

[Docs verification](../../../verification/2026-10-13-terraform-v2-reconciliation.md);
Người dùng đã duyệt chặng 1 trong yêu cầu mới; chuyển sang I04-01 sau validation và commit/push sửa đổi.


Codex giao Claude source/scope/criteria/checks qua clauded trong tmux task riêng
khi code stage được duyệt. Claude không sửa docs/commit/push. Codex review và
verify độc lập; kiểm tiến độ bình thường mỗi 240 giây, không poll liên tục.
Evidence thật/fake và checks skipped phải phân biệt; failed video/phase marks
giữ lại. Code-map/CURRENT_STATE/traceability chỉ ghi actual results sau delivery.
