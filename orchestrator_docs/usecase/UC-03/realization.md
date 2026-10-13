---
id: UC-03-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-13
---

# UC-03 — Use Case Realization

## Trách nhiệm và participants

ResourceDefinitionController kiểm session Organization, PE/Admin và payload
giới hạn. ResourceDefinitionService điều phối Type/Driver/criteria/inputs/
references/provision rules validation và insert-only registration.

ModuleSourcePolicy kiểm allowlist của Admin; ModuleBundleResolver tải/cache
revision bất biến và kiểm digest/transitive content ngoài DB transaction.
DriverContractInspector đọc bundle đã kiểm chứng để xác định types/defaults/
required, provider requirements và mappings tương thích Resource Type.
ResourceTypeRepository, ModuleBundleRepository, ResourceDefinitionRepository
và UnitOfWork chịu trách nhiệm persistence; unique key là chốt concurrent duplicate.

## System operations

- `RegisterResourceDefinition(ctx, cmd)` kiểm module/Type/mappings và lưu Definition.
- `ListDefinitionCapabilities(ctx, org)` chỉ đọc verified registry/Type schemas.
- `ListRenderBundles()` chỉ đọc registry bundle bất biến, trả safe ID/version/status.
- `PreviewMatching(ctx, org, query)` đọc snapshot context/catalog và tính matching
  một candidate chưa lưu; không phải full graph Preview hoặc deployment token.

## Trace main flow

| Step | Collaboration |
|---|---|
| MS-01..03 | Controller parse; service kiểm ID, known fields, criteria không rỗng; `{}` là wildcard score 0. |
| MS-04 | Kiểm Type/driver/provider binding; từ chối public existing-cluster/reserved key và AWS account override. |
| MS-05 | Module policy/resolver/inspector kiểm immutable source, inputs/mappings/references/provision rules. |
| MS-06 | Unique org/key và insert-only repository bảo vệ duplicate, không overwrite criteria. |
| MS-07 | Kiểm output contract Type, gồm secret flags không được downgrade. |
| MS-08..09 | Một giao dịch lưu bundle association + Definition/criteria; công bố cho planning. |

Definition profile được lọc trước năm criteria có trọng số. Internal cluster
dùng ADR-013, không bị candidate/author criteria điều khiển. Mọi AWS binding
lấy account/region của Environment; Kubernetes bind selected target hoặc EKS output.
Workload score-k8s bundle được kiểm cho cả hai profiles; UC-06 preflight và render
sau UC-08, không đưa renderer vào resource batches.

## Read-only metadata và matching

Capabilities chỉ project metadata đã inspect; executor-owned/password/credential
không thành controls. Type param ưu tiên Definition fallback; defaults/presets
chỉ áp dụng tường minh. Typed/advanced JSON giữ kiểu và dữ liệu khi đổi mode.

PreviewMatching dùng một scoped ReadSnapshot và chỉ replace same-key candidate
trong memory. DefinitionMatcher giữ profile filter, BestCriterion/score/ties và
winning ConnectionMismatch, không chọn winner khác khi conflict. Thiếu context
trả INCOMPLETE; không có renderer match trả NO_MATCH. Marker là safe catalog/
Environment snapshot hash, không deploy token. Không gọi inspector/provider/
secret store, không materialize system record và không ghi persistence.
Các controls criteria/provision tiếp tục consumer -> provider và fixed-point expansion.

## Transactions và validation

Không giữ transaction qua download/provider calls. Source drift/input/output/
credential/duplicate failure không lưu Definition/criteria nửa chừng. Scope/key
collision không được overwrite winner. Context chỉ dùng env.profile/env.region;
Application aliases đã bỏ không được rewrite. Seed chỉ thêm missing definitions,
không sửa catalog người dùng.

Tests cần có: immutable source/digest/allowlist, mapping/schema/secret rejection,
insert-only race, pure capabilities/matching, class/no-match/tie, profile/target
guards và bundle unavailable. Contract chi tiết: [OC-04/04b/04c/04d](../../architecture/contracts/operation-contracts.md)
và [module contract](../../architecture/contracts/terraform-modules.md).
