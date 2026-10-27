---
id: VERIFY-ENV-UX-20261027
artifact: verification-record
status: current
last_reviewed: 2026-10-27
related: UC-12, UC-16, UC-05, UC-06, UC-08
---

# Direct workload variable entry — real kind/UI acceptance

## Outcome

PASS on `features/refactor_nhap_bien`, based on main `4764b11`. Approved mock
was committed first as `fc23308`. The product now uses Name/Value entry and an
optional combined reference picker. It retains existing app/environment Secret
storage and create-only APIs. No backend/schema/Score contract change.

All application/workload/configuration mutations in the recorded acceptance
used actual Console form controls. No Score file was generated/imported, no
workload apply or deployment mutation API was used by the runner. The backend
used real Kubernetes/Terraform/Vault adapters. UI source served by Vite on port
5188 proxied to the retained real backend on loopback 8080. A separate review
used the production frontend build via Vite preview on port 5189.

## Runtime

- Run: `env-form-20261027-r4`, 2026-10-27 12:39:59–12:42:27 UTC.
- Application: `2480c9c6-c221-4f83-bd9e-fb8c2491f9e2`, environment `staging`.
- Cluster/context: existing `k8s-4f` / `kind-k8s-4f`, node Ready.
- Namespace: `app-2480c9c6-c221-4f83-bd9e-fb8c2491f9e2-staging`.
- Reused available acceptance images: frontend/backend/worker tag `e2e-f3dc811`.
- Secret Store explicitly selected through inline UI: `k8s-4f-vault`.
- Shared resource IDs: `env-form-20261027-r4-db` and `env-form-20261027-r4-cache`.

| Check | Observation |
|---|---|
| Direct entry | PORT, cache scope/TTL, secret digest and worker interval entered as ordinary values on the workload form |
| Shared Variable | ACCEPTANCE_CONFIG created inline, row shows Application reference |
| Secret | ACCEPTANCE_SECRET entered only in password input, created in app/staging store, row holds reference; raw value absent from page text |
| Resource references | PostgreSQL and Redis declared via form with class/typed params/shared IDs; each output explicitly selected, secret outputs classified from catalog |
| Service reference | Frontend BACKEND_URL selects backend/http from picker |
| Deploy | UI Preview reports 3 affected workloads; deploy succeeds; 3 Deployments, 2 StatefulSets, 2 PVCs verified in run-owned namespace |
| Application checks | backend connection, environment, secret, database and cache all PASS |
| Jobs | Submitted through acceptance application's form; DONE with worker identity and result |
| Reopen/update | Deployed backend reopened through UI, PORT retained; ENV_UI_UPDATED=true added and deployed as one workload update |
| Reuse | PostgreSQL, Redis and both PVC UIDs unchanged after valid update; technical read confirms updated ordinary env value in backend Deployment |
| Browser | No JavaScript errors in recorded run |

Retained resource identity:

| Kind | Name | UID |
|---|---|---|
| Deployment | backend | `65504375-a164-4c2c-8901-55843e91eb18` |
| Deployment | frontend | `527286ab-b563-468f-a3a9-acf173090285` |
| Deployment | worker | `59a234ba-4f4c-40c8-8a6b-cd416dae6d95` |
| StatefulSet | postgres-c0ea89a5998a1f40 | `d7d087d5-95aa-4a1d-9b51-5bd432031549` |
| StatefulSet | redis-5ab2146bac4ea660 | `2cf36940-3898-431a-a9a8-1dfb35dc37a5` |
| PVC | postgres-c0ea89a5998a1f40-data | `ee66ff2c-5651-4fbb-b8d6-9b58f811f3b8` |
| PVC | redis-5ab2146bac4ea660-data | `72119594-a705-49e2-a026-8921dd28b8d3` |

## Checks

- `npm run typecheck`: pass.
- `npm run lint`: pass on updated main baseline.
- `npm test -- --maxWorkers=2`: 77 files, 917 tests pass; no skip.
- After final focus/coverage refinements, focused table/editor/i18n run: 69 pass;
  this adds coverage for Object prototype names and late scope catalog responses.
- `npm run build`: pass; existing bundle-size warning remains.
- `cd e2e && npm test`: 6/6 pass. Both scenario files pass `node --check`.
- Production build UI review: VI/EN × 1512/1280 desktop, four combinations pass;
  no page overflow, no mandatory source selector, scoped source labels, focus
  and Escape verified.
- `python3 scripts/check_docs.py`, PlantUML `-checkonly` for UC-16 sequence and
  `git diff --check`: pass.
- No Go changes; backend unit/build and AWS runs omitted because behavior/API
  reuse is frontend-only. Real kind runtime is the requested external acceptance.

## UI reference and copy review

Reused AppShell, Button, shared Dialog, existing container/editor/styles and
catalogs. Compared real workload form and production-build screenshots with the
approved mock. Removed mandatory source column, repeated source controls and
always-visible source lists; explanatory text moved into one details control.
Application references are explicitly labelled; local variables never appear as
source candidates. Secret scope/independent commit remains visible at creation.
No new shell/navigation/theme or workload wizard.

## Video and evidence

[GitHub evidence release](https://github.com/pr3s3nt/poc/releases/tag/evidence-env-ux-20261027-r4)
contains [MP4 recording](https://github.com/pr3s3nt/poc/releases/download/evidence-env-ux-20261027-r4/acceptance-ui-kind.mp4).
The MP4 concatenates the Console recording (about 151 seconds) and acceptance
application recording (about 28 seconds); the application segment is placed
last for review, and overlaps the Console update chronologically. Original WebM
recordings, phase timestamps, result and source images remain outside Git.
Reviewed contact sheets including initial Secret creation, deployment progress,
update and application results; password controls are masked. No raw Secret,
credential, state or browser session is uploaded. Video is fast automated UI,
not manual human-paced evidence.

Private evidence: `/tmp/env-form-20261027-r4/`; production-build review in
`review-final/`. New scenario and procedure:
[`e2e/scenarios/workload-env-form`](../../e2e/scenarios/workload-env-form/README.md).

## Earlier attempts and retention

R1–R3 stopped before any workload Save/Preview/Deploy while fixing runner
interaction/label/readiness assumptions (checkbox opens a dialog rather than
staying checked; class accessible label; waiting for store choices to load).
No product guard or assertion was weakened. Their failure videos are retained
privately. Applications/configuration metadata remain for these owned attempts:

- R1 `3cdb50b9-3133-43a3-aea8-aafb95bc4526`.
- R2 `d3c80147-ecc5-4d75-abbd-e78dd0a4d63f`.
- R3 `9a29039e-18eb-4864-9618-0f93bfb13d32`.

Read-only namespace checks confirm none has a runtime namespace. The Console
currently has no application deletion flow; no retained database is reset or
manually edited to hide these attempts.

R4 application/runtime/PVCs are intentionally retained for user review.
Port-forward and browser sessions from the runner ended in finally. Older apps,
cluster, platform PostgreSQL, Vault and VSO were preserved. Later cleanup must
use product workload/resource cleanup with the exact app/namespace identities
above, and verify ownership/UID before deleting any resource or data.
