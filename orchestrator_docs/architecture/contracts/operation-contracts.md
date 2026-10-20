---
id: OPERATION-CONTRACTS
artifact: operation-contracts
status: current
last_reviewed: 2026-10-18
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
- Application Save/Get/List store only ID/key, Organization, name, subdomain and version; no profile/connection/region/runtime/provider fields or binding backfill from Application (T26). Environment owns execution/store selections; historical target snapshots and operation generation/fencing are preserved.
- No external provision; desired endpoints remain unprovisioned.
- Query: GET /api/v1/application-connections retains safe Organization-scoped READY choices/default marker for Settings, no credentials or UC-04 management rights.

## OC-01b `ApplicationService.SetConnection`

Environment selection dùng `{connectionKey,expectedVersion}` và scoped CAS
trước runtime, không Organization default. Configured same key/current version
là no-op; stale 409, active operation BUSY. Sau runtime cần transition riêng
theo [ADR-012](../decisions/ADR-012-environment-stores-and-transitions.md);
metadata Save không tự chuyển dữ liệu hoặc đổi target của resource cũ.
Không còn set-once/legacy backfill: lịch sử contract ở `a897d6b`/`3367f7e`.

## OC-03 `ResourceTypeService.RegisterResourceType`

- Use case: UC-02.
- Preconditions: schemas hợp lệ; key chưa tồn tại trong Organization.
- Creates: `ResourceType` contract.
- Postconditions: available cho Score validation và Resource Definition registration.
- Persistence: insert `resource_types`; unique constraint là final duplicate guard.
- Registration repository operation is insert-only; concurrent duplicate returns
  conflict without overwriting the winning contract. Seed Save/upsert is separate.

## OC-04 `ResourceDefinitionService.RegisterResourceDefinition`

- Role/org: session PE/Admin, scoped Type/registry và insert-only org/key.
- Kiểm criteria không rỗng (`{}` wildcard 0), profile filter trước weighted
  criteria, immutable source allowlist/revision/digest, input/output mappings,
  references/provision rules và symbolic provider contract theo UC-03 BR-08..16.
- Module download/inspection ngoài transaction; planner/metadata reads chỉ đọc
  verified registry. Không raw credential/commands/branch/latest. AWS không
  explicit Connection/account/region override, kể cả Aurora; Kubernetes chỉ
  Environment/EKS target. Registration không cần một Environment cụ thể.
- Public existing-cluster/reserved keys bị từ chối trước insert; chỉ trusted
  system admission ADR-013 dùng nội bộ. Không biến fixture harness thành public API.
- Một giao dịch insert Definition + criteria/bundle association. Duplicate race
  không overwrite winner. Workload chỉ score-k8s cho cả hai profiles; unavailable
  bundle hoặc contract mismatch không lưu. Không tự provision khi đăng ký.

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
  HOST_CONTEXT or inferred authentication fail closed. AWS onboarding qua các
  ô được che trên Console theo ADR-009; verify account/principal/allowed regions/
  provisioning capabilities, lưu Vault ref và metadata READY. STS chưa đủ;
  không nhận backend config hoặc host credential. Seed thiếu credential không READY.
- Execution uses [credential resolution](../connection-credentials.md); no secret
  material or temporary paths are persisted in Target/outputs/state/snapshots.

## OC-06 `PreviewService.PreviewDeployment`

- Use case: UC-05.
- Preconditions: Planning snapshot đọc được; Score hợp lệ.
- Returns: `DeploymentPreview` gồm Delta, Candidate Set, graph/matches/batches/
  classification, safe Type/class/module/revision/target/ownership và versions;
  không state/credential/secret bytes.
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
- Public-route rule: `service.publicRoutes` survives the Candidate Set;
  `service.publicPort` is rejected at Score/public projection boundaries, including
  empty/null values and when publicRoutes is also present. Public Score/import
  errors use `INVALID_SCORE`; rejection never writes desired/runtime state. No
  automatic conversion of old drafts or route fallback is permitted; planning
  rejects duplicate/invalid paths or
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
- Pin required score-k8s Definition/bundle cho mỗi Workload, verified module/
  provider/binary digests, stable state/target/generation/resource versions.
  No-match/tie hoặc known input/mapping sai chặn trước execution.
- Side effects: none; không fetch module/CLI/provider/secret resolution.

## OC-08 `DeploymentService.DeployWorkload`

- Use case: UC-06.
- Preconditions: PRE-01..PRE-05; Environment version khớp base snapshot.
- Creates: Deployment, immutable DeploymentDeltaSnapshot, Candidate Set, DeploymentPlan, deployment-resource progress, Active Resources/Workload Instance as execution proceeds.
- Planning record: `PLANNING` được insert với nullable Snapshot ID; planning
  failure có thể thành `FAILED` không Snapshot. Transaction tạo plan gắn
  Snapshot trước khi chuyển `PROVISIONING`; association đã có là immutable.
- Postconditions: UC-08 complete; workload ready với declared container requests/limits; current-set pointer atomically đổi; AWS Environment runtime `READY`; Deployment `SUCCEEDED`.
- Trước UC-08 kiểm known inputs/module/bundle integrity, nhận Environment
  operation bằng atomic snapshot CAS/owner-fence. Terraform state locks bảo vệ
  từng instance; write-ahead intent có trước provider side effects.
- External side effects: Terraform/provider/delivery ngoài DB transaction.
- Required score-k8s sinh manifests sau outputs/Secret refs trên cả profiles,
  không renderer fallback. Release operation chỉ khi processes đã stopped;
  interrupted outcome giữ state/progress/hold theo state contract.
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

- Preconditions: immutable Plan, resource-only DAG/batches và admitted
  Environment owner/fence, Connections READY, verified module pins.
- Terraform CREATE/UPDATE cho namespace/Redis/PostgreSQL Kubernetes và
  VPC/EKS/ElastiCache/Aurora AWS; ExistingClusterAdapter chỉ resolve ADR-013.
- VPC sở hữu private network; các Aurora/ElastiCache instances nhận subnet IDs
  qua outputs/Resource References của cùng VPC node. Không tạo lại subnet/CIDR
  cho từng consumer và không chuyển quyền xóa shared network cho consumer.
- AWS nodes inherit cùng Environment account/region; Kubernetes bind Connection/
  completed EKS target. Reuse kiểm state owner/org/generation/target/account
  trước execution; lookup/mismatch lỗi không fallback hoặc đổi winner.
- Mỗi node ghi instance/action/module/physical intent/ownership trước call,
  resolve typed inputs, khóa state, init/validate/plan, reject UPDATE delete/
  replace, apply dưới deadline. State backend acknowledge rồi validate
  outputs/readiness, store secret refs và commit result/progress nguyên tử.
- Retry transient tối đa một lần sau state/actual-result checks; input/auth/
  permission/unknown outcome dừng và giữ metadata/state/hold. Full resume/import
  deferred D12/D13; state thiếu không tự biến thành CREATE.
- Returns: safe outputs/typed Secret refs và scoped target. Identity/stable
  names không chứa run ID hoặc Definition version. CPU/memory thuộc renderer,
  Workload Definition không thuộc resource batches. Protocol OC-18/state contract.

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
- Đọc safe stage/attempt/module/target/cleanup progress; poll không gọi provider.
  ResourceQueryService.GetResource đọc resource detail gồm UNREFERENCED sau
  gỡ Workload; ResourceCleanupQueryService.GetOperation đọc confirmed scope/
  verification/remaining IDs. Role đọc Environment không cấp quyền destroy.
- Postconditions: no mutation and no runtime adapter call.

## OC-12 `ApplicationConfigurationService.ListKeys` / `PutKey` / `RenameKey` / `DeleteKey`

- Use case: UC-12. Scope is an authorized Application and exactly one of
  `staging` or `production`. Secret store selection is per Environment (ADR-012).
- Key name is a valid process-environment identifier and unique across both
  Variable and Secret in one Environment. A Secret's read representation has
  name, type and configured status only; Variable may return its value.
- A Secret write first creates an immutable Vault KV v2 value at an opaque path;
  the state store then atomically saves a new immutable desired revision and
  advances the Environment's desired pointer/version. Previous revision and
  applied workload pointers are unchanged. If the pointer update conflicts,
  an unreachable Vault value may remain for later cleanup; no runtime changes.
- Rename/delete copy or omit the old opaque reference into a new revision.
  They warn about affected workloads. Delete and rename-only keep UC-16 bindings; explicit rename opt-in follows the atomic T30 extension below.
- Secret bytes never enter state snapshots, Score documents, logs, API reads,
  preview responses hoặc Pod spec/annotations. VSO chỉ đồng bộ sang scoped
  Kubernetes Secret theo ADR-008; Pod dùng secretKeyRef, không literal secret.

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
- Lỗi external ghi FAILED/safe stage; retry transient có giới hạn theo state
  contract. Automatic resume/rollback không thuộc phạm vi; interruption giữ hold.
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

## Environment destinations và transitions

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
  ordinary values in metadata, never Secret values. Store identity must be explicit;
  runtime resolves scoped private credentials only. No legacy flags/global token,
  store backfill or Compose conversion (T23). Variables have only metadata Value,
  no StoreKey/ValueRef; CRUD never invokes a store. Old store-backed Variables
  are invalid, with no materialization/backfill (T24). Store switch copies only
  Secrets and preserves Variables and old applied owning-store refs. Public
  configuration DTOs have no Legacy field.

Transition states, failures and retained recovery artifacts follow
[ADR-012](../decisions/ADR-012-environment-stores-and-transitions.md).

## Implicit cluster execution binding (ADR-013)

OC-07 binds the internal profile cluster node directly to the pinned Environment
Connection through a trusted system Definition, exempt from catalog matching.
OC-06/OC-07 Preview/planning never persist it. OC-10 admits the reserved system
record idempotently before FK-dependent progress/resource writes, validates scope,
READY/kind/target and uses existing-cluster; catalog contents cannot override it.
Other nodes vẫn cần một matching Definition; mọi AWS node dùng Environment
account/region theo UC-08 BR-07. Xem
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
  `CONNECTION_CONFLICT`, `SYSTEM_BINDING`; `missingFields`,
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
| `COMMIT_UNCERTAIN` | 503 | Chưa rõ commit đã thành công; giữ credential có thể đã commit, GET và xem kết quả trước khi thao tác tiếp; không retry tự động. |
| `ACCOUNT_TARGET_MISMATCH` | 422 | Credential thay thế trỏ sang đích khác; đăng ký tài khoản mới. |
| `ENVIRONMENT_BUSY` | 409 | Đợi/xem trạng thái operation; giữ edits. |
| `ENVIRONMENT_UNCONFIGURED` | 422 | Chọn kết nối trong Cài đặt môi trường, field `connectionKey`. |
| `CONNECTION_UNAVAILABLE` | 422 (UC-05 standalone READY rejection giữ 400) | Kết nối không hợp lệ/không READY trong scope. |
| `VERIFICATION_FAILED` | 422 | Xác minh kết nối/kho thất bại; không trả provider diagnostics. |
| `VERIFICATION_CONTEXT_MISSING` / `VERIFICATION_UNREACHABLE` / `VERIFICATION_PERMISSION_DENIED` / `VERIFICATION_CREDENTIAL_REJECTED` | 422 | Typed context/network/permission/auth rejection; hướng dẫn cố định, không raw cause. |
| `KUBECONFIG_INVALID` / `KUBECONFIG_NO_VALID_CONTEXT` / `KUBECONFIG_CONTEXT_REQUIRED` / `KUBECONFIG_CONTEXT_NOT_FOUND` | 400 | Document/context không hợp lệ hoặc chưa chọn; inspect/review lại. |
| `KUBECONFIG_UNSUPPORTED_AUTH` / `KUBECONFIG_UNSUPPORTED_CLUSTER` / `KUBECONFIG_INVALID_SERVER` / `KUBECONFIG_INVALID_TLS` | 400 | Phương thức/cluster setting/server/TLS không hợp lệ; không chạy plugins/files ngoài. |
| `SECRET_STORE_REQUIRED` / `SECRET_STORE_UNAVAILABLE` | 422 | Chọn/kiểm tra kho, field `secretStoreKey`. |
| `REFERENCE_REWRITE_UNSAFE` | 422 | Không thể cập nhật tham chiếu mà giữ đủ cấu hình; sửa workload tường minh, giữ dialog và dữ liệu. |
| `SECRET_COPY_FAILED` | 502 | Copy không thành công; selection/revision cũ giữ nguyên. |
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

## T27 — Execution scope admission

Planning and binding persistence accept only ENVIRONMENT infrastructure scope.
Retired/unknown values fail closed before side effects, including JSON reopen
and SQL constraint validation. Normal Save cannot normalize a hostile scope
into ENVIRONMENT or overwrite a binding. Historical snapshots remain immutable;
trusted internal cluster Application resource scope is preserved under ADR-013.

## OC-12 atomic desired-reference rename — T30 / CFG-01

GET `/api/v1/applications/{app}/environments/{env}/configuration` returns the
existing safe view plus `environmentVersion` and `draftVersion`, read together
with configuration `version`/revision from one consistent scoped snapshot.
PATCH `.../configuration/keys/{key}` keeps `newName` and `version` (expected
configuration version). Optional `updateReferences` defaults to false. When true,
require integer `environmentVersion` and `draftVersion` from that view; missing,
null or malformed guards are `INVALID_REQUEST` (400), with no writes.

Prepare desired rewrites from one consistent scoped snapshot. Upsert drafts
supersede current modules; pending deletes are excluded. For deployed consumers
without drafts, reconstruct lossless reference-based Score. Rewrite exact KEY
tokens referencing resources of virtual type `environment`, including imported
resource aliases; preserve container aliases/many-to-one mappings and every
other field/reference. Never replace references with values or read Secret bytes.
Unsupported/lossy reconstruction fails with `REFERENCE_REWRITE_UNSAFE` (422)
and fixed safe message; Console asks the user to edit workloads explicitly.
Duplicate key returns `DUPLICATE`; scoped missing key is `NOT_FOUND`.

One persistence command atomically checks config/env/draft versions, current-set
identity and no active operation, inserts immutable config revision, updates its
scope pointer/version and writes all changed upsert drafts with one draft-version
advance when drafts changed. Environment version/current-set pointer, pending
deletes and applied revisions stay unchanged. Guard conflicts return
`STALE_VERSION` or `ENVIRONMENT_BUSY` (409); no partial writes. Rename-only keeps
its existing configuration-only behavior. Return safe authoritative configuration
view. No external/provider call occurs inside or because of rename. OC-17 reads
new desired references/versions and invalidates old preview tokens.

## OC-04d `ResourceDefinitionService.ListDefinitionCapabilities` (T31)

- GET `/api/v1/resource-definition-capabilities`, session org and PE/Admin;
  401/403 independently enforced. No org override or request body.
- 200 `{capabilities: [...]}` sorted by resourceType/driverType/module. Each
  entry has `resourceType`, `driverType`, `module`, verified source/revision và provider/class metadata,
  `variables` sorted by name, and `executorOwned` names. Variables carry
  `name`, `type`, `required`, `hasDefault`, optional typed `default`,
  `resourceParam` boolean and optional `preset` placeholder string.
  Required reflects module/executor contract; resourceParam indicates an accepted
  source at registration in the actual scoped Resource Type input contract.
  Namespace name counts only for required string name, per BR-12; optional or
  wrong-type name does not satisfy its required Definition input. Supported pairs are registration policy;
  never derive custom Resource Types from module variable names.
- Exclude executor-owned variables from editable variables and secrets/defaults;
  Terraform region/tags/master_password identified by inspector. Metadata không chứa private cache path, credential, raw source/template/binary
  contents hoặc provider diagnostics. Source ID/revision an toàn được công bố
  cho PE authoring; chỉ defaults từ verified module, không tự đoán capacity.
- Only scoped metadata repository reads and verified registry contract reads;
  no transaction writes, system admission, execution or provider calls. Internal
  errors use safe T20 500 envelope, no raw inspector content.
- Registration OC-04, deferred placeholder typing, resource param precedence,
  graph edges and matching unchanged. Metadata is guidance, not deployment proof.



## OC-18 `TerraformStateBackend.Read` / `Write` / `Lock` / `Unlock`

Internal HTTP adapter, not a Console/public Terraform-state API. Preconditions:
invocation capability, exact org/instance/operation owner/fence and action. Write
requires current Terraform lock ID; lock and state commit/lineage/serial checks
are atomic. Read decrypts restricted state for executor only. State encryption
key is outside database; failed decryption cannot be treated as empty state.
Expiry does not unlock; old owners cannot write/unlock. Raw state is never API
read data for Developer/PE. Full protocol/security rules and tests:
[state contract](terraform-state.md), [schema](../database/schema.md).

## OC-19 `ResourceCleanupService.PrepareDeletion` / `CleanupResources`

**User deletion:** session PE/Admin đúng Organization/Environment; root managed
UNREFERENCED, không consumer và Environment operation trống theo UC-07 DEL-*.
Developer 403 cả preview/execute. Root platform hoặc ngoài ownership không xóa.

PrepareDeletion đọc snapshot root/current + pending/retained graphs và versions.
Trả root và các dependency candidates cùng Environment/generation, các consumer
blockers và ownership/status/versions; loại platform khỏi danh sách. Token pin
actor/root/candidate IDs/versions, Environment/current-set/draft và original
targets/generation; không provider/secret calls hoặc side effects. UI chỉ tick
root mặc định; chọn/bỏ chọn tuân UC-07 DEL-BR-01.

Execute nhận `{token,confirmationName,selectedResourceIds}`. Selected set bắt
buộc có root, không trùng ID và chỉ gồm candidates đã pin. So exact tên server,
không trim/case-fold. Trong một transaction kiểm role/scope/token/versions,
UNREFERENCED/ownership và consumers của mọi mục, no active operation; nhận
Environment owner/fence và persist đúng selected set, actor/token hash/REQUESTED.
Không thêm dependency. Tập không hợp lệ bị từ chối toàn bộ trước provider call.
Snapshot đổi trả stale/busy/refused. Replay cùng token/actor và cùng selected set
trả operation cũ; đổi selected set sau admission trả 409 STALE_PREVIEW.

CHECKING kiểm original module/target/state/ownership. DESTROYING khóa state và
xóa consumer trước provider. Consumer fail thì giữ provider còn cần; chỉ tiếp
tục nhánh độc lập an toàn. VERIFYING kiểm absence từng physical ID; chỉ khi
toàn bộ confirmed set đã hết mới SUCCEEDED. Persist per-instance remaining
IDs/safe reason và state cuối/audit; không sửa historic Deployment snapshots.
Không Undo dữ liệu, rollback tự động hoặc force-unlock vì timeout.

**Run cleanup:** capability nội bộ riêng với explicit run/IDs, chỉ CREATED_BY_RUN.
PREEXISTING/UPDATED_BY_RUN không được xóa bởi run cleanup. Không giả tên/token
UI để nâng quyền. Cả hai dùng chung engine reverse graph/verification, giữ
platform/foreign/referenced resources, state và progress khi cleanup incomplete.
Full protocol [state contract](terraform-state.md); public mapping dưới đây.

### Public HTTP mapping

Prefix `/api/v1/applications/{app}/environments/{env}`; phiên lấy Organization/role.

| Operation | Route | Kết quả |
|---|---|---|
| Resource navigation (OC-11/UC-09) | GET `/resources` | 200 safe summaries/instance IDs trong Environment, kể cả UNREFERENCED; không provider refresh hoặc secret/state access. |
| Resource detail (OC-11/UC-09) | GET `/resources/{resourceId}` | 200 safe detail/eligibility/descriptor; không provider refresh. |
| PrepareDeletion | POST `/resources/{resourceId}/deletion-preview` với `{}` | 200 token, confirmationName, candidates và consumer blockers/versions/expiry. |
| CleanupResources | POST `/resources/{resourceId}/deletions` với `{token,confirmationName,selectedResourceIds}` | 202 operationId/REQUESTED; token đã nhận trả operation cũ. |
| Query operation (OC-11/UC-09) | GET `/resource-cleanups/{operationId}` | 200 scoped persisted stages/confirmed IDs/absence/remaining IDs. |

Bodies bounded/known-field; resource IDs được resolve theo scope. Missing
session 401, wrong mutation role 403, foreign/missing scope 404. Wrong typed
name 400 `RESOURCE_CONFIRMATION_MISMATCH` field confirmationName, không echo
submitted value. Stale 409 `STALE_PREVIEW`, busy 409 `ENVIRONMENT_BUSY`,
ineligible 409 `CLEANUP_REFUSED` với allowlisted reason. Missing backend/module/
credential service 503; provider failure ghi safe operation result. Network
response loss không tự replay mutation; query operation để biết đã nhận chưa.
Read routes theo quyền xem Environment, không cấp quyền Prepare/Execute.

## Resource execution failure/progress envelope

Keep T20 stable envelope and Vietnamese code mapping. Add typed reason codes
`MODULE_UNAVAILABLE`, `MODULE_INTEGRITY_FAILED`, `MODULE_INPUT_INVALID`,
`PROVIDER_BINDING_INVALID`, `STATE_UNAVAILABLE`, `STATE_LOCKED`,
`RESOURCE_REPLACEMENT_REQUIRED`, `RESOURCE_EXECUTION_FAILED`, `CLEANUP_INCOMPLETE`.
Validation/planning rejection remains before execution; BUSY/LOCKED/replacement
returns 409, invalid module input/provider binding 422 (existing Score shape
validation retains INVALID_SCORE 400), unavailable state/module service 503.
Runtime failure is persisted safe progress even when HTTP operation response
already returned; do not mistake failure status for successful cleanup.

Safe progress fields: server-known node descriptor in scoped plan, module ID/
revision, stage enum PREPARE/VALIDATE/PLAN/APPLY/OUTPUTS/READINESS/CLEANUP/VERIFY,
attempt/status and timestamps. Error params remain allowlisted stage/reason and
registered Type; field path only known schema fields, no arbitrary user keys or
submitted values. Unknown provider/CLI/decode cause uses safe fallback. No raw
stdout/stderr, plan JSON, backend capability, credential refs or state payload.

Cleanup refusal reasons được phép: NOT_UNREFERENCED, HAS_CONSUMERS,
PLATFORM_RESOURCE, OWNERSHIP_UNVERIFIED, ORIGINAL_TARGET_UNAVAILABLE.
Operation progress stage enum riêng: REQUESTED/CHECKING/DESTROYING/VERIFYING;
status SUCCEEDED/FAILED/INTERRUPTED kèm remaining IDs đã scope. Error envelope
không raw diagnostics/typed name; safe resource IDs chỉ ở scoped query result.

## Resource Type class read projection — I04-07

GET `/api/v1/resource-types` giữ session Organization/all-authenticated read
boundary; `resourceTypes` entries thêm computed `classes: string[]`, sorted/
unique, không persistence hay POST field. Query đọc cùng scoped snapshot của
Types/Definitions và local verified registry metadata, không provider/CLI/
secret-store calls/fetch hoặc writes. Module pins/provider controls vẫn chỉ
ở PE/Admin capabilities OC-04d. Class metadata không thay matching/Preview;
unknown/custom/default semantics và imported selections không bị rewrite.
Failure safe error, không fake empty/guessed class fallback. Chi tiết ở
[UC-16 API mapping](../../usecase/UC-16/ui/api-mapping.md#metadata-class-và-source--i04-07).


## OC-05a — V01 infrastructure account management

Use case: UC-04 AC-01..12. Decision: ADR-015. Exact wire contract belongs to
[UC-04 API mapping](../../usecase/UC-04/ui/api-mapping.md#v01-infrastructure-account-api).
Organization/PE/Admin scope precedes all secret/provider access. List/get are safe
metadata only; metadata rename cannot alter identity/target/ref. Creation starts
at version1; updates require version CAS; no external I/O inside database transactions. Verify is
read-only externally and commits only successful verification/version. Explicit
same-target replacement writes an immutable attempt then CAS publishes metadata/
ref/version; proven rollback/stale failure removes only attempt-owned objects.
Uncertain commit retains potentially committed objects and requires GET recovery
and explicit user review without mutation replay. Replacement AWS credentials
must pass verification in every retained legacy allowed region. Keep all prior
committed objects for pinned consumers. No secret-ref input/read endpoint or
host fallback. New account types/authentication and verification regions do not
introduce applicability, matching rules or Environment bindings. V02–V03 old-data
migration is not required; no implicit retained-state reset is authorized.

## V02 replacements for OC-04, OC-04c, OC-04d and Definition matching

Canonical behavior: UC-03 V02-01..09; this section supersedes profile filtering,
Environment AWS account selection and insert-only statements in older contracts.

- OC-04 create: POST `/api/v1/resource-definitions` accepts strict Definition
  authoring DTO `key`, `name`, `resourceType`, `driverType`, `driverAccount?`,
  `driverInputs`, `criteria`, `provision?`; returns201 with positive `version`.
  Server-owned fingerprint/version cannot be injected at creation. Scoped
  PE/Admin only; validate shared schema/module/account before atomic insert.
- OC-04 update: PUT `/api/v1/resource-definitions/{key}` accepts same editable
  fields plus required `version`; path/body key and stored Resource Type must
  agree. Returns200 with incremented version; 404 scoped missing, 409 stale,
  400 invalid strict input, 403 role. Same validation as create, atomic
  Definition/criteria/bundle CAS; no delete endpoint, automatic retry or writes
  to historical plan/resource pins. Public list includes Name/version and safe
  selected account key, never credential references.
- OC-04c keeps candidate/context route and request envelope. Same weighted
  matcher as planner without profile/Connection/account prefilter; ordinary
  matching does not require Environment target. Reads include scoped safe account
  metadata. Winner includes safe selected account identity/version and region or
  inherited cluster source; incompatible/missing/not-READY winner is explicit
  account/binding conflict without fallback. Existing status naming may retain
  CONNECTION_CONFLICT as safe binding-conflict compatibility code. INCOMPLETE,
  NO_MATCH, AMBIGUOUS, MATCHED semantics remain. SYSTEM_BINDING applies only to
  actual legacy implicit node until V03. Response includes reason/criterion/score;
  marker covers catalog/context/account versions without secret bytes.
- OC-04d capabilities publish account requirement/kind and target source along
  with current verified drivers/module schemas. AWS region is editable required
  literal input; credentials/password/tags remain executor-owned. Installed
  score-k8s/render-bundle discovery remains safe and exposes no Generic Workload
  capability. UI never invents an unsupported driver.
- OC-06/07/08/10: all Definition ranking follows UC-03 V02-02. Persist actual
  winner account/region/Definition version/content pins; preview stale on edit.
  Runtime checks identity/kind/READY and account snapshot consistency before
  provision; cannot substitute Environment/host account. Historical cleanup and already-admitted execution use
  original stored pins rather than rereading mutable Definition. Existing state
  target mismatch cannot trigger an implicit move/adoption. Legacy implicit
  enrichment is reserved for V03 removal.


## V03 operations — Definition-selected internal lifecycle

[ADR-016](../decisions/ADR-016-definition-selected-workload-targets.md) replaces internal system-binding/Environment-target preconditions in OC-04/04c/06/07/08/10 and the internal target Settings operation. Registration permits existing-cluster with scoped READY Internal account and workload reference/provision configuration. Planning selects workload first, expands all references/rules, validates typed placement/DAG and pins versions without writes/provider calls. Deploy validates same snapshot, claims operation, resolves resource outputs and dispatches selected workload lifecycle to its exact target. Update/Remove restore applied target after restart. Cleanup uses original owned state; existing cluster is never eligible. V03.10 rejects retired internal deployment-Connection writes and removes that selector after live replacement verification. Secret Store operations remain. Multi-cluster Candidate Sets fail clearly before side effects in V03.

## V04–V06 Definition-driven operation consistency

[ADR-017](../decisions/ADR-017-definition-dependency-execution.md) replaces
remaining Environment-profile target inference in OC-04/06/07/08/10/19.
Registration accepts existing or EKS-producing matched cluster sources under the
same explicit resource-reference contract. Preview is pure, exposes typed unknown
output provenance and per-resource account/region without credentials. Deploy
revalidates pins/CAS, provisions actual DAG, resolves original-identity auth and
checks readiness before consumers. Update/restart reuse original instance/state;
Remove and confirmed owned deletion use original target/reverse graph with
consumer/ownership/UID and service-API absence verification. Public errors/status
must remain safe and truthful on dependency/typing/auth/network/readiness failure.


### V06.08 retired Organization default marker

The historical OC-01 choice/default query and OC-01b deployment-setting commands
are absent after the final replacement gate. Organization deployment-default
fields/FK and seed/account-save marker updates are removed. Surviving OC-05
registration preserves the whole Organization identity/metadata; account ownership,
scoped credentials, role gates and per-Definition target pins stay unchanged.

### Internal operator safety-net selection — V04 provenance

Stable Environment Preview run IDs do not separate negative instances. An optional
explicit instance-ID set narrows existing RUN_CLEANUP recorded-run/generation
ownership. Complete validation precedes claim/effects; duplicate/empty/unknown/
foreign/wrong-generation selection rejects. Nil preserves whole-run behavior.
Unselected instances stay unchanged with NOT_SELECTED; partial NEEDS_ATTENTION
is not whole-run CLEAN. Existing pinned version/module/state/target/dependency
guards remain. Historical probe selector admits PENDING only with complete owned
CREATE/FAILED/PLANNING history, no observed IDs/state serial/recovery; engine
independently checks no-state/no-effect absence. No public API or authority added.
Live probe scenarios are no longer required by the19-10 acceptance decision.

## OC-EC2 — RegisterEC2Role / VerifyAccount

UC-04 EC2-01..05: PE/Admin and session Organization gate. Strict registration
{name,roleArn,verificationRegions}; read-only role/region/permission verification,
insert-only version 1, empty secret reference. Verify {version,verificationRegions?}
revalidates the pinned ARN/RoleId then CAS increments version; failure preserves
last verification. Unknown commit requires GET recovery, never automatic replay.
Credential replacement is incompatible. Safe DTO includes canonical role metadata.
