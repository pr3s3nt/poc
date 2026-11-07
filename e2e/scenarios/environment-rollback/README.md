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
