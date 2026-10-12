---
id: ADR-011
artifact: architecture-decision
status: current
last_reviewed: 2026-10-13
---

# ADR-011 — Environment execution binding set once

Status: Superseded in part — permanent set-once selection/admission/UI rules are
replaced by [ADR-012](ADR-012-environment-stores-and-transitions.md). This record
preserves historical design and current generation-0 scope/naming decisions.
T27 supersedes product legacy scope/name and seed-template upgrade support.
The internal cluster user-Definition matching requirement is replaced by
[ADR-013](ADR-013-implicit-existing-cluster.md); other target guards remain.
Supersedes the shared Application target and new AWS scopes in ADR-001;
T27 supersedes ADR-001 migrated Application-scope infrastructure compatibility.

T28 (LEG-08) supersedes all historical startup migrations/backfills described
in this record. PostgreSQL initializes an empty database with the current baseline
and rejects incompatible existing schemas before serving. JSON reopens only the
current complete format, without inferring bindings or reconstructing history.
Generation, immutable target/history, CAS and operation fencing remain unchanged.
See [schema](../database/schema.md#fresh-persistence-baseline-t28--leg-08).
Retained dev data requires the separately approved T29 reset.

## V2 supersession — ADR-014

[ADR-014](ADR-014-terraform-v2-lifecycle.md) replaces, for v2, embedded-only/
Kubernetes application provisioning, run-derived temporary state/names, explicit
Aurora account override and optional/internal-only/native-default rendering.
V2 uses registered Terraform modules, Environment account/region, encrypted
PostgreSQL state and required score-k8s on both profiles. Historical naming/
verification below remains provenance of the implemented baseline; stable v2
resource identity has no run ID. Other descriptor/matching/generation/credential
constraints remain unless explicitly superseded. V1 import is not required.

## Decision

T26 (LEG-01) supersedes the retained Application target/provider columns and
Application-to-Environment migration below. Application owns only identity and
current metadata (ID/key, Organization, name, subdomain and version). It has no
profile, connection, region, runtime status or configuration provider. No old
Application binding is copied or consulted. Environment owns all execution/store
selections; immutable historical targets, generation/version/fencing and
Organization scoping are preserved. Development state requires reset before use
(T29); T28 supplies the final fresh baseline. T27 (LEG-02) removes
LEGACY_APPLICATION from product scopes and all seed-template
upgrade heuristics. Only ENVIRONMENT is valid; retired/unknown scope values fail
closed without conversion. Historical fixture semantics belong solely to the
conformance harness. Generation-0 current Environment identities and later
generation isolation remain unchanged; the trusted internal cluster retains its
connection descriptor and Application resource scope under ADR-013.

Application identity remains Application-owned. Execution Connection,
Execution Profile, region and runtime status belong to Environment. New
Applications create staging/production UNCONFIGURED without any Connection;
Developer selects in each Environment Settings and sets exactly once. Both
profiles may coexist in one Application. A configured binding is immutable even
before deployment; duplicate same-key set is a conflict. Organization default
is only a choices marker, never an execution fallback.

All product paths resolve one Environment target: snapshot, pending/standalone
Preview, matching guards, enrichment, provisioning, rendering, workload apply,
routes, Jobs and queries. ConnectionMismatch compares selected Environment
connection for internal Kubernetes and AWS VPC/EKS. Public Terraform VPC/EKS registration now requires an empty ConnectionKey (UC-03 BR-08, RD-02); execution always inherits the pinned Environment AWS account. The mismatch guard remains defensive for malformed internal/harness catalogs, without changing ranking or ties. Registration needs no Environment and rejects all explicit VPC/EKS keys, including an equal target key. External database Definitions
keep independent Driver Account semantics. Context `env.profile`/`env.region`
are the only product profile/region context keys. T21 (LEG-04) removes the
former `${context.app.profile}` and `${context.app.region}` aliases: resolution
returns unknown context, without rewrite or fallback for old Definitions.
Application identity keys remain supported. Never persist projected target back
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
`environments.<app>.<env>` at generation 0 and the ADR-012 generation path otherwise.
`vpc.default#@infra` works for EKS and Aurora consumers; `#@` alone is insufficient
for Aurora because its private dependency path is different. Seeded AWS
Definitions use @infra; seed inserts missing catalog records and never upgrades
or overwrites any existing Definition, including exact old seeded templates.
Explicit hardcoded applications VPC/EKS
references in an Environment fail planning safely with guidance to @infra;
no product scope accepts those old infrastructure references. Do not silently
remap arbitrary references.

Cloud name templates use `${context.infra.resourceName}`. For ENVIRONMENT it is
`orch-<app-prefix>-<env-prefix>-<digest>`: ASCII lowercase alphanumeric prefix of
Application key up to 8 chars (fallback app), Environment key up to 10 (fallback
env), and first 24 lowercase hex chars of SHA-256 over full Application key,
Environment key and run ID joined by NUL delimiters. Length is at most 49 and
starts with a letter, with no consecutive/trailing hyphens. Changing app/env/run
changes the digest, preserving collision isolation when prefixes match. This
reserves space for embedded module suffixes such as -writer and -cluster.
`context.infra.name` remains the readable app-env stem, not a provider-safe
physical identifier. New seed Definitions must use resourceName; scope-aware
references still use @infra. No exact-template migration of old app-run or interim
infra.name-run inputs
is supported; existing catalog records remain untouched.
Limits verified against [IAM role name quota](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html)
and [Aurora cluster identifier constraints](https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_CreateDBCluster.html).
Stable run-independent names remain D01; removing the retired scope does not
change current ENVIRONMENT generation-0 names.
State/workspace names derive from descriptors. New AWS VPC/EKS are Environment-owned
and follow existing UNREFERENCED classification when absent from desired graph;
classification does not destroy infrastructure. Binding cannot change, and regular
AWS enrichment includes both nodes, so normal deploy keeps them referenced.
Internal existing-cluster descriptor remains `connections.<key>`; namespace and
workloads remain Environment-scoped. No cluster is provisioned by connection set.

## Historical legacy migration (superseded by T26/T27)

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

Validate current identity-only Application SQL/JSON reopen and atomic race,
independent kinds/regions, AWS distinct descriptors/workspaces, rejection of
retired scopes, matching conflict and immutable save guards. Live proof uses existing
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
