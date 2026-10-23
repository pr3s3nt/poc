---
id: VERIFY-PREVIEW-KIND-LIVE-20261023
artifact: verification-report
status: current
last_reviewed: 2026-10-23
---

# Preview checkout — persistent real kind runtime

## Outcome and retained access

User requested a complete usable kind deployment path after confirming that the
port 18093 review demo used fake execution. The new independent Compose project
`preview-kind` builds this checkout and runs real Kubernetes/Terraform/score-k8s,
PostgreSQL with separate Owner/Console/State roles, encrypted Terraform state,
durable Vault credentials/output secrets and persistent module/provider caches.
No fake adapter, stub or browser interception was used in this execution.

- Console: <http://100.77.224.24:18094/ui/>.
- Retained sample: [Kind Preview](http://100.77.224.24:18094/ui/applications/7de8dc65-afcf-4808-9809-340c0225c79a).
- Deployed sample: <http://100.77.224.24:18096/> (owned `preview-kind-app` tmux
  Service port-forward, reconnects after Pod replacement).
- Seeded local login: `platform-engineer` / `test-password`.
- Backend loopback port 18095; no public database/Vault listener.
- Cluster/context: existing `k8s-4f` / `kind-k8s-4f`, node Ready, Kubernetes v1.36.1.

The old port 18093 demo, its drafts/history and all existing user tmux sessions,
the retained `orchestrator-poc` installation, both kind clusters and their existing
workloads/operators/volumes were preserved. No AWS action or database reset ran.
The new project's default catalog is empty; onboarding below authored the usable
global internal-cluster/template and namespace/in-cluster database/cache Definitions.
New Applications still explicitly select a workload Secret Store when needed.

## Actual execution

1. Built both product images from this checkout; startup selected installed
   score-k8s 0.15.0 and Terraform 1.9.8 explicitly. All four retained services
   reached healthy; DB setup and fresh credential provisioning exited 0.
2. Through the Console: created the sample Application, uploaded a private
   flattened kubeconfig as Internal account `kind-k8s-4f`, verified READY against
   `https://k8s-4f-control-plane:6443`, registered five Types, existing-cluster,
   namespace/PostgreSQL/Redis module Definitions and the workload render template.
   Registered existing `k8s-4f-workload-vault` with its private scoped token;
   selected this ordinary store in staging and saved variables/one secret.
3. Imported the repository acceptance Scores (existing loaded amd64 images from
   V04), then ran Preview and Deploy through browser buttons. Frontend, backend
   and worker reached Ready; PostgreSQL/Redis each reached Ready with a Bound
   1 GiB PVC. Namespace:
   `app-7de8dc65-afcf-4808-9809-340c0225c79a-staging`.
4. Real inventory returned HTTP200 with three READY managed resources, replacing
   the memory demo's unsupported inventory503 for this new runtime. Three
   successful workload histories contain persisted applied manifest reviews.
   All five sample browser checks passed: backend connection, environment,
   secret, database and cache. Browser-submitted jobs completed with a worker
   identity; the API subsequently reported a Redis cache hit.
5. Updated only frontend image from
   `acceptance-frontend:v46-20261018-512d0d3-amd64` to the preloaded `-b` variant
   via Score import, preserving the existing advanced Score. Preview code showed
   red/green lines and the exact previous frontend Deployment baseline ID.
   PARTIAL is correct for these redacted reviews; it is not a missing baseline.
   Explicit Deploy succeeded and Kubernetes reports the new image.
6. Restarted only `preview-kind-backend-1`, then signed in again and observed
   inventory, all workload manifests/history and the original completed job.
   A new browser job completed. All three resource identities/descriptors/statuses
   match before update/restart; five Pods are Ready and two PVCs remain Bound.
7. Created a separate cleanup smoke Application via UI, deployed one real web
   workload, marked it for removal, Previewed and Deployed removal. Its namespace
   became UNREFERENCED. Opened resource detail and the explicit destructive
   confirmation dialog, entered the actual confirmation name, then observed
   DESTROYED. An independent successful `kubectl get namespace --ignore-not-found`
   returns no object for the exact namespace below. No host-side delete bypass
   was used. The empty Application/history remains as an audit record.

Cleanup smoke identity: Application `3e494fd9-f8b0-46af-8b75-19f1e7552cb5`,
namespace `app-3e494fd9-f8b0-46af-8b75-19f1e7552cb5-staging`, original UID
`a42010d1-5a0f-4dfb-a098-6f81797a9662`. The retained sample's five Pods and two
PVCs are deliberately kept for the user; they are not leftover disposable tests.

## Checks and failure history

- Both Docker builds and image frontend TypeScript/build checks passed.
- Full frontend: 64 files, **825 tests passed** (158.95s).
- Final affected helper checks: 3 files, 15 tests passed.
- Changed JavaScript ESLint, Node syntax, shell syntax, merged Compose config,
  documentation checker and `git diff --check` passed.
- Full-directory lint has the 46 preexisting errors in unchanged historical
  prototype/E2E files recorded by the preceding diagnosis; it is not claimed
  clean. No new product Go/TypeScript logic or schema changed here; the previous
  Go validation is reused and actual product build/live execution added.
- No AWS, production profile, HA/backup recovery or database upgrade acceptance
  is claimed. This is a persistent personal-machine kind installation.

Private execution history remains in `/tmp/preview-kind-20261023/`:
`e2e-r1` exposed stale criteria-radio automation; `e2e-r2` exposed the smoke
script's incorrect Application DTO read; `e2e-r3` exposed the collapsed mapping
section; `e2e-r4` exposed the helper's open-attribute handling; `e2e-r5` reached
successful store registration but its status assertion failed. Shared helpers
were corrected for current v3 criteria, configuration tab, collapsed advanced
section and a scoped success status; `e2e-r6` completed the real deployment.
Existing records were compared/reused explicitly, never reset.

`check-r1` assumed the advanced Score could be edited by the simple form;
`check-r2` incorrectly demanded AVAILABLE instead of redacted PARTIAL. Both
automation assumptions were corrected; `check-r3` passed valid update with exact
baseline provenance. `check-r4` hit the temporary Service port-forward bound to
the replaced frontend Pod; it reconnected, and the observer now waits for a
healthy forward before navigation. `check-r5` passed restart/job persistence
and verified product cleanup, with no browser page exception. An initial helper
test command used Node's runner for Vitest files; it was corrected and the final
15-test focused run passed. Failures were retained rather than counted as PASS.

## Reviewed public evidence

- [Console after restart](artifacts/preview-kind-20261023/console.png).
- [Exact applied baseline and code diff](artifacts/preview-kind-20261023/update-code.png).
- [Real checks and completed jobs](artifacts/preview-kind-20261023/application.png).
- [Independent runtime summary](artifacts/preview-kind-20261023/runtime-summary.json).
- [Cleanup identity](artifacts/preview-kind-20261023/cleanup-identity.json) and
  [final browser assertions](artifacts/preview-kind-20261023/assertions.json).

Screenshots were visually reviewed. Credential inputs stay private/masked; no
Terraform state, token or kubeconfig is published. The product's existing safe
review/API redaction remains unchanged. See the
[operational procedure](../operations/preview-kind.md).
