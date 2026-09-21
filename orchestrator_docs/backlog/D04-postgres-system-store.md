---
id: D04
artifact: backlog-item
status: deferred
last_reviewed: 2026-09-21
---

# D04 — PostgreSQL system-of-record adapter

Architecture đã định nghĩa PostgreSQL schema và repository/UnitOfWork boundary,
nhưng executable baseline dùng in-memory aggregate maps cộng JSON snapshot.

Adapter PostgreSQL được lên kế hoạch sau UC-09, cùng migration, transaction và
repository tests. Implementation phải bám schema canonical; không đổi schema để
hợp thức hóa snapshot format.
