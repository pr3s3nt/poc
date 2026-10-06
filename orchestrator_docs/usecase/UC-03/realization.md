---
id: UC-03-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-02
---

# UC-03 — Use Case Realization

## Trách nhiệm

Đăng ký Resource Definition, criteria, driver inputs và provision rules; chứng minh Definition tham chiếu Resource Type/connection hợp lệ và có contract tương thích trước khi planner sử dụng.

## System operation

```go
ResourceDefinitionService.RegisterResourceDefinition(ctx context.Context, cmd RegisterResourceDefinitionCommand) (*ResourceDefinition, error)
```

## Participants

- `ResourceDefinitionController` — boundary nhận Definition document.
- `ResourceDefinitionService` — control điều phối validation/persist.
- `ResourceDefinition`, `MatchingCriterion`, `ResourceReference` — domain model.
- `ResourceDefinition.execution_profile` — optional eligibility guard checked
  before five-field criteria scoring; it has no specificity weight.
- `DefinitionValidator` — validate driver inputs, references, rules và output contract.
- `ResourceTypeRepository`, `ConnectionRepository`, `ResourceDefinitionRepository` — persistence ports.
- `DriverContractInspector` — port kiểm tra Terraform/Kubernetes contract tĩnh.

MVP registration chỉ nhận driver/type pair hiện có. Terraform dùng embedded
module inspector; Kubernetes/existing-cluster dùng output contract tĩnh của
executor. HTTP POST kiểm tra Platform Engineer/Admin; list theo Organization.
UI dùng form có criteria từng dòng và JSON nâng cao cho variables/provision.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-03 | Controller parse; Service tạo Definition và `ValidateStructureAndCriteria`, gồm BR-07. |
| MS-04 | Load Resource Type và connection/Driver registration. |
| MS-05 | `DefinitionValidator.ValidateReferencesAndRules`. |
| MS-06 | Repository/unique constraint bảo vệ Definition ID. |
| MS-07 | `DriverContractInspector.Inspect` đối chiếu outputs với Resource Type. |
| MS-08–MS-09 | Save Definition + criteria atomically và publish qua repository query. |
| VAR-01/VAR-02 | Filter Definition by Application profile, then match criteria; driver type/connection khác nhau, service flow không đổi. |

## Transaction boundary

Contract inspection hoàn tất trước transaction. Definition thiếu criterion bị
từ chối trước persistence; criterion `{}` khai báo tường minh là wildcard điểm
`0`. Definition hợp lệ và toàn bộ criteria được lưu trong một transaction;
source fingerprint được lưu cùng Definition metadata.

Registration dùng insert-only `CreateResourceDefinition` trong transaction,
không dùng seed/upsert `SaveResourceDefinition`. PostgreSQL unique
`(organization_id,key)` là final duplicate guard kể cả concurrent requests.
Lệnh trùng key không được sửa Definition hay xóa/thay criteria của lệnh đã thắng.

## Planned tests

- `TestRegisterDefinition_TerraformAWS`.
- `TestRegisterDefinition_KubernetesInternal`.
- `TestRegisterDefinition_OutputContractMismatch`.
- `TestRegisterDefinition_InvalidResourceReference`.
- `TestRegisterDefinition_RequiresAtLeastOneCriterion`.
- `TestMatching_ExplicitEmptyCriterionIsWildcard`.
- `TestConformanceLoader_SkipsDefinitionWithoutCriteria`.

## Input validation collaboration

Definition registration validates the public ID and strict Driver Inputs shape, variable names/literal types and placeholders before insert, per specification BR-10–BR-14. DriverContractInspector supplies embedded Terraform types; static executor contracts supply Kubernetes/existing-cluster types. Errors carry field paths, not submitted values. Existing criteria, connection, output and insert-only checks remain required.

## Definition-selected workload rendering collaboration

See [ADR-010](../../architecture/decisions/ADR-010-score-k8s-workload-rendering.md)
and [rendering contract](../../architecture/contracts/workload-rendering.md).
Catalog validates the installed bundle; shared planning selects/pins the renderer.
UC-06 validates availability before provisioning, then dispatches the existing
WorkloadRenderer port after output binding. UC-08 still executes resource-only
batches. Kubernetes/Fleet apply/readiness and Environment route ownership stay
in their existing adapters. No database transaction spans a CLI/external call.
