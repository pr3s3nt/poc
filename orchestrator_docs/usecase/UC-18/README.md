---
id: UC-18-CONTEXT
artifact: use-case-context
status: current
last_reviewed: 2026-10-27
---

# UC-18 context — Xóa Application

## Trạng thái thực hiện

Implemented 2026-10-27. Quyết định người dùng ở
[ADR-025](../../architecture/decisions/ADR-025-application-deletion.md); trạng
thái và evidence ở [CURRENT_STATE](../../CURRENT_STATE.md), runbook nghiệm thu
[application deletion](../../operations/application-deletion-acceptance.md).

## Read in this order

1. [ADR-025](../../architecture/decisions/ADR-025-application-deletion.md)
2. [Specification](specification.md)
3. [Realization](realization.md)
4. [Sequence](sequence.puml) ([PNG](sequence.png))
5. [VOPC](vopc.puml) ([PNG](vopc.png))
6. [UI screens](ui/screens.md), [states](ui/states.md), [API mapping](ui/api-mapping.md)

## Reused design

- Workload removal: [UC-07 VAR-02, RB-03, VSO-01](../UC-07/specification.md)
- Confirmed resource destroy: [UC-07 DEL](../UC-07/specification.md#luồng-riêng-xóa-tài-nguyên)
- Reverse-graph destroy and original targets: [UC-08 V06-01](../UC-08/specification.md)
- Operation claim/fence and cleanup persistence: [database schema](../../architecture/database/schema.md#cleanup-persistence)

## Implementation entry points

- [`backend/internal/application/appdeletion`](../../../backend/internal/application/appdeletion/)
- [`resourcecleanup/environment.go`](../../../backend/internal/application/resourcecleanup/environment.go)
- [`delivery/http/application_deletion.go`](../../../backend/internal/delivery/http/application_deletion.go)
- [`frontend/src/features/applications/deletion`](../../../frontend/src/features/applications/deletion/)
- [`test/e2e/application_deletion_test.go`](../../../backend/test/e2e/application_deletion_test.go)
