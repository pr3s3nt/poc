---
id: UC-03-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-17
---

# UC-03 — Use Case Realization

## Trách nhiệm và participants

ResourceDefinitionController kiểm session Organization, PE/Admin và payload
giới hạn. ResourceDefinitionService điều phối Type/Driver/criteria/inputs/
references/provision rules validation, create và version-CAS update.

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
| MS-04 | Kiểm Type/driver/provider binding và scoped READY Driver Account; từ chối public existing-cluster/reserved key. |
| MS-05 | Module policy/resolver/inspector kiểm immutable source, inputs/mappings/references/provision rules. |
| MS-06 | Unique org/key cho create; update CAS version giữ immutable ID/type và không ghi đè concurrent winner. |
| MS-07 | Kiểm output contract Type, gồm secret flags không được downgrade. |
| MS-08..09 | Một giao dịch lưu/CAS bundle association + Definition/criteria; publish version mới cho planning. |

DefinitionMatcher scores five fields without profile guard. Internal system
cluster remains ADR-013 until V03. AWS binds Definition Driver Account + inputs
region; Kubernetes inherits dependency target or explicitly retained legacy
Environment target. Workload score-k8s bundle stays rendering-only until V03.

## Read-only metadata và matching

Capabilities chỉ project metadata đã inspect; executor-owned/password/credential
không thành controls. Type param ưu tiên Definition fallback; defaults/presets
chỉ áp dụng tường minh. Typed/advanced JSON giữ kiểu và dữ liệu khi đổi mode.

PreviewMatching dùng một scoped ReadSnapshot và chỉ replace same-key candidate
trong memory. DefinitionMatcher dùng BestCriterion/score/ties không profile filter
rồi kiểm winning account/target, không chọn winner khác khi conflict. Thiếu context
trả INCOMPLETE; không có renderer match trả NO_MATCH. Marker là safe catalog/
Environment/account snapshot hash, không deploy token. Không gọi inspector/provider/
secret store, không materialize system record và không ghi persistence.
Các controls criteria/provision tiếp tục consumer -> provider và fixed-point expansion.

## Transactions và validation

Không giữ transaction qua download/provider calls. Source drift/input/output/
credential/duplicate failure không lưu Definition/criteria nửa chừng. Scope/key
collision không được overwrite winner. AWS identity/region theo Definition, không lấy Environment fallback;
Application aliases đã bỏ không được rewrite. Seed chỉ thêm missing definitions,
không sửa catalog người dùng.

Tests cần có: immutable source/digest/allowlist, mapping/schema/secret rejection,
create/edit CAS race/rollback, pure capabilities/matching, class/no-match/tie, account/target
guards và bundle unavailable. Contract chi tiết: [OC-04/04b/04c/04d](../../architecture/contracts/operation-contracts.md)
và [module contract](../../architecture/contracts/terraform-modules.md).

## V02 collaboration

UC-03 V02-01..09 owns behavior. Controller parses strict public DTO with Name,
Driver Account and version, rejects legacy authoring fields. Service shares one
validation pipeline between Create and Update; scoped account lookup validates
READY/type before persistence and before provision. Module inspection occurs
outside transaction. Create inserts version1; Update compares expected version
and immutable key/type then atomically replaces Definition/criteria/bundle link.
Repositories implement identical CAS/rollback semantics for memory/JSON/SQL.

DefinitionMatcher and PreviewMatching use shared weighted ranking without profile
or account filtering, then validate winning account/target without fallback.
Snapshot includes safe account metadata/version. Runtime binds per-node AWS
identity/region from Definition, pins it with module/state/plan and refuses stale
or incompatible identity before execution. Kubernetes resource references still
create consumer -> cluster dependencies; legacy Environment target is explicit
until V03. Generic Workload is not implemented here. Persisted execution and
cleanup never reread mutable Definition to replace original pins.
