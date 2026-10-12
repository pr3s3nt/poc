---
id: D13
artifact: backlog-item
status: deferred
last_reviewed: 2026-10-13
related: [ADR-014, D12, UC-08]
---

# D13 — Reconcile v2 resources created before state acknowledgement

Provider có thể tạo object trước khi runner/backend lưu state thành công.
Người dùng hoãn automated discovery/import reconciliation. V2 giữ write-ahead
identity/tags/physical intent, private recovery artifacts và chặn CREATE retry
khi outcome chưa rõ; operator xử lý/cleanup có kiểm chứng ownership.

## Phạm vi tương lai

- Discover bằng original target, run tags và module object mapping.
- Chứng minh ownership bằng nhiều nguồn; không nhận object chỉ do tên giống.
- Review reconciliation plan, import vào đúng state có lock/fencing, validate
  dependency/outputs trước resume; ambiguous candidates dừng để operator xử lý.
- Không dùng orphan cleanup để xóa resource preexisting/updated của run khác.

## Điều kiện hoàn thành

Failure injection giữa provider create và backend acknowledgement chứng minh
khôi phục đúng một instance không duplicate hoặc data loss; foreign/same-name/
missing evidence bị từ chối. Có tests cho từng supported module object mapping
và scoped real-provider proof khi được giao. Không tiếp quản tài nguyên v1;
generic Echo/External vẫn [D11](D11-existing-external-resources.md).
