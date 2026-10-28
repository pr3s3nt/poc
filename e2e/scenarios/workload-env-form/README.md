# Workload environment form on kind

Verify [UC-16](../../../orchestrator_docs/usecase/UC-16/specification.md) BR-15–27,
[UC-12](../../../orchestrator_docs/usecase/UC-12/specification.md) and
[acceptance app deployment](../../../orchestrator_docs/operations/acceptance-app-manual-k8s4f.md).
All application creation, variable/Secret creation, resource declarations,
workload configuration, Preview, Deploy and update use Console controls. No
Score file/import, direct mutation API or kubectl apply is used. Read-only
kubectl inventory and a port-forward verify the real runtime.

## Prerequisites

- Explicit permission to deploy onto the chosen kind cluster.
- Real Console/backend, ready Internal account, namespace/workload/postgres/redis
  staging catalog and ready Vault/VSO store according to
  [platform setup](../../setup/README.md). Do not reset retained state.
- Three compatible acceptance images already loaded in kind; image tag is explicit.
- Node, kubectl, Chromium; `cd e2e && npm ci`. Shared Console account configuration
  from [e2e guide](../../README.md); developer role and private password file.
- Evidence directory outside Git, with enough space for WebM video/screenshots.

## Run from repository root

```sh
ORCH_PLATFORM_URL=http://localhost:5188 \
ORCH_PLATFORM_USERNAME=developer \
ORCH_PLATFORM_PASSWORD_FILE="$HOME/.orch/local-password" \
ORCH_HEADLESS=1 \
ORCH_ENV_RUN=env-form-unique-run \
ORCH_ACCEPTANCE_IMAGE_TAG=<loaded-tag> \
ORCH_KUBE_CONTEXT=kind-k8s-4f \
ORCH_EVIDENCE_DIR=/tmp/env-form-unique-run \
node e2e/scenarios/workload-env-form/run.mjs
```

`ORCH_ENV_RUN` must be unique. Secret store `k8s-4f-vault` is explicitly selected
on initial inline Secret creation; it is never auto-selected by product code.
The scenario expects Vietnamese Console. It creates its own application and
automatically generated shared PostgreSQL/Redis IDs; the worker selects the
backend resources through the existing-resource picker. It never changes an older application.

`ORCH_SECRET_STORE_KEY` optionally selects another explicitly configured store
(default `k8s-4f-vault`). The disposable [Application deletion wrapper](../application-deletion-form/README.md)
sets `ORCH_DELETE_APPLICATION=1` and its private `WM_*` verification handles to
delete the new Application through Settings after update. This mode verifies
native namespace/PV and exact Vault value/bundle absence. It does not retain
the application. `ORCH_SECRET_SCAN_FILE`, when supplied by that wrapper, records
the generated secret only in its private scanner input, never public evidence.

## Assertions

- Create backend/worker/frontend using the form, direct values, shared Variable,
  inline masked Secret, resource outputs and workload Service references.
- Explicit Preview and Deploy report three successful workloads.
- Actual kind deployments/StatefulSets/PVCs exist.
- Acceptance application reports five PASS checks; submitting a job reaches DONE.
- After each submitted job, all five checks pass again and the read-only cache
  evidence confirms the worker's Redis write (none failed or unverified).
- Reopen deployed backend on the form; direct values survive. Add an ordinary
  value and deploy a valid update; PostgreSQL/Redis/PVC UIDs remain unchanged.
- Verify the new value on the live Deployment, the original job remains DONE,
  and a new job submitted through the application UI also completes.
- Browser JavaScript has no errors. Secret input is password type and its raw
  value never appears in page text. No browser session/trace is persisted.

## Side effects, failure and cleanup

The new application and its runtime are retained for user review, including
PostgreSQL/PVC/Redis and Vault-backed configuration. Port-forward and browser
session end in finally. A failed pre-deploy run may leave application/config
metadata but no runtime. Do not rerun under an existing name or silently delete
it. To remove later, use Console workload deletion and resource cleanup, verifying
namespace/resource UID and ownership against the run's `objects.json`. Do not
delete shared/preexisting cluster resources, Vault or PVCs belonging to other runs.

## Evidence

Private run manifest, assertions/phases, screenshots and console/application
WebM videos go to `ORCH_EVIDENCE_DIR` outside Git. Review video for secrets before
publication. A dated verification record owns actual results, runtime identities,
retention and GitHub video links. Screenshots include data entered in ordinary
fields; use synthetic non-sensitive fixture values there. Secret values are
random and only entered in masked fields; do not enable Playwright tracing.

## Read-only review of a completed run

Use the same ORCH_PLATFORM_* credentials and URL, plus `ORCH_APP_ID` and a new
`ORCH_EVIDENCE_DIR`, then run `node e2e/scenarios/workload-env-form/review.mjs`.
It opens the deployed backend, checks VI/EN at 1512/1280px, source search,
keyboard focus/Escape and page overflow. It changes only browser language/display;
no configuration or deployment mutation. The shared login helper sets Vietnamese
only when no locale is stored, so an explicit language change survives reload.
