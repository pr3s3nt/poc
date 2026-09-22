---
id: DOC-RULES
artifact: documentation-standard
status: current
last_reviewed: 2026-09-22
---

# Documentation rules

## Artifact states

| Status | Meaning |
|---|---|
| `draft` | Nội dung đề xuất, chưa được chấp nhận. |
| `current` | Nội dung hiện hành trong declared scope. |
| `superseded` | Đã được artifact được chỉ rõ thay thế. |
| `deferred` | Vấn đề đã biết và chủ ý hoãn. |
| `historical` | Chỉ giữ provenance, không normative. |
| `evidence` | Observation của một lần execution, không normative. |

## Required metadata

Mọi Markdown artifact dưới `orchestrator_docs/` phải bắt đầu bằng:

```yaml
---
id: UC-06-SPEC
artifact: use-case-specification
status: current
last_reviewed: 2026-09-21
---
```

`id` phải duy nhất. Dùng thêm `related`, `supersedes` hoặc `superseded_by` khi
cải thiện navigation. Ngày review không tự quyết định authority.

## Content boundaries

- Specification định nghĩa required externally observable behavior.
- Realization/diagram định nghĩa collaboration để thực hiện behavior.
- Architecture định nghĩa shared structure và cross-cutting constraint.
- ADR ghi accepted decision và rationale.
- Backlog ghi open/deferred problem, không phải current design.
- Implementation docs ánh xạ design sang code và công khai deviation.
- Operation docs ghi procedure hiện hành.
- Verification record ghi execution cụ thể và không được sửa để mô tả run mới.
- Current state tổng hợp implemented scope; không định nghĩa requirement.
- Milestone/iteration docs sở hữu work order, scope và exit criteria; không sao
  chép hoặc thay thế use-case/architecture requirement. `WORK_ITEMS.md` chỉ là
  implementation checklist; execution evidence vẫn nằm trong `verification/`.

Một khái niệm current chỉ có một canonical owner. Artifact khác nên link hoặc
tóm tắt rõ ràng, không tạo định nghĩa độc lập.

## Links, IDs and diagrams

- Dùng repository-relative links; không dùng absolute filesystem link trong
  tài liệu repository.
- Giữ ổn định ID requirement như `MS-01`, `BR-02` trong phạm vi UC và artifact
  ID như `ADR-004`, `D01`.
- Khi đổi path hoặc ID, cập nhật inbound links trong cùng logical change.
- PlantUML `.puml` là source chuẩn; mỗi diagram phải có `.png` cùng basename và
  được liên kết từ một context/index Markdown.
- Regenerate PNG trong cùng change khi sửa PlantUML source.

## Semantic removal gate

Trước khi xóa hoặc thay một tài liệu hợp nhất:

1. Đọc toàn bộ predecessor, không chỉ tìm một vài identifier.
2. Chỉ rõ canonical destination hoặc historical rationale cho mọi khái niệm còn
   mang tính hiện hành.
3. Đối chiếu behavior với code/test và schema với executable artifact khi cần.
4. Migrate mọi current gap trước khi retire predecessor.
5. Ghi commit/path để có thể truy xuất bằng `git show`.
6. Chạy documentation checker và tìm stale references.

Reconciliation của `plan.md` cũ nằm tại
[documentation reconciliation](implementation/documentation-reconciliation.md).

## Completion checklist

1. Sửa canonical owner trước.
2. Dùng change-impact map trong root `AGENTS.md`.
3. Cập nhật ADR/backlog/deviation/current state nếu scope thực sự thay đổi.
4. Cập nhật traceability và tests khi behavior thay đổi.
5. Tạo dated verification record nếu có execution mới.
6. Chạy `python3 scripts/check_docs.py` và `git diff --check`.
