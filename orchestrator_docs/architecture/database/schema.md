---
id: DATABASE-SCHEMA
artifact: database-schema
status: current
last_reviewed: 2026-10-07
---

# Database Schema

PostgreSQL là system of record cho logical orchestration state. JSONB chỉ dùng cho immutable contract/plan documents; identity, lifecycle và relations quan trọng được chuẩn hóa thành columns/tables.

## Tables

### `organizations`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `organization_key` | text | UNIQUE, NOT NULL |
| `name` | text | NOT NULL |
| `default_connection_id` | uuid | FK connections, NOT NULL for onboarding-enabled Organization |

The Organization default is only a marker in Environment Settings choices.
UC-01 creates UNCONFIGURED Environments and never resolves a default. Targets are independently editable with versions and explicit transitions; see
[ADR-012](../decisions/ADR-012-environment-stores-and-transitions.md).

### `user_accounts`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `organization_id` | uuid | FK organizations, NOT NULL |
| `username` | text | normalized, UNIQUE, NOT NULL |
| `password_hash` | text | NOT NULL |
| `role` | text | `ADMIN`, `PLATFORM_ENGINEER` or `DEVELOPER`, NOT NULL |
| `status` | text | `ACTIVE` or `DISABLED`, NOT NULL |
| `created_at`, `updated_at` | timestamptz | NOT NULL |

Fixed test accounts are seed-only for `local`/`test`; they are not production records.

### `sessions`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `user_account_id` | uuid | FK user_accounts, NOT NULL |
| `token_hash` | text | UNIQUE, NOT NULL |
| `expires_at` | timestamptz | NOT NULL |
| `revoked_at` | timestamptz | nullable |
| `created_at` | timestamptz | NOT NULL |

Only token hashes are persisted. A revoked or expired session is invalid.

### `connections`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `organization_id` | uuid | FK organizations, NOT NULL |
| `connection_key` | text | NOT NULL |
| `name` | text | NOT NULL; legacy rows backfill connection_key |
| `authentication_type` | text | NOT NULL; `HOST_CONTEXT`, `KUBECONFIG` or future `AWS_ACCESS_KEY`; kind-compatible |
| `kind` | text | `AWS` or `KUBERNETES` |
| `config` | jsonb | non-secret config, NOT NULL |
| `secret_ref` | text | NOT NULL |
| `status` | text | `READY` in MVP |
| `verification` | jsonb | principal/cluster metadata, NOT NULL |
| `created_at`, `updated_at` | timestamptz | NOT NULL |

Unique: `(organization_id, connection_key)`.

New kubeconfig records store cluster/context/endpoint metadata and a scoped
opaque credential reference. Secret values never enter config/verification.
`name` and `authentication_type` are additive columns; migration backfills
legacy Kubernetes as `HOST_CONTEXT`, AWS as `AWS_ACCESS_KEY` legacy process
metadata. Missing new JSON fields retain explicit legacy compatibility.
AWS migration does not claim seeded metadata has durable credentials.
Registration is insert-only; generated key collisions never overwrite rows or
change Organization default. Existing reference formats must stay compatible.

Host-context records use `host-kube-context://<context>` and still require the
same host context. New records use credential store independently of host.
See [credential design](../connection-credentials.md).

### `applications`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `application_key` | text | stable API lookup key, UNIQUE, NOT NULL; generated Applications use the same UUID text as `id`, legacy seed may use a readable alias |
| `organization_id` | uuid | FK organizations |
| `name` | text | NOT NULL |
| `subdomain` | text | normalized DNS label, NOT NULL |
| `execution_profile` | text | legacy only: `aws-eks`/`internal-k8s`; empty for new unbound Application |
| `connection_id` | uuid | nullable FK connections; retained legacy binding, NULL for new Application |
| `region` | text | legacy only; empty for new Application |
| `runtime_status` | text | legacy PENDING/READY; UNCONFIGURED for new Application |
| `version` | bigint | optimistic version |
| `configuration_provider` | text | legacy only; current Secret Store selection belongs to Environment |

`id` is system-generated and immutable. Application Name is unique within an
Organization without regard to case, enforced by a unique index on
`(organization_id, lower(name))`. `subdomain` is normalized to lowercase and
unique globally within the configured platform base domain.

### `environments`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `application_id` | uuid | FK applications |
| `environment_key` | text | NOT NULL |
| `name`, `environment_type` | text | NOT NULL |
| `namespace_identity` | text | NOT NULL |
| `connection_id` | uuid | nullable FK connections; NULL means UNCONFIGURED, never inherit Organization default |
| `execution_profile` | text | empty while unset, else aws-eks/internal-k8s derived from selected Connection |
| `region` | text | empty while unset/internal; nonempty for configured AWS |
| `runtime_status` | text | UNCONFIGURED, PENDING (AWS not provisioned), READY |
| `infrastructure_scope` | text | ENVIRONMENT for new, LEGACY_APPLICATION only for migrated bindings |
| `current_deployment_set_id` | uuid | nullable FK deployment_sets, deferred |
| `version` | bigint | optimistic version |
| `draft_version` | bigint | optimistic UC-16 draft version |
| `public_routes_pending` | boolean | true while route reconciliation has not completed after pending Deploy; route-only retry survives process restart |
| `desired_config_revision_id` | uuid | nullable FK configuration_revisions, deferred |

`environment_key` is system-owned and limited to `staging` or `production`.
Unique: `(application_id, environment_key)` and `(application_id, namespace_identity)`.
The application-creation transaction inserts exactly those two rows, unset.

Selection changes use Environment.version CAS and operation-owner checks, not a
permanent immutable-binding trigger. Dedicated binding commands own writes;
ordinary Save cannot bypass them. Generation 0 retains old identities; later
targets are isolated. Legacy conversion is idempotent and has no default fallback.
See [ADR-012](../decisions/ADR-012-environment-stores-and-transitions.md).

### `configuration_revisions`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK; immutable |
| `environment_id` | uuid | FK environments, NOT NULL |
| `version` | bigint | monotonically increasing within Environment |
| `created_at` | timestamptz | NOT NULL |

Unique `(environment_id, version)`. A new revision copies prior entry refs and
changes only the named key. Saving desired configuration moves
`environments.desired_config_revision_id` but never changes applied workload
revisions. Revision rows contain no values.

### `configuration_revision_entries`

| Column | Type | Constraint |
|---|---|---|
| `revision_id` | uuid | PK/FK configuration_revisions |
| `key_name` | text | PK; valid environment variable name |
| `kind` | text | `VARIABLE` or `SECRET` |
| `value_ref` | text | immutable Secret ref or legacy Variable ref; nullable for new ordinary Variable |
| `store_key` | text | owning store identity for Secret/legacy refs |
| `variable_value` | text | ordinary Variable value only; NULL for Secret |

The primary key gives one namespace across Variable and Secret. Secret plaintext is forbidden; ordinary Variable values are allowed in this table
and JSON state. Legacy Variable refs materialize outside schema migrations.

### `workload_drafts`

| Column | Type | Constraint |
|---|---|---|
| `environment_id` | uuid | PK/FK environments |
| `workload_id` | text | PK |
| `score_document` | jsonb | one validated Score workload; nullable for pending delete |
| `state` | text | `PENDING_UPSERT` or `PENDING_DELETE` |
| `updated_at` | timestamptz | NOT NULL |

The Environment `draft_version` advances on each draft save/delete/undo.
Drafts do not alter `deployment_sets` until successful deployment.

### `resource_types`

Columns: `id uuid PK`, `organization_id FK`, `resource_type_key text`, `input_schema jsonb`, `output_schema jsonb`, timestamps. Unique `(organization_id, resource_type_key)`.

### `resource_definitions`

Columns: `id uuid PK`, `organization_id FK`, `definition_key text`, `resource_type_id FK`, `execution_profile text nullable` (`internal-k8s` or `aws-eks`), `driver_type text`, `connection_id FK nullable`, `driver_inputs jsonb`, `provision_rules jsonb`, `source_fingerprint text nullable`, timestamps. Unique `(organization_id, definition_key)`. A NULL profile is shared across profiles; a non-NULL profile filters eligible Definitions before criteria scoring.

### `matching_criteria`

Columns: `id uuid PK`, `resource_definition_id FK ON DELETE CASCADE`, optional `env_type`, `app_id`, `env_id`, `res_id`, `class`, and computed `specificity_score int`. Index by `resource_definition_id` and match fields.

Năm field này cùng trọng số `env_type=1`, `app_id=2`, `env_id=4`, `res_id=8`, `class=16` là contract chuẩn của UC-03 BR-06. `execution_profile` nằm trên Definition, không nằm trong Matching Criterion và không đổi trọng số. Vì vậy Application mới vẫn chọn được Aurora hoặc StatefulSet theo profile, không cần seed thêm criterion `app_id`.

### `deployment_sets`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `environment_id` | uuid | FK environments |
| `created_by_deployment_id` | uuid | nullable FK deployments, deferred |
| `document` | jsonb | canonical Deployment Set |
| `document_hash` | text | deterministic fingerprint |
| `created_at` | timestamptz | NOT NULL |

Deployment Sets are immutable. Canonical shape:

```json
{
  "modules": {
    "<workload-id>": {
      "profile": "humanitec/default-module",
      "spec": { "containers": { "<name>": { "image": "...", "variables": { "PGHOST": "${shared.acceptance-db.host}" }, "resources": { "requests": { "cpu": "100m", "memory": "128Mi" }, "limits": { "cpu": "500m", "memory": "512Mi" } } } } },
      "externals": { "<name>": { "type": "redis", "class": "default", "params": { } } }
    }
  },
  "shared": { "<shared-id>": { "type": "postgres", "class": "default", "params": { } } }
}
```

For the internal-kind public route extension, each workload module may carry
`spec.service.publicRoutes: [{path, port}]`, where `port` names a declared
`spec.service.ports` entry. The legacy `spec.service.publicPort` denotes `/`.
Paths are unique across the Environment; multiple workloads may be public on
distinct paths. The route itself is one Environment-owned Kubernetes Ingress,
not a secret or database row.

Private dependency của một workload nằm trong `modules.<id>.externals.<name>`; shared dependency nằm ở `shared.<id>`.
Placeholder trong `spec` tham chiếu resource bằng `${externals.<name>[.<output>]}` và `${shared.<id>[.<output>]}`.

### `deployment_delta_snapshots`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `deployment_id` | uuid | FK deployments, UNIQUE, NOT NULL |
| `document` | jsonb | canonical Humanitec-shaped Delta, NOT NULL |
| `document_hash` | text | deterministic content fingerprint, NOT NULL |
| `metadata` | jsonb | actor/source metadata without secrets, NOT NULL |
| `created_at` | timestamptz | NOT NULL |

Deployment Delta Snapshots bất biến ngay khi persist. `document` có shape:

```json
{
  "modules": {
    "add": { "<workload-id>": { "profile": "humanitec/default-module", "spec": {} } },
    "remove": ["<workload-id>"],
    "update": { "<workload-id>": [{ "op": "replace", "path": "/spec/containers/main/image", "value": "image:v2" }] }
  },
  "shared": [{ "op": "add", "path": "/database", "value": { "type": "postgres", "class": "default" } }]
}
```

Các nhánh rỗng bị omit và no-op Delta là `{}`. Module patch relative với module;
shared patch relative với object `shared`.

Table này không mô hình hóa mutable Humanitec Delta lifecycle. Standalone Delta
API/update/archive được deferred tại D05.

Snapshot ownership is normalized through `deployment_id`; its Application is
always derived through Deployment → Environment → Application. This prevents
orphan Snapshots and mismatched tenant ownership without triggers. The foreign
key uses `ON DELETE CASCADE` for a future controlled history purge; normal
product operations do not delete Deployment history.

### `deployments`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `environment_id` | uuid | FK environments |
| `action` | text | `DEPLOY`, `UPDATE`, `REMOVE` |
| `workload_id` | text | NOT NULL |
| `actor_ref` | text | NOT NULL |
| `status` | text | state-machine value |
| `base_environment_version` | bigint | NOT NULL |
| `base_deployment_set_id` | uuid | FK deployment_sets |
| `candidate_deployment_set_id` | uuid | FK deployment_sets |
| `started_at`, `finished_at` | timestamptz | lifecycle timestamps |

Index `(environment_id, started_at desc)`.

Không có Snapshot row nghĩa là planning chưa tạo được Delta Snapshot.
`PLANNING` và `FAILED` do planning có thể chưa có row; một `FAILED` sau planning
giữ Snapshot đã có. Deferred constraint trigger bắt buộc Deployment ở
`PROVISIONING`, `DEPLOYING`, `SUCCEEDED` có đúng một Snapshot trước transaction
commit. `deployment_delta_snapshots.deployment_id UNIQUE NOT NULL` bảo vệ quan
hệ một-một và cấm Snapshot orphan.

```sql
-- enforced by a DEFERRABLE CONSTRAINT TRIGGER across the two tables
```

### `deployment_plans`

One-to-one with Deployment: `deployment_id uuid PK/FK`, `score_before jsonb`, `score_after jsonb`, `resource_graph jsonb`, `matched_definitions jsonb`, `provision_batches jsonb`, `classification jsonb`, `plan_hash text`, `created_at`. Snapshot được nối bằng `deployment_delta_snapshots.deployment_id`. Snapshot bất biến ngay khi persist; Plan bất biến sau khi Deployment rời `PLANNING`.

### `active_resources`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `organization_id` | uuid | FK organizations |
| `descriptor` | text | canonical `type.class#res_id` |
| `scope_type` | text | `APPLICATION`, `ENVIRONMENT`, `WORKLOAD`, `SHARED` |
| `scope_id` | text | stable scoped identity |
| `resource_definition_id` | uuid | FK resource_definitions |
| `connection_id` | uuid | FK connections, nullable |
| `status` | text | state-machine value |
| `executor_state_ref` | jsonb | opaque backend/workspace/object refs |
| `outputs` | jsonb | non-secret or secret references only |
| `input_fingerprint`, `source_fingerprint` | text | nullable |
| `last_deployment_id` | uuid | FK deployments |
| `version` | bigint | optimistic version |

Unique: `(organization_id, descriptor, scope_type, scope_id)`.

### `deployment_resources`

Columns: `deployment_id FK`, `node_descriptor text`, `active_resource_id FK nullable`, `status text`, `resolved_inputs jsonb`, `output_snapshot jsonb`, `batch_index int`, timestamps. PK `(deployment_id, node_descriptor)`.

### `deployment_workloads`

Per-Deployment workload execution snapshot: `deployment_id FK`, `workload_id
text`, `status text`, `target_ref jsonb`, `manifest_digest text`,
`applied_config_revision_id uuid nullable` and `observed_at timestamptz`. Primary
key is `(deployment_id, workload_id)`. A row may be updated while its owning
Deployment is running and is immutable after the Deployment reaches a terminal
state. UC-09 reads this table, not the mutable current `workload_instances`
row, so an older Deployment never shows status from a later run.

When upgrading stores that predate this snapshot, only the latest known
`workload_instances` row can be backfilled to its `last_deployment_id`. Earlier
runs have no recoverable workload snapshot and return no workload rows; the
migration must not copy current state to every historical Deployment.

### `workload_instances`

Columns: `id uuid PK`, `environment_id FK`, `workload_id text`, `last_deployment_id FK`, `applied_config_revision_id uuid FK configuration_revisions nullable`, `target_ref jsonb`, `manifest_digest text`, `status text`, `observed_at timestamptz`. Unique `(environment_id, workload_id)`.

Per-workload applied revision allows an honest partial-success state when a
multi-workload configuration rollout fails. The desired Environment pointer
does not imply every workload has consumed that revision.

## Transaction contracts

1. Create Environment: Environment + empty Deployment Set + current pointer atomically.
2. Create deployment plan: insert Deployment `PLANNING` trước, chưa có Snapshot;
   sau đó persist DeploymentDeltaSnapshot + Candidate Set + DeploymentPlan và
   chuyển Deployment sang `PROVISIONING` atomically; current pointer unchanged.
3. Resource completion: Active Resource upsert + Deployment Resource status atomically per node.
4. Workload execution progress: update the current Workload Instance and the
   owning Deployment's workload snapshot together.
5. Final deployment commit: compare Environment version, update current pointer/version, mark cloud Application runtime `READY`, upsert Workload Instances and mark Deployment `SUCCEEDED` atomically.

## Secret rule

Credential values and secret resource outputs are never stored directly. `secret_ref` or structured secret references are stored; UC-09 redacts fields marked secret by contract metadata and never returns persisted resource `resolved_inputs`.

## ADR-010 rendering intent persistence

Rendering selections (Definition/content hash, driver, bundle/version/content
digest) are additive non-secret fields inside the immutable deployment plan JSON
and therefore its plan hash. Resource Definition driver_type is text; inputs and
source fingerprint retain existing storage. No new table/column or secret storage
is introduced. Pending Preview hashes bind the rendering intent through existing
per-workload plan hashes. Logical resource and workload identities are unchanged.

## Historical Environment target migration constraints (ADR-011)

Permanent immutability described below is superseded by ADR-012; generation-0
backfill/identity preservation remains.

Nullable legacy applications.connection_id is preserved; new Applications write
NULL connection, empty profile/region, runtime UNCONFIGURED. Add Environment target
columns with empty profile/region, UNCONFIGURED runtime, ENVIRONMENT scope defaults.
Backfill only unset Environments whose Application has non-NULL legacy connection;
copy all target fields and mark LEGACY_APPLICATION. Reopen is idempotent.
Consistency CHECK and immutable binding trigger protect configured connection,
profile, region and scope. Runtime status/version updates remain legal. Atomic
SetConnection verifies ownership and READY selection plus unset/expected version.

## Environment stores and transitions (ADR-012 migration)

Add Organization-owned `secret_store_connections`: immutable ID/key/name/provider,
backend/workload addresses, KV/auth mounts, nonsecret TLS configuration,
credential_ref/status/timestamps. Unique `(organization_id, store_key)`. References
must be Organization-scoped. Credential bytes live only in platform credential store.

Extend `environments` with selected store key/FK, target_generation bigint default
0, active_operation_id nullable, selected/active runtime generation metadata as
needed for transition staging. Environment.version remains Settings CAS. Remove
permanent binding immutability trigger through additive versioned migration; retain
consistency checks and enforce dedicated CAS/owner commands. Do not clear bindings.

Add `environment_operations`: ID, Environment FK, owner, fencing generation
(bigint starting at 1, incremented on recovery), kind, status/stage,
source/destination binding JSON (nonsecret), pinned Environment/draft/config
versions/current Set, start/update/deadline, safe failure/recovery metadata.
Unique active owner per Environment, enforced by row claim or partial unique
index. Claim/release and version guards run inside one transaction. A restart
preserves INTERRUPTED recovery state; no automatic expiry bypass of live owner.
Ownership must change on recovery (owner identity or a monotonic fencing token).
Heartbeat, release and every owner-only write, including transition bookkeeping,
check the current ownership atomically; a reused operation ID is insufficient.
Persist safe confirmation/audit metadata that prior execution was stopped before
recovery takes ownership. Enforce recorded deadlines on executor contexts.

Add persisted transition/retained-target records: operation ID, target generation,
source/destination TargetRefs, old Set, resource/executor state identity, writer
replicas at source and attempt-owned destination, private backup handle/hash,
source authority/quiesce/needs-attention/cleanup state and stage results. Historical target/resource
records must not be overwritten by new generation. Extend ActiveResource identity
and applied WorkloadInstance/deployment snapshot identity with target generation
or retain equivalent immutable generation-owned records. Generation 0 IDs and
all old plan/state remain valid; new Terraform/namespace identities are isolated.

Secret refs/revisions pin store identity; old applied refs are immutable. New
Variable values are metadata, never Secret values. Legacy explicit platform-store
backfill preserves refs; Vault network reads are excluded from SQL migrations.
JSON and SQL adapters expose equivalent CAS/claim/transition persistence semantics.

Local Compose bootstrap (UC-04 SS-07..08) retains the existing
`secret_store_connections` schema. Its explicit managed-store admission may
atomically convert matching `platform-vault` legacy metadata or refresh a token's
opaque credential reference after verification, preserving the row ID/key and
endpoint/mount identity. CAS checks protect concurrent startup. No secret/token
plaintext is stored, and Environment/revision/value references remain unchanged.
