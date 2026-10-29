# Workload catalog — real kind acceptance

Requirement: [specification](../../../orchestrator_docs/usecase/UC-16/workload-catalog-specification.md),
[contract](../../../orchestrator_docs/architecture/contracts/workload-catalog.md).

Run from repository root with installed frontend/e2e dependencies, Chromium,
kubectl/kind and the new backend/frontend code. Use a dedicated empty Console
project connected to an existing local kind cluster. Common sign-in configuration
comes from [console helper](../../README.md); no hard-coded login in runner.

```sh
ORCH_PLATFORM_URL=http://127.0.0.1:19825 \
ORCH_PLATFORM_USERNAME=platform-engineer \
ORCH_PLATFORM_PASSWORD_FILE=/private/password \
ORCH_HEADLESS=1 WC_CONTEXT=kind-idp-internal \
WC_KUBECONFIG_FILE=/private/internal.kubeconfig \
WC_SOURCE_REF=<pushed-immutable-commit> WC_RUN_ID=<unique-id> \
WC_EVIDENCE=/private/evidence node e2e/scenarios/workload-catalog/run.mjs
```

Setup/catalog mutations go through Console UI, reusing existing registration
helpers. Browser workload authoring uses the real picker/form/Preview/Deploy.
Technical kubectl/HTTP reads prove object identity and data retention. Pod deletion
for restart verification affects only recorded run-owned Pods. No fake backend.
Uses a public image available to the cluster; override WC_IMAGE for a prepared image.
Source uses repo Git URL, pinned commit and current module/render bundle paths.

Side effects: dedicated app/env, Definitions/Types/account in run Console,
Terraform-managed namespace/PVC, StatefulSet/Service/CronJob and scheduled Jobs.
No Passbolt migration/adoption. Failure preserves evidence/resources for diagnosis.
Cleanup uses product workload removal and separate confirmed resource deletion;
verify UID/ownership before any technical safety-net deletion. Do not delete the
shared cluster or preexisting namespace. No screenshots/logs/session/credentials
are stored in Git. Scope is generic acceptance, not actual Passbolt image validation.

Node 24.0+ is required (`node:module.registerHooks` unifies Playwright resolution
for reused legacy helpers). Install both frontend and e2e dependencies via
`npm ci` in each directory. `WC_PRIVATE_DIR` must name a private writable directory
outside Git for helper inputs. Store evidence on persistent disk; `/tmp` may be
cleared by a machine restart.

Assertions include image/config update with the same PVC UID/data, explicit
re-creation using the same resource descriptor to reuse retained storage,
CronJob completion in the live Job dialog, suspension/removal, namespace retained
data refusal, separate confirmed PVC deletion and verified namespace cleanup.
Successful completion prints PASS only after all assertions and product cleanup.

`WC_PRIVATE_DIR` is required; add it to the command above, for example
`WC_PRIVATE_DIR=/private/workload-catalog-inputs`. Use a unique WC_RUN_ID. A retry
may reuse an Application only when its exact ID matches the run's existing
scope.json in WC_EVIDENCE; a matching display name alone is insufficient.
Pod recreation verifies the StatefulSet owner UID and application label, then
uses a UID delete precondition. Cleanup follows persisted allVerified results
and removes the run Application through the confirmed UI after resources are gone.
Optional WC_BUILD_REF records the running product build separately from the
immutable module/render WC_SOURCE_REF. No existing Application is adopted.

With the integrated UC-18 application deletion surface, the runner also deletes
its own Application after resource cleanup, using the typed-name dialog and
persisted terminal status. Git module pins may intentionally remain at an older
compatible commit while WC_BUILD_REF records the newer integrated product build.
