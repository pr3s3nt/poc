---
id: OPERATION-CONTRACTS
artifact: operation-contracts
status: current
last_reviewed: 2026-10-02
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
- Preconditions: Organization tồn tại; default execution target có connection `READY`; Name/Subdomain hợp lệ và chưa trùng.
- Creates: system-ID `Application` với profile/connection binding, cùng `staging` và `production` Environments, immutable empty Deployment Sets và stable namespace identities.
- Postconditions: AWS Application `PENDING`; internal Application bind existing cluster; desired endpoints được suy ra nhưng chưa có infrastructure, workload hoặc route/ingress.
- Persistence: insert Application, hai Environment, hai Deployment Set và current pointers atomically trong một transaction.

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
  Terraform modules; connection tường minh của Terraform/existing-cluster phải
  READY trong cùng Organization. Không nhận remote source.
- Creates: `ResourceDefinition` và một hoặc nhiều `MatchingCriterion`.
- Postconditions: queryable cho deterministic matching; `{}` tường minh là
  wildcard điểm `0`; optional `execution_profile` filters eligibility before
  scoring the five standard criteria; source fingerprint được ghi nếu có.
- Persistence: insert-only definition + criteria atomically; concurrent duplicate
  conflicts without changing the existing Definition/criteria. Seed Save/upsert
  is separate from public registration.

## OC-05 `ConnectionService.InspectKubeconfig` / `RegisterKubeconfig` / `RegisterKubernetesCluster` / `RegisterAWSDriverAccount`

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
- Explicit legacy host-context registration/records remain compatible, never a
  fallback for upload failures. AWS operation is future design under ADR-009.
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
- Descriptor rule: `@app`, `@env`, `@connection` được resolve từ planning
  context trước khi parse descriptor; `@` kế thừa class/ID hiện tại giữ nguyên
  semantics.
- Postconditions: graph là DAG; mỗi resource node match đúng một Definition; contracts valid; provider-first batches; workload node không thuộc resource-execution batches.
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
- Postconditions: UC-08 complete; workload ready với declared container requests/limits; current-set pointer atomically đổi; AWS Application runtime `READY`; Deployment `SUCCEEDED`.
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
- For each node: resolve prior state and inputs -> executor provision/reconcile -> validate outputs -> atomically persist Active Resource + deployment-resource progress.
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
  and identifies every affected workload, including those referencing changed
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
- Domain errors are typed Go errors; delivery maps them to API status later.
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
