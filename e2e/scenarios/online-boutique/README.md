# Online Boutique through Console UI — issue #47

Goal: render compatible security defaults and deploy the eleven Online Boutique
services plus Redis using the real Console forms, Preview and Deploy.

Prerequisites: separate feature Console stack, private template/module repository,
Git read/write credentials, authorized kind-k8s-4f account, existing Fleet/VSO/Vault,
private credential files and sufficient image/network/cluster capacity.

Source: https://docs.score.dev/docs/examples/microservices/ ; upstream Score files
are analyzed from score-spec/examples commit 23eb62304e00eea5ccf01c4b54c2915a64c13611.
No Score import or generator is executed. Existing product security guards stay.

Runner uses the shared Console login helper. Set ORCH_PLATFORM_URL,
ORCH_PLATFORM_USERNAME, ORCH_PLATFORM_PASSWORD_FILE and ORCH_HEADLESS per e2e/README.
Private runtime state and evidence live outside Git. Stage-specific inputs are
supplied through an owner-only file; no credential is printed or recorded.
Business writes use browser controls only; API reads/response observation are
technical assertions, never mutation shortcuts.

Acceptance condition (user decision 2026-11-08): create and save all eleven
workloads and their resource bindings through the UI before deployment. Preview
the complete batch, then click Deploy once for the Environment. Record the
Preview workload list and the resulting Operation. Deploying individual workloads
one at a time is not a substitute for this acceptance flow. `workloads.mjs`
saves the full set without deploying; `deploy.mjs` submits one batch Deploy.
Backend sequencing is independent of the number of UI Deploy actions. A retry
is a separately reported attempt, never evidence of first-attempt success.

Assertions: private Git source pins, workload save/reopen, manifest/controller
security defaults, Pod readiness, Redis secret delivery and interservice calls,
shop/product/cart/checkout browser flow and loadgenerator logs/status.

Side effects: task-owned Git/Definition/Application records, Git delivery repo,
namespace/Pods/Service/PVCs from product deploy. Technical access forwarding and
namespace-scoped Fleet RBAC must be recorded separately and checked for ownership.
No shared controller/release reset or cluster-wide permissions are inferred.

Keep final Console/workload Tailscale forwards active for user acceptance. Cleanup
uses the product UI first and verifies exact run-owned UIDs before any technical
recovery; retained storage is not deleted implicitly. Remove only task-owned
forwards/stack/namespace when acceptance authorizes teardown. Never report setup
or a generated manifest as successful deployment.

Canonical sources: orchestrator_docs/architecture/contracts/workload-defaults.md,
orchestrator_docs/operations/kind.md and operations/tailscale-handoff.md.

## Run from repository root

Install `npm ci --prefix e2e` and `npm ci --prefix scripts/bootstrap`, then
`cd e2e && npx playwright install chromium`. Kubectl is required by the narrowly
scoped namespace setup and runtime observation. Use the shared Console environment
variables above plus `ORCH_TEST_RUN_DIR` outside Git (directory mode 0700).

The technical setup prepares `kubeconfig`, `vault-token` (0600) and `config.json`
in that directory. The latter contains `gitScope`, `gitUsername`, `gitTokenFile`,
`kubernetesEndpoint`, and `workloadVaultAddress`. It must point to the authorized
cluster and task Vault; it contains file paths, never secret values. Establish
reachable API/Vault forwards and Vault Kubernetes auth before this scenario.
The application, Vault and CD names in the runner identify this isolated trial.
Do not reuse its state with another Console origin or application.

```sh
node e2e/scenarios/online-boutique/platform.mjs
node e2e/scenarios/online-boutique/definitions.mjs
node e2e/scenarios/online-boutique/workloads.mjs
# Set ORCH_CLUSTER_KUBECONFIG to the authorized host-side kubeconfig.
node e2e/scenarios/online-boutique/deploy.mjs   # Preview only
ORCH_DEPLOY=1 node e2e/scenarios/online-boutique/deploy.mjs
node e2e/scenarios/online-boutique/observe.mjs
# Set ORCH_WORKLOAD_URL to the actual, checked Tailscale workload URL.
node e2e/scenarios/online-boutique/shop.mjs
```

The setup/definitions forms reuse existing named records. Workload reruns compare
image, resources, variables and Service before reuse; they stop on a mismatch.
Deploy records the attempt before submitting and will not replay an unknown or
partial attempt. The UI submit is followed by GET polling of the operation and
deployment history, so page navigation cannot lose the final response evidence.
Inspect the error and fix its cause first. A stopped PARTIAL
operation, or a locally UNKNOWN submit whose Console operation is confirmed
INTERRUPTED, may be recovered explicitly using `recover.mjs`, which checks the UI
recovery result and retains the prior attempt. A released FAILED operation needs
`ORCH_RETRY_CONFIRMED=1` to make the reviewed retry explicit. Never clear state or
patch the database to bypass those guards. Recovery can return 409 when Fleet or
secret delivery still lacks proof; retain the hold until the underlying state is
resolved. No cleanup or namespace deletion is performed automatically on failure.

## Adaptations from the pinned example

- Images, CPU/memory and topology come from the upstream fixture. Environment
  references between gRPC services use their literal stable Service DNS name and
  port in the shared namespace. The Console's current Service output is an HTTP
  URL, unsuitable for gRPC `host:port`; this run does not claim a cross-workload
  dependency edge for those literals.
  For example, frontend's `AD_SERVICE_ADDR=ad:9555` addresses the Kubernetes
  Service named `ad`, on its Service port `9555`, in the same namespace. It is
  a literal gRPC endpoint, not a Console resource-output reference or an HTTP URL.
  The short Service names match the workload names in this fixture; renaming a
  Service requires updating its literal consumers as well.
- Redis uses the private Terraform module and its sensitive `connection_string`
  output, selected through the resource picker. VSO delivers it as a Secret
  reference. Neither the fixture nor the Git manifests contain its value.
- Email's Service port 5000 forwards to its container port 8080. Loadgenerator
  points to frontend:8080, matching the declared Service port (upstream says 80).
- Frontend probes use `/_healthz` on 8080. The current workload probe form has no
  custom HTTP header field; this scenario must verify that headerless health
  requests succeed rather than claiming exact Score header support.
- All workload presets use the private v0.3.0 template source. This checks the
  compatible default in real Pods; it is not proof of full Pod Security Admission
  Baseline enforcement, admission injection, or the stronger nginx-only profile.

A rebuilt backend changes the executable pin of the renderer. Reinspect and save
`boutique-workload`, then select its new revision through Settings with `repin.mjs`
before Preview/Deploy. `ORCH_SELECT_ONLY=1` only selects an already-saved latest
revision after inspecting its history. Do not rebuild a retained acceptance stack
just to restart it, edit stored pins, or count an engine mismatch as a render pass.

Deployment polling requires the exact reviewRequestId Operation to appear before
interpreting an empty history as completion. A just-submitted request can still
be in admission; the runner keeps its browser open until that exact Operation is
terminal/interrupted, and reports an admission timeout honestly if it never appears.

After a verified SUCCEEDED attempt, `ORCH_NEW_RELEASE=1` together with the existing
explicit confirmation flags permits a new reviewed release. It archives the
successful attempt unchanged; it never relabels success as failure or replays an
UNKNOWN/PARTIAL attempt. Use this after an intentional renderer revision upgrade.

A FAILED recorded batch can still hold an INTERRUPTED operation when its first
workload failed after submission. Recovery supports this case only after matching
the active INTERRUPTED operation to the exact recorded reviewRequestId; a released
FAILED batch is never recovered by this runner.
