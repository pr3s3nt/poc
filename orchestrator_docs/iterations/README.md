---
id: ITERATION-INDEX
artifact: iteration-index
status: current
last_reviewed: 2026-09-23
---

# Iteration index

## Active

- [M00-a — Developer onboarding](M00-developer-onboarding/README.md)
  - [I00-00 — UC-00 and UC-01 developer onboarding](M00-developer-onboarding/I00-00-uc00-uc01-developer-onboarding/README.md)

## Roadmap

1. [M00-a — Developer onboarding](M00-developer-onboarding/README.md): thiết kế
   UI và implement UC-00 sign-in + UC-01 self-service Application. Current.
2. [M01 — Contract hardening](M01-contract-hardening/README.md): IMP-010
   (done, I06-05) → IMP-008 (done, I06-06) → IMP-009 (done, I06-07).
   Historical; hoàn thành 2026-09-22.
3. [M02 — Use-case completion](M02-usecase-completion/README.md): hoàn thiện
   UC-09, UC-05, UC-07 và management flows còn lại của UC-01..04. Deferred
   sau M00-a.
4. [M03 — Production and external compatibility](M03-production-compatibility/README.md):
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
