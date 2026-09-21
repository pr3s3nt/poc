---
id: D01
artifact: backlog-item
status: deferred
last_reviewed: 2026-09-21
---

# D01 — Durable Terraform state and stable physical names

## Problem

Terraform state hiện nằm trong thư mục tạm theo run ID; tên VPC/EKS/Aurora cũng
chứa run ID. Mỗi verification vì vậy tạo hạ tầng vật lý mới dù logical resource
identity đã ổn định theo Application.

## Deferred decision

Chọn durable backend, locking, ownership và migration/reconciliation policy.
Không bỏ run ID khỏi physical names trước khi state lifecycle được quyết định,
vì có thể làm mất khả năng cleanup chính xác.
