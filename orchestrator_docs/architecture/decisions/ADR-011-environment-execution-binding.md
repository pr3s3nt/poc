---
id: ADR-011
artifact: architecture-decision
status: current
last_reviewed: 2026-10-07
---

# ADR-011 — Environment execution binding set once

Status: Superseded in part — permanent set-once selection/admission/UI rules are
replaced by [ADR-012](ADR-012-environment-stores-and-transitions.md). This record
preserves historical design and generation-0 scope/naming/legacy decisions.
Supersedes the shared Application target and new AWS scopes in ADR-001;
ADR-001 legacy identity remains supported as documented below.

## Decision

Application identity/provider remain Application-owned. Execution Connection,
Execution Profile, region and runtime status belong to Environment. New
Applications create staging/production UNCONFIGURED without any Connection;
Developer selects in each Environment Settings and sets exactly once. Both
profiles may coexist in one Application. A configured binding is immutable even
before deployment; duplicate same-key set is a conflict. Organization default
is only a choices marker, never an execution fallback.

All product paths resolve one Environment target: snapshot, pending/standalone
Preview, matching guards, enrichment, provisioning, rendering, workload apply,
routes, Jobs and queries. ConnectionMismatch compares selected Environment
connection for internal Kubernetes and AWS VPC/EKS. External database Definitions
keep independent Driver Account semantics. Context `env.profile`/`env.region`
are canonical; legacy Definition `${context.app.profile}` and
`${context.app.region}` aliases resolve the selected Environment values to keep
existing Definition contracts functional. Never persist projected target back
onto Application.

Environment binding is set in one local transaction using unset check + expected
version + scoped Connection validation. Increment version; pin nonsecret
connection key/profile/region/scope mode in snapshot/plan hash and preview token.
Persistence save methods cannot overwrite a configured binding. UNCONFIGURED
cannot preview/deploy and must fail before executor/provisioning side effects.

## New AWS resource identity

New AWS Environments use scope ENVIRONMENT with scope ID `<app>.<env>`:
- `vpc.default#environments.<app>.<env>`
- `k8s-cluster.eks#environments.<app>.<env>`
Namespace retains `k8s-namespace.default#environments.<app>.<env>`.
Terraform workspace/state references derive from these distinct descriptors;
Environment also selects region and Driver Account. Two environments never
reuse each other's new VPC/EKS even when selecting the same Connection.
Definition references use new `@infra` token for AWS infrastructure path:
`applications.<app>` in LEGACY_APPLICATION, `environments.<app>.<env>` otherwise.
`vpc.default#@infra` works for EKS and Aurora consumers; `#@` alone is insufficient
for Aurora because its private dependency path is different. Seeded AWS
Definitions use @infra; seeded catalog records are refreshed idempotently without
overwriting Platform-authored Definitions. Explicit hardcoded applications VPC/EKS
references in a new-scope Environment fail planning safely with guidance to @infra;
legacy scope keeps old references valid. Do not silently remap arbitrary references.

Cloud name templates use `${context.infra.resourceName}`. For LEGACY_APPLICATION
this is exactly `<app>-<run>` to preserve old physical names. For new scope it is
`orch-<app-prefix>-<env-prefix>-<digest>`: ASCII lowercase alphanumeric prefix of
Application key up to 8 chars (fallback app), Environment key up to 10 (fallback
env), and first 24 lowercase hex chars of SHA-256 over full Application key,
Environment key and run ID joined by NUL delimiters. Length is at most 49 and
starts with a letter, with no consecutive/trailing hyphens. Changing app/env/run
changes the digest, preserving collision isolation when prefixes match. This
reserves space for embedded module suffixes such as -writer and -cluster.
`context.infra.name` remains the readable app/app-env stem, not a provider-safe
physical identifier. New seed Definitions must use resourceName; scope-aware
references still use @infra. Legacy exact template migration handles both old
`${context.app.id}-${context.run.id}` and interim
`${context.infra.name}-${context.run.id}`, without rewriting authored inputs.
Limits verified against [IAM role name quota](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html)
and [Aurora cluster identifier constraints](https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_CreateDBCluster.html).
This changes new-scope naming only; stable run-independent names remain D01.
State/workspace names derive from descriptors. New AWS VPC/EKS are Environment-owned
and follow existing UNREFERENCED classification when absent from desired graph;
classification does not destroy infrastructure. Binding cannot change, and regular
AWS enrichment includes both nodes, so normal deploy keeps them referenced.
Internal existing-cluster descriptor remains `connections.<key>`; namespace and
workloads remain Environment-scoped. No cluster is provisioned by connection set.

## Legacy migration

SQL additive Environment columns store nullable connection_id, execution_profile,
region, runtime_status and infrastructure_scope. New Environment scope defaults
ENVIRONMENT. Existing Application columns are retained as legacy data, nullable
connection; new Applications keep legacy profile/connection/region empty and
runtime UNCONFIGURED. On schema migration, copy old Application binding onto
existing unset Environments and mark infrastructure_scope LEGACY_APPLICATION.
Existing normalized tables/JSON loader must distinguish newly created unbound
Applications from old bound Applications, apply migration idempotently and never
consult Organization default. Migration does not rewrite old plan/resource
identity, TargetRef, executor state, namespace, Vault revision or credentials.

Legacy AWS Environments keep application-scope VPC/EKS descriptors and scope,
shared only for those migrated Environments. New Applications use ENVIRONMENT.
This exception preserves physical resources, prevents state collision and is
explicitly exposed/documented; no automatic cloud migration or resource copying.
Legacy seeded conformance contexts without target fields retain historical
semantics only via compatibility mode, not as new product default.

## Consequences and validation

No app-level dropdown or implicit target. Settings shows UNCONFIGURED, editable
selection before set, saving/errors, then read-only target. Application home and
Preview show currently selected Environment binding and switch safely on tabs.
UC-12 config/draft operations work while UNCONFIGURED. Safe choices contain no
credentials or verification data. Same organization scoping and role rules remain.

Validate SQL pre-change migration/reopen/idempotence and atomic race, JSON legacy
restart, independent kinds/regions, AWS distinct descriptors/workspaces, legacy
AWS reuse, matching conflict and immutable save guards. Live proof uses existing
kind-idp-internal through real Kubernetes adapter, human UI recording; AWS
behavior is tested without cloud mutation. No AWS cloud run is authorized.

## Version/conflict and persisted consistency

Set checks existing configured target first and returns a safe already-configured
409 message (including same key); unset stale expectedVersion returns distinct
stale-version 409. UI reloads authoritative Environment. Draft/config edits keep
their own versions; if any unrelated Environment version changes, stale retry
reloads it without bypassing the unset CAS. No binding_version column is needed.
SQL adds consistency check: NULL connection requires UNCONFIGURED and empty
profile/region; configured connection requires supported profile, AWS region and
runtime PENDING/READY. Binding write port and save guards are mandatory; DB
immutability trigger rejects changing/clearing configured connection/profile/region/
scope while permitting runtime status and operational versions to advance.
Migration runs before trigger installation/backfill, and repeated runs never reset
configured Environment values. Conformance compatibility is explicit test/legacy
handling, never product fallback to default or Application.
