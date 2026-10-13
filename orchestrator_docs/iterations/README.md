---
id: ITERATION-INDEX
artifact: iteration-index
status: current
last_reviewed: 2026-10-13
---

# Iteration index

## Active

- [M04 — Terraform v2](M04-terraform-v2/README.md)
  - [I04-00 — A–D và duyệt chặng 1](M04-terraform-v2/I04-00-design-review/README.md): tài liệu đang hoàn thiện, chờ người dùng duyệt trước code.
- [M02 — Use-case completion](M02-usecase-completion/README.md) và I06-10 deferred do reprioritize, chưa complete.

## Roadmap

1. [M00-a — Developer onboarding](M00-developer-onboarding/README.md): UC-00
   sign-in + UC-01 self-service Application. Historical; hoàn thành 2026-09-30.
2. [M01 — Contract hardening](M01-contract-hardening/README.md): IMP-010
   (done, I06-05) → IMP-008 (done, I06-06) → IMP-009 (done, I06-07).
   Historical; hoàn thành 2026-09-22.
3. [M02 — Use-case completion](M02-usecase-completion/README.md): hoàn thiện
   UC-09, UC-05, UC-07 và management flows còn lại của UC-02..04. Deferred;
   I06-04/I06-08/I06-09 đã hoàn thành; I06-10 còn pending, M04 ưu tiên hiện tại.
4. [M03 — Production and external compatibility](M03-production-compatibility/README.md):
   IMP-001, rồi IMP-006/007 khi boundary tương ứng được yêu cầu.

5. [M04 — Terraform v2](M04-terraform-v2/README.md): current, ba chặng có gate duyệt; code bắt đầu sau gate 1.

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
