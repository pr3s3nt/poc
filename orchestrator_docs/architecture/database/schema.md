---
id: DATABASE-SCHEMA
artifact: database-schema
status: current
last_reviewed: 2026-10-24
---

# Database Schema

PostgreSQL là system of record cho logical orchestration state. JSONB dùng cho immutable contract/plan documents và bounded telemetry checkpoints (ADR-023); identity, lifecycle và relations quan trọng được chuẩn hóa thành columns/tables.

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

Final V06.08 removes the unused Organization default-choice marker with the
retired Connection selector: no `default_connection_key`, `default_connection_id`
or default-Connection FK. Driver Accounts still belong to their Organization;
Definitions select them explicitly. The marker/selector/transition contract is
historical under [ADR-012](../decisions/ADR-012-environment-stores-and-transitions.md).
Until its replacement/removal gate passes, intermediate code may retain these
legacy columns; they must never supply a deployment target. No automatic
migration/conversion/reset of retained stores is introduced.

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
| `authentication_type` | text | NOT NULL; `KUBECONFIG`, `AWS_ACCESS_KEY` or `AWS_EC2_ROLE`; kind-compatible |
| `kind` | text | `AWS` or `KUBERNETES`; V01 account API projects AWS/INTERNAL |
| `version` | bigint | NOT NULL, default 1, CHECK(version >= 1); CAS guard for V01 updates |
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
| `infrastructure_scope` | text | NOT NULL, ENVIRONMENT only; reject retired/unknown values (T27) |
| `current_deployment_set_id` | uuid | nullable FK deployment_sets, deferred |
| `version` | bigint | optimistic version |
| `draft_version` | bigint | optimistic UC-16 draft version |
| `public_routes_pending` | boolean | true while route reconciliation has not completed after pending Deploy; route-only retry survives process restart |
| `desired_config_revision_id` | uuid | nullable FK configuration_revisions, deferred |
| `secret_store_id` | uuid | nullable scoped FK secret_store_connections |
| `target_generation` | bigint | immutable execution generation, default 0 |
| `generation_high` | bigint | NOT NULL DEFAULT 0; retained generation metadata read by the private State owner/scope check; unrelated to retired deployment profiles |
| `active_operation_id` | uuid | nullable FK environment_operations; owner/fence admission |

`environment_key` is system-owned and limited to `staging` or `production`.
Unique: `(application_id, environment_key)` and `(application_id, namespace_identity)`.
The application-creation transaction inserts exactly those two rows, unset.

The final ADR-017 baseline has no Environment deployment Connection/profile/region/
runtime selector. Secret Store/configuration selection uses version CAS and
operation-owner checks. Generation0 retains stable resource identities; original
per-resource/workload targets stay pinned. No Application/Environment target
conversion or default fallback. The ADR-012 deployment-transition binding design
is historical after V06.08; its surviving store/CAS/owner protections still apply.

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

Columns: `id uuid PK`, `organization_id FK`, `definition_key text`, `definition_name text NOT NULL`, `resource_type_id FK`, `driver_type text`, `connection_id FK nullable` (Driver Account backing identity), `driver_inputs jsonb`, `provision_rules jsonb`, `source_fingerprint text nullable`, `version bigint NOT NULL DEFAULT 1 CHECK(version >= 1)`, timestamps. Unique `(organization_id, definition_key)`. Name is nonblank, <=253 characters, no control characters. ID/type immutable for public edits; version CAS atomically replaces editable content and criteria. No Definition execution_profile column or profile matching predicate in V02.

### `matching_criteria`

Columns: `id uuid PK`, `resource_definition_id FK ON DELETE CASCADE`, optional `env_type`, `app_id`, `env_id`, `res_id`, `class`, and computed `specificity_score int`. Index by `resource_definition_id` and match fields.

Năm field này cùng trọng số `env_type=1`, `app_id=2`, `env_id=4`, `res_id=8`, `class=16` là contract chuẩn của UC-03 BR-06. Không có profile/account/region filter ngoài năm criteria. Fresh seed phân biệt nhu cầu bằng class/context tường minh; account không có phạm vi áp dụng.

### `deployment_sets`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `environment_id` | uuid | FK environments |
| `created_by_deployment_id` | uuid | nullable FK deployments, deferred |
| `document` | jsonb | canonical Deployment Set |
| `document_hash` | text | deterministic fingerprint |
| `created_at` | timestamptz | NOT NULL |

Deployment Sets are immutable. `document` also carries an optional private
`literalProofs` member beside `modules` and `shared` (DEVEX-ENV, UC-16 BR-23):
`{ "<workload>": { "<container>": { "<variable>": "<sha256 of workload, container,
name and exact value>" } } }`. It records which ordinary string variables a
successful UC-16 deploy accepted. It is not part of `document_hash`, the planner
Delta or any Score; no DDL changed. A set without the member (every set written
before this feature) proves nothing, so its literals stay unreadable.

An optional private `resourceAliases` member preserves UC-16 BR-17 shared
resource names per workload: `{ "<workload>": { "shared": { "<alias>":
"<shared-id>" }, "references": { "<JSON pointer>": "<Score reference text>" }
} }`. References retain which alias a binding used when multiple aliases name
one shared ID. Store only text containing an unescaped shared-resource reference;
never use this metadata to authorize literal readback. A restored reference must
resolve to exactly the current reconstructed reference. Metadata is captured
from validated Score on deploy, carried only for remaining workloads, and lives
outside Document, DocumentHash and planner/Delta semantics. JSON snapshots and
PostgreSQL's existing `document` JSONB preserve it; no DDL or reset is needed.
Absent metadata keeps legacy generated aliases; do not infer historical names.

Canonical shape:

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

`target_ref.objects` (ADR-024) is the applied object inventory of a Git-rendered
workload: an array of `{apiVersion, kind, name, owners[]}` without namespace
(always the target namespace) or object content. Absent means no inventory
(native/legacy renders); no column or migration is added.

Per-workload applied revision allows an honest partial-success state when a
multi-workload configuration rollout fails. The desired Environment pointer
does not imply every workload has consumed that revision.

### `effectiveness_baselines`

UC-17 baseline versions. Append-only: Console login has only `SELECT` and
`INSERT`; no runtime login updates or deletes a version.

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `organization_id` | uuid | FK organizations, NOT NULL |
| `application_id` | uuid | nullable FK applications; NULL = Toàn tổ chức; Application must belong to the same Organization (checked by the command) |
| `metric_id` | text | NOT NULL, `work_duration` only in đợt 1 |
| `version` | bigint | NOT NULL, CHECK(version >= 1) |
| `retired` | boolean | NOT NULL DEFAULT false; true = ngừng áp dụng từ `effective_from` |
| `statistic` | text | `MEDIAN` when not retired, NULL when retired |
| `value_seconds` | bigint | CHECK > 0 when not retired, NULL when retired |
| `source` | text | NOT NULL, 1..2000 characters after trim |
| `sample_size` | integer | nullable, CHECK > 0 |
| `measured_from`, `measured_to` | date | nullable; both or neither; from ≤ to |
| `effective_from` | date | NOT NULL; report-timezone calendar date |
| `created_by` | uuid | FK user_accounts, NOT NULL |
| `created_at` | timestamptz | NOT NULL DEFAULT now() |

Unique index `(organization_id, metric_id, COALESCE(application_id,
'00000000-0000-0000-0000-000000000000'), version)`. A new version is inserted
as `expectedVersion + 1`; a unique violation is `ErrVersionConflict` and writes
nothing. A report uses, among the scope's versions with `effective_from ≤
period.to`, the one with the latest `effective_from` (ties: highest version);
if it is retired, no baseline applies. Older periods keep the version that was
effective for them. The JSON development snapshot stores the same records under
`effectivenessBaselines`.

UC-17 adds no event table: Deployment and `RESOURCE_CLEANUP` history are the
event source. The baseline table changes the T28 baseline digest, so a database
or JSON snapshot created before it requires the documented development reset;
no automatic migration is provided.

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

Environment Secret Store selection, identity generation and active operation
remain in `environments`; Environment.version remains Settings CAS. ADR-017
retires deployment-Connection selection/transition storage, preserving original
resource/workload targets and immutable Secret Store references. No startup
conversion, clearing retained data or implicit target reconstruction.

### `environment_operations`

Fields: ID, Environment FK, owner, fencing generation
(bigint starting at 1, incremented on recovery), kind, status/stage,
nonsecret `pins`/`detail` JSON with Environment/draft/config versions/current Set
and resource identities, start/update/heartbeat/deadline and safe failure metadata.
Deployment-transition source/destination records are retired after V06.08.
Unique active owner per Environment, enforced by row claim or partial unique
index. Claim/release and version guards run inside one transaction. A restart
preserves INTERRUPTED recovery state; no automatic expiry bypass of live owner.
Ownership must change on recovery (owner identity or a monotonic fencing token).
Heartbeat, release and every surviving owner-only metadata write,
check the current ownership atomically; a reused operation ID is insufficient.
Persist safe confirmation/audit metadata that prior execution was stopped before
recovery takes ownership. Enforce recorded deadlines on executor contexts.

### Historical transition records and current target provenance

The source/destination migration fields below describe the retired ADR-012
transition design, not a new V06 table/API. After V06.08 current provenance lives
in immutable plans, per-resource instances and workload/deployment `target_ref`.
Historical evidence is kept with its original label; incompatible old stores fail
explicitly instead of being imported or reset.


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
JSON and SQL adapters expose equivalent supported CAS/claim/target-pin persistence semantics.

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
plan pins immutable bundle snapshot/digests. Issue #7 adds optional contract
`sourceUrl` containing a credential-free HTTPS origin for directly inspected Git
modules. PE/Admin may prepare these without a source allowlist (ADR-014 amendment);
legacy allowlist remains platform configuration. No column/constraint migration.

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


## V01 infrastructure-account persistence

[ADR-015](../decisions/ADR-015-infrastructure-driver-accounts.md) keeps the existing
`connections` identity/FKs as backing account storage. It is not a second account
table. Public `type` = AWS/INTERNAL projects the persisted kind; authentication
remains explicit. Config keeps verified account identity or selected Internal
cluster descriptor; raw credentials are excluded. Account API has no execution
profile, applicability or Environment assignment columns.

`version` starts at 1 on newly created/seeded records. Name edit, successful
reverification and credential replacement use `WHERE organization_id = ... AND
connection_key = ... AND version = expected` and atomically increment it. CAS
failure does not alter winner metadata/ref; transaction does not call providers
or Vault. JSON state must preserve the same version/CAS semantics; old snapshots
missing it are incompatible, never silently filled during reopen.

Credential update preserves stable ID/key/kind/authentication and target identity;
new ref/config/verification/version publish together. Attempt-only rollback cannot
delete a committed credential. Prior successful objects remain in Vault because
persisted execution targets may still reference them. No account GC in V01.
AWS verification regions are evidence; new public account schema contains no
allowed deployment-region scope. Legacy `config.allowedRegions` remains for the
temporary legacy execution consumers only, and must not be advertised as the
new account applicability policy.

Fresh baseline/digest/catalog checks include the version column/constraint/grants.
Existing baseline or JSON incompatibility fails startup without DDL/reset or
credential access. User decision 2026-10-17 removes V02–V03 old-data migration
requirements; it does not authorize destructive reset of retained state/resources.


## Quyền reset dữ liệu phát triển — bổ sung 2026-10-17

Người dùng yêu cầu xóa dữ liệu sản phẩm cũ vì chưa có người dùng thực tế.
V01 được phép reset database/state phát triển cũ của dự án sau khi xác định rõ
instance/namespace/ownership và có baseline mới để khởi tạo. Không cần migration
hay giữ tương thích dữ liệu cũ. Đây là quyền reset tường minh của task, không phải
cơ chế tự reset khi startup hoặc quyền xóa tài nguyên không thuộc dữ liệu sản phẩm.
Codex thực hiện và ghi evidence phạm vi/kết quả; writer không truy cập retained
state hoặc tự chạy thao tác hạ tầng. Các yêu cầu secret scope, CAS, pinned targets
trên baseline mới và bảo vệ tài nguyên ngoài task vẫn áp dụng.

## V02 Resource Definition baseline — authoritative replacement of Definition profile model

UC-03 V02-01..09 owns behavior. `resource_definitions` adds `definition_name text NOT NULL` (public `name`)
(nonblank, bounded253, no control characters), `version bigint NOT NULL DEFAULT 1
CHECK(version >= 1)`. Unique org/key and Type FK remain; key/type immutable on
update. Existing `connection_id` nullable FK backs public `driverAccount`, with
org/kind/READY validation in service. Remove `execution_profile` from fresh
Definition table/model; Environment profile remains for legacy graph enrichment
until V03. No new account table, criteria account field or region-wide constraint.
Driver Account key is serialized as `driverAccount`; AWS region is stored in
`driver_inputs.values.variables.region`. No legacy authoring field conversion.

One transaction CASes version and replaces Definition, matching_criteria and
verified bundle association; failed criterion/bundle write rolls back everything.
Version increments exactly once. CAS loser cannot overwrite winner. No secret
bytes/ref in Definition. Existing plan/resource targets, module fingerprints,
state IDs/ownership/fencing and cleanup records are immutable across edits.

Fresh SQL baseline/digests/catalog validation and JSON format change together.
JSON requires positive explicit version and nonblank Name, no inference from ID
or retired profile/Connection field during reopen. Old state fails safely with
reset-required diagnostic; startup never migrates/resets. Current constructors
and seeds supply explicit Name/version/account and unambiguous five-field
criteria. No retained data reset without instance/ownership verification.


## V03 per-resource placement baseline

[ADR-016](../decisions/ADR-016-definition-selected-workload-targets.md) owns placement behavior. Environment deployment connection/profile is not authoritative for internal execution; no single Application/Environment target column substitutes for per-resource target data. Environment Secret Store, version, current Set, namespace base identity and operation owner/fence remain. Retire internal deployment-binding persistence/compatibility on a new baseline after V03.10; AWS legacy fields, if still needed until V05–V06, cannot activate internal fallback.

Persist optional workload class (Score metadata.class; missing means default) in module contribution, drafts and immutable plan. Workload plan selections include namespace/cluster descriptor, safe target/account identity/version and Definition/bundle provenance. ActiveResource, Terraform state provider pins and WorkloadInstance/deployment_workloads target_ref preserve original target plus namespace; reuse checks compare scoped cluster identity even when namespace names match. Target type/identity must survive PostgreSQL/JSON roundtrip. No raw credentials, kubeconfig/temp paths or public secret refs. Existing Definition/version/account FK and CAS semantics remain.

Existing cluster gets a normal user Definition FK and PREEXISTING ownership, no reserved builtin catalog admission. Its logical descriptor is explicit reference identity, not inferred from Environment connection key. Namespace ownership/UID and cluster-qualified execution pins guard cleanup. Fresh SQL/JSON format/digest validation changes together if serialized schema changes; old data fails explicitly, no migration/backfill/reset at startup. Codex owns any separately authorized verified dev reset.


V03 plan JSON additionally persists afterWorkloads: resource descriptor ->
transitive workload provider descriptors, hashed with the complete full-graph
DAG. UC-08 executes resource-only subsets; UC-06 coordinates workload positions
and persists lifecycle progress before a final success commit. This field never
permits ignoring a required dependency merely because it is a workload.

## V04–V06 placement and profile retirement baseline

[ADR-017](../decisions/ADR-017-definition-dependency-execution.md) extends the
V03 per-resource placement contract to Terraform-created EKS. Plan JSON pins
producing descriptor/Definition/version/account/version/region while values are
unknown. After provision, ResourceInstance/ActiveResource and applied workload
targets preserve original opaque provider=eks/cluster/region/Organization/account
identity. Persist no auth token, endpoint credentials, temporary kubeconfig/path
or secret bytes. Outputs/status remain typed; secrets remain opaque refs.

No new Environment-wide target/account/region constraint is introduced. Stable
logical descriptor/state identity survives restart/redeploy. Shared consumers
reuse the same resource instance; cleanup dependency records preserve the actual
reference graph and original pins. Catalog edits never rewrite historical targets.

V06.08 removes execution-profile and retired deployment-Connection binding fields
from fresh SQL/JSON baseline after replacement coverage; Secret Store bindings,
Environment version/current Set/operation owner/fence and per-resource target
records remain. Baseline/schema digest/JSON format and validation change together
when serialized shape changes; old incompatible state fails clearly without
migration, startup reset or implicit inference. Exact executable baseline/ERD
changes must be reviewed on final source before acceptance/commit.

## EC2 authentication metadata (ADR-018)

AWS_EC2_ROLE is valid only for kind AWS. Its secret_ref is the empty string;
config holds roleArn, roleId, accountId, principalArn, region and verificationRegions
(plus legacy allowedRegions metadata). No credential values or expiration are
persisted. Existing name/version and identity immutability apply. The fresh
baseline constraint accepts this enum; no startup conversion/reset is provided.
The Connection ERD relationship is unchanged (no new entity/column).

## Visual review persistence — ADR-019 (required design)

The following additions are required for UC-05 VIS-* / UC-09 OBS-* and are not
an assertion that the current baseline already implements them:

- `environment_operations.review_request_id uuid NULL`: optional explicit Deploy
  request correlation, bound atomically to the admitted operation; unique partial
  index `(environment_id, review_request_id) WHERE review_request_id IS NOT NULL`.
- `deployments.review_request_id uuid NULL`: copied only from its admitted
  operation, immutable once assigned. Index
  `(environment_id, review_request_id, started_at)` for scoped progress correlation.
  No association inferred for historical/standalone runs; no credential semantics.
- `deployment_plans` is physically stored as existing `document jsonb`; the
  logical graph/matches/batches fields above are members of that document.
  Add optional `document.review` version 1, containing sanitized desired node
  artifacts, availability and provenance from the admitted plan. Persist before
  leaving PLANNING; existing plan immutability applies. Missing means unavailable.
- `deployment_workloads.manifest_review jsonb NULL`: bounded sanitized rendered
  manifest projection with version/provenance/redaction metadata. Capture after
  render validation and before apply in the same scoped workload progress write.
  It is rendered intent until this workload's status proves READY; terminal
  snapshots are immutable. No secrets/state/raw execution inputs in this column.

Domain/repository values, JSON store and PostgreSQL mappings must preserve these
fields on reopen and status updates. No additional lifecycle enum/table or raw
artifact blob store is introduced. Workload snapshot uses its existing applied
association for the baseline; failed deployment is not a successful baseline.
Fresh baseline SQL/digest, serializer validation, grants as necessary and parity
contracts change together. Do not auto-migrate/reset existing user databases or
rewrite historical rows; unsupported baseline follows existing explicit setup
rules. Additive absent artifacts on otherwise compatible rows remain unavailable.

## Git render-bundle persistence — ADR-021

Implemented 2026-10-24 in the fresh baseline (`render_bundles`):

| Column | Contract |
|---|---|
| `id` | UUID primary key, server-issued |
| `organization_id` | UUID NOT NULL FK organizations; every read/write scoped |
| `source_url` | text NOT NULL, credential-free canonical HTTPS origin |
| `source_path` | text NOT NULL, normalized clean directory or `.` |
| `source_revision` | text NOT NULL, full 40/64 lowercase hex commit |
| `content_sha256` | text NOT NULL, 64 lowercase hex digest |
| `engine_contract` | text NOT NULL, `git-render-v1` |
| `renderer_version` | text NOT NULL, `0.15.0` |
| `renderer_binary_sha256` | text NOT NULL, 64 lowercase hex digest |
| `contract` | jsonb NOT NULL object: validated manifest, input schema/defaults and input-contract version; no raw credentials/file paths |
| `created_at` | timestamptz NOT NULL |

Unique `(organization_id,source_url,source_path,source_revision,engine_contract)`;
conflicting digest/contract/binary is rejected, identical registration reuses
record. Published metadata is immutable; Console role SELECT/INSERT only and
immutability trigger like terraform_module_bundles; State role needs no grants.
Availability is computed from metadata/cache/runtime, not a mutable DB status.

Definition `driver_inputs.values.variables.render_bundle` references either a
legacy installed ID or same-org verified UUID; optional `render_inputs` holds
supplied non-secret JSON. Existing Create/CAS transaction verifies association
and scope, updates Definition/criteria/version atomically; no new nullable FK on
Definitions is needed (legacy and Git use the same JSON field). Repositories must
enforce existence/scope transactionally. Metadata is insert-only with no delete
endpoint, so validated pin cannot disappear during that write.

Immutable plan rendering selection holds source pin/engine/binary and effective
inputs/hash; old selections deserialize unchanged. Workload manifest review stays
in existing deployment_workloads JSON; no raw Secret data or arbitrary template
output. Memory/JSON repositories mirror scoped immutable semantics. JSON snapshots
add optional empty registry for old complete snapshots; preserve old plans.

Content files live in the durable verified cache, not SQL. No GC in v1. Include
that cache in backup/restore instructions; metadata alone does not restore content.
Fresh baseline DDL, canonical validation fingerprint, role grants and schema tests
must add this table. Existing retained DBs are not auto-migrated/reset: use a new
isolated database for acceptance under T28. Keep Terraform state/owner/roles and
all resource identity constraints unchanged. [ERD](erd.puml) shows the relation.

## Native observability — ADR-023

Required by [ADR-023](../decisions/ADR-023-native-observability.md).
Telemetry identity and time-series fields are normalized; only bounded stream
checkpoints (pod UID/container/restart/timestamp) use mutable JSONB. No credential
or kubeconfig is stored in these tables. Console has CRUD; State has no grant.
Owner setup adds them only to a fresh baseline; runtime performs no DDL/reset.

| Table | Required columns and constraints |
|---|---|
| `telemetry_sources` | `source_key text PK` (64 hex), `environment_id uuid FK environments`, `target_generation bigint >=0`, `workload_id text`, `target_hash text` (64 hex), UNIQUE(environment_id,generation,workload_id,target_hash); `checkpoints jsonb object`; `lease_owner text`, `fence bigint >=0`, `lease_until timestamptz`; `logs_code text`, `metrics_code text`; `observed_at`, `last_log_at`, `last_metric_at` nullable timestamptz; `created_at timestamptz`. Scope derives through Environment → Application → Organization. |
| `telemetry_logs` | `id text PK` (64 hex), `source_key FK telemetry_sources`, `timestamp timestamptz`, `pod_uid`, `pod_name`, `container_name text`, `restart_index bigint >=0`, `level text` (ERROR/WARN/INFO/DEBUG/UNKNOWN), `message text` <=16KiB. Index(source_key,timestamp DESC,id DESC), time/level query indexes. IDs distinguish repeated equal lines within a timestamp. |
| `telemetry_samples` | `id text PK` (64 hex), `source_key FK`, `observed_at timestamptz`, `metrics_at timestamptz nullable`; `cpu_milli`, `memory_bytes`, `cpu_limit_milli`, `memory_limit_bytes double precision nullable/nonnegative`; `ready_pods`, `total_pods`, `restarts bigint nonnegative`; index(source_key,observed_at DESC). Null usage or limit is not zero. |
| `telemetry_events` | `id text PK` (64 hex), `source_key FK`, `timestamp timestamptz`, `pod_uid`, `pod_name`, `container_name text`, `kind text`, `reason text` (safe code), `occurrences bigint >0`; index(source_key,timestamp DESC,id DESC). Deterministic observed events survive pod removal. |

Leases/fences and batches are atomic per source. Retention: logs seven days;
samples/events fifteen days. Checkpoint advancement follows the corresponding
successful insert batch; fencing and deterministic IDs tolerate retry/unknown
commit. Prune only expired telemetry, not provider resources. Per-source quotas
in ADR-023 reject excess new observations with a visible storage-limit code;
no automatic early deletion or change to Terraform tables.

## Account diagnostics — ADR-022

`account_diagnostic_reports` is a separate Organization/account aggregate for
[UC-04 DIAG-01..14](../../usecase/UC-04/specification.md#account-diagnostics--diag-0114).
It never stores history inside Connection verification or increments account
configuration version. The protocol owns lifecycle/finding semantics in
[the diagnostic contract](../contracts/account-diagnostics.md).

| Column | Type | Constraint/meaning |
|---|---|---|
| `id` | uuid | PK; backend-generated |
| `organization_id` | uuid | FK organizations, NOT NULL |
| `account_id` | uuid | FK connections, NOT NULL; account belongs to organization |
| `created_by` | uuid | FK user_accounts, NOT NULL; actor belongs to organization |
| `account_version` | bigint | NOT NULL, positive; immutable config version pin |
| `purpose` | text | GENERAL or USE_CASE, NOT NULL |
| `mode` | text | CONDITIONS or EXECUTION; GENERAL requires CONDITIONS |
| `scope` | jsonb | Immutable validated scope snapshot: safe account identity, region, use cases and optional Application/Environment versions |
| `status` | text | QUEUED, RUNNING, CANCELLING, COMPLETED, CANCELLED or INTERRUPTED |
| `revision` | bigint | Positive CAS version for worker/cancel updates |
| `owner` | text | Nullable current process/worker token, no secrets |
| `lease_expires_at`, `heartbeat_at` | timestamptz | Nullable bounded ownership facts |
| `findings` | jsonb | Validated bounded safe finding documents; terminal findings immutable |
| `completed_checks`, `total_checks` | integer | Nonnegative; completed <= total; no synthetic success |
| `probe_resources` | jsonb | Safe run-owned names/UIDs, target and ownership records for cleanup; no credentials |
| `cleanup_status` | text | NOT_REQUIRED, PENDING, VERIFIED or UNKNOWN; native absence required for VERIFIED |
| `created_at`, `updated_at` | timestamptz | NOT NULL |
| `started_at`, `finished_at` | timestamptz | Nullable lifecycle timestamps |

Indexes: `(organization_id, account_id, created_at DESC, id)` for scoped paged
history and a unique partial index on active account rows in QUEUED/RUNNING/
CANCELLING. Cross-organization account/actor insert is rejected by the repository
transaction and corresponding constraints/joins; every read/update scopes org,
account and report ID. Worker writes also guard owner/revision/lease. General
scope excludes app/environment/use-case fields; safe JSON documents are bounded.

Console runtime gets SELECT/INSERT/UPDATE only, with application-layer terminal
immutability and CAS. State runtime gets no access. Baseline owner setup/grant
validation and memory/JSON snapshot counterparts must include the aggregate.
Fresh-baseline policy stays unchanged: no runtime DDL/automatic migration/reset;
new acceptance databases use the updated baseline. Retained databases are not
reset or converted by this task.

## Workload execution extension — ADR-025

`workload_drafts.score_document.workloadExecution` và
`deployment_sets.document.modules.<id>.spec.workloadExecution` chứa typed v1
execution policy/template pin. Extension là phần của normalized desired intent
và document hash, không private metadata; omission giữ workload legacy.
Chỉ lưu refs không secret bytes. Existing JSONB columns không đổi DDL/ERD ở
checkpoint domain này; template publication tables sẽ được định nghĩa trước
checkpoint catalog persistence. Không reset database để nhận JSONB member mới.
