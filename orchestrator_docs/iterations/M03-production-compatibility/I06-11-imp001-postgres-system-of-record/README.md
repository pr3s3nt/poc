---
id: I06-11
artifact: iteration-plan
status: historical
last_reviewed: 2026-09-29
related: IMP-001, D04, D08
---

# I06-11 — Implement PostgreSQL system of record

## Objective

Đóng IMP-001 bằng PostgreSQL migrations/adapters/UnitOfWork theo canonical
schema, thay in-memory map + JSON snapshot làm authoritative logical state.

## In scope

- Resolve D08 trước khi khóa migration Snapshot.
- Versioned migrations, constraints/indexes và repository adapters cho current
  aggregates/read models.
- Transaction boundaries ADR-002, optimistic Environment version và per-resource
  progress persistence.
- Integration tests trên disposable PostgreSQL, backup/recovery/config runbook.
- Migration/bootstrap policy cho executable baseline data nếu cần.

## Out of scope

- Terraform physical state trong PostgreSQL; D01 sở hữu backend/state lifecycle.
- Secret value persistence.
- Remote Terraform, public error compatibility hoặc D05 lifecycle.

## Exit criteria

- Product restart giữ logical state và repository contract tests pass trên
  PostgreSQL.
- Không giữ DB transaction qua cloud/Kubernetes/Terraform calls.
- Concurrency/current-set/one-to-one Snapshot constraints được verify.
- IMP-001 và D04 được đóng; D08 được resolve; operations/current state/evidence
  được cập nhật.

## First next action

Review schema/ERD against implemented repository contracts, resolve D08, rồi
viết migration contract tests trước adapter code.

## Outcome

Implemented normalized repositories and versioned migration ledger for every
current persistence port. PostgreSQL `UnitOfWork`, Environment optimistic
commit, Snapshot ownership/lifecycle constraints and restart behavior are
covered by integration verification. In-memory/JSON remains available for
local tests; PostgreSQL is selected explicitly by `-database-url-file`.
