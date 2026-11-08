# Environment rollback — issue #49

Goal: review and explicitly restore the entire Environment using Console controls,
then verify workload state and the business flow. Required behavior: UC-06 V2-07,
ADR-040 and the delivery-reconciliation contract. Follow the kind and Tailscale
handoff runbooks; this runner does not authorize another cluster.

Prerequisites: an isolated task Console with a verified CD binding/Definitions,
authorized kind target, credentials supplied through the shared Console helper,
and a successful whole-Environment release checkpoint. To test initial-empty
rollback use a separate owned test Application without any successful release.
Never destroy the retained Boutique application to substitute for this case.

Use ORCH_PLATFORM_URL, ORCH_PLATFORM_USERNAME, ORCH_PLATFORM_PASSWORD_FILE,
ORCH_HEADLESS=1, ORCH_TEST_RUN_DIR outside Git and ORCH_ROLLBACK_APP (Application
key). ORCH_ROLLBACK_ENV defaults to staging. Without ORCH_ROLLBACK_CONFIRM=1 the
runner only opens Preview Rollback. With confirmation it types the exact app key
and clicks Rollback. All business writes use UI; HTTP response observation and
scoped GETs are technical assertions. It refuses an active/interrupted operation.

```sh
node e2e/scenarios/environment-rollback/run.mjs
ORCH_ROLLBACK_CONFIRM=1 node e2e/scenarios/environment-rollback/run.mjs
```

Assertions: correct Environment and whole-Environment review, typed confirmation,
no unrequested mutation during preview, successful rollback response, no active
claim after success, and retained screenshot. A failure is preserved and reported;
never replay a lost/failed mutation automatically. Keep independent before/after
cluster UIDs, manifests and data/business assertions for the specific run.

Side effects: confirmation restores declared workloads/configuration and can
remove workloads absent from the target. Initial-empty confirmation deletes the
exact owned resource selection after ownership checks. It does not restore prior
database contents. Keep protected resources/credentials/controller identity intact.
Evidence stays outside Git. Cleanup uses product operations with exact ownership
verification; keep trial stack/forwards active when handing links to the user.


For the retained Boutique release, `stage-draft.mjs` with
`ORCH_ROLLBACK_STAGE=1` first verifies an idle Environment and eleven workloads
without drafts, then records the original configuration and workloads privately.
It adds an unused nonsecret configuration marker and stages a loadgenerator image
edit through forms without deploying. Run the rollback runner afterwards and
compare the restored workloads/configuration with `before-stage.json`; then run
the independent runtime observer and checkout runner. This case verifies discarding
pending edits and reapplying a proven release, not a live partial rollout failure.
Partial rollout failure and first-failure cleanup are separately verified by the
PostgreSQL and Terraform integration tests described in the issue evidence.


`verify.mjs` reads the recorded `before-stage.json`,
`before-rollback-inventory.json` (kind/name/UID for Deployments, StatefulSets,
Services and PVCs) and `before-stateful-pods.json` (name/UID). Supply the authorized
`ORCH_CLUSTER_KUBECONFIG`. It compares complete workload declarations and safe
configuration projections, requires no drafts/active operation, preserves resource
and stateful Pod UIDs, and opens the actual Console page. The independent Boutique
observer/checkout scripts still verify readiness and business behavior afterwards.

`initial-empty.mjs` requires `ORCH_EMPTY_ROLLBACK=1` and a fresh private evidence
directory without `empty-application.json`. It creates only the test Application
`rollback-empty-issue49` and one pending workload draft, confirms initial-empty
rollback, and verifies zero workloads and an idle Environment. No Deploy is
submitted and no Kubernetes resources are created. This verifies the Console
empty-baseline flow, not cleanup of a failed real Kubernetes rollout. Record the
created identity privately and clean up this empty test Application through its
Settings deletion dialog after verifying the deletion preview has no workloads,
resources or pending drafts in either Environment. Never use this cleanup on the
retained Boutique Application.


## Deferred live scenario 2 — user decision 2026-11-08

**UNVERIFIED. Do not run further scenario 2 attempts without a new user request.**
`prepare-failure.mjs` and `fail-release.mjs` are retained as unfinished acceptance
work. Preparation timed out after creating/selecting an unused test Definition and
saving a configuration marker. No controlled failing deployment or restorative
rollback was submitted. The marker/selection are cleaned through existing UI
operations; the unused type/Definition and private fixture branch remain for audit.

The planned fixture is pinned to commit
`d9ffa3fb47e4fcd3e156d5c9a534d1b43e6de18c`, path `terraform/rollback-gate`,
on the existing private template repository's isolated test branch. It uses only
terraform_data and an explicit false precondition; local validate and expected
Plan failure passed without Apply. This is fixture verification, not application
rollback acceptance. Future preparation also needs ORCH_BOUTIQUE_STATE_FILE pointing
to the private successful Boutique run state, in addition to the shared Console
credentials/evidence/app settings. Existing evidence files prohibit blind replay.
