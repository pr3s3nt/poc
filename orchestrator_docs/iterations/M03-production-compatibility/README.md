---
id: M03
artifact: milestone-plan
status: deferred
last_reviewed: 2026-09-22
related: IMP-001, IMP-006, IMP-007
---

# M03 — Production and external compatibility

## Objective

Thay persistence baseline bằng PostgreSQL và khóa các external compatibility
boundaries thực sự cần cho production rollout. Structured errors và remote
Terraform là conditional, không tự trở thành requirement chỉ vì nằm trong
roadmap.

## Iteration order

1. [I06-11 — PostgreSQL system of record](I06-11-imp001-postgres-system-of-record/README.md).
2. [I06-12 — Structured planner errors](I06-12-imp006-structured-errors/README.md),
   chỉ activate nếu public/challenge error compatibility được duyệt.
3. [I06-13 — Remote Terraform runtime](I06-13-imp007-remote-terraform/README.md),
   chỉ activate nếu embedded allowlisted modules không còn đủ.

## Milestone exit criteria

- IMP-001 được đóng bằng migrations, PostgreSQL adapters và transaction tests.
- IMP-006/007 được implement hoặc giữ deferred bằng quyết định release rõ ràng;
  milestone không ngầm phê duyệt hai capability này.
- Security, secret, tenant isolation, backup/recovery và operational runbooks
  cho capability đã activate được verify.
- Release gate còn lại như D01 durable Terraform state được xử lý hoặc ghi rõ là
  blocker; milestone này không tự sở hữu D01.

## Out of scope

- D05 full Humanitec deployment lifecycle nếu chưa được duyệt riêng.
- Rollback/retry/RBAC/audit ngoài use-case/production scope đã chọn.
- Cloud mutation không được phép chỉ vì iteration được activate; mỗi external
  verification vẫn theo runbook và user authorization.
