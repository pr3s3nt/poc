---
id: ADR-012
artifact: architecture-decision
status: current
last_reviewed: 2026-10-07
---

# ADR-012 — Editable Environment Connections, secret stores and transitions

Status: Accepted — 2026-10-07, user-approved feature scope.
Supersedes ADR-011 permanent binding lock and ADR-006 Application-level provider
selection/Variable storage. Preserves ADR-011 legacy identities until an explicit
target change, ADR-008 scoped immutable delivery and ADR-002 external calls
outside persistence transactions.

## Scope and ownership

Each Environment independently selects an execution Connection and a Secret
Store Connection. Neither selection is permanently locked. UC-04 owns PE/Admin
registration of Organization-scoped stores. UC-01 owns selection, target change
and migration interaction in Settings. UC-12 owns secret copy and revisions.
UC-05/06/08 own preview admission, pinned execution and target resource identity.
First secret-store adapter is Vault KV v2. First data transfer is PostgreSQL to
compatible PostgreSQL with downtime; arbitrary database/storage transfer and
zero-downtime migration are not implied. AWS onboarding remains ADR-009 future
scope; this task permits live kind/Vault verification, not AWS cloud mutation.

Variables are ordinary configuration metadata in Orchestrator persistence.
Secret bytes reside in the selected store only. The platform credential store
for Connection credentials remains a separate trust boundary: registering a
workload store does not move backend/control-plane credentials into that store.

## Secret Store Connection

A distinct Organization-scoped SecretStoreConnection contains generated key,
name, provider VAULT_KV_V2, backend address, workload-reachable address, KV mount,
Kubernetes auth mount, safe TLS settings, immutable credential reference and
READY verification status. PE supplies a token over authenticated bounded API;
metadata reads never return token or credential reference. Runtime lookup is
Organization-scoped and never falls back to another store or host credential.
Addresses/mount identity cannot be edited behind an existing key. Registration
verifies KV v2 and backend read/write permissions in an attempt-owned path,
removes the probe and records READY only after verification/credential persistence.
Workload Kubernetes-auth availability is checked before secret-dependent deploy;
backend connectivity alone is not proof of workload access.

## Versions and atomic execution admission

Settings write uses expected Environment version; configuration edits retain
configuration version; draft edits retain draft version. Preview pins all of
these, desired/current-set identity, execution binding including generation,
secret-store binding and immutable revisions. Any change invalidates the token.
The client reloads and previews again after safe 409, without replaying a stale
mutation automatically.

Checking a token then executing without atomic admission is forbidden. A short
transaction verifies the complete snapshot and claims a persisted Environment
operation ID. Only one active deploy/migration/store-copy may own an Environment.
All Settings/config/draft mutations check that claim in their own transaction;
they return 409 ENVIRONMENT_BUSY while an operation is active. Internal owner
writes explicitly carry the operation ID. Different Environments remain independent.
No DB transaction stays open during network calls. Owner/deadline/stage persist;
process restart exposes INTERRUPTED/recovery status. Expiry alone never permits
a second operation while old execution may still run; recovery must terminate
or confirm prior execution stopped before releasing the claim. In-memory mutex
alone is insufficient for PostgreSQL multi-process concurrency.

## Initial selection and target replacement

New Applications create unconfigured execution and secret-store selections;
ordinary variables/drafts work immediately. Writing a secret requires an
explicit selected store. Legacy Vault deployments/references are backfilled
with a named explicit platform Vault store, preserving physical values and
bundle references. Missing configured legacy Vault fails safely, never fabricated
READY. Old Application provider field is retained as legacy metadata only.

Before runtime resources exist, changing execution selection is a versioned
metadata operation, with no provisioning. Same selected key may be an idempotent
no-op only when the expected version is current. Blank/unset after configured
is not required; selecting another READY scoped Connection is supported.

After runtime exists, Settings offers explicit deploy-new or migrate-data mode.
Changing a selection must force deployment of the complete desired Environment,
including unchanged workloads. Destination Preview shows the current Set merged
with pinned pending Score upserts/deletes and the desired configuration revision,
so the user reviews those changes before Execute. Successful cutover consumes only
those pinned drafts and commits their resulting Set; failure preserves them for
retry. A deleted desired workload is absent at the destination, never an implicit
source deletion. Unmapped durable source data fails migration preflight.
It must not reuse old-target ActiveResource state
or mark an old-target workload as already applied. Historical deployments and
resources retain their actual Connection/TargetRef for reads and cleanup.
New target gets an immutable generation. Generation 0 preserves current legacy
resource/namespace/state identity; later targets include generation in physical
namespace/resource/state identity so even two Connections pointing at the same
cluster cannot overwrite each other's database. Hash/truncation must preserve
provider bounds. Existing AWS ENVIRONMENT/LEGACY_APPLICATION descriptors remain
unchanged at generation 0; new AWS target generations do not reuse old VPC/EKS
or Terraform workspaces. Cross-profile new deployment retains supported matching
rules and target capabilities, with no fallback to old Definitions/credentials.

## Secret store change

Claim operation after checking expected Environment/config versions. Copy all
secrets of the desired revision to immutable attempt-owned objects in the new
store, reading each original ref through its owning store. Validate every copy
internally without logging bytes. Variables stay in Orchestrator. On success,
commit new desired revision/refs and selected store together under the owner
and original version checks, advance versions and release claim. Failure never
changes selection/revision, reports a safe error and cleans only attempt-owned
new objects where safe. Old revisions and per-workload bundles remain immutable
and retain their store identity for existing Pods/history. Pending preview must
include every workload whose secret delivery store changes, even unchanged values.
Do not rewrite old references in place or delete old stores/values automatically.

## Target migration with downtime

Settings requests a versioned operation with desired target, explicit PostgreSQL
source-resource mapping, transfer mode and downtime acknowledgement. Validate
all mappings, DB/server/tool compatibility, target capability, credentials and
route ownership before stopping source writers. Unsupported types/paths fail
before mutation; never return simulated transfer success.

Stages: PREFLIGHT -> QUIESCING -> BACKUP -> PROVISIONING -> RESTORING ->
DEPLOYING -> VERIFYING -> CUTOVER -> SUCCEEDED. Failed/interrupted stages persist.
Record source and destination bindings/generations, original current Set and
configuration revision, source workload replica counts, backup safe handle/hash,
destination resource identities and per-stage outcomes (no credentials/bytes).
Backup artifacts contain application data: private 0700 directory/0600 files,
bounded subprocesses, remove on success or safely report retained recovery handle
on failure; never Git/GitHub/video/public API contents. Use PostgreSQL pg_dump
and pg_restore/psql appropriate to archive format and server version. No shell
interpolation of secret credentials or raw user commands.

Quiesce application writers before backup; record/restore replica counts.
Provision destination resources while destination writers are stopped; restore
and validate data before starting destination applications. A restored database
must not be overwritten by the ordinary provisioning step. Deploy complete
workloads with updated generated database outputs/secret refs. Verify readiness
and application data, then cut over owned routing and commit target/current Set.
Do not serve new traffic or declare success before restore verification.

Managed Kubernetes routing uses the product's owned Environment Ingress.
Source routes are retained until target is ready, then transferred with bounded
failure handling. For distinct external ingress addresses, a controllable routing
capability is required; DNS/provider automation is not invented. Missing capability
fails preflight or exposes an explicit operator cutover step, never falsely
claims traffic moved. The live test must prove routing and restored records.

Before successful cutover, failure keeps the old binding/current Set authoritative
and first stops attempt-owned destination writers that have started, then attempts
to restore source writer counts/routes, recording each failure explicitly. Persist
enough destination writer identity for interrupted execution recovery; never claim
a successful rollback while destination writers/routes still conflict with source.
When writer/routing compensation cannot restore a safe authority, persist an
interrupted held operation and needs-attention source state; recovery must report
failed steps and cannot release the claim on missing/read-failed evidence.
After cutover, source stays retained and quiesced; no automatic reverse migration
or deletion. Developer explicitly requests cleanup of the old generation after
review; cleanup resolves stored old credentials, checks ownership/references and
must not delete the current generation. Recovery and cleanup outcomes are visible.

## Validation

Concurrent CAS saves; edit between Preview/Deploy; admission races with config,
draft and Settings; two backend instances sharing PostgreSQL; persisted interrupted
claim; independent Environments; store registration scope/redaction/probe cleanup;
copy failure/CAS conflict and retained old refs; provider selection on delivery;
legacy revisions/backfill and variable conversion; target generation identity and
force-redeploy; unsupported migration preflight; writer quiescing, backup/restore,
route failure, source preservation and explicit cleanup. Real kind/Vault human
Playwright recording must show two stores, a store transfer, stale-preview conflict
and a target transition with nonempty PostgreSQL data retained. Both logical
connections may use the same physical kind cluster only if isolated generations
are explicit; such a test does not prove two physical cluster or AWS migration.

## Concrete first-delivery choices

Use Environment.version as Settings CAS version (safe conflicts with unrelated
operational changes are acceptable); keep configuration and draft versions separate.
SecretStoreConnection is a separate domain/repository, not an execution kind;
no change to AWS/KUBERNETES matching compatibility is needed for store registration.
Token authentication only; KV v2 mount defaults to kv, auth mount to kubernetes;
TLS verification remains enabled with optional CA PEM, no user shell/exec inputs.
Backend token needs value-path create/read/update/delete plus workload-bundle
policy/role management under the Orchestrator naming prefix; verifier checks
required capabilities and cleanup without modifying unrelated paths/roles.
Vault Kubernetes auth must be provisioned/trusted for the selected runtime cluster.
VSO VaultConnection/VaultAuth names include store identity, preserving objects
and synced Secrets referenced by old deployments. Store addresses/auth/mount
travel with pinned bundle/access metadata rather than global flags.

Copy the current desired revision's Secrets only. Applied/historical revision
refs stay with their original stores; next successful Deploy moves workloads to
the new store. Same key/current versions is a no-op. Failed attempt copy objects
are immutable and scoped; cleanup failure is recorded safely, never reported as
successful rollback. No store/old-value delete UI is required in first delivery.
New Variable entries store an ordinary value, Secret entries store owning store
key plus opaque ref. Legacy Variable refs remain readable through explicit legacy
store, and are materialized into ordinary Variable metadata on a successful edit
or store transition; never do network I/O inside SQL schema migrations. Missing
legacy access produces actionable failure and preserves state.

Legacy platform Vault store is created per Organization only from explicitly
configured existing Vault flags/credential store, preserving old KV references.
New Environments do not auto-select it. Existing configured revisions receive
that explicit identity idempotently. Do not read real operator secrets in tests.

First automatic PostgreSQL transfer supports Kubernetes-managed PostgreSQL 16
resources on direct Kubernetes delivery. Preflight resolves explicit logical
source/destination resource mappings and all persisted targets, rejects unmanaged
DBs/unsupported versions or route delivery before quiescing. Every in-scope
PostgreSQL resource must be accounted for; no silently skipped DB. Streaming
or private archive pg_dump/pg_restore are both allowed; restore is checked using
schema/table inventory and table row counts before application startup. Acceptance
adds a known persisted jobs record and verifies its content after transfer.
AWS target generation/new deployment is supported by existing provisioning and
local tests; live Aurora transfer is not claimed or simulated.

Real-kind first proof uses two logical Kubernetes Connections to the existing
kind-idp-internal with distinct target-generation namespaces. Two real Vault KV
stores are run-owned and reachable by VSO, with Kubernetes auth configured only
for this authorized cluster. This avoids altering unrelated clusters and is
explicitly documented as same-physical-cluster isolation rather than multi-cluster
proof. Source route ownership is removed/transferred only when target is ready;
Ingress selection and application checks must actually resolve to destination.
Distinct physical ingress/DNS requires an adapter or explicit operator step.

## Provider references

- [PostgreSQL 16 pg_dump](https://www.postgresql.org/docs/16/app-pgdump.html) and
  [pg_restore](https://www.postgresql.org/docs/16/app-pgrestore.html): archive/restore
  tooling and version compatibility; backup alone is not completed migration.
- [VSO API reference](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/vso/api-reference):
  explicit VaultConnection address and VaultAuth connection/mount references.
- [VSO authentication](https://developer.hashicorp.com/vault/docs/deploy/kubernetes/vso/sources/vault/auth):
  workload namespace/service-account scoping.

## Target generation identity contract

Generation 0 preserves exact ADR-011 descriptors/names. For generation g>0,
Environment resource scope uses `<app>.<env>~g<g>` and @infra resolves that
scope (including namespace, VPC/EKS and dependent DB paths). Runtime namespace
must be DNS-1123 <=63 characters, begin with the original bounded readable prefix
and end with a hash of full app/env/generation; it never truncates away generation
uniqueness. Physical AWS name digest includes generation in addition to app/env/run,
retaining ADR-011 provider length bounds. Connection/shared resources retain
connection scope; Environment-owned ActiveResource lookup includes generation.
Historical generation cleanup resolves its original target, never current Settings.
Descriptor IDs have a different grammar from resource scope IDs: environment-owned
namespace/VPC/EKS descriptor paths use `environments.<app>.<env>.g<g>` for g>0,
while their resource scope uses `~g<g>`. Descriptor IDs cannot contain `~`; workload
descriptors begin with `modules.` and cannot collide with these infra paths.
Generation 0 preserves both legacy descriptor and scope identities.

The generation delimiter is `~g`, not a dot-prefixed workload suffix: generation-0
workload scope `<app>.<env>.<workload>` must never be mistaken for a later target.
`~` is scope metadata, not a Kubernetes namespace character. Include tests that
generation-0 filtering excludes every later generation and vice versa.

## Recovery ownership fencing

Operation ID alone is not proof of current ownership after recovery. Release,
heartbeat and owner-only repository writes must check current owner/fencing token
so a paused old process cannot resume and release or mutate a recovered operation
before its next heartbeat notices loss. Operation contexts carry that ownership.
Recovery requires explicit confirmation that prior execution is stopped; stale
heartbeat is only an interruption signal, not evidence that subprocesses ended.
Enforce bounded operation/executor contexts; report recovery rather than silently
letting an expired executor continue indefinitely. PostgreSQL tests use separate
Store/pool/manager instances to cover stale owner release/write races.
