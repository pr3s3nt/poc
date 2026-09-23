---
id: DATABASE-SCHEMA
artifact: database-schema
status: current
last_reviewed: 2026-09-23
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
| `kind` | text | `AWS` or `KUBERNETES` |
| `config` | jsonb | non-secret config, NOT NULL |
| `secret_ref` | text | NOT NULL |
| `status` | text | `READY` in MVP |
| `verification` | jsonb | principal/cluster metadata, NOT NULL |
| `created_at`, `updated_at` | timestamptz | NOT NULL |

Unique: `(organization_id, connection_key)`.

### `applications`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `organization_id` | uuid | FK organizations |
| `name` | text | NOT NULL |
| `subdomain` | text | normalized DNS label, NOT NULL |
| `execution_profile` | text | `aws-eks` or `internal-k8s` |
| `connection_id` | uuid | FK connections |
| `region` | text | required for `aws-eks` |
| `runtime_status` | text | `PENDING` or `READY` |
| `version` | bigint | optimistic version |

`id` is system-generated and immutable. Unique: `(organization_id, name)` and
`subdomain` globally within the configured platform base domain.

### `environments`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `application_id` | uuid | FK applications |
| `environment_key` | text | NOT NULL |
| `name`, `environment_type` | text | NOT NULL |
| `namespace_identity` | text | NOT NULL |
| `current_deployment_set_id` | uuid | nullable FK deployment_sets, deferred |
| `version` | bigint | optimistic version |

`environment_key` is system-owned and limited to `staging` or `production`.
Unique: `(application_id, environment_key)` and `(application_id, namespace_identity)`.
The application-creation transaction inserts exactly those two rows.

### `resource_types`

Columns: `id uuid PK`, `organization_id FK`, `resource_type_key text`, `input_schema jsonb`, `output_schema jsonb`, timestamps. Unique `(organization_id, resource_type_key)`.

### `resource_definitions`

Columns: `id uuid PK`, `organization_id FK`, `definition_key text`, `resource_type_id FK`, `driver_type text`, `connection_id FK nullable`, `driver_inputs jsonb`, `provision_rules jsonb`, `source_fingerprint text nullable`, timestamps. Unique `(organization_id, definition_key)`.

### `matching_criteria`

Columns: `id uuid PK`, `resource_definition_id FK ON DELETE CASCADE`, optional `env_type`, `app_id`, `env_id`, `res_id`, `class`, and computed `specificity_score int`. Index by `resource_definition_id` and match fields.

Năm field này cùng trọng số `env_type=1`, `app_id=2`, `env_id=4`, `res_id=8`, `class=16` là contract chuẩn của UC-03 BR-06. Không có field riêng cho Execution Profile: profile gắn cố định vào Application nên `app_id` đủ để `postgres` resolve sang Aurora hoặc StatefulSet theo UC-06 MS-07.

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

Private dependency của một workload nằm trong `modules.<id>.externals.<name>`; shared dependency nằm ở `shared.<id>`.
Placeholder trong `spec` tham chiếu resource bằng `${externals.<name>[.<output>]}` và `${shared.<id>[.<output>]}`.

### `deployment_delta_snapshots`

| Column | Type | Constraint |
|---|---|---|
| `id` | uuid | PK |
| `application_id` | uuid | FK applications, NOT NULL |
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
| `delta_snapshot_id` | uuid | FK deployment_delta_snapshots, UNIQUE, NOT NULL |
| `started_at`, `finished_at` | timestamptz | lifecycle timestamps |

Index `(environment_id, started_at desc)`.

### `deployment_plans`

One-to-one with Deployment: `deployment_id uuid PK/FK`, `score_before jsonb`, `score_after jsonb`, `resource_graph jsonb`, `matched_definitions jsonb`, `provision_batches jsonb`, `classification jsonb`, `plan_hash text`, `created_at`. Snapshot được tham chiếu qua `deployments.delta_snapshot_id`. Snapshot bất biến ngay khi persist; Plan bất biến sau khi Deployment rời `PLANNING`.

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

### `workload_instances`

Columns: `id uuid PK`, `environment_id FK`, `workload_id text`, `last_deployment_id FK`, `target_ref jsonb`, `manifest_digest text`, `status text`, `observed_at timestamptz`. Unique `(environment_id, workload_id)`.

## Transaction contracts

1. Create Environment: Environment + empty Deployment Set + current pointer atomically.
2. Create deployment plan: Deployment + DeploymentDeltaSnapshot + Candidate Set + DeploymentPlan atomically; current pointer unchanged.
3. Resource completion: Active Resource upsert + Deployment Resource status atomically per node.
4. Final deployment commit: compare Environment version, update current pointer/version, mark cloud Application runtime `READY`, upsert Workload Instances and mark Deployment `SUCCEEDED` atomically.

## Secret rule

Credential values and secret resource outputs are never stored directly. `secret_ref` or structured secret references are stored; UC-09 redacts fields marked secret by contract metadata.
