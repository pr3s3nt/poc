---
id: VERIFY-2026-10-13-TERRAFORM-V2-RECONCILIATION
artifact: verification-record
status: evidence
last_reviewed: 2026-10-13
related: [ADR-014, M04, IMP-021]
---

# A–D: rule reconciliation, deletion UI design và plan M04

## Scope và user decisions

User review yêu cầu sửa trực tiếp canonical rules, giữ stable IDs, chuyển
code status khỏi spec/realization, thêm PE/Admin deletion UI và lập M04 ba chặng.
Trong lượt này người dùng xác nhận thêm: **xóa cả dependency đủ điều kiện,
liệt kê trong hộp xác nhận**. AWS access-key UI/Vault/host-credential boundary
được giữ nguyên. Không có product/external-scope decision tự mở rộng khác.

Working tree lúc bắt đầu sạch tại 3367f7e, branch refactor_ui. Tất cả thay đổi
thuộc documentation/PlantUML PNG; không sửa source/test hoặc external resources.
Không tạo Claude tmux session vì chặng 1 chỉ sửa docs; chặng 2 chờ duyệt.

## Canonical reconciliation

[Rule map](../implementation/v2-rule-reconciliation.md) ghi toàn bộ predecessor
a897d6b/3367f7e và canonical destination cho ID/nội dung gộp/retire. Đã đọc
spec/realization/contract/schema/domain/ADR cũ trước sửa. Implementation mô tả
chuyển sang CURRENT_STATE/IMP-021, evidence cũ giữ nguyên. ADR-001 giữ scope/
enrichment, ADR-010 thay một phần renderer policy, ADR-011 superseded.
Deletion preview/confirmed scope/role/name/consumer/ownership/cascade/versions/
progress/verification đồng bộ UC-07/09, OC-19, schema, diagrams và A01 runbook.
M04 là current work order, M02/I06-10 deferred nhưng chưa complete.

## Validation

- `REQUIRE_PLANTUML=1 python3 scripts/check_docs.py`: PASS, 329 Markdown
  files và 281 artifact IDs; links/iteration package rules và PlantUML syntax.
- `git diff --check`: PASS sau sửa whitespace và table continuations.
- Render 11 changed/new diagrams về sibling PNG; reviewed deletion sequence
  ở ảnh render, gồm atomic admission, 202 operation và stages trước calls.
- Search toàn repository cho bốn cụm stale policy được yêu cầu: không còn
  match trong working tree. Specs/realizations/README trong scope không còn
  đoạn mẫu V2 design scope hoặc implementation-status section.
- Review confirmed cascade/retained dependency rules, role/token/name checks,
  graph-version persistence, current ID traceability và ba gate M04.

Lần checker đầu phát hiện relative link khi chuyển UC-03 prose sang
CURRENT_STATE; đã sửa path theo destination, giữ full checker requirements.

Go/frontend/real PostgreSQL/live k8s/AWS checks không chạy vì documentation-only
checkpoint. Các gates đó được giao rõ trong M04, không claim pass từ old evidence.
Full recovery/orphan reconciliation/platform backup vẫn D12–D14 deferred.

## Approval handoff

Sau docs review/validation và commit/push, **dừng chờ người dùng duyệt chặng 1**.
Không code/test mutation trước gate 1. Sau chặng 2, video A01 và AWS estimate
phải được review/duyệt trước AWS create. Final AWS handoff/absence proof có
điểm duyệt riêng; cleanup owned assets vẫn tiếp tục nếu cần để không để tài
nguyên tồn tại trong khi chờ review. Không coi documentation PASS là approval.
