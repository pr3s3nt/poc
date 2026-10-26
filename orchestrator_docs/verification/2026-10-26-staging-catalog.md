---
id: VERIFY-STAGING-CATALOG-20261026
artifact: verification
status: current
last_reviewed: 2026-10-26
---

# Staging bootstrap and operator catalog

User scope: remove Connections UI, stop sample account/Application/catalog seed,
seed Kubernetes contracts, and supply Playwright registration after the operator
creates Internal Driver Account `k8s-4f`. No staging deployment or database cleanup.

## Checks

- `cd backend && go test ./... && go build ./...`: PASS. Integration tests retain
  their existing environment requirements; no live PostgreSQL fixture was provisioned.
- Initial `cd frontend && npm run typecheck && npm run lint`: PASS before
  the last navigation assertion. This did not cover the final committed tree;
  see the correction below.
- `cd frontend && npm test`: final run PASS, 920 tests / 77 files. The initial
  run failed one observability follow-interval test; its isolated rerun passed
  5/5 and the final full run passed without changing observability code/tests.
- Initial `cd frontend && npm run build`: PASS before that last assertion;
  the claim did not establish buildability of commit `75f064a`.
- Focused final App/routes/bootstrap tests: PASS, 46/46.
- `bash -n backend/deploy/docker-entrypoint.sh`, Node script syntax: PASS.
- Documentation checker and `git diff --check`: PASS.

## Browser verification

Real Console UI and backend ran locally with fake execution adapters, disposable
JSON state outside Git and a synthetic READY `k8s-4f` account. Installed renderer
used the real local pinned score-k8s binary; no cluster credential was read and
no cluster/database/cloud resource was provisioned.

From an empty Definition catalog, operator Playwright created all five entries
(cluster, namespace, PostgreSQL, Redis, renderer), read back exact pins, mappings,
references and staging criteria, then reran and retained all five unchanged.
The partial-run resume path was also exercised during locator corrections.
No writes were made to the user's staging platform.

UI source/reference: existing AppShell, shared styles/Button and current Driver
Accounts navigation. Browser screenshots before/after outside Git were visually
compared at 1440x1000: Connections is removed, Infrastructure accounts remains,
and shell/layout/styles are retained. Content review: removal needs no added
product instructions; operator details live in the runbook.

## Limits and handoff

[Runbook](../operations/staging-catalog.md) supplies inputs and execution commands.
Operator must deploy the new images, register the real account and run the script.
Live kubeconfig verification, Vault delivery, module provisioning, registry pull
and acceptance Application deployment were not run: external verification is
outside this task. Existing PVC/database data is preserved; reapplying manifests
with the same PostgreSQL PVC does not remove old seed records. No preexisting
user changes were present at task start.

## Correction: role-query options (2026-10-26)

User reported `npm run typecheck` failing on commit `75f064a`. Reproduced
TS2769 at `App.test.tsx:436`: DOM Testing Library `ByRoleOptions` does not
accept `exact`. Removed that option; a string `name` already uses exact
accessible-name matching. Playwright's role locators have different options
and are unaffected. The initial verification missed typechecking the final
assertion added after the earlier typecheck/build; the initial build claim
above is corrected accordingly.

Post-fix checks: typecheck, lint, production build, App tests (36/36), docs
checker and diff check PASS. Full frontend run: 919/920 PASS; the same unrelated
observability follow-interval test failed under full-suite execution and its
isolated rerun passed 5/5. No observability implementation/test was changed;
this full run is not reported as completely passing. No backend/runtime changes
were made for this correction, so Go and live cluster checks were not rerun.
