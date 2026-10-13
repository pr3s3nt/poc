---
id: ADR-011
artifact: architecture-decision
status: superseded
superseded_by: ADR-014
last_reviewed: 2026-10-13
related: [ADR-012, ADR-013]
---

# ADR-011 — Environment execution binding set once (lịch sử)

Quyết định này được thay bởi ADR-012/013/014. Toàn bộ predecessor, gồm rationale,
namespace/state/physical-name format, legacy migration và old verification
limits, có thể xem bằng `git show a897d6b:orchestrator_docs/architecture/decisions/ADR-011-environment-execution-binding.md`
hoặc `git show 3367f7e:<cùng-path>`. Không dùng record này như luật hiện hành.

## Canonical destinations

| Nội dung cũ | Nguồn chuẩn hiện hành |
|---|---|
| Set-once selection, immutable Save/admission/UI | [ADR-012](ADR-012-environment-stores-and-transitions.md): selection/version/operation/generation và transition có xác nhận |
| User Definition cho internal cluster | [ADR-013](ADR-013-implicit-existing-cluster.md): system Connection binding |
| Logical Environment descriptors, @infra và generation isolation | UC-06 BR-02/12, [module contract](../contracts/terraform-modules.md), [state identity](../contracts/terraform-state.md), schema |
| AWS database dùng account riêng | [ADR-014](ADR-014-terraform-v2-lifecycle.md): mọi AWS node cùng Environment account/region |
| Physical names chứa run ID/state tạm | ADR-014/state contract: stable resource instance identity; run ID là audit |
| Application target/backfill/legacy scope/migration ledger | [schema](../database/schema.md): identity-only Application, ENVIRONMENT scope, fresh baseline; không v1 import/backfill |
| Old same-key conflicts/permanent SQL immutability trigger | OC-01b và ADR-012: versioned CAS/no-op, temporary operation fencing |
| Kết quả code/tests/kind và T26..T29 | CURRENT_STATE, IMP-021 và verification lịch sử; không proof cho v2 |

V2 không đổi logical descriptor grammar hoặc viết lại historical snapshots.
Tài nguyên tạo ở generation cũ vẫn dùng stored original target khi query/cleanup;
không tham chiếu lại lựa chọn Environment hiện tại để đổi ownership.
