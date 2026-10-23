---
id: VERIFY-PREVIEW-ERRORS-DIAGNOSIS-20261023
artifact: verification-record
status: evidence
last_reviewed: 2026-10-23
---

# Preview connectors and Application errors — diagnosis and fixes

## Report and reproduced causes

User pointed to Application `bcbe4959-8a85-4998-8ad8-86584789527e` on the
Tailscale review server and reported confusing lines under Namespace/api/web and
many Application errors. Coordinator inspected that exact live Application and
all five current Applications before changing code. Base: `169c70e`.

1. The planner returns five valid dependencies for the four-node Internal graph:
   Cluster→Namespace, Cluster→api, Cluster→web, Namespace→api, Namespace→web.
   There is no api↔web dependency. The old renderer routed the long Cluster edges
   behind the Namespace card and overlaid segments/ports with Namespace edges,
   so the origins were visually misleading. This was a presentation defect.
2. Every supported Application's resource inventory returns HTTP503 with safe
   code SERVICE_UNAVAILABLE. The isolated memory/fake backend has no
   `terraformstate.ResourceReader`; resourcecleanup.readable fails closed. This
   is a real demo capability limitation, not proof of a failed deployment or
   an empty resource set. No real provider was contacted.
3. Seeded `acceptance` has only Environment `dev`. App mapping fabricated default
   staging/production metadata, then Home queried nonexistent staging Workload
   and Deployment endpoints and received 404. Existing user-created Applications
   have the advertised staging/production pair and did not have those 404s.
4. The document had no favicon and Chrome requested `/favicon.ico`, producing
   another 404. A data-URI brand icon now avoids that request.
5. The initial unauthenticated `/auth/session` 401 is normal session restoration;
   no browser page exception was observed. It is not changed to a false success.

## Fixes and boundaries

- `reviewEdges.ts` groups each endpoint pair while retaining every original
  reason/path. Separate attachment ports and exterior lanes prevent long edges
  intersecting intermediate cards. Dashed nonadjacent links, solid immediate
  links, tooltips and selected/hovered-node highlighting clarify the origins.
  No planner edge, ordering condition or scheduler behavior is removed/changed.
- Preserve exact advertised Environment keys in Application metadata. The shell
  prevents unsupported child routes from mounting/requesting nonexistent data,
  labels the Application's legacy Environment and offers explicit navigation
  when another supported Environment exists. The Application remains visible.
  No Environment is created, migrated, renamed or deleted.
- Resource inventory now presents known503 as explicitly unavailable with an
  explicit read retry. Ordinary errors/malformed responses remain errors. HTTP503
  remains unchanged and observable; no fake records or empty-success response
  replaces it. Backend state/cleanup configuration is still unavailable in this
  memory/fake demo. No claim is made that this backend limitation was enabled.
- Add an inline SVG favicon without another network dependency.

## Observed validation

- Focused tests: 49 tests PASS across App, EnvironmentResources, DeploymentReview
  and edge geometry. Tests assert no connector crosses another node's interior,
  separate incoming ports, preservation of duplicate-pair reasons/source paths,
  wider intermediate rows, no nonexistent Environment requests, and explicit
  unavailable versus empty/error inventory with GET retry.
- Full Console suite: PASS: 64 files / 825 tests, bounded to two workers (158.10s).
- Strict TypeScript, production build and changed-file ESLint PASS. Full lint
  retains the previously recorded 46 errors outside this task.
- Actual Tailscale browser audit: 33 pages across all current Applications,
  staging/production Home, Settings, history and standalone Preview. No visible
  error alerts, unexpected HTTP failures or page exceptions after fixes. The
  known resource-list 503 responses remain explicit and are retained in logs.
- Exact reported Application checked at 1512px and 1280px. Browser geometry
  sampling confirmed both long Cluster→Workload paths avoid Namespace; hovering
  web highlights its two incoming links. Node graph and relationships remain
  from the real Preview response. Cloud was also checked at both widths: the
  deduplicated connectors cross no unrelated node interiors.
- Docs checker / diff: PASS: 372 Markdown files / 321 artifact IDs, plus `git diff --check`. Backend code and diagrams did not change;
  their previous verification is reused instead of repeating Go/cloud/runtime
  provisioning suites for this UI correction.

The first focused run placed the new App test outside its existing setup scope;
it observed Sign In instead of the mocked Application. The test was moved into
the correct fixture scope and all focused tests passed. No assertion was removed.

## Visual evidence and running review

[Before](artifacts/preview-diagnosis-20261023/before.png),
[after at 1512px](artifacts/preview-diagnosis-20261023/after-1512.png),
[after at 1280px](artifacts/preview-diagnosis-20261023/after-1280.png),
[explicit unavailable inventory](artifacts/preview-diagnosis-20261023/inventory-unavailable.png),
[legacy Environment](artifacts/preview-diagnosis-20261023/legacy-environment.png),
[cloud routing](artifacts/preview-diagnosis-20261023/cloud-fixed-1512.png).

The running backend/tmux was kept intact and only the built frontend was
refreshed at `http://100.77.224.24:18093/ui/`. User drafts, Applications,
Definition pins and existing deployment history were not changed. Diagnosis and
browser audit were read-only apart from normal test authentication and read-only
Preview POSTs. No cloud/cluster, retained database or other user tmux was mutated.
Evidence remains at `/tmp/poc-preview-debug-20261023/`: inspection/audit reports,
actual graph metadata, geometry captures and validation logs. No state or binary
is committed.
