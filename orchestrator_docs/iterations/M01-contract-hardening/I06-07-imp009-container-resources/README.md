---
id: I06-07
artifact: iteration-plan
status: historical
last_reviewed: 2026-09-22
related: IMP-009, UC-05, UC-06
---

# I06-07 — Preserve Score container resources

## Objective

Đóng IMP-009 bằng cách bảo toàn optional CPU/memory requests/limits từ Score qua
Candidate Deployment Set và resolved workload tới Kubernetes manifests.

## Canonical inputs

- UC-05 BR-07 và UC-06 BR-11.
- Domain `ContainerResourceRequirements`/`ComputeResources`.
- Compatibility matrix row Container resources.

## In scope

- Parse/validate `containers.*.resources.requests/limits` với `cpu`/`memory`.
- Preserve typed values qua conversion, candidate, binding và renderer.
- Renderer dùng declared values; chỉ dùng documented defaults khi Score không
  khai báo.
- Unit/integration tests và kind happy-path regression.

## Out of scope

- Resource Graph node mới cho CPU/memory.
- Autoscaling, quota/policy, cost estimation hoặc full Score specification.
- AWS verification; cloud executor contract không đổi.

## Exit criteria

- Valid requests/limits round-trip nguyên vẹn; invalid/unknown fields bị reject.
- Kubernetes manifests phản ánh đúng declared values và documented defaults.
- Go test/build/vet, 33 fixtures và kind verification pass; cleanup được xác
  nhận.
- IMP-009 được xóa và implementation/status/evidence docs được cập nhật.

## First next action

Thêm parser/conversion tests và renderer tests cho declared, omitted và invalid
container resources trước khi đổi model.

## Outcome

Hoàn thành 2026-09-22; mọi exit criteria pass. IMP-009 đã đóng.

- `backend/internal/domain/environment/document.go` thêm typed
  `ComputeResources` và `ContainerResourceRequirements` dưới
  `Container.Resources`; strict decoder vẫn reject unknown fields.
- `backend/internal/planning/score/resources.go` reject `null`, number, empty
  string, branch/key ngoài `requests`/`limits` và `cpu`/`memory`; không kiểm
  Kubernetes quantity semantics (UC-05 BR-07).
- Giá trị khai báo đi nguyên văn qua Score fragment, Candidate Set, Delta
  Snapshot, persisted set và Kubernetes renderer; không có CPU/memory Resource
  Graph node.
- `containerResources` trong Kubernetes renderer thay requests hard-code:
  request field thiếu lấy limit cùng field, thiếu cả hai thì `10m`/`32Mi`;
  limits chỉ gồm field khai báo, module input không bị sửa (UC-06 BR-11). Policy
  được sửa sau review để limit thấp hơn default (ví dụ `cpu: 5m`) không sinh
  manifest không hợp lệ. Request khai báo vượt limit được giữ nguyên; Kubernetes
  API reject khi apply. Hai nhánh này chỉ có unit test.
- Seeded acceptance Scores bao phủ full (backend), partial (worker) và omitted
  (frontend). Kind run `kind-20260922093228-12536` (trước bản sửa review) và
  rerun `kind-20260922114440-17489` trên code cuối xác nhận live resources của
  ba seeded case và cleanup.
- 33/33 fixtures, `go test/build/vet ./...` và documentation checks pass.
- Evidence: [2026-09-22 IMP-009 container resources](../../../verification/2026-09-22-imp009-container-resources.md),
  [2026-09-22 IMP-009 kind rerun](../../../verification/2026-09-22-imp009-kind-rerun.md).

M01 đã hoàn thành. Iteration kế tiếp:
[I06-04](../../M02-usecase-completion/I06-04-uc09-observability/README.md).
