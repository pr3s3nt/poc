---
id: OPERATION-CONTRACTS
artifact: operation-contracts
status: current
last_reviewed: 2026-10-12
---

# Operation Contracts

Các contract dưới đây dùng tên method cố định cho realization và Go implementation.

## OC-00 `AuthenticationService.SignIn` / `SignOut`

- Use case: UC-00 MS-01–MS-05.
- Preconditions: fixed internal account tồn tại, `ACTIVE`; test accounts chỉ được load ở profile `local`/`test`.
- Creates: opaque Session gắn với User, Organization và role; delivery đặt raw token vào same-origin `HttpOnly` cookie.
- Postconditions: raw token không được persist/log; only token hash được lưu; revoked/expired session không authenticate được.
- Persistence: insert/revoke `sessions` trong một transaction; không mutate UserAccount khi sign-in thành công.

## OC-01 `ApplicationService.CreateApplication`

- Use case: UC-01 MS-01–MS-08.
- Preconditions: authenticated Developer and Organization; valid unique name/subdomain; no default Connection required.
- Input: name/subdomain only; unknown create connectionKey rejected 400.
- Creates: Application identity, staging/production UNCONFIGURED Environments, empty Sets and stable namespaces, atomically.
- No external provision; desired endpoints remain unprovisioned.
- Query: GET /api/v1/application-connections retains safe Organization-scoped READY choices/default marker for Settings, no credentials or UC-04 management rights.

## Historical OC-01b `ApplicationService.SetConnection` (ADR-011)

Superseded by the Editable Environment destinations contracts below.

- Use case: UC-01 ES-01..06, BR-07..14.
- Input: scoped app/env, connectionKey and expectedVersion; session Organization/role only.
- Preconditions: Environment unset; supported READY Connection in Organization; AWS nonempty region.
- Transaction: scoped ownership + unset/version CAS + derive profile/region/runtime, store binding and increment Environment version. No external call.
- Immutable: any repeat including same key 409; no reset/change. Normal Save cannot overwrite target.
- Failure: malformed/missing/blank/null key/version 400; scoped missing app/env 404; unavailable/foreign/nonREADY/unsupported Connection 422 field connectionKey; stale/alreadyset 409, no mutation.
- Result: safe Environment binding/profile/region/status/scope/version. Other Environment unchanged.
- Migration: preserve old bindings as locked with LEGACY_APPLICATION; new AWS ENVIRONMENT VPC/EKS; see ADR-011.

## OC-03 `ResourceTypeService.RegisterResourceType`

- Use case: UC-02.
- Preconditions: schemas hợp lệ; key chưa tồn tại trong Organization.
- Creates: `ResourceType` contract.
- Postconditions: available cho Score validation và Resource Definition registration.
- Persistence: insert `resource_types`; unique constraint là final duplicate guard.
- Registration repository operation is insert-only; concurrent duplicate returns
  conflict without overwriting the winning contract. Seed Save/upsert is separate.

## OC-04 `ResourceDefinitionService.RegisterResourceDefinition`

- Use case: UC-03.
- Preconditions: Resource Type, Driver/connection tồn tại; có ít nhất một
  Matching Criterion; references/rules hợp lệ; output contract tương thích.
- MVP registration chỉ chấp nhận runtime-supported driver/type pairs và embedded
  Terraform modules. VPC/EKS registration requires empty `connectionKey`; any nonempty key returns validation 400 before insert, including a key equal to an Environment target. Registration needs no Environment. Aurora explicit connections must be READY and correct kind in the same Organization; optional Kubernetes connections retain these checks. Không nhận remote source.
- Every public `existing-cluster` registration returns validation 400 before insert, regardless of key; reserved system keys remain protected for every driver. Trusted internal constructor/executor and idempotent UC-08 admission remain available. Existing records are not deleted; ReferenceCluster fixtures use only the reference harness.
- Creates: `ResourceDefinition` và một hoặc nhiều `MatchingCriterion`.
- Postconditions: queryable cho deterministic matching; `{}` tường minh là
  wildcard điểm `0`; optional `execution_profile` filters eligibility before
  scoring the five standard criteria; source fingerprint được ghi nếu có.
- Persistence: insert-only definition + criteria atomically; concurrent duplicate
  conflicts without changing the existing Definition/criteria. Seed Save/upsert
  is separate from public registration.

## OC-05 `ConnectionService.InspectKubeconfig` / `RegisterKubeconfig` / `RegisterAWSDriverAccount`

- Use case: [UC-04](../../usecase/UC-04/specification.md).
- Inspect: role/org gated, bounded document, parse/validate; returns context,
  cluster and endpoint only. No secret/Connection write or cluster request.
- Register upload: name/document/context, revalidate selected context; no external
  file/exec dependencies. Read-only API/RBAC verification before any persistence.
- Creates: immutable scoped credential object then insert-only `READY` Connection.
  Generated key collision resolves without overwrite; no default linkage change.
- Postconditions: no secret bytes in logical state/public response; credential
  accessible internally only for the owning Organization/Connection.
- Failure: no READY on verification/store failure; database failure cleans only
  attempt's credential, with bounded cleanup independent of canceled request.
- Public DTO/errors exclude secret refs/raw parser/provider output.
- Reject the old `{key,clusterId,kubeContext}` body, including mixed upload bodies.
  Kubernetes execution requires explicit scoped KUBECONFIG; missing credentials,
  HOST_CONTEXT or inferred authentication fail closed. AWS operation is future
  design under ADR-009. Seed without credential is VERIFYING, never READY.
- Execution uses [credential resolution](../connection-credentials.md); no secret
  material or temporary paths are persisted in Target/outputs/state/snapshots.

## OC-06 `PreviewService.PreviewDeployment`

- Use case: UC-05.
- Preconditions: Planning snapshot đọc được; Score hợp lệ.
- Returns: `DeploymentPreview` gồm transient Humanitec-shaped Delta, Candidate Set, graph, matches, batches, classification và environment version.
- Postconditions: không persist Deployment; không gọi executor/deployer; runtime state không đổi.
- Invariant: base set + Delta = Candidate Set.
- Standalone HTTP/UI boundary: [UC-05 mapping](../../usecase/UC-05/ui/api-mapping.md).
  Scope is session-derived and repositories use one consistent read-only
  snapshot. Explicit DTO excludes connection credentials, secret values and
  resolved resource inputs. No pending-change deployment token is issued.

## OC-07 `PlanningService.Plan`

- Use cases: UC-05, UC-06, UC-07.
- Preconditions: immutable base snapshot, Resource Types/Definitions/connections và Active Resources đã load.
- Returns: deterministic `DeploymentPlan` với plan hash.
- Delta rule: output có `modules.add/remove/update` và `shared`; module/shared patches có relative root đúng contract, array diff deterministic và `base + delta = candidate`.
- Workload rule: optional `containers.*.resources.requests/limits` được validate và bảo toàn trong Candidate module; không biến thành resource node.
- Public-route rule: `service.publicRoutes` (and legacy `publicPort` for `/`)
  survives the Candidate Set; planning rejects duplicate/invalid paths or
  undeclared Service ports in the complete Environment state.
- Service-reference rule: `${context.service.<workload>.<port>}` must resolve
  in the final Candidate Environment. Single-operation Preview/Deploy validate
  the Candidate; pending multi-workload Preview validates the final batch state
  and may permit intermediate references only while executing that already
  validated batch, as for public routes. No runtime change starts for an
  invalid final Candidate.
- Before-state rule: module và từng shared entry do `before Score` khai báo phải deep-equal current set.
- Candidate rule: một shared ID mới không được ghi đè entry khác nội dung; shared entry bị workload bỏ chỉ rời Candidate khi không còn module khác tham chiếu.
- Context rule (T21/LEG-04): profile/region placeholders resolve only through
  `env.profile`/`env.region` from the selected Environment. `app.profile` and
  `app.region` return unknown context errors; no rewrite or fallback. Application
  identity keys and other context values keep their existing semantics.
- Descriptor rule: `@app`, `@env`, `@connection`, `@infra` được resolve từ planning
  context trước khi parse descriptor; `@` kế thừa class/ID hiện tại giữ nguyên
  semantics.
- Postconditions: graph là DAG; mỗi resource node có một matched Definition hoặc trusted implicit-cluster binding; contracts valid; provider-first batches; workload node không thuộc resource-execution batches.
- Unreferenced classification for an Environment denotes `READY` resources
  absent from its desired graph within that Environment/shared/workload scope.
  Already-unreferenced, failed/provisioning and Application-wide resources are
  not candidates for that Environment's status-marking operation.
- Side effects: none.

## OC-08 `DeploymentService.DeployWorkload`

- Use case: UC-06.
- Preconditions: PRE-01..PRE-05; Environment version khớp base snapshot.
- Creates: Deployment, immutable DeploymentDeltaSnapshot, Candidate Set, DeploymentPlan, deployment-resource progress, Active Resources/Workload Instance as execution proceeds.
- Planning record: `PLANNING` được insert với nullable Snapshot ID; planning
  failure có thể thành `FAILED` không Snapshot. Transaction tạo plan gắn
  Snapshot trước khi chuyển `PROVISIONING`; association đã có là immutable.
- Postconditions: UC-08 complete; workload ready với declared container requests/limits; current-set pointer atomically đổi; AWS Environment runtime `READY`; Deployment `SUCCEEDED`.
- External side effects: Terraform/Kubernetes outside DB transaction.
- Commit rule: Candidate Set never becomes current before readiness.
- Internal-kind route rule: after affected workloads are ready, reconcile the
  Environment-owned Traefik Ingress from the complete Deployment Set. Pending
  multi-workload Deploy defers route reconciliation until the batch finishes.
  Direct mode uses Kubernetes API; Fleet mode owns a `_routes` GitOps bundle,
  waits for Fleet observation and exact Ingress revision, and prunes on removal.
- Retry rule: if a pending batch committed workloads but route reconciliation
  failed, persist `Environment.publicRoutesPending`; Preview exposes a route-only
  retry action. Retrying routes must not call WorkloadDeployer.Apply.
- Fleet GitRepo variant: workload adapter commits only non-secret manifests,
  waits for the commit to be observed and the exact workload revision Ready;
  UC-08 resources remain outside the GitOps repository.

## OC-09 `DeploymentService.UpdateWorkload` / `RemoveWorkload`

- Use case: UC-07.
- Preconditions: before Score deep-equals current module và từng shared entry mà workload khai báo.
- Updates: only target workload contribution and legitimately owned shared contribution through Humanitec-shaped module/shared Delta; conflicting shared ID is rejected before execution.
- Postconditions: other modules preserved; referenced shared resources preserved; stale resource `UNREFERENCED`; no destroy.
- Commit rule: same optimistic final transaction as OC-08.
- `UNREFERENCED` markers commit with current pointer/`SUCCEEDED`, after runtime
  success; failure/rollback leaves them unchanged. Preserve resource identity,
  outputs and executor state. Console dispatch uses
  [UC-07 pending-flow mapping](../../usecase/UC-07/ui/api-mapping.md).

## OC-10 `ResourceProvisioningService.Provision`

- Use case: UC-08.
- Preconditions: persisted immutable plan; resource-only DAG/batches; matched Definitions; connections `READY`.
- Target binding: UC-06 BR-20 retains winning-Definition ConnectionMismatch guards; UC-08 BR-07 rechecks before executor calls. Terraform VPC/EKS always resolve the pinned READY AWS Environment connection, with effective identity in plan/hash and ActiveResource. Empty Definition keys inherit without mismatch; malformed conflicting keys fail without alternate candidate selection. Aurora requires its explicit READY AWS Driver Account in the same Organization and never falls back to the Environment account.
- For each node: resolve prior state and inputs -> validate persisted account identity against the effective connection before reuse -> executor provision/reconcile -> validate outputs -> atomically persist Active Resource + deployment-resource progress. Do not pass prior executor state from another AWS account; unavailable repository reads fail closed.
- Returns: `ProvisionResult` mapping descriptor to outputs and deployment target.
- Postconditions: all desired resource nodes `READY`; output bindings available for workload render.
- Idempotency key: logical identity `(organization, descriptor, scope_type, scope_id)` plus executor state.
- Boundary: container CPU/memory requests/limits are not UC-08 resources; UC-06 Workload Renderer applies them to manifests.

## OC-11 `DeploymentQueryService.ListDeployments` / `GetDeployment`

Failure reasons use UC-09 BR-07 safe summaries. Unknown legacy error text is
replaced in list/detail projection, without mutating history.

- Use case: UC-09.
- Preconditions: authenticated Organization plus explicit Application and
  Environment scope; detail additionally requires a Deployment in that exact
  scope. Optional list status is one valid Deployment lifecycle state.
- Returns: newest-first filtered history, or persisted detail with set snapshot,
  graph, matches, batches, deployment-resource progress, per-Deployment
  workload snapshots and non-secret outputs. Resource resolved inputs are not
  returned.
- Postconditions: no mutation and no runtime adapter call.

## OC-12 `ApplicationConfigurationService.ListKeys` / `PutKey` / `RenameKey` / `DeleteKey`

- Use case: UC-12. Scope is an authorized Application and exactly one of
  `staging` or `production`. Provider binding is per Application, not per key.
- Key name is a valid process-environment identifier and unique across both
  Variable and Secret in one Environment. A Secret's read representation has
  name, type and configured status only; Variable may return its value.
- A value write first creates an immutable Vault KV v2 value at an opaque path;
  the state store then atomically saves a new immutable desired revision and
  advances the Environment's desired pointer/version. Previous revision and
  applied workload pointers are unchanged. If the pointer update conflicts,
  an unreachable Vault value may remain for later cleanup; no runtime changes.
- Rename/delete copy or omit the old opaque reference into a new revision.
  They warn about affected workloads and never rewrite UC-16 bindings.
- Secret bytes never enter state snapshots, Score documents, logs, API reads,
  preview responses, Kubernetes Secrets or Pod annotations.

## OC-16 `WorkloadConfigurationService.List` / `Save` / `Delete` / `Undo`

- Use case: UC-16. Persist versioned Environment-scoped desired Score documents
  separately from the current Deployment Set. Save/delete/undo never deploy.
- UI form and Score import share the same validation. Container bindings may
  only be UC-12 keys, declared resource outputs or same-Environment Service
  outputs; direct literals are rejected on this UI/API path.
- The form reads Resource Type input contracts and writes typed dependency
  values to `resources.<alias>.params`; required inputs must be present.
- `resources.env` is the built-in virtual resource of type `environment`.
  `${resources.env.KEY}` resolves dynamically from the selected UC-12 revision;
  it never provisions a resource. Secret keys must occupy the whole binding
  and can appear only in the Secret section.
- Changes advance the Environment draft version; preview pins this version
  together with the UC-12 desired revision and current Deployment Set/version.

## OC-17 `PreviewService.PreviewDesired` / `DeploymentService.DeployPreview`

- Use cases: UC-05/06/07 with pending UC-12/UC-16 changes. Preview is read-only
  and identifies every affected workload, including those whose selected workload renderer changed and those referencing changed
  values; missing keys/Services block Deploy.
- Deploy requires the exact preview token and rejects any changed draft,
  configuration revision or base Deployment Set/version. It pins a separate
  immutable Vault bundle with only the selected keys for each affected
  workload. On Kubernetes VSO mode, the bundle is synchronized to a
  revision-specific Secret before the workload is applied; Pod env uses
  `secretKeyRef`. Legacy Agent mode still renders a shell-safe file.
- Each successful workload records its own applied configuration revision.
  A failure reports successful/failed workload names and leaves the
  Environment in partial state, retryable with a fresh Preview. No automatic
  rollback is implied. Unaffected workloads and the other Environment are
  unchanged.

## Shared error/consistency rules

- Optimistic conflict uses Environment `version`; no last-write-wins on current-set pointer.
- Domain errors are typed Go errors; delivery maps them to HTTP status and the stable public error contract below.
- Failed external execution may mark Deployment/Resource `FAILED`, but retry/resume/rollback remains outside current happy path.
- Secret values never enter logs, plan hash input exposed to clients, database JSONB or UC-09 view.

## Registration and desired-input validation (2026-10-02)

- OC-03/OC-04 apply the new-public-ID policy in UC-02 BR-05 and UC-03 BR-10;
  OC-03 also rejects virtual Resource Type keys per UC-02 BR-06. Legacy reads,
  fixture catalog loads and planner matching are not migration targets.
- OC-04 enforces UC-03 BR-11–BR-14 before persistence: known nested fields,
  runtime driver variable schemas, placeholder-aware type checks and no raw
  credential inputs. Invalid documents return actionable safe validation.
- OC-16 enforces UC-16 BR-14 on both Save and ValidateImport before mutation.
  Backend owns enforcement even when a caller bypasses form validation.

## UC-16 existing-key selection delivery boundary

UC-16 BR-15–BR-18 changes the Console selection UI only. Checkbox selections
and optional container-name overrides serialize to the existing environment
resource references of OC-16. Validate complete/unique per-container names
before serialization; preserve import/edit mappings and unavailable references.
No new API payload, persistence column or configuration revision is introduced.
Only explicitly selected keys are used. OC-17, Vault bundle scoping and
Preview/Deploy behavior remain unchanged.

## Workload rendering extension (ADR-010)

OC-04 validates the supported workload/bundle pair before registration. OC-06/07
pin safe rendering selection in plan hashes. OC-08 validates runtime availability
before UC-08, dispatches rendering after outputs and preserves readiness/commit
rules. OC-10 excludes workload Definitions from infrastructure execution.
See [rendering contract](workload-rendering.md) for the detailed boundary.

## Editable Environment destinations (supersedes OC-01b set-once)

- `ApplicationService.SetConnection`: existing PUT connection route retains
  `{connectionKey, expectedVersion}`; initial/pre-runtime changes are scoped CAS,
  configured same key/current version no-op, stale version 409 STALE_VERSION,
  active operation 409 ENVIRONMENT_BUSY. Runtime replacement requires explicit
  transition command; no hidden runtime migration inside metadata Save.
- `SecretStoreService.Register` / `List`: PE/Admin POST/GET `/api/v1/secret-stores`;
  registration accepts name, provider, backendAddress, workloadAddress, mount,
  authMount, TLS config and concealed token. Strict bounded input; safe DTO only.
  Developer GET `/api/v1/secret-store-choices` yields scoped READY choices.
- `EnvironmentSettingsService.SetSecretStore`: PUT
  `/api/v1/applications/{app}/environments/{env}/secret-store` with
  `{secretStoreKey, expectedVersion, expectedConfigVersion}`; operation claim,
  immutable copy/verify, CAS selection+new desired revision. No old ref rewrite.
- `EnvironmentTransitionService.Preview` / `Execute`: Environment-scoped
  `/connection-transition/preview` then POST `/connection-transitions` with
  server-issued preview token, destination key, mode `DEPLOY_NEW` or
  `MIGRATE_POSTGRES`, resource mapping and `acknowledgeDowntime` for migration.
  Token pins all source/destination identities/capability/versions; admission is
  atomic. Preview includes pending draft/config impact and every resulting desired
  workload; successful cutover commits the desired Set and consumes only pinned
  drafts. Failure preserves pending edits and the authoritative source Set.
  GET transition detail/progress is scoped and safe; explicit recovery
  and source-generation cleanup endpoints reject current/referenced targets.
  Recovery requires explicit confirmation that prior execution has stopped; stale
  heartbeat alone cannot authorize takeover. Release and internal writes check
  current owner/fencing token, including transition status/compensation writes.
- Pending/standalone execution admission must use persisted operation ownership;
  rechecking token followed by an unlocked executor call is insufficient.
- Selected-store resolution returns typed opaque ref `{storeKey, valueRef}` or an
  equivalent versioned encoding; no raw values in refs. Variable revisions carry
  ordinary values in metadata, never Secret values. Legacy raw Vault refs resolve
  only through explicit backfilled legacy store identity.

Transition states, failures and retained recovery artifacts follow
[ADR-012](../decisions/ADR-012-environment-stores-and-transitions.md).

## Implicit cluster execution binding (ADR-013)

OC-07 binds the internal profile cluster node directly to the pinned Environment
Connection through a trusted system Definition, exempt from catalog matching.
OC-06/OC-07 Preview/planning never persist it. OC-10 admits the reserved system
record idempotently before FK-dependent progress/resource writes, validates scope,
READY/kind/target and uses existing-cluster; catalog contents cannot override it.
Other nodes still require exactly one matching Definition. Cloud VPC/EKS/Aurora
contracts and external Driver Accounts remain unchanged. See
[ADR-013](../decisions/ADR-013-implicit-existing-cluster.md).

## OC-04b `ResourceDefinitionService.ListRenderBundles`

- Use case: UC-03 BR-18/19; HTTP GET `/api/v1/render-bundles`.
- Preconditions: authenticated Organization session, Platform Engineer/Admin.
- Returns: `renderBundles` array with only ID/version/status `AVAILABLE`, sorted
  by ID; empty array when no valid installed entries. Same immutable registry
  and availability predicate as OC-04, shared process configuration across orgs.
- Read only: no repository, inspector, binary, provider or credential call;
  no registration, planning, render or persistence. No source/path/digest exposure.
- Console selects one only for an unselected draft; multiple require explicit
  selection. Loading/error/empty block submit; retry preserves edits and late
  replies cannot overwrite a user choice. POST revalidates availability.

## OC-04c `ResourceDefinitionService.PreviewMatching`

- Use case: UC-03 BR-20–22; không thay UC-05 full graph Preview.
- HTTP: POST `/api/v1/resource-definitions/matching-preview`; PE/Admin và org từ
  session; 401/403 authorization, scoped missing app/env 404, invalid strict
  document 400, oversize 413, unavailable/inconsistent target 422, generic 500.
- Input: `{definition: <unsaved Definition>, context: {applicationId,
  environmentId, resourceId, class}}`. Không nhận org/profile/Connection context.
  Definition shape có bound body như registration; chỉ structural matching
  validation, không gọi registration hoặc driver contract inspection.
- Reads: một `ReadSnapshot` scoped app/env/target/types/definitions; candidate
  replaces only same-key entry in memory. Reserved system key/public
  existing-cluster candidate bị từ chối; persisted system binding không bị đổi.
- Returns 200: `status` trong `INCOMPLETE`, `MATCHED`, `NO_MATCH`, `AMBIGUOUS`,
  `CONNECTION_CONFLICT`, `SYSTEM_BINDING`, `NATIVE_RENDERER`; `missingFields`,
  `candidates` (key, eligible, matched, bestCriterion/score nếu match), `winner`
  hoặc `ties`, `connectionConflict`, `snapshot` (marker và environmentVersion).
  Candidate eligibility gồm type/profile; score chỉ tính năm criteria. Incomplete
  không có winner/ties. Ties là sorted keys có winning score, không gồm tie thấp.
  Winner conflict giữ winner và báo conflict, không retarget/fallback.
  SYSTEM_BINDING chỉ cho đúng implicit descriptor/class theo ADR-013;
  workload chỉ xét score-k8s như renderer selection của planner.
- Marker là content hash catalog và Environment matching/target identity/version
  trước candidate replacement; không hash credentials, không phải mutation token.
- Creates/updates: none. No system materialization, store write, secret resolution,
  inspector, render, provision hoặc external call. Các read SQL được phép.
- Client: kết quả gắn candidate/context request; đổi input hoặc session/scope
  xóa/invalidate result, late reply không cập nhật; lỗi transport tách khỏi trạng
  thái matching, retry chỉ gửi phép tính read-only.

## Public HTTP error contract — T20 / VI-03

Mọi lỗi HTTP của các luồng Console UC-00/01/02/03/04/05/07/09/12/16 có
`code` ổn định và `error` là câu cố định an toàn, không phải `err.Error()`.
`field` tùy chọn chỉ từ tên field/schema path được server kiểm soát; không
sao chép unknown JSON key, resource alias, tên biến hoặc giá trị do caller gửi.
Không nhận diện lỗi bằng prefix/substring câu tiếng Anh. Dùng sentinel/typed
error và stage/reason của planner; lỗi chưa phân loại fail closed thành lỗi nội bộ.
HTTP status, org/app/env scoping, CAS, fencing và transaction semantics giữ nguyên.

| Code | HTTP | Ý nghĩa/hành động Console |
|---|---|---|
| `INVALID_REQUEST` | 400 | Body/field không hợp lệ; giữ draft để sửa. |
| `INVALID_ID` | 400 | ID sai quy tắc; không normalize. |
| `INVALID_VARIABLE` | 400 | Tên/giá trị/tham chiếu biến không hợp lệ; không trả giá trị. |
| `INVALID_SCORE` | 400 | Score/import/resource params không hợp lệ. |
| `PLANNING_REJECTED` | 400 | Không thể lập kế hoạch; không provision/fallback. |
| `DUPLICATE` | 409 | Trùng định danh trong scope; không overwrite. |
| `UNAUTHORIZED` | 401 | Thiếu/hết phiên; giữ session-expiry flow. |
| `INVALID_CREDENTIALS` | 401 | Đăng nhập sai, không tiết lộ tài khoản hay field nào sai. |
| `FORBIDDEN` | 403 | Không đủ quyền; không lộ tenant khác. |
| `NOT_FOUND` | 404 | Không tìm thấy hoặc không truy cập được trong scope. |
| `REQUEST_TOO_LARGE` | 413 | Giảm kích thước body/document. |
| `STALE_VERSION` / `STALE_PREVIEW` | 409 | Reload/review hoặc Xem trước lại; không tự gửi mutation. |
| `ENVIRONMENT_BUSY` | 409 | Đợi/xem trạng thái operation; giữ edits. |
| `ENVIRONMENT_UNCONFIGURED` | 422 | Chọn kết nối trong Cài đặt môi trường, field `connectionKey`. |
| `CONNECTION_UNAVAILABLE` | 422 (UC-05 standalone READY rejection giữ 400) | Kết nối không hợp lệ/không READY trong scope. |
| `VERIFICATION_FAILED` | 422 | Xác minh kết nối/kho thất bại; không trả provider diagnostics. |
| `VERIFICATION_CONTEXT_MISSING` / `VERIFICATION_UNREACHABLE` / `VERIFICATION_PERMISSION_DENIED` / `VERIFICATION_CREDENTIAL_REJECTED` | 422 | Typed context/network/permission/auth rejection; hướng dẫn cố định, không raw cause. |
| `KUBECONFIG_INVALID` / `KUBECONFIG_NO_VALID_CONTEXT` / `KUBECONFIG_CONTEXT_REQUIRED` / `KUBECONFIG_CONTEXT_NOT_FOUND` | 400 | Document/context không hợp lệ hoặc chưa chọn; inspect/review lại. |
| `KUBECONFIG_UNSUPPORTED_AUTH` / `KUBECONFIG_UNSUPPORTED_CLUSTER` / `KUBECONFIG_INVALID_SERVER` / `KUBECONFIG_INVALID_TLS` | 400 | Phương thức/cluster setting/server/TLS không hợp lệ; không chạy plugins/files ngoài. |
| `SECRET_STORE_REQUIRED` / `SECRET_STORE_UNAVAILABLE` | 422 | Chọn/kiểm tra kho, field `secretStoreKey`. |
| `SECRET_COPY_FAILED` | 502 | Copy không thành công; selection/revision cũ giữ nguyên. |
| `LEGACY_VALUE_UNREADABLE` | 422 | Không đọc được giá trị cũ; code tạm đến cleanup T24. |
| `TRANSITION_REQUIRED` | 409 | Có runtime; dùng flow chuyển kết nối tường minh. |
| `TRANSITION_UNSUPPORTED` | 422 | Chế độ chuyển không được hỗ trợ. |
| `TRANSITION_SAME_DESTINATION` / `DESTINATION_UNAVAILABLE` | 422 | Chọn đích khác/hợp lệ; không suy fallback. |
| `NO_RUNTIME` | 409 | Không có runtime để chuyển. |
| `DOWNTIME_ACK_REQUIRED` / `RECOVERY_CONFIRMATION_REQUIRED` | 422 | Cần xác nhận theo flow hiện hành. |
| `CLEANUP_REFUSED` / `NOT_RECOVERABLE` / `RECOVERY_INCOMPLETE` | 409 | Cleanup/recovery không được phép hoặc chưa hoàn tất; giữ hướng dẫn can thiệp. |
| `OPERATION_LOST` | 409 | Operation đã gián đoạn/mất ownership; không tự takeover. |
| `ROUTE_RECONCILE_FAILED` | 502 | Workload đã được áp dụng; Preview lại để retry route, không tự restart. |
| `SERVICE_UNAVAILABLE` | 503 | Provider/credential store không sẵn sàng; không hiển thị cause. |
| `INTERNAL_ERROR` | 500 | Lỗi không phân loại; fallback Việt an toàn. |

`params` tùy chọn là object metadata an toàn; T20 cho phép `stage`/`reason`
từ enum planner do server định nghĩa, và `resourceType` từ Type đã đăng ký trong
scoped catalog để giữ hướng dẫn UC-16 BR-14. `resourceType` phải qua giới hạn ID
kỹ thuật (ASCII chữ thường/số/gạch ngang, bắt đầu bằng chữ/số, tối đa 63 ký tự); không phản chiếu
unknown Type do caller gửi. Unknown Type dùng field cố định `resources` và
hướng dẫn kiểm catalog. Không gồm param values hoặc unknown param keys. Bỏ unknown keys/values; không trả submitted
values, credential refs, kubeconfig, token, provider output hoặc raw decode text.
Có thể thêm code cụ thể cho typed validation/kubeconfig nếu cùng safety/status
contract và có mapper/test; không thêm behavior mới. Không thêm correlation ID
nếu chưa có cơ chế server phát sinh/kiểm soát. Không log raw cause chứa secrets.

Client đọc code/field/params, chọn câu Việt hard-code theo glossary. Unknown,
missing hoặc malformed code dùng fallback Việt theo thao tác/status an toàn;
không đọc `error` để dịch hay phân loại. Field/params không được nội suy trực tiếp
vào UI nếu chưa qua allowlist. Retry read và mutation vẫn tường minh, không tự
replay POST/PUT/DELETE. Selected renderer failure giữ nguyên lỗi, không chuyển
sang native. Planner classification/conformance và historical safe-summary
projection không thay đổi bởi public HTTP envelope.
