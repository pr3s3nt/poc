---
id: RUNBOOK-GIT-RENDER-BUNDLES-ACCEPTANCE
artifact: operations-runbook
status: current
last_reviewed: 2026-10-24
---

# Git rendering — real end-user acceptance

Required execution procedure for [ADR-021](../architecture/decisions/ADR-021-git-render-bundles.md).
The runner/scenario named here are implementation deliverables, not existing
verified commands. The user explicitly requires Playwright through the new UI,
real adapters and an experience like an end user. A mock, fake adapter, stub CLI,
intercepted success response or direct API seed cannot establish acceptance.

## Environment and authorization boundary

Implementation includes preparing local Docker/kind resources needed for this
acceptance. Use a **new run-owned kind cluster**, `manifest-bundle-<run-id>`, and
an isolated Compose project with fresh PostgreSQL/Vault/state/cache volumes and
loopback ports. Do not reset/restart/mutate retained `orchestrator-poc`,
`orchestrator-manifest`, `preview-kind`, `dev-ex`, `i18n`, `k8s-4f` or `idp-internal`
installations. No AWS calls/deployment, paid resources or shared-cluster mutations.
Cluster creation and deletion are runner-owned setup/cleanup, not product flow.

No additional approval checkpoint for this scoped local run. Preflight checks
Docker, kind, kubectl, Helm for isolated VSO, browser, Git/HTTPS access, disk/ports
and actual pinned binaries/provider cache. Capture non-secret versions and image
IDs. Build current product images and acceptance app once; configure real
Kubernetes/Terraform adapters, actual score-k8s 0.15.0, PostgreSQL system/state
stores, durable render/module caches and Vault output/configuration storage.
Seed only local authentication accounts and the same ordinary platform bootstrap
required by the product; catalog starts empty. Explicitly select the renderer
binary (plain base Compose may not enable it).

Install the needed Vault/VSO components only in the new cluster/runtime, using
existing acceptance helpers and checked-in deployment configuration. Create
private kubeconfig and token inputs with mode0600, reachable container/Pod
addresses, and named run-owned mounts/policies. No credentials in Git, terminal
output, browser traces, screenshots or public artifacts. Technical setup of the
test platform is allowed; user-visible catalog/app actions below must use UI.

Use the actual origin `https://github.com/pr3s3nt/poc.git` for the reference bundle.
Prepare A and B as two actual Git commits for the **same directory**, using a
separate task-owned worktree/ref `test/manifest-bundle/<run-id>` if needed. Push
that new ref only, never move/delete another ref or force-push. The implementation
task may create/remove its own fixture ref as part of test preparation. Use the
real remote inspect/fetch; no local downloader shim. Keep A/B reachable until
scenario/cleanup complete, retain their commit IDs/digests in sanitized evidence.
The final standard bundle stays on the implementation branch for other authors.

If a prerequisite is unavailable, diagnose and repair the owned setup. Do not
borrow secrets from retained installations, silently switch cluster/context or
fall back to fake adapters. An unresolved external outage is a truthful blocker,
not a reason to ask the user to choose implementation behavior already settled.

## End-user flow and assertions

Use Playwright Chromium with real rendered controls and accessible role/label
locators. Run headed on a local display or Xvfb; no forced
cursor animations, artificial slow typing, captions, minimum video duration or
publication requirement. Speed is acceptable; assertions must wait for actual UI
states/readiness. Record sanitized screenshots/phase assertions. Avoid raw tracing
around credentials because traces may retain request bodies.

Backend API reads, observed browser responses, kubectl reads and app port-forward
are allowed for independent assertions. No `page.route` fulfillment, response
stubbing, fake adapter or direct API/database writes to create/edit the feature's
catalog, Definitions, config, workloads or deployment. Product mutations below
must be clicks/forms/uploads a real user can perform.

| ID | User action | Required evidence/assertion |
|---|---|---|
| RB-E01 | Sign in as PE; create Internal account via kubeconfig upload; register Types, workload Secret Store and existing-cluster/namespace/PostgreSQL/Redis Definitions | Account READY; real Kubernetes/Terraform provider verification; scoped criteria; no pre-seeded feature Definitions |
| RB-E02 | Open Mẫu dựng ứng dụng; enter URL, directory, ref A; Kiểm tra bundle; configure typed inputs, placement and criteria; save | Real inspect success and pinned commit A; schema fields visible; inputs/form↔JSON survive; save/reopen preserves pin. Screenshot and observed response contain no credentials |
| RB-E03 | Sign in as Developer; create Application, select registered workload Secret Store, add one plain variable and one masked secret; add frontend/backend/worker workloads | Actual Console flow with matching bundle and Terraform dependencies; resource requests absent on one container and explicit on another for precedence checks |
| RB-E04 | Preview; inspect graph, source/short commit and manifest diff | Bundle A provenance; chosen defaults/custom annotations visible; unresolved dependency paths honestly unknown; no resources created by Preview |
| RB-E05 | Deploy; open deployed diagnostic application; create a job and observe completion | Real Deployment/Service ready; PostgreSQL/Redis/PVC from unchanged Terraform; app→backend→PostgreSQL/worker/Redis business assertions pass; Secret reference works without revealing value |
| RB-E06 | As PE, edit the same template to ref B, inspect and save | B is same source directory with changed normalization/default policy and patches; output-only provisioner used for real bindings; no backend/frontend rebuild or restart between A and B |
| RB-E07 | As Developer, with no Score draft, Preview then Deploy | Pending workload UPDATE; A→B source/default/annotation/securityContext diff; actual Pod/Deployment match B; explicit CPU/memory/ports/images preserved; jobs still pass |
| RB-E08 | Refresh, then restart only the owned backend; reopen template/deployment; Preview unchanged workload | Pin B, typed inputs, applied baseline and jobs persist; unchanged intent is no-op; durable cache works without a new inspect. Record backend image digest unchanged, no Git fetch during render/restart-use |
| RB-E09 | As PE select verified A again, save; Developer Preview/Deploy | Explicit B→A change, original policy restored; no silent rollback of input/config/Score history; business flow still passes |
| RB-E10 | Remove workload(s) and clean owned resources using product UI; exit | Original target/ownership used; independent API reads prove absence of run-owned workload/resource objects, no false success from failed API connection; then tear down run-owned test platform |

Version A/B fixtures must exercise **policy**, not only a bundle label: reference
A uses e.g. fallback CPU20m, B30m for an otherwise unspecified field; a container
with explicit CPU request keeps it. Include explicit-limit-without-request
same-field fallback and missing memory default. Change non-reserved Pod annotation
`example.com/render-policy` and a valid `securityContext` through the bundle.
Choose values compatible with the actual app image (e.g. `seccompProfile: RuntimeDefault`);
do not require a root-only image to become nonroot. Normalization conversion and
provisioner resource outputs must be asserted in actual manifest/working DB/Redis
bindings, plus focused tests proving source changes in each stage are consumed.

Capture backend image ID/process start before A and after B to prove no restart/
rebuild was needed. The intentional restart in RB-E08 is a separate persistence
check. Record all source/cache pins and compare raw applied manifests privately;
only sanitized allowlisted projections are retained for sharing.

After RB-E05, record Terraform instance/state identities, cluster/namespace UID,
PostgreSQL/Redis workload and PVC UIDs, database record/job identity. Across
RB-E07..09 assert those identities and the database record remain unchanged;
no destroy/recreate or hidden database deployment by score-k8s. Infrastructure
no-op refresh/read is allowed; new provisioning/replacement is not.

Do not add intentional failing cluster scenarios to this happy-path run. Unsafe
bundle/source/identity/schema/cache/stale/error handling belongs in focused unit,
HTTP, UI and real-PostgreSQL tests. Existing negative tests still must pass.

## Ownership, evidence and completion

Create a run manifest before mutation containing run ID, owned cluster/Compose
project, newly created refs, ports, volumes, app IDs and later observed UIDs.
Install exit/signal cleanup before setup. On failure capture sanitized evidence,
inspect the error, fix code/setup and retry with a new owned run after verifying
cleanup. If product cleanup fails, record that failure and recover only resources
whose exact owned identity/UID matches; recovery never counts as RB-E10 pass.
Never run broad deletion against a shared context. Delete only the new cluster,
project volumes, temporary private credentials/forwarders and own fixture ref.
Retain non-secret evidence outside Git and a dated summary in verification/.

Required evidence: commit under test, A/B pins, real runtime versions/adapters,
per-step PASS assertions, UI screenshots, no browser errors, safe source/manifest
diffs, business job/DB/Redis results, protected Secret scan, unchanged infrastructure
UID/state facts, restart proof and cleanup observations. Missing required step,
skipped test, nonzero worker/runner or unresolved cleanup is a failed gate.

Run required code/frontend/docs/shell checks from the
[implementation handoff](../implementation/git-render-bundles.md). New real
runner command must appear there and in the implementation report. Update
CURRENT_STATE/IMP-034/traceability only from actual results. Complete the GitHub
issue only when end-user acceptance and checks pass; otherwise report the exact
blocker and retained owned resources, never claim a fake/partial run as success.
