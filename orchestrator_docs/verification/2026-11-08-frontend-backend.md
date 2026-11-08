---
id: VERIFY-2026-11-08-FRONTEND-BACKEND
artifact: verification
status: current
last_reviewed: 2026-11-08
---

# Frontend and backend through Console — issue #47

## Scope

User requested verification of
https://docs.score.dev/docs/examples/frontend-and-backend/ on the integrated
`features/organization_seed` branch, with a usable endpoint. The existing
Online Boutique app is retained. The separate test Application uses the real
Console forms for Definitions, workloads, references, Preview and one whole-batch
Deploy per attempt. No Score import/generator or product API mutation replaces
UI authoring. The new runner and prerequisites are documented in
[the scenario README](../../e2e/scenarios/frontend-and-backend/README.md).

Topology: separate Backstage frontend and backend plus PostgreSQL; two native
endpoint resources share one hostname with `/` and `/api`. These become
HTTPRoutes published through Fleet, not hand-applied application manifests.
The browser calls the public backend origin; a cluster-internal Service URL
cannot substitute for the browser's API URL. The current source picker supports
declared resource outputs and other workloads' internal Service URLs, but not
another workload's public endpoint output. Both workloads therefore select the
Environment variable `PUBLIC_ORIGIN`; its value is configured once through UI.
The backend's PostgreSQL values use resource references, including a sensitive
password delivered through VSO as a Kubernetes Secret reference.

## Attempts and diagnosis

- Both workload drafts were saved and reopened before the first batch Deploy.
  The first attempt failed with `TEMPLATE_UNAVAILABLE` before namespace creation:
  the new Environment inherited an unavailable historical workload Definition
  revision. An available revision for the running renderer was then explicitly
  selected through the Environment UI. No database pin/reset bypass was used.
- The second attempt provisioned PostgreSQL and both workload containers became
  Ready, but it is **not successful deployment evidence**. A trial Vault reviewer
  token had expired; its identity and policy were retained while the operator
  replaced only the reviewer JWT. VSO then synchronized the password. Separately,
  Fleet GitRepo reported the desired commit while all Bundle/BundleDeployment
  commit labels still pointed to the preceding commit. The exact revision guard
  kept the Operation from succeeding and it became INTERRUPTED.
- Recovery used the Console confirmation/action. One additional operator
  resync of this owned GitRepo, guarded by its UID, resourceVersion, exact
  intended commit and four non-Job paths, advanced forceSyncGeneration from 1
  to 2. The commit labels converged and recovery released the Operation. This
  controller assistance is recorded separately and is not proof of unattended
  product convergence. No manifest, native readiness or Operation status was
  patched to report success.
- HTTP 200 alone initially hid a blank frontend. Browser errors identified
  missing `app.title`; the upstream frontend entrypoint replaces its compiled
  configuration with APP_CONFIG variables. `app.title` and `organization.name`
  from the source configuration were added through UI. A backend startup probe
  protects database/plugin initialization. All images are digest-pinned.
- The corrected batch contains both backend UPDATE and frontend DEPLOY, with
  one UI Deploy request. Prior failed/interrupted attempts remain recorded.

## Result and checks

On 2026-11-08 the user requested stopping further verification and accepted
the current deployed state as sufficient for this trial. The third batch
Operation **SUCCEEDED**, with one UI Deploy request containing backend UPDATE
and frontend DEPLOY. Both Deployments and PostgreSQL were Ready, with no Pod
restarts at the observation point. Cluster verification confirmed the namespace
UID, password secretKeyRef and both HTTPRoutes Accepted/ResolvedRefs. The real
Console Application page loaded through Tailscale and its screenshot was saved.

The public root returned HTTP 200 and rendered the Backstage Guest sign-in
screen. Clicking Enter failed with `No implementation available for apiRef{core.toast}`
in the upstream frontend image. The upstream
[app-defaults changelog](https://github.com/backstage/backstage/blob/master/packages/app-defaults/CHANGELOG.md)
records a fix for that error in version 1.7.9. Guest sign-in and catalog data
acceptance **did not pass**; verify.mjs correctly failed and no passing
verification.json was produced. An older frontend image was inspected in an
isolated diagnostic Docker container only; it was not deployed to Kubernetes
and the diagnostic container was removed at the user's stop request. The app
retains its original digest-pinned frontend image. No additional deployment or
upstream source/image fix is included.

Node syntax checks for both scenario scripts, documentation validation and
whitespace checks pass. Product Go/frontend suites were not rerun because only
scenario scripts/documentation changed. PlantUML is unavailable locally; no
diagram changed. These results establish successful deployment and endpoint
reachability, not completed Backstage business acceptance or unattended recovery.

Private state, screenshots, controller retry receipt, token renewal metadata and
forward restart scripts remain outside Git. No credential or Tailscale address
is published here. The app, PostgreSQL/PVC and forwards are retained for user
review, with ownership/UID cleanup required later. No shared Gateway/controller
replacement, main merge or rollback scenario 2 run is included.
