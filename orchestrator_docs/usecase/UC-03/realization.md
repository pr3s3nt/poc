---
id: UC-03-REALIZATION
artifact: use-case-realization
status: current
last_reviewed: 2026-10-21
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


## V03 collaboration replacement

[ADR-016](../../architecture/decisions/ADR-016-definition-selected-workload-targets.md) replaces internal profile enrichment/system binding in the collaboration above. Catalog validates public existing-cluster/Internal account and workload typed namespace/cluster refs plus provision rules. Planning matches workload first and expands all nodes to fixed point; same normal matcher selects cluster Definition. Target resolver builds/pins per-consumer placement, rejects mismatch/ambiguous/cycle/multi-cluster V03. Shared snapshot/admission compare pins/versions; UC-08 resolves cluster then owned namespace/resources. UC-06 renders/applies/waits on selected workload target; no global target or builtin persistence. Applied target restored for Update/Remove/restart; separate cleanup preserves preexisting cluster. AWS legacy collaboration remains until V05–V06.


## V04–V06 collaboration

[ADR-017](../../architecture/decisions/ADR-017-definition-dependency-execution.md)
replaces the remaining profile-driven collaboration above. Select workload
Definition, expand all authored references/provision/provider bindings to fixed
point, validate full DAG and pin each node's Definition/account/region/placement.
Planning keeps unknown typed outputs and safe provenance without provider calls.
Provisioning dispatches matched driver with private invocation credentials,
validates completed outputs/readiness and persists original state/target pins.
Namespace/workload resolve their explicit cluster; EKS target auth is reconstructed
per invocation after API/network/node readiness. OutputBindingResolver renders
only after dependencies pass. Failure blocks consumers and preserves progress.
Cleanup walks reverse actual graph using original state/module/identity and
ownership/UID guards, protecting shared/preexisting objects. See cloud sequence
and module/state contracts for target and secret boundaries.


## Git inspection collaboration (issue #7)

`InspectTerraformModule(ctx, org, request)` kiểm scoped Type rồi gọi module
inspector resolve/fetch/full-commit/content inspection/cache/persist bundle.
Controller chỉ cho PE/Admin, strict bounded body, deadline; response dùng
`terraformCapability` giống metadata GET. Không gọi Definition mutation, provider
hay executor. Registration sau đó tái sử dụng verified pin và CAS pipeline.

## Resource Definition v3 Console collaboration (issue #8)

Theo UI3-01..04, ResourceDefinitionsPage sở hữu draft/pin/CAS xuyên hai tab;
CriteriaEditor dùng rule projection cho Definition, giữ projection của Mẫu
dựng ứng dụng. Các editor mapping/variables/provision tái sử dụng converters
hiện hành. Native disclosures chỉ ẩn nội dung, không unmount matching: scoped
snapshot signature và session/late-response guards tiếp tục hoạt động khi ẩn.
Copy tạo giá trị độc lập có confirmation; local overlap không xếp hạng winner.
Full draft review serialize cùng payload Save và không gọi API. Không thay
participants/system operations, transaction hay sequence backend hiện hành.

V3 visual correction: catalog and detail share the same mounted page state.
Catalog Create/Edit and editor breadcrumb change presentation only; save/CAS,
metadata loaders, MatchingPreviewPanel and GitModuleSource lifetimes stay owned
by the page. Numbered cards, overview and fixed actions reuse that state.
Mapping and variable provenance disclosures retain existing converters/guards.
