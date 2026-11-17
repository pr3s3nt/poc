---
id: VERIFY-20261117-ECHO-GATEWAY
artifact: verification-record
status: current
last_reviewed: 2026-11-17
---

# Echo Gateway metadata and target verification — issue #84

The user approved the Echo refinement on 2026-11-17, then requested current main
merged before continuing. Product implementation `35bcd06c`; merge `df719115`
includes `origin/main` `ab37e2d4` and was pushed to `features/Httproute`. Template
Definition sample `987c5ce`, compatibility/evidence documentation `a984df1`;
Terraform/module/HTTPRoute template bytes unchanged.
No merge back into main, user acceptance, release tag or retained-stack rollout
is claimed by this record.

Required behavior: [ADR-043 refinement](../architecture/decisions/ADR-043-namespace-placement-template-routes.md)
and [GW-01/02 contract](../architecture/contracts/namespace-template-routes.md).
The [2026-11-16 record](2026-11-16-namespace-template-routes.md) describes the
preceding existing-gateway implementation.

## Resulting boundary

- New Gateway Definitions use Echo with exactly one required nonsecret gateway
  output envelope. The generic identity executor receives no Connection/Target,
  emits no manifests/physical instance/target and has no destroy operation.
- Catalog/planner validate supported representation, output/policy contract,
  whole typed cluster reference and same-cluster placement. Public Echo
  registration remains Gateway-scoped; arbitrary Echo resources are not
  reclassified as PREEXISTING.
- Runtime refuses developer params, secret-derived metadata and stale/foreign
  cluster/account/version/descriptor before Echo executes. Explicit legacy
  upgrades keep logical ActiveResource identity and original account provenance.
- Standalone GatewayVerifier observes readiness, listener/HTTPRoute support,
  hostname and namespace admission before render/CD. Legacy existing-gateway
  keeps its flat inputs, adapter and immutable revisions; edits preserve its
  stored driver/layout/version. No pin is silently rewritten.
- Existing clusters retain the dedicated existing-cluster adapter and Driver
  Account, including planning/pin and actual API/auth/readiness checks. Namespace
  inheritance, cloud database behavior and PostgreSQL schema are unchanged.

## Main integration and focused checks

The incoming main merge retained Score form/document synchronization. Six
conflicts in VI/EN messages and four documentation files were resolved preserving
both Score and Gateway/Echo requirements, traceability and execution evidence.
In-progress verification notes were restored after the merge. Ancestry confirms
main `ab37e2d4` is included in the feature branch.

Focused registration/planner/runtime and legacy Gateway/cluster readiness,
namespace-provider and pin checks passed. Runtime tests invoke real pure Echo
and check no executor access or physical ownership, retained logical identity,
secret/param rejection, stale/foreign provenance and original EKS account selection
(unit only; no cloud run). Related frontend suites passed 96 tests; after merge,
18 ScoreDocuments/EndpointDefinition tests and typecheck passed. Backend build,
frontend lint/build, docs and whitespace passed. Existing PlatformSettingsPage
lint and bundle-size warnings remain. PlantUML 1.2025.2 regenerated the sequence
PNG; the local docs checker separately reports no PlantUML on PATH.

## Real merged application lifecycle

The task Docker installation was upgraded to merged code with existing volumes,
credential accounts and template pins preserved. Shared installation, Gateway
spec/controller/DNS/TLS and cloud resources were not modified.

1. Created a fresh Application through Console and separate Echo Gateway/endpoint
   Definitions using app_id criteria in the existing editor. Read-back confirmed
   the driver, envelope and exact criteria, without changing the legacy trial.
2. Saved native and Score drafts before one Environment Preview/Deploy. Both
   succeeded through real Git/Fleet, sharing one namespace-bound PostgreSQL.
3. Native reads confirmed one Bound PVC, successful init clients/authenticated
   TCP SQL, intended HTTPRoute Service ports/hosts/paths/parents and
   Accepted/ResolvedRefs. Browser reached both actual nginx workloads over
   Tailscale, including the Console Application/API; no shell/mock substituted.
4. Saved both updated drafts before one batch Deploy. Both succeeded and show
   the new environment variable. Namespace/PostgreSQL/PVC/Gateway/Deployment/
   HTTPRoute UIDs remained unchanged.
5. Recreated only the native web Pod after app/namespace/controller checks and
   API UID/resourceVersion preconditions. A new ready Pod appeared; SQL, init
   clients, route status, preserved UIDs and browser checks passed again.
6. Removed both workloads through Console confirmations and one Environment
   Deploy. Deployment/HTTPRoute absence preceded removal of only receipt-owned
   setup RoleBinding/Role with owner/UID/API guards.
7. Console Application deletion succeeded on its first admitted operation,
   destroying PostgreSQL then namespace. Native checks confirmed namespace/PV
   absence and the preexisting Gateway UID unchanged. Cluster system namespace
   identity was also checked across cleanup.

A fresh merged review Application deployed both workloads using the released
endpoint names, with new namespace/PostgreSQL/PVC identities. Native SQL/route
checks and browser checks of the exact Console and both workload URLs passed.
Deployment resource views confirmed both executions selected the scoped Echo
Gateway Definition, reached READY and produced the gateway output. Browser
verification was repeated at 11:24 UTC. Services, forwards and review data remain
running for user acceptance. The
legacy Gateway/endpoint forms were reopened after the upgrade without saving;
versions/values were unchanged. Its original workload URLs were browser checked
as well. No new legacy redeployment is claimed.

The earlier pre-merge Echo execution at `35bcd06c` independently passed the same
native/Score deploy/update/restart/product cleanup and identity/SQL/browser
checks. Its evidence is attributed to that source, not relabelled as merged.

## Full checks and source attribution

Pre-merge full run: fresh candidate `35bcd06c`, explicitly reused completed main
`cd40a792`. Candidate/main: 1590/1568 passing top-level tests, identical 103 failing
top-level tests (143 including subtests), 20 failing packages and 8 skips, with
both builds passing and no missing main pass. Exact failure/skip set differences
are empty; all 22 added top-level tests pass. Frontend: 113 files / 1069 tests and
install/typecheck/lint/build pass. Ownership cleanup confirmed namespace absence.
This reused baseline is not presented as a fresh main execution.

A separate fresh run uses merged candidate `df719115` and main `ab37e2d4`, with
independent real PostgreSQL 16, Go 1.25.14 and Terraform 1.9.8. REQUIRE_POSTGRES=1
and REQUIRE_TERRAFORM=1 are enabled; both run
`go test -json -p 2 -parallel 4 -count=1 -timeout 30m ./...` and `go build ./...`.
Merged frontend on Node 22.23.4 passes all 113 files / 1076 tests and
install/typecheck/lint/build. The fresh backend comparison completed:

| Backend top-level test outcome | Merged candidate | Current main |
|---|---:|---:|
| Pass | 1579 | 1566 |
| Fail events | 101 | 101 |
| Skip | 8 | 8 |
| Failed packages | 20 | 20 |
| Build exit code | 0 | 0 |

Exact failure sets, including subtests (141 events per side), failed-package and
skip sets match. No main passing test is missing; all 13 additional top-level
tests pass. Both backend test processes exit 1. The PostgreSQL package times out
after 30 minutes on both sides with `TestVersionThreeRollbackPreservesVersionTwo`
unfinished. This is not a full green backend gate or complete PostgreSQL suite.

The 101 emitted top-level failures are precisely the [previous 103-name
inventory](2026-11-16-namespace-template-routes.md#identical-failing-test-inventory)
except `TestUpgradeHTTPLoginPreviewDeploy` and
`TestVersionSevenPreservesConnectionsAndAdmitsGit` in adapters/postgres, which
emit no completed outcome in this run because the package timed out earlier.
Their absence does not establish fixes. Both runs share the same eight skips:
six separately opted-in live conformance cases without their explicit environment
and two historical Xvfb launch/cleanup cases without Xvfb. These remain skips;
the real application lifecycle above does not substitute for those scenarios.
The disposable full-check namespace was deleted with owner/UID/API preconditions
and its absence confirmed. Earlier pre-merge full and both application acceptance
namespaces are also cleaned; only the explicitly retained review apps remain.

The full source archives retain their exact SHA. Later changes are E2E/docs-only:
merged backend tree `a6c9c7a8f26a99b564d8886d2794ccdcb44b69c6`, frontend tree
`9fe243dd2eb989f040e7634880b9f352f8a0a6be`. The E2E adaptation clears extension text
when importing native data after the merged editor auto-exports saved documents.
The browser phase also verifies the selected Echo Definition and executed READY
resource/output through each workload's Deployment API.

## Harness attempts and UI/access evidence

An initial merged browser attempt overlapped frontend container replacement:
session discovery returned HTTP 404 before Application mutation; retry after
stack health checks succeeded. The first update runner stopped at an exact label
locator before native Save, and admission guards prevented partial Deploy. It
now uses the existing main Score scenario's textbox role/name locator and clears
extension text for native import. Both drafts and the full operation were retried;
failed screenshot/error evidence remains private. No product assertion or guard
was weakened. A task-owned web forward initially lacked read permission to its
mounted kubeconfig; it was replaced with the required container UID without
changing credentials, TLS or shared infrastructure.

UI reference: existing signed-in Score Console Resource Definitions, AppShell,
shared styles/tokens and Button. Legacy and new Echo forms were inspected live
with unchanged field layout, short labels and collapsed optional help. No extra
instructional block or unrelated redesign was introduced. Private receipts,
logs/screenshots, login-file location, URLs, forward/restart commands and retained
data boundaries remain outside Git/public issues.
