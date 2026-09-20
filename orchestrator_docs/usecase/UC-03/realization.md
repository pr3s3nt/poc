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
- `DefinitionValidator` — validate driver inputs, references, rules và output contract.
- `ResourceTypeRepository`, `ConnectionRepository`, `ResourceDefinitionRepository` — persistence ports.
- `DriverContractInspector` — port kiểm tra Terraform/Kubernetes contract tĩnh.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01–MS-03 | Controller parse; Service tạo Definition và validate cấu trúc. |
| MS-04 | Load Resource Type và connection/Driver registration. |
| MS-05 | `DefinitionValidator.ValidateReferencesAndRules`. |
| MS-06 | Repository/unique constraint bảo vệ Definition ID. |
| MS-07 | `DriverContractInspector.Inspect` đối chiếu outputs với Resource Type. |
| MS-08–MS-09 | Save Definition + criteria atomically và publish qua repository query. |
| VAR-01/VAR-02 | Driver type/connection khác nhau, service flow không đổi. |

## Transaction boundary

Contract inspection hoàn tất trước transaction. Definition và toàn bộ criteria được lưu trong một transaction; source fingerprint được lưu cùng Definition metadata.

## Planned tests

- `TestRegisterDefinition_TerraformAWS`.
- `TestRegisterDefinition_KubernetesInternal`.
- `TestRegisterDefinition_OutputContractMismatch`.
- `TestRegisterDefinition_InvalidResourceReference`.
