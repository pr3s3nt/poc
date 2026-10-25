---
id: CONTRACT-ACCOUNT-DIAGNOSTICS
artifact: architecture-contract
status: current
last_reviewed: 2026-10-25
---

# Account diagnostic contract

Implements [UC-04 DIAG-01..14](../../usecase/UC-04/specification.md#account-diagnostics--diag-0114)
under [ADR-022](../decisions/ADR-022-account-diagnostics.md). Mock fixtures and
scenario controls are review artifacts only.

## Scope and identity

A scope pins Organization, stable account ID/key and account version,
`purpose=GENERAL|USE_CASE`, `mode=CONDITIONS|EXECUTION`, AWS region when relevant,
selected use cases and optional Application/Environment identity/versions.
Supported use-case keys: `preview`, `deploy`, `database`, `cache`, `secrets`,
`https`, `lifecycle`. GENERAL has no Application, Environment or use-case list
and only CONDITIONS mode. USE_CASE requires at least one distinct supported key.
An Environment requires its Application. Context must belong to the authenticated
Organization; account or Application names never authorize lookup.

AWS region is one explicit valid region of the requested diagnostic scope,
not a new account deployment policy. Kubernetes rejects AWS-only scope fields.
Account identity and trust configuration come from the saved, scoped credential;
the API accepts no credential, kubeconfig, URL, namespace, command or raw policy.
Missing credentials or identity/trust mismatch fails safely, without host fallback.
Changing an account version during execution invalidates remaining probes;
cleanup still honors the original physical target and ownership. Historical
reports retain their snapshot and are labeled stale when known versions differ.

## API

All routes require Platform Engineer/Admin and derive Organization from session.
Unknown or foreign scoped identities return 404. Developer requests return 403.

| Operation | Route | Behavior |
|---|---|---|
| Start | `POST /api/v1/driver-accounts/{key}/diagnostics` | Strict bounded scope + expected account `version`; 202 with persisted report ID/state. No automatic replay on uncertain response. |
| List | `GET /api/v1/driver-accounts/{key}/diagnostics` | Newest first; optional purpose, limit/cursor; bounded default 20, maximum 100. |
| Report | `GET /api/v1/driver-accounts/{key}/diagnostics/{id}` | Safe scope, lifecycle, progress, findings, times, cleanup facts, stale indication. |
| Cancel | `POST /api/v1/driver-accounts/{key}/diagnostics/{id}/cancel` | Idempotent cancellation request; cleanup completes before claiming clean cancellation. |

The Account read model may add a safe latest-diagnostic summary containing report
ID, scope, state, verdict counts and times; no diagnostic write changes the
Connection status/version. JSON export can be built from the safe report DTO in
the browser and must retain scope, timestamp, capability limits and actual facts.

Invalid field/combination: 422 with existing safe error envelope/field mapping;
stale account version: 409; conflicting active run for an account: 409 BUSY;
body too large: 413; unauthenticated: 401. Storage failures are distinct from
findings: inability to load a report must never render a clean/empty report.
Cancellation, GET and reload never repeat a start or credential mutation.

## Lifecycle and persistence

States: `QUEUED`, `RUNNING`, `CANCELLING`, `COMPLETED`, `CANCELLED`, `INTERRUPTED`.
A completed run may contain FAIL/BLOCKED/UNKNOWN findings; COMPLETED means the
diagnostic run finished, not that infrastructure is ready. State, safe partial
findings, completed check count and cleanup are persisted at probe boundaries.
Pending probes have no successful finding; progress is based on completed probes,
not a UI timer. A run survives tab changes and reload. At most one active run
per account prevents conflicting execution probes; retries create new reports.

Workers hold an owner token with a bounded lease/heartbeat. Expired ownership
is reconciled to INTERRUPTED, preserving findings and explicit unknown cleanup;
startup must not interrupt another live instance's lease. A stale worker cannot
overwrite a new owner or terminal report. Transactions contain no remote calls.
Restart never resumes an external create blindly or rewrites history as success.
An INTERRUPTED execution report with remaining resources cannot claim cleanup
complete. Private recovery records retain only safe target/name/UID/ownership
facts; credential material is resolved transiently through the existing port.

The [schema](../database/schema.md#account-diagnostics--adr-022) owns table and
grant details. Memory/JSON test stores preserve equivalent isolation, snapshots,
ordering and state invariants; production history uses PostgreSQL. Completed
scope/findings are immutable. No new history is stored in `connections.verification`.

## Finding semantics

Each finding carries a stable ID/code, group, verdict, safe structured observation,
expected condition, remediation key/arguments, affected selected use cases,
dependency IDs, observed time and optional duration. All text interpolation is
bounded and allowlisted. Localization uses VI/EN catalogs, not raw error parsing.

| Verdict | Required evidence |
|---|---|
| PASS | The stated condition was actually observed in this scope. |
| FAIL | An applicable required condition was observed not to hold. |
| BLOCKED | Named prerequisite prevents this check; dependency is visible. |
| UNKNOWN | No sufficient evidence: missing context/permission, timeout, unavailable adapter or unsupported execution proof. |
| NA | Condition genuinely does not apply to the selected path, with reason. |

UNKNOWN and NA never increment PASS counts. Summary is derived from findings
and run lifecycle. Unfinished/cancelled/interrupted runs cannot claim full
readiness. Conditions success does not prove workload apply, scheduling, volume
binding, image pull or cleanup. Check failures do not become a generic successful
connection check. Missing Application context must not invent a matching
Definition, selected image, Secret Store, hostname or required StorageClass.

Never return/persist provider stderr, decoder text, HTTP bodies, environment
values, secret values, credential references, tokens, kubeconfig, PEM contents,
Terraform state or arbitrary file paths. Technical details contain only safe
codes and scoped resource identifiers. A probe error selects a fixed safe code;
unknown errors remain UNKNOWN with generic actionable guidance.

## Probe catalog and evidence boundaries

Use the smallest applicable probes and existing adapter contracts. Probe execution
is bounded by per-probe and overall deadlines; cancellation is observed between
and during remote operations. A failure does not prevent independent checks.

**Kubernetes general:** saved identity/endpoint access, explicit TLS trust and
hostname/certificate validity, authentication, discovery/read permissions,
node readiness and storage inventory. Inspect installed relevant controllers
without requiring every optional add-on. A listed StorageClass alone proves
inventory only; provisioner/CSI readiness requires its observed configuration.
Missing read permissions are UNKNOWN, not proof a controller/resource is absent.
An insecure-skip-verify connection cannot pass a verified-TLS claim. Distinguish
untrusted CA, expired certificate, hostname mismatch, authentication, authorization
and unreachable endpoint when the adapter has evidence for that category.

**AWS general:** resolve the account's access key or EC2 role through existing
credential providers; signed identity/region/read-service probes; inspect the
ability to verify IAM capabilities. Reuse current capability/action groups and
context rules rather than introducing divergent policy guesses. Missing IAM
simulation authority/SCP evidence is UNKNOWN or an explicit failed applicable
verification prerequisite, never inferred provisioning permission. Quota checks
state whether only inventory or actual selected demand was compared. Account
identity success alone is not full readiness. Simulations are identified as
policy evidence and never represented as real cloud provisioning.

**Use cases:** determine relevant requirements from selected work and, when
provided, actual Application/Environment drafts and Definition/Type/module/render
pins. Use pure planning/validation for matching, target agreement and missing
inputs. Check Git/module/render/registry access with existing hardened transports.
Use exact required resource fields rather than module-name heuristics. When a
selected path needs secrets, inspect configured Vault trust/auth and VSO resources
without reading or publishing workload secret values. HTTPS checks use the
configured route/certificate source; do not universally require cert-manager.
Provisioned future targets cannot be probed as if already existing: return
BLOCKED/UNKNOWN with dependency. Multi-account plans identify dependencies that
belong elsewhere; never silently execute them as the selected account.

Every catalog entry exposed to users must either execute a meaningful probe,
explain missing prerequisites, or clearly identify unavailable proof. Unsupported
provider-specific execution and cleanup checks cannot be shipped as fixture PASS.
Provider capability limitations remain visible in report/export.

## Execution probes and cleanup

Kubernetes EXECUTION is explicit and uses a cryptographically unique run name,
run/Organization/account ownership labels, a recorded target and a new namespace.
It never applies into an Application namespace or overwrites an existing object.
Persist the intended resource identity before create and the observed UID after
acknowledgement. Probe workload scheduling/readiness and volume provisioning only
when applicable; fixtures/images and resource bounds are platform-owned and
must not be arbitrary client-provided commands. No workload secret values are
copied into probes. Missing required conditions block dependent execution.

Cleanup uses the same pinned physical target, validates run ownership and exact
UID before deletion, and verifies native absence. A same-name replacement or
foreign UID is preserved and reported. Cleanup has an independent bounded
context after cancellation/failure; inability to confirm deletion is explicit
UNKNOWN/failure with safe resource references. Report completion never masks
remaining billable or cluster resources. Recovery must not remove preexisting
or shared resources based only on a name prefix.

This feature does not authorize arbitrary cloud create/destroy in production.
An AWS execution check without a supported bounded real probe reports UNKNOWN
and an explanation; a policy simulation remains CONDITIONS evidence. Do not
offer an execution mode that silently downgrades to synthetic success.

## Acceptance

Required user-flow cases and live/fake evidence distinctions are in
[the delivery handoff](../../implementation/account-diagnostics.md#acceptance).
Readiness findings are validated against actual runtime facts, including repair
of owned fixtures and rerun, rather than merely asserting that badges exist.
