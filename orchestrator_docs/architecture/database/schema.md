---
id: DATABASE-SCHEMA
artifact: database-schema
status: current
last_reviewed: 2026-10-13
---

# Database Schema

PostgreSQL là system of record cho logical orchestration state. JSONB chỉ dùng cho immutable contract/plan documents; identity, lifecycle và relations quan trọng được chuẩn hóa thành columns/tables.

## Tables

### `schema_baseline`

Technical singleton marker: `singleton boolean PK CHECK(singleton) DEFAULT true`,
`baseline text NOT NULL`, `baseline_sha256 text NOT NULL`,
`catalog_sha256 text NOT NULL` (hai digest SHA-256 gồm 64 hex),
`installation_id uuid NOT NULL`, `schema_name text NOT NULL`,
`created_at timestamptz NOT NULL DEFAULT now()`.
Exactly one row identifies the current baseline and installation. Owner setup
records the embedded baseline digest, created catalog digest and schema;
runtime checks these read-only and checks that Console/State connections refer
to the same installation/database/schema. It carries no product state
and is not an upgrade ledger; structural validation is required on reopen.


### `organizations`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `organization_key` | text | UNIQUE, NOT NULL |
| `name` | text | NOT NULL |
| `default_connection_id` | uuid | nullable FK connections; when present references an existing scoped record |

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
| `name` | text | NOT NULL; supplied explicitly |
| `authentication_type` | text | NOT NULL; `KUBECONFIG` or `AWS_ACCESS_KEY`; kind-compatible |
| `kind` | text | `AWS` or `KUBERNETES` |
| `config` | jsonb | non-secret config, NOT NULL |
| `secret_ref` | text | NOT NULL |
| `status` | text | `VERIFYING`, `READY` or `REJECTED` |
| `verification` | jsonb | principal/cluster metadata, NOT NULL |
| `created_at`, `updated_at` | timestamptz | NOT NULL |

Unique: `(organization_id, connection_key)`.

New kubeconfig records store cluster/context/endpoint metadata and a scoped
opaque credential reference. Secret values never enter config/verification.
Kubernetes authentication is explicitly KUBECONFIG; missing authentication or
HOST_CONTEXT bị từ chối. AWS_ACCESS_KEY bắt buộc scoped Vault ref/account/
allowedRegions, không host/process-configured execution. Registration insert-only; keys không
overwrite rows or change Organization default.

A seeded Kubernetes marker without uploaded private credential is VERIFYING,
with no fabricated credential reference or successful verification. It may satisfy
the Organization default FK but is not selectable or executable. New Applications
still create UNCONFIGURED Environments. No Kubernetes legacy data conversion is
provided. Startup không tự upgrade/reset retained state.
See [credential design](../connection-credentials.md).

### `applications`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `application_key` | text | stable API lookup key, UNIQUE, NOT NULL; generated Applications use the same UUID text as `id`, local seed may use a readable alias |
| `organization_id` | uuid | FK organizations |
| `name` | text | NOT NULL |
| `subdomain` | text | normalized DNS label, NOT NULL |
| `version` | bigint | optimistic version |

`id` is system-generated and immutable. Application Name is unique within an
Organization without regard to case, enforced by a unique index on
`(organization_id, lower(name))`. `subdomain` is normalized to lowercase and
unique globally within the configured platform base domain.

T26 removes all Application execution/provider columns. Old Application-bound
state is unsupported and must fail startup with a safe reset-required error,
without copying target fields, exposing row values or rewriting historical
targets. T28 initializes only an empty database with the current baseline; retained dev
state không được tự upgrade/reset. Lịch sử reset T29 thuộc verification/current state.

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
| `infrastructure_scope` | text | NOT NULL, ENVIRONMENT only; reject retired/unknown values (T27) |
| `current_deployment_set_id` | uuid | nullable FK deployment_sets, deferred |
| `version` | bigint | optimistic version |
| `draft_version` | bigint | optimistic UC-16 draft version |
| `public_routes_pending` | boolean | true while route reconciliation has not completed after pending Deploy; route-only retry survives process restart |
| `desired_config_revision_id` | uuid | nullable FK configuration_revisions, deferred |
| `secret_store_id` | uuid | nullable scoped FK secret_store_connections |
| `target_generation` | bigint | immutable execution generation, default 0 |
| `active_operation_id` | uuid | nullable FK environment_operations; owner/fence admission |

`environment_key` is system-owned and limited to `staging` or `production`.
Unique: `(application_id, environment_key)` and `(application_id, namespace_identity)`.
The application-creation transaction inserts exactly those two rows, unset.

Selection changes use Environment.version CAS and operation-owner checks, not a
permanent immutable-binding trigger. Dedicated binding commands own writes;
ordinary Save cannot bypass them. Generation 0 retains old identities; later
targets are isolated. No Application target conversion/default fallback is supported (T26).
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
| `value_ref` | text | immutable Secret ref; NULL for Variable |
| `store_key` | text | explicit owning store identity for Secret; empty for Variable |
| `variable_value` | text | ordinary Variable value only; NULL for Secret |

The primary key gives one namespace across Variable and Secret. Secret plaintext is forbidden; ordinary Variable values are allowed in this table
and JSON state. Variable entries require nonempty variable_value and no store/ref;
Secret entries require explicit store/ref and no variable_value. Old store-backed
Variables are invalid; no materialization, backfill or network migration (T24).

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
`spec.service.ports` entry. `spec.service.publicPort` is not accepted in current
product documents; old drafts are not converted automatically (dev reset T29).
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

Startup never reconstructs workload history from `workload_instances` (T28).
Current per-Deployment snapshots are preserved on reopen. Missing history is
not fabricated from current runtime state; unsupported older stores require a
development reset before serving.

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
5. Final deployment commit: compare Environment version, update current pointer/version, mark AWS Environment runtime `READY`, upsert Workload Instances and mark Deployment `SUCCEEDED` atomically.

## Secret rule

Credential values and secret resource outputs are never stored directly in logical metadata. `secret_ref` or structured secret references are stored; UC-09 redacts fields marked secret by contract metadata and never returns persisted resource `resolved_inputs`. V2 Terraform state is a separate restricted encrypted blob as specified below; it may contain provider secret material and is never a public read model.

Terraform plan JSON pin `providerBindings` của từng node: tên/kind provider,
target Environment hoặc resource descriptor, Connection key/kind, region,
profile và target generation. Các trường này tham gia plan hash; không chứa
credential, token, kubeconfig hoặc path của invocation. Resource target snapshot
giữ scoped opaque EKS identity (Organization, Connection, cluster name, region)
để cleanup tái tạo credential cho đúng target lịch sử.

## ADR-010 rendering intent persistence

Rendering selections (Definition/content hash, driver, bundle/version/content
digest) are additive non-secret fields inside the immutable deployment plan JSON
and therefore its plan hash. Resource Definition driver_type is text; inputs and
source fingerprint retain existing storage. Rendering selections không chứa secret values; state blobs dùng schema riêng
bên dưới, không nằm trong rendering intent. Pending Preview hashes bind the rendering intent through existing
per-workload plan hashes. Logical resource and workload identities are unchanged.

## Fresh persistence baseline (T28 / LEG-08)

Only an empty PostgreSQL schema can be initialized. Owner setup creates the
complete current schema and a baseline identity atomically under a database
advisory transaction lock. A second boot validates that identity and the required
schema (tables, column types/nullability, keys, indexes and integrity triggers)
before serving, without DDL repair or row conversion. A pre-baseline ledger,
unmarked nonempty schema, incompatible marker or damaged current schema fails
closed with a safe development-reset-required error. Deleting an old migration
ledger does not make its schema empty. No old versioned migrations are replayed.
The identity is technical adapter metadata, not an upgrade path.

Fresh schema includes UUID account/session IDs, case-insensitive Organization
Application-name uniqueness, explicit kind-compatible Connection authentication,
identity-only Applications, ENVIRONMENT-only infrastructure scope, editable
versioned Environment selections, ordinary Secret Stores without a legacy flag,
and metadata-only Variable versus store/ref-only Secret constraints. All existing
FK, unique, deferred Snapshot, CAS, ownership/fencing, generation and immutable
history invariants remain. No permanent binding immutability trigger is installed.

Current JSON snapshots reopen without defaults, Connection inference, historical
workload backfill or identity rewrites. Every aggregate map written by the current
serializer is required and non-null (empty maps are valid). Required current
binding/authentication/configuration data must be valid; retired fields/formats
fail with a safe reset-required error rather than silently disappearing. Startup
must not rewrite the rejected file or echo its data. In-memory fresh construction
still supplies current defaults through normal domain/seed commands.

Verification dùng database mới/cô lập; không tự reset retained database,
Compose volumes, Vault hoặc cluster. Không import state/dữ liệu v1.

## Environment stores, operations và retained targets

### `secret_store_connections`

Organization-owned fields: immutable ID/key/name/provider,
backend/workload addresses, KV/auth mounts, nonsecret TLS configuration,
credential_ref/status/timestamps. Unique `(organization_id, store_key)`. References
must be Organization-scoped. Credential bytes live only in platform credential store.

Environment selections/generation/active operation nằm trong bảng
`environments` ở trên; retained generation records lưu target staging riêng. Environment.version remains Settings CAS. Do not install a
permanent binding immutability trigger in the fresh baseline; retain
consistency checks and enforce dedicated CAS/owner commands. Do not clear bindings.

### `environment_operations`

Fields: ID, Environment FK, owner, fencing generation
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

### Retained target records

Fields: operation ID, target generation,
source/destination TargetRefs, old Set, resource/executor state identity, writer
replicas at source and attempt-owned destination, private backup handle/hash,
source authority/quiesce/needs-attention/cleanup state and stage results. Historical target/resource
records must not be overwritten by new generation. ActiveResource identity
and applied WorkloadInstance/deployment snapshot identity with target generation
hoặc immutable generation-owned association tương đương. Generation-0 logical
descriptors giữ identity; physical names/state mới theo instance, không run ID.
Historical snapshots không rewrite hoặc import để phục vụ v2.

Secret refs/revisions pin store identity; old applied refs are immutable. New
Variable values are metadata, never Secret values. Store identities are explicit;
T23 removes legacy platform-store backfill. Vault network reads are excluded
from SQL migrations. T24 removes legacy Variable reads/materialization; the fresh
SQL constraint baseline is defined by T28, without converting retained data.
JSON and SQL adapters expose equivalent CAS/claim/transition persistence semantics.

Local Compose bootstrap (UC-04 SS-07..08) retains the existing
`secret_store_connections` schema. Its explicit managed-store admission may
create an ordinary `platform-vault` record or atomically refresh a managed token's
opaque credential reference after verification, preserving the row ID/key and
endpoint/mount identity. Admission checks the prior compose-managed marker as
well as expected ID/credential ref; an unmarked user record cannot be refreshed.
CAS checks protect concurrent startup. No secret/token
plaintext is stored, and Environment/revision/value references remain unchanged.

## System existing-cluster Definition compatibility

[ADR-013](../decisions/ADR-013-implicit-existing-cluster.md) keeps existing
Definition FKs and tables unchanged. Execution admits a reserved system-owned
`builtin-existing-cluster` row idempotently before dependent resource/progress
writes; public registration cannot claim this key. Preview performs no writes.
Existing authored Definitions/history remain intact. Connection identity stays on
Active Resource and plan binding; no per-Connection user Definition is needed.

T23 removes Secret Store legacy domain/DTO/admission flags. T28 omits the retired
`legacy` SQL column entirely; no old-record conversion path remains.

## T27 — Infrastructure scope constraint

Environment infrastructure_scope accepts only ENVIRONMENT in the fresh baseline.
JSON startup and persistence binding/save reject retired or unknown values safely.
Generation 0 preserves current namespaces/descriptors; historical plan/TargetRef
snapshots are not rewritten. Retained development state must be reset through the
approved T29 run; old SQL migration chains are unsupported.

## T30 atomic desired rename command

Desired rename không thêm columns riêng. Một metadata transaction kiểm expected
configuration version, Environment version/draft_version, current-set identity
and no active operation; inserts new configuration revision/entries and all
changed upsert drafts together. Configuration version advances once; draft_version
advances once only when drafts changed. Pending deletes, Environment version,
current/applied pointers and old immutable refs remain unchanged. Rollback/CAS
failure leaves all metadata unchanged. See OC-12 T30 for public behavior.

<a id="terraform-v2-schema-design-adr-014-not-yet-implemented"></a>

## Terraform resource identity and state

Các bảng sau là required schema theo ADR-014. Không migration/import v1 hoặc
reset retained database. Tình trạng adapter thật thuộc CURRENT_STATE/IMP-021.

### `terraform_module_bundles`

Verified platform module registry: `id uuid PK`, `source_id text`, `module_id
text`, `revision text`, `subdirectory text`, `content_sha256 text`,
`terraform_version text`, `terraform_binary_sha256 text`, `provider_lock_digest
text`, `contract jsonb`, `provider_requirements jsonb`, `status text`, timestamps.
Unique `(source_id,module_id,revision,subdirectory,content_sha256)` và unique
pin `(source_id,module_id,revision,subdirectory)`: một revision/path chỉ có một
digest. Concurrent different-digest registration bị từ chối; cùng digest chỉ
reuse khi contract/release giống nhau. Content/contract/release immutable,
không UPDATE/DELETE verified bundle. Console có SELECT + INSERT riêng cho bảng
này, không UPDATE/DELETE; State role không cần đọc bundle trong I04-02.

CHECK source_id/module_id là identifier 1–63 ký tự lowercase chữ/số/hyphen,
revision là identifier release 1–128 ký tự chữ/số/dot/underscore/hyphen; Git
phải full commit 40/64 hex theo source policy. Subdirectory là `.` hoặc path
segments tương đối bằng chữ/số/dot/underscore/hyphen, không `.`/`..` segment.
Ba digest phải 64 lowercase hex; Terraform version exact `major.minor.patch`.
Contract/provider_requirements là JSON object; status VERIFIED trong registry
insert-only hiện hành (không mở API revoke/update). Trigger giữ bundle bất biến. Contract holds input/output/schema mappings,
provider pins and transitive bundle metadata, never credentials/cache path.
Definition `driver_inputs` references verified bundle identity plus mappings;
plan pins immutable bundle snapshot/digests. Admin source allowlist is platform
configuration, not a PE-authored trust root in this table.

### `terraform_resource_instances`

Write-ahead resource identity before provider calls: `id uuid PK`, Organization/
Application/Environment FKs, `target_generation bigint`, `descriptor text`,
`scope_type text`, `scope_id text`, `state_key text UNIQUE`, `active_resource_id
uuid FK nullable`, `module_bundle_id uuid FK`, `target_ref jsonb`, `physical_intent
jsonb`, `created_by_run_id text`, `status text`, `version bigint`, timestamps.
Unique `(organization_id,environment_id,target_generation,descriptor,scope_type,
scope_id)`; all referenced rows must share Organization. Identity immutable;
module binding advances only under admitted owner after reviewed compatible update.
No secret values in target_ref/physical_intent; stable names/tags/state refs only.

Status enum: `PENDING`, `PROVISIONING`, `READY`, `FAILED`, `UNREFERENCED`,
`INTERRUPTED`, `NEEDS_ATTENTION`, `DESTROYING`, `DESTROYED`. ActiveResource có READY/UNREFERENCED/DESTROYING/DESTROYED bên cạnh
PROVISIONING/FAILED; instance giữ interruption/needs-attention details.
Deployment vẫn dùng các trạng thái ở state machine: failed execution uses FAILED with safe reason,
Environment operation remains INTERRUPTED/held when old process outcome is unknown.
Created/unreferenced/destroyed records retained, not cascade-deleted with drafts.

### `terraform_resource_dependencies`

`consumer_instance_id uuid FK`, `provider_instance_id uuid FK`,
`organization_id FK`, `environment_id FK`, `target_generation bigint`,
`version bigint`. PK `(consumer_instance_id,provider_instance_id)`; FK/scope
checks buộc hai instance cùng ownership scope. Edges từ live/applied dependency
graph được lưu dưới Environment owner trước execution và cập nhật nguyên tử
với resource intent. Không suy dependencies chỉ từ current graph sau Remove.
Platform/existing-cluster refs nằm trong safe binding snapshot và luôn được giữ,
không biến thành managed instance được xóa. Edges/audit giữ đến absence proof;
consumer DESTROYED không còn giữ provider, failed/unknown consumer vẫn giữ.
Delete token pin edge/resource versions; graph đổi buộc review lại.

### `terraform_states`

`resource_instance_id uuid PK/FK`, `organization_id FK`, `ciphertext bytea`,
`nonce bytea`, `key_id text`, `aad_revision bigint`, `lineage text`, `serial bigint`,
`version bigint`, `updated_at timestamptz`. Empty state before first acknowledgement
has no blob row; instance already exists. AES-GCM authenticated bytes according
to state contract; key is outside PostgreSQL. State serial/lineage validated
under current lock/fence before write. Identical same-lineage/serial write replay
is acknowledged idempotently; differing bytes at same serial and older serial
are rejected, without public secret-derived checksum. Ciphertext + serial/version commit atomically;
no public state checksum based on raw secret-containing data. State and output
metadata reads require distinct service capabilities. Empty terminal Terraform
snapshot after destroy is retained for audit, not purged by generic DELETE.

### `terraform_state_locks`

One row per locked instance: `resource_instance_id uuid PK/FK`, `lock_id text`,
`environment_operation_id uuid FK`, `owner text`, `fencing_generation bigint`,
`lock_metadata jsonb`, `acquired_at`, `heartbeat_at`, `deadline timestamptz`.
Safe lock metadata cannot contain raw Terraform lock diagnostics/host paths.
Atomic claim and write/unlock compare operation current owner/fence + lock ID.
Expired deadline does not automatically delete lock or release operation hold.
Short-lived executor capabilities are stored in `terraform_executor_capabilities`:
`token_hash text PK`, `organization_id FK`, `resource_instance_id FK`,
`environment_operation_id FK`, `fencing_generation bigint`, `action text`,
`expires_at timestamptz`. No plaintext token; invalidate on owner/fence change,
never expose token read API. Every referenced scope/owner must be consistent.

### `terraform_resource_attempts`

`id uuid PK`, `resource_instance_id FK`, `environment_operation_id FK`,
`deployment_id FK nullable`, `run_id text`, `attempt_number int`, `action text`
(`CREATE`,`UPDATE`,`DESTROY`), `ownership text` (`CREATED_BY_RUN`,`PREEXISTING`,
`UPDATED_BY_RUN`), `stage text`, `status text`, `module_snapshot jsonb`,
`target_snapshot jsonb`, `observed_physical_ids jsonb`, `state_serial bigint nullable`,
`safe_error_code text`, `recovery_handle text nullable`, timestamps/deadline.
Unique `(environment_operation_id,resource_instance_id,attempt_number)`.
Write before side effects; safe stage/provenance committed after each boundary.
Unknown ownership cannot authorize cleanup. DeploymentResource projection carries
safe stage/attempt/status references, never raw resolved inputs or CLI artifacts.

Console đọc projection theo `deployment_id` của Deployment đã được kiểm tra đầy
đủ Organization/Application/Environment; instance của attempt phải thuộc chính
Environment đó. Projection gồm descriptor/scope, số lần, action/ownership,
stage/status, mã lỗi an toàn, module ID/revision và timestamps. Các mã không nằm
trong danh sách được phép phải đổi thành thông báo chung, không trả nguyên văn
dữ liệu đã lưu. Đọc lại lịch sử không gọi provider hoặc Terraform.
Quyền SELECT của Console trên instances/attempts giới hạn theo cột metadata cần
dùng; không đọc `target_ref`, `physical_intent`, `module_snapshot`,
`target_snapshot`, `observed_physical_ids`, `state_serial` hoặc `recovery_handle`.
Không cấp SELECT toàn bảng hoặc quyền ghi attempts cho Console. Kiểm bằng login
Console riêng trên PostgreSQL thật, kể cả truy vấn join và chuyển cả row sang JSON.

Destroy attempts giữ opaque `verifyHandle` trong `module_snapshot` cho một
chu kỳ destroy. Handle trỏ tới artifact state trước xóa được mã hóa riêng tư,
không chứa raw state hoặc đường dẫn trong metadata. Các lần retry dùng lại
handle; artifact phải tồn tại qua restart cho tới khi xác minh cleanup xong.
Mất hoặc không đọc được artifact sau effects thì giữ hold, không coi current
state rỗng là bằng chứng physical absence. Không cần cột SQL mới.

### Cleanup persistence

Output metadata của Type `vpc` bổ sung optional `privateSubnetIds` (schema type
`any`, giá trị module là list(string)); giữ trong output schema/JSON hiện hành,
không thêm SQL column. Provider edges của Aurora/ElastiCache vẫn trỏ tới VPC
instance; private subnets/route table thuộc state VPC, không tách instance riêng.

`environment_operations.kind = RESOURCE_CLEANUP` lưu:
`authorization_mode` (`USER_CONFIRMED` hoặc `RUN_CLEANUP`), `actor_ref`,
`root_resource_instance_id`, `confirmed_scope jsonb` (đúng selectedResourceIds đã được kiểm, bắt buộc có root;
versions, original target/module/state refs và retained reasons),
`deletion_request_hash text nullable`, `confirmation_name text` (server descriptor),
`scope_snapshot jsonb` (Environment/current/draft/resource versions), run ID
nếu là test cleanup, owner/fence và kết quả từng instance từ attempts.
Unique deletion_request_hash trong Organization/actor bảo vệ idempotent token
replay; token plaintext không persist. Hash yêu cầu bind cả selected set để
replay không đổi phạm vi. Candidate snapshot không phải quyền tự thêm mục xóa. User confirmation chỉ có quyền trên
confirmed set đã pin. Run mode chỉ CREATED_BY_RUN, không dùng user role để
nâng quyền. Atomic admission kiểm toàn bộ selected set: quyền/ownership/UNREFERENCED/consumers/versions/no
active operation rồi lưu claim + confirmed scope nguyên tử. Không cascade
FK deletion từ root làm mất dependency/audit trước verification. Operation stages REQUESTED/CHECKING/DESTROYING/VERIFYING; terminal
SUCCEEDED/FAILED/INTERRUPTED and safe NEEDS_ATTENTION metadata. SUCCEEDED requires
provider absence checks for every requested owned instance; unresolved IDs remain
queryable. No schema/table cascade may destroy audit/state before cleanup proof.

### Consistency rules

- Environment claim before state lock; foreign instance/token requests rejected.
- Lock/state write and corresponding owner/fence checks are atomic; failed writes
  cannot advance serial/version. State acknowledgement does not imply node READY:
  outputs/secrets/readiness still must validate before ActiveResource result commit.
- State key never includes run ID/Definition version; different generation or
  Environment never aliases the same instance. Shared logical scope includes env.
- Platform PostgreSQL persists restricted ciphertext alongside logical metadata;
  ordinary secret values remain in Vault. Generated database/cache password refs
  are persisted before apply and preserved across attempts.
- Fresh baseline and JSON test format changes must fail closed for unsupported
  stores; no automatic retained reset/backfill. Real v2 state backend requires
  PostgreSQL, not local JSON/memory pretending to be durable state.
- D14 backup/restore is deferred; this schema extension does not claim DR or key
  rotation. Source of locking/encryption protocol is the
  [state contract](../contracts/terraform-state.md).

## DB privilege boundary cho Terraform v2 — lựa chọn A

Ba role trên cùng database theo ADR-014/state contract:

| Role | Quyền |
|---|---|
| Owner | Khởi tạo baseline trên DB rỗng, sở hữu schema/bảng, cấp quyền; không dùng cho runtime. |
| Console runtime | CRUD/query các bảng product theo nhu cầu; không SELECT ciphertext/nonce/key data của terraform_states hoặc token_hash của terraform_executor_capabilities; không DDL hoặc chuyển sang Owner/State. |
| State runtime | Quyền cần cho state/locks/capabilities/instances/attempts và kiểm Environment owner/fence; không DDL hoặc quyền owner. |

Operation release/recovery cần đọc metadata lock và thu hồi capability, nhưng
không cấp Console quyền đọc blob/hash. Quyền cụ thể/cột được giới hạn theo SQL
thực sự cần dùng; kiểm bằng login riêng trên PostgreSQL thật. Runtime startup
chỉ validate baseline, không khởi tạo bằng tài khoản runtime hoặc fallback Owner.
Bootstrap owner là bước setup rõ ràng cho database mới. Credentials file nằm
ngoài database/repo, quyền OS hạn chế, cùng database/schema/identity đã cấu hình.
Không SECURITY DEFINER state access trong phương án này, không reset/regrant
retained database. Tables/graph/lifecycle và encrypted-state contract giữ nguyên.
