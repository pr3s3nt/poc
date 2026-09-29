---
id: D04
artifact: backlog-item
status: historical
last_reviewed: 2026-09-29
---

# D04 — PostgreSQL system-of-record adapter

Architecture đã định nghĩa PostgreSQL schema và repository/UnitOfWork boundary,
nhưng executable baseline dùng in-memory aggregate maps cộng JSON snapshot.

Adapter PostgreSQL được lên kế hoạch sau UC-09, cùng migration, transaction và
repository tests. Implementation phải bám schema canonical; không đổi schema để
hợp thức hóa snapshot format.

## Resolution — 2026-09-29

Đã thay atomic JSON document bằng normalized PostgreSQL repositories, versioned
migration ledger và PostgreSQL-backed `UnitOfWork`. D08 được resolve bằng
`deployment_delta_snapshots.deployment_id UNIQUE NOT NULL`; integration test
kiểm tra restart, orphan rejection và deferred lifecycle constraint.
