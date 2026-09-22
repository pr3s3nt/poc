---
id: ITERATION-INDEX
artifact: iteration-index
status: current
last_reviewed: 2026-09-22
---

# Iteration index

## Active

- [M01 — Contract hardening](M01-contract-hardening/README.md)
  - [I06-06 — Implement Deployment Delta Snapshot](M01-contract-hardening/I06-06-imp008-delta-snapshot/README.md)

## Roadmap

1. [M01 — Contract hardening](M01-contract-hardening/README.md): IMP-010
   (done, I06-05) → IMP-008 → IMP-009.
2. [M02 — Use-case completion](M02-usecase-completion/README.md): hoàn thiện
   UC-09, UC-05, UC-07 và management flows UC-01..04.
3. [M03 — Production and external compatibility](M03-production-compatibility/README.md):
   IMP-001, rồi IMP-006/007 khi boundary tương ứng được yêu cầu.

## Folder contract

Mỗi milestone folder có `README.md` sở hữu objective, order và milestone exit
criteria. Mỗi iteration folder chỉ có:

- `README.md`: charter, scope, dependencies, exit criteria, status và outcome;
- `WORK_ITEMS.md`: implementation order, code/test impact và handoff checklist.

Iteration không sở hữu product requirement hoặc architecture. Agent phải đọc
canonical specification/realization/architecture được link, không sửa chúng để
hợp thức hóa code. Execution evidence nằm trong `verification/`, không sao chép
vào iteration folder.

Chỉ một iteration có status `current`. Iteration chưa đến lượt dùng `deferred`;
khi hoàn thành, ghi outcome/verification links và chuyển sang `historical`.
