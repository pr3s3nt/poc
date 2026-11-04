---
id: IMP-DIRECT-BOOTSTRAP
artifact: implementation-handoff
status: current
last_reviewed: 2026-11-04
---

# Direct Definition bootstrap — issue #33

Approved operational scope: schemaVersion 3 input selects the environment and
explicit accounts, verified Terraform module pins, template sources, Definitions,
Secret Store and CD. Reuse existing bootstrap helpers and the
[Generic Workload contract](../architecture/contracts/generic-workload.md).
No change to product matching, rendering or delivery behavior is authorized.

The full command runs platform configuration, then a dedicated acceptance
Application without a Catalog, Preview/Deploy, real CD/Fleet readiness and app/job
checks. Only all phases passing means success. Platform-only mode has no
Application/image/upgrade/Catalog prerequisite and reports BOOTSTRAPPED only.
SchemaVersion 2 Catalog configuration remains supported separately.

Read, compare, create missing objects and read back each mutation. Equal input
reuses identities, revisions, immutable pins and deployments, including acceptance.
Differences block rather than publishing/promoting/updating automatically. Unknown
mutation outcomes require server reconciliation, never blind replay. Config files
and state identify an environment; credentials/evidence live outside Git. No
shared database reset/upgrade or shared controller/RBAC replacement is permitted.

Configuration and procedure: [bootstrap README](../../scripts/bootstrap/README.md),
[execution flow](../../scripts/bootstrap/EXECUTION_FLOW.md).
Runtime acceptance is restricted to the authorized kind cluster; environment
input examples do not authorize staging/production or cloud mutation.
Implementation complete. Focused bootstrap tests and real kind acceptance,
platform-only/full resume and unchanged rerun passed; see
[execution/handoff](../verification/2026-11-04-direct-bootstrap.md). The runtime
used Git templates and installed verified Terraform sources; ZIP/Inline and Git
Terraform paths were not separately deployed. No Go/frontend product code or
schema was changed. Task Application/data/forward are retained for review.

## Approved Git-only and backend PVC follow-up

User decision: fetch Definition documents, Resource Types, Terraform modules,
workload template sources and acceptance workload documents from Git, without
Application Catalog selection. Environment config and private credential references
remain local inputs. Git-only mode requires HTTPS repository, full commit and
SHA-256 for every input file; materialize verified regular files outside Git before
any product mutation. No mutable-ref/latest/local/inline/installed fallback.
Keep prior schema-3/file inputs compatible when gitOnly is absent.

Acceptance includes backend managed storage at `/data` and
`ACCEPTANCE_STORAGE_DIR=/data`, using existing product mount/resource contracts.
The full bootstrap verifies the actual PVC/mount and browser file write/read;
a separate ownership/UID-guarded Pod recreation proves persistence. Keep old
Definitions/Application/data unchanged; use an isolated schema-8 Console and new
acceptance Application for the Git-source run, then prove unchanged rerun.
This extends operational inputs/acceptance, not renderer, schema or matching rules.

Git-only/PVC follow-up implemented and accepted on kind; see
[execution and final video scope](../verification/2026-11-04-git-bootstrap-pvc.md).
The original unknown publication remains held; final acceptance uses a separate
Application and preserves the failed run's resources. Recording is opt-in and
publishes only the complete successful scenario's latest video.

## Enterprise staging portability — approved 2026-11-04

Target GitHub Enterprise via environment inputs, without company-specific code
edits. Keep business documents Git-owned and verify their raw bytes before
expanding explicit nonsecret `${bootstrap.NAME}` parameters. Missing parameters
fail before product mutations; existing `${context...}`/`${resources...}` and
`${templateSources...}` keep their product meaning. Scalar substitution occurs
in parsed JSON/YAML values, never by injecting unescaped serialized text.

Portable documents and a staging preset separate host/account/Vault/registry/
access/identity data from reusable operations. A read-only lock helper reads exact
commits from trusted local HTTPS-origin checkouts, computes file/tree digests and
source identities, and writes a private locked input; it never fetches/pushes or
changes a cluster. Custom module contract changes still require verified Console
inspection; the helper does not confer release/executor trust.

ADR-033 already permits HTTPS Enterprise API/clone roots. Correct the platform
Git binding path to inherit its actual host, `/api/v3` API and CA; GitHub.com keeps
its public API. Repository-hosting HTTPS trusts the declared CA and never disables
TLS or forwards credentials through redirects. No staging mutation is authorized
by a preset. The internal-AI handoff must list prerequisites, inputs, permissions,
checks, resume/conflict behavior, retention and known limits.
