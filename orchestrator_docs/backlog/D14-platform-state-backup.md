---
id: D14
artifact: backlog-item
status: deferred
last_reviewed: 2026-10-13
related: [ADR-014, D12, D13]
---

# D14 — Platform metadata và Terraform state backup/restore

Người dùng hoãn backup/restore automation và disaster-recovery rehearsal của
platform. V2 lưu durable encrypted Terraform state trong PostgreSQL, nhưng không
cam kết phục hồi khi database hoặc encryption key bị mất. Không suy durability
thành backup hoặc dùng evidence restore T28/T29 làm proof cho Terraform v2.

## Phạm vi tương lai

- Chốt RPO/RTO, lịch/retention, storage, quyền và encryption-key recovery.
- Backup nhất quán platform metadata, state blobs, ownership/lock audit và secret
  reference dependencies; bảo vệ private secret stores/key material tương ứng.
- Restore trong isolation; invalidate executor capabilities/owners cũ, đối chiếu
  provider thực tế trước mở execution và xử lý stale snapshot.
- Backup không đưa state/secrets vào Git, public evidence hoặc video.

## Điều kiện hoàn thành

Runbook đã review; encrypted backup và key recovery thực sự restore được trên
môi trường cô lập; chứng minh refs/state lineage/ownership nhất quán và không
replay stale apply/destroy. RPO/RTO measured, assertions và cleanup evidence
được lưu. Chốt quyền external verification riêng khi triển khai backlog này.
