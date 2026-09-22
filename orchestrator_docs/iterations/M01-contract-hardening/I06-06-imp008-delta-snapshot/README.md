---
id: I06-06
artifact: iteration-plan
status: deferred
last_reviewed: 2026-09-22
related: IMP-008, UC-05, UC-06, UC-07
---

# I06-06 — Implement Deployment Delta Snapshot

## Objective

Đóng IMP-008 bằng typed immutable `DeploymentDeltaSnapshot` có Humanitec-shaped
document, thay whole-document flat JSON Patch trong execution/persistence path.

## Canonical inputs

- [Domain objects](../../../architecture/domain/domain-objects.md), Deployment
  Delta Snapshot contract.
- [Database schema](../../../architecture/database/schema.md),
  `deployment_delta_snapshots`.
- UC-05 BR-05/06, UC-06 BR-10 và UC-07 BR-07.
- D05 là compatibility scope bị loại khỏi iteration này.

## In scope

- Typed Snapshot/ModuleDelta/JSONPatchOperation model và deterministic builder.
- `modules.add/remove/update`, `shared`, relative patches, array index/`/-` và
  no-op `{}`.
- Persist Snapshot riêng, gắn một-một với Deployment và expose qua read model
  khi UC-09 cần quan sát plan.
- Remove flat Delta use from product plan path; giữ invariant
  `base + delta = candidate`.
- Product tests ngoài 33-fixture harness và regression toàn backend.

## Out of scope

- Standalone/mutable Humanitec Delta, create/update/archive API, deploy bằng
  `delta_id`, async/full-set/incremental mode.
- PostgreSQL adapter hoặc migration dữ liệu production.
- Container resources IMP-009.

## Exit criteria

- Add/update/remove/shared/no-op và deterministic array diff tests pass.
- Deployment persists/reloads typed Snapshot; Candidate invariant pass.
- Không còn product field `Plan.Delta []jsonpatch.Op` cho whole document.
- 33 fixtures và Go test/build/vet pass.
- IMP-008 được xóa; current state/code map/compatibility/traceability/evidence
  được cập nhật.

## First next action

Viết characterization tests cho Delta hiện tại và acceptance tests cho canonical
shape trước khi đổi domain model.

## Outcome

Chưa thực hiện.
