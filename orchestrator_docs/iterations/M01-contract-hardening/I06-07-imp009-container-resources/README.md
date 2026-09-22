---
id: I06-07
artifact: iteration-plan
status: deferred
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

Chưa thực hiện.
