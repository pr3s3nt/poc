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

## Standalone platform resource repository — approved 2026-11-04

`projects/platform-resources/` is the publishable Git-owned asset set: Terraform modules,
workload templates and portable bootstrap Types/Definitions/Workloads/policy.
Publishing its contents at a new repository root uses `terraform/`, `templates/`
and `bootstrap/` paths. Company input points to that repository/full commits;
the existing offline locker recomputes source IDs, module IDs and content pins.
No Orchestrator Go/React code or credentials belong in the resource repository.

Embedded Terraform/template files under backend remain frozen runtime release
assets required by Go embed and legacy pinned inputs. They are retained for
compatibility, not the directory the company should publish. Packaging copies
only tracked regular source assets and never runtime state/cache. Historical
Git references pinned to old commits retain their original valid paths.

Validation cho layout repo tài nguyên: 59 bootstrap tests pass, gồm offline lock
và load Git-only với các path root mới; đối chiếu 8 module snapshot giữ nguyên
bytes; docs checker và diff whitespace pass. Không chạy lại backend/frontend
suite vì không đổi runtime code; chưa chạy target staging/GHES công ty.

## Company infrastructure image inputs

Enterprise input exposes `parameters.postgresImage` and `redisImage`, expanded
into Definition Terraform variables. Both require digest-pinned images compatible
with existing module entrypoints; registry host ports are supported. Pull access
and registry CA on cluster nodes remain prerequisites. Embedded backend modules
are unchanged. The earlier MySQL withdrawal was superseded by the user request below.

Validation: bootstrap tests cover company image interpolation; PostgreSQL/Redis
Terraform modules validated with Kubernetes provider 2.38.0. No staging mutations.

## MySQL, MariaDB and MongoDB resource assets — approved 2026-11-04

Add Git-owned Kubernetes modules, Resource Types and Definitions for MySQL,
MariaDB and MongoDB. Keep existing MySQL Type input/output names for compatibility;
MariaDB and MongoDB use the same nonsecret Secret-reference input pattern.
MongoDB creates a database-scoped readWrite application user at first empty-volume
initialization, separate from its root user. One StatefulSet and owned PVC per
instance; no HA, operator, backup or automatic DB credential rotation claim.
Image references, storage/class and Secret/key names come from private input.
Secrets must already be delivered into the application namespace by an approved
process; modules never read Secret contents or publish passwords. Module/Definition
registration does not extend the PostgreSQL/Redis acceptance app or authorize
cluster mutations. Retain embedded release assets and existing resource identities.

Validation for the added database assets: 59/59 bootstrap tests (nine Definitions,
Git-only pinning and company image/Secret interpolation); Terraform fmt/validate
for all three modules with Terraform 1.9.8 and Kubernetes provider 2.38.0; six
mocked contract tests passed for registry/pull/auth Secret references, ownership
labels, PVC/outputs/readiness settings, MongoDB init code and mutable-image
rejection. Tests are retained under each module's `tests/contract.tftest.hcl`.
Docs checker and diff whitespace passed. No backend/frontend runtime code changes,
no live database/cluster/GHES mutations and no company staging success claim.

## Console-only staging bootstrap — approved 2026-11-04

Separate the schema-3 staging operational path from dev acceptance tooling.
The default schema-3 runtime profile is `console`: local prepared business inputs,
Playwright UI mutations and authenticated Console read-back only. No shell Git,
kubectl, direct Kubernetes reads/RBAC writes or port-forward on the runner.
Backend Git module/template inspection and backend Kubernetes operations remain
unchanged. Kubeconfig/token files needed for UI onboarding are still input data.
The optional local preparation helper reads files already provided by the company
AI, computes hashes/pins and emits private snapshots without subprocesses. It
verifies local bytes, not the claimed Git provenance; backend source inspection
must still verify module/template commit and digest. Do not report local document
SHA verification as proof that a remote Git commit was inspected.

Explicit `runtime.profile=dev-test` retains old CLI Git/native cluster checks and
scoped acceptance setup for authorized dev tests; legacy schema-2 defaults remain
compatible. Profile is pinned in state so native and Console-only proof are not
silently mixed. Console reports claim Console deployment/selection and browser
business/file checks only; native Fleet/PVC UID/recreation proof remains dev-only.
Console profile rejects native setup flags instead of silently skipping them.

Focused validation: 19 tests across console-profile, direct-bootstrap and
enterprise-lock passed. Console test clears PATH while preparing/loading input,
running preflight and reading deployment/browser-file evidence; absent/altered
prepared documents and native setup/profile mixing are rejected. Existing
Git-lock/direct-bootstrap regression coverage passed. No full backend/frontend
suites or runtime deploy reruns: no product runtime code changed.
